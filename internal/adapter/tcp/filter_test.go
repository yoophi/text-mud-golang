package tcp

import (
	"net"
	"testing"
	"time"
)

func newFilter() *telnetFilter {
	s := &session{id: "s1", conn: &fakeConn{}, out: make(chan string, 8), done: make(chan struct{})}
	return newTelnetFilter(s)
}

func feedLine(t *testing.T, f *telnetFilter, bytes ...byte) string {
	t.Helper()
	for _, b := range bytes {
		line, ok := f.feed(b)
		if ok {
			return line
		}
	}
	return ""
}

func TestFilterPlainLine(t *testing.T) {
	f := newFilter()
	if got := feedLine(t, f, []byte("보기\n")...); got != "보기" {
		t.Fatalf("line = %q", got)
	}
}

func TestFilterCRLFAndTrailingSpace(t *testing.T) {
	f := newFilter()
	if got := feedLine(t, f, []byte("북쪽  \r\n")...); got != "북쪽" {
		t.Fatalf("line = %q", got)
	}
}

func TestFilterStripsIACSequences(t *testing.T) {
	f := newFilter()
	// IAC WILL ECHO, IAC DONT SGA, escaped IAC byte, then text.
	got := feedLine(t, f,
		255, 251, 1,
		255, 254, 3,
		'b', 255, 255, 'o', 'k', '\n',
	)
	if got != "b\xffok" {
		t.Fatalf("line = %q", got)
	}
}

func TestFilterSubnegotiation(t *testing.T) {
	f := newFilter()
	got := feedLine(t, f, 'a', 255, 250, 24, 0, 65, 66, 255, 240, 'z', '\n')
	if got != "az" {
		t.Fatalf("line = %q", got)
	}
}

func TestFilterBackspace(t *testing.T) {
	f := newFilter()
	if got := feedLine(t, f, 'x', 'y', 8, 'z', '\n'); got != "xz" {
		t.Fatalf("line = %q", got)
	}
}

func TestFilterDropsControlBytes(t *testing.T) {
	f := newFilter()
	if got := feedLine(t, f, 1, 2, 'o', 3, 'k', 0, '\n'); got != "ok" {
		t.Fatalf("line = %q", got)
	}
}

func TestFilterRefusesNegotiation(t *testing.T) {
	f := newFilter()
	s := f.session
	feedLine(t, f, 255, 251, 1) // IAC WILL ECHO
	select {
	case ctrl := <-s.out:
		want := string([]byte{255, 252, 1}) // IAC WONT ECHO
		if ctrl != want {
			t.Fatalf("refusal = % x, want % x", ctrl, want)
		}
	default:
		t.Fatal("refusal should be queued")
	}
}

// fakeConn satisfies net.Conn for filter tests.
type fakeConn struct{}

func (c *fakeConn) Read(b []byte) (int, error)       { return 0, net.ErrClosed }
func (c *fakeConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *fakeConn) Close() error                     { return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return nil }
func (c *fakeConn) RemoteAddr() net.Addr             { return nil }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }
