package tcp

import (
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoophi/text-mud-golang/internal/application"
	"github.com/yoophi/text-mud-golang/internal/domain"
)

func TestWriteToSlowClientKicksInsteadOfBlocking(t *testing.T) {
	inputs := make(chan application.Input, 16)
	gw := NewGateway(slog.Default(), inputs)
	addr, err := gw.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gw.Stop()

	client, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Wait for the session to register.
	var sessionID string
	select {
	case in := <-inputs:
		sessionID = string(in.Session)
	case <-time.After(3 * time.Second):
		t.Fatal("no connect event")
	}

	// Flood a client that never reads. Write must stay non-blocking.
	big := strings.Repeat("x", 1<<20)
	start := time.Now()
	for i := 0; i < 300; i++ {
		gw.Write(domain.SessionID(sessionID), big)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("writes to a slow client blocked the loop for %v", elapsed)
	}

	// The slow client must eventually be disconnected.
	client.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	for {
		if _, err := client.Read(buf); err != nil {
			break // kicked: EOF or reset
		}
	}
}

func TestStopDisconnectsClients(t *testing.T) {
	inputs := make(chan application.Input, 16)
	gw := NewGateway(slog.Default(), inputs)
	addr, err := gw.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-inputs:
	case <-time.After(3 * time.Second):
		t.Fatal("no connect event")
	}

	gw.Stop()
	client.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 16)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("client should be disconnected after Stop")
	}
}

// gateConn is a net.Conn whose Write blocks until released, so tests can
// hold the session writer mid-write deterministically.
type gateConn struct {
	mu      sync.Mutex
	writes  []string
	entered chan struct{}
	release chan struct{}
	closed  chan struct{}
	closeOK sync.Once
}

func newGateConn() *gateConn {
	return &gateConn{
		entered: make(chan struct{}, 16),
		release: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (c *gateConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	c.writes = append(c.writes, string(b))
	c.mu.Unlock()
	c.entered <- struct{}{}
	<-c.release
	return len(b), nil
}

func (c *gateConn) Read(b []byte) (int, error) {
	<-c.closed
	return 0, net.ErrClosed
}

func (c *gateConn) Close() error {
	c.closeOK.Do(func() { close(c.closed) })
	return nil
}

func (c *gateConn) LocalAddr() net.Addr              { return nil }
func (c *gateConn) RemoteAddr() net.Addr             { return nil }
func (c *gateConn) SetDeadline(time.Time) error      { return nil }
func (c *gateConn) SetReadDeadline(time.Time) error  { return nil }
func (c *gateConn) SetWriteDeadline(time.Time) error { return nil }

func (c *gateConn) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.writes...)
}

// TestSessionDrainsQueuedMessagesOnClose pins the graceful-shutdown
// behavior: messages already queued when shutdown starts must still be
// delivered, and the writer must finish within a bounded time.
func TestSessionDrainsQueuedMessagesOnClose(t *testing.T) {
	bc := newGateConn()
	s := &session{
		id:      "s1",
		conn:    bc,
		out:     make(chan string, 8),
		done:    make(chan struct{}),
		flushed: make(chan struct{}),
	}
	go s.writeLoop()

	s.enqueue("first")
	select {
	case <-bc.entered: // writer is now blocked inside conn.Write
	case <-time.After(3 * time.Second):
		t.Fatal("writer never started")
	}

	s.enqueue("second")
	s.enqueue("third")
	s.close() // shutdown requested while the writer is blocked
	close(bc.release)

	select {
	case <-s.flushed:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not flush in bounded time")
	}

	got := bc.snapshot()
	want := []string{"first", "second", "third"} // enqueue sends raw text; Gateway.Write adds CRLF
	if len(got) != len(want) {
		t.Fatalf("drained writes = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("drained writes = %q, want %q", got, want)
		}
	}
}
