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

func TestNegotiationResponses(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []byte // nil means no reply
	}{
		{"WILL은 DONT로 거절", []byte{255, 251, 1}, []byte{255, 254, 1}},
		{"DO는 WONT로 거절", []byte{255, 253, 24}, []byte{255, 252, 24}},
		{"WONT는 무응답", []byte{255, 252, 3}, nil},
		{"DONT는 무응답", []byte{255, 254, 5}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFilter()
			s := f.session
			feedLine(t, f, tc.in...)
			if tc.want == nil {
				select {
				case got := <-s.out:
					t.Fatalf("unexpected reply % x", got)
				default:
				}
				return
			}
			select {
			case got := <-s.out:
				if got != string(tc.want) {
					t.Fatalf("reply = % x, want % x", got, tc.want)
				}
			default:
				t.Fatal("refusal should be queued")
			}
		})
	}
}

func TestFilterBackspaceKorean(t *testing.T) {
	f := newFilter()
	input := append(append([]byte("안녕"), 8), []byte("하세요\n")...)
	if got := feedLine(t, f, input...); got != "안하세요" {
		t.Fatalf("line = %q, want 안하세요", got)
	}

	f = newFilter()
	if got := feedLine(t, f, []byte("쥐")...); got != "" {
		t.Fatalf("incomplete feed should not emit: %q", got)
	}
	f.feed(8)
	if got := feedLine(t, f, []byte("bread\n")...); got != "bread" {
		t.Fatalf("backspace on empty line must be a no-op, got %q", got)
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
