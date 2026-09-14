// Package tcp implements the network adapter: a Telnet-friendly line
// protocol over TCP. It never touches game state; it only forwards
// framed input to the application loop and writes output back.
package tcp

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yoophi/text-mud-golang/internal/application"
	"github.com/yoophi/text-mud-golang/internal/domain"
)

// Gateway is the TCP implementation of application.NetGateway.
type Gateway struct {
	log      *slog.Logger
	inputs   chan<- application.Input
	ln       net.Listener
	sessions sync.Map // domain.SessionID -> *session
	next     atomic.Int64
	wg       sync.WaitGroup
	stopOnce sync.Once
	done     chan struct{} // closed by Stop
}

// NewGateway creates a gateway that reports input events on inputs.
func NewGateway(log *slog.Logger, inputs chan application.Input) *Gateway {
	return &Gateway{log: log, inputs: inputs, done: make(chan struct{})}
}

// Listen starts accepting connections on addr and returns the bound
// address (useful when addr ends in :0).
func (g *Gateway) Listen(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("tcp listen: %w", err)
	}
	g.ln = ln
	g.wg.Add(1)
	go g.acceptLoop()
	return ln.Addr().String(), nil
}

func (g *Gateway) acceptLoop() {
	defer g.wg.Done()
	for {
		conn, err := g.ln.Accept()
		if err != nil {
			select {
			case <-g.done:
				return
			default:
				g.log.Error("accept 실패", "err", err)
				return
			}
		}
		go g.handle(conn)
	}
}

func (g *Gateway) handle(conn net.Conn) {
	s := &session{
		id:   domain.SessionID(fmt.Sprintf("s%d", g.next.Add(1))),
		conn: conn,
		out:  make(chan string, 128),
		done: make(chan struct{}),
	}
	g.sessions.Store(s.id, s)
	g.send(application.Input{Session: s.id, Kind: application.InputConnected})

	go s.writeLoop()

	defer func() {
		s.close()
		g.sessions.Delete(s.id)
		g.send(application.Input{Session: s.id, Kind: application.InputDisconnected})
	}()

	filter := newTelnetFilter(s)
	reader := bufio.NewReader(conn)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return
		}
		if line, ok := filter.feed(b); ok {
			g.send(application.Input{Session: s.id, Kind: application.InputLine, Line: line})
		}
	}
}

// send delivers an input event, giving up once the gateway has stopped.
func (g *Gateway) send(in application.Input) {
	select {
	case g.inputs <- in:
	case <-g.done:
	}
}

// Write queues text for the session. A slow client whose queue is full
// is kicked instead of blocking the game loop.
func (g *Gateway) Write(id domain.SessionID, text string) {
	v, ok := g.sessions.Load(id)
	if !ok {
		return
	}
	s := v.(*session)
	s.enqueue(text + "\r\n")
}

// Close disconnects the session.
func (g *Gateway) Close(id domain.SessionID) {
	if v, ok := g.sessions.Load(id); ok {
		v.(*session).close()
	}
}

// Stop stops accepting connections and disconnects every session,
// letting writers drain briefly.
func (g *Gateway) Stop() {
	g.stopOnce.Do(func() {
		close(g.done)
		if g.ln != nil {
			g.ln.Close()
		}
		g.sessions.Range(func(_, v any) bool {
			v.(*session).close()
			return true
		})
		g.wg.Wait()
	})
}

// session owns one client connection.
type session struct {
	id   domain.SessionID
	conn net.Conn
	out  chan string
	done chan struct{}
	once sync.Once
}

func (s *session) close() {
	s.once.Do(func() {
		close(s.done)
		s.conn.Close()
	})
}

func (s *session) enqueue(text string) {
	select {
	case <-s.done:
	case s.out <- text:
	default:
		// Slow client: kick rather than block the game loop.
		s.close()
	}
}

func (s *session) writeLoop() {
	for {
		select {
		case <-s.done:
			s.drain()
			return
		case text := <-s.out:
			if !s.write(text) {
				s.close()
				return
			}
		}
	}
}

// drain flushes whatever is already queued before exiting.
func (s *session) drain() {
	for {
		select {
		case text := <-s.out:
			if !s.write(text) {
				return
			}
		default:
			return
		}
	}
}

func (s *session) write(text string) bool {
	_ = s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := s.conn.Write([]byte(text))
	return err == nil
}

// sendControl queues raw Telnet control bytes for the client.
func (s *session) sendControl(bytes ...byte) {
	s.enqueue(string(bytes))
}

// telnetFilter converts a byte stream into complete input lines,
// stripping Telnet control sequences, CR/NUL, and handling backspace.
type telnetFilter struct {
	session *session
	line    []byte
	state   int
	refusal byte // WONT or DONT to send once the option byte arrives
}

const (
	tfNormal = iota
	tfIAC
	tfOption // expecting option byte after WILL/WONT/DO/DONT
	tfSub    // inside subnegotiation
	tfSubIAC // IAC inside subnegotiation
)

// Telnet protocol constants.
const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240
)

func newTelnetFilter(s *session) *telnetFilter {
	return &telnetFilter{session: s}
}

// feed consumes one byte and reports a completed line when ready.
func (f *telnetFilter) feed(b byte) (string, bool) {
	switch f.state {
	case tfNormal:
		switch {
		case b == iac:
			f.state = tfIAC
		case b == '\r' || b == 0:
			// ignore CR and NUL
		case b == '\n':
			return f.flush(), true
		case b == 8 || b == 127: // backspace / DEL
			if len(f.line) > 0 {
				f.line = f.line[:len(f.line)-1]
			}
		case b < 32:
			// drop other control characters
		default:
			if len(f.line) < 1024 {
				f.line = append(f.line, b)
			}
		}
	case tfIAC:
		switch {
		case b == iac:
			if len(f.line) < 1024 {
				f.line = append(f.line, iac)
			}
			f.state = tfNormal
		case b == will:
			f.refusal = wont
			f.state = tfOption
		case b == do:
			f.refusal = wont
			f.state = tfOption
		case b == wont || b == dont:
			f.state = tfOption
		case b == sb:
			f.state = tfSub
		default:
			f.state = tfNormal
		}
	case tfOption:
		f.session.sendControl(iac, f.refusal, b)
		f.state = tfNormal
	case tfSub:
		if b == iac {
			f.state = tfSubIAC
		}
	case tfSubIAC:
		if b == se {
			f.state = tfNormal
		} else if b != iac {
			f.state = tfSub
		}
	}
	return "", false
}

func (f *telnetFilter) flush() string {
	line := strings.TrimRight(string(f.line), " \t")
	f.line = f.line[:0]
	return line
}
