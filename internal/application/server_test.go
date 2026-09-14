package application

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoophi/text-mud-golang/internal/domain"
)

// fakeNet records everything the loop writes, without real sockets.
type fakeNet struct {
	mu      sync.Mutex
	writes  map[domain.SessionID][]string
	closed  []domain.SessionID
	stopped bool
	got     chan struct{} // signaled on every Write for polling
}

func newFakeNet() *fakeNet {
	return &fakeNet{writes: map[domain.SessionID][]string{}, got: make(chan struct{}, 256)}
}

func (f *fakeNet) Listen(addr string) (string, error) { return "fake", nil }
func (f *fakeNet) Write(s domain.SessionID, text string) {
	f.mu.Lock()
	f.writes[s] = append(f.writes[s], text)
	f.mu.Unlock()
	select {
	case f.got <- struct{}{}:
	default:
	}
}
func (f *fakeNet) Close(s domain.SessionID) {
	f.mu.Lock()
	f.closed = append(f.closed, s)
	f.mu.Unlock()
}
func (f *fakeNet) Stop() {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
}
func (f *fakeNet) texts(s domain.SessionID) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.writes[s]...)
}

// waitFor polls until the session received text containing want.
func (f *fakeNet) waitFor(t *testing.T, s domain.SessionID, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, text := range f.texts(s) {
			if strings.Contains(text, want) {
				return
			}
		}
		select {
		case <-f.got:
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatalf("session %s never received %q; got %+v", s, want, f.texts(s))
}

// fakeRepo is an in-memory CharacterRepository.
type fakeRepo struct {
	mu     sync.Mutex
	saved  []*domain.Character
	exists map[string]*domain.Character
}

func newFakeRepo() *fakeRepo { return &fakeRepo{exists: map[string]*domain.Character{}} }

func (r *fakeRepo) Load(_ context.Context, name string) (*domain.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.exists[name]; ok {
		return c.Copy(), nil
	}
	return nil, ErrCharacterNotFound
}

func (r *fakeRepo) Save(_ context.Context, c *domain.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved = append(r.saved, c.Copy())
	r.exists[c.Name] = c.Copy()
	return nil
}

func (r *fakeRepo) Close() error { return nil }

func (r *fakeRepo) savesFor(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.saved {
		if c.Name == name {
			n++
		}
	}
	return n
}

// character returns a copy of the stored character under lock. Tests
// must use this instead of touching r.exists directly: the server
// goroutine keeps calling Save concurrently.
func (r *fakeRepo) character(name string) (*domain.Character, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.exists[name]
	if !ok {
		return nil, false
	}
	return c.Copy(), true
}

func testWorld() *domain.World {
	w, err := domain.NewWorld([]*domain.Room{
		{ID: "plaza", Name: "마을 광장", Description: "광장이다.", Exits: map[domain.Direction]domain.RoomID{domain.DirNorth: "alley"}},
		{ID: "alley", Name: "뒷골목", Description: "골목이다.", Exits: map[domain.Direction]domain.RoomID{domain.DirSouth: "plaza"}},
	}, "plaza", "")
	if err != nil {
		panic(err)
	}
	return w
}

func newLoopServer(t *testing.T) (*Server, *fakeNet, *fakeRepo, chan Input) {
	t.Helper()
	net := newFakeNet()
	repo := newFakeRepo()
	inputs := make(chan Input, 16)
	game := domain.NewGame(testWorld(), SystemClock{}, SystemRandom{})
	server := NewServer(Config{TickInterval: 5 * time.Millisecond}, game, repo, net, inputs, slog.Default())
	return server, net, repo, inputs
}

func TestLoopProcessesQueuedInputsAndSavesOnDisconnect(t *testing.T) {
	server, net, repo, inputs := newLoopServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	inputs <- Input{Session: "s1", Kind: InputConnected}
	net.waitFor(t, "s1", "이름을 입력하세요")

	inputs <- Input{Session: "s1", Kind: InputLine, Line: "영희"}
	net.waitFor(t, "s1", "생성할까요")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "네"}
	net.waitFor(t, "s1", "마을 광장")

	inputs <- Input{Session: "s1", Kind: InputLine, Line: "북쪽"}
	net.waitFor(t, "s1", "뒷골목")

	inputs <- Input{Session: "s1", Kind: InputDisconnected}
	// Wait until the disconnect save landed (the loop goroutine may
	// still be saving when the first poll succeeds on an earlier save).
	deadline := time.Now().Add(3 * time.Second)
	saved, ok := repo.character("영희")
	for (!ok || saved.Room != "alley") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		saved, ok = repo.character("영희")
	}
	if !ok || saved.Room != "alley" {
		t.Fatalf("disconnect should persist the character at alley: %+v", saved)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop")
	}
	net.mu.Lock()
	defer net.mu.Unlock()
	if !net.stopped {
		t.Fatal("shutdown should stop the gateway")
	}
}

func TestLoopRejectsInvalidNamesWithoutStateChange(t *testing.T) {
	server, net, _, inputs := newLoopServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	inputs <- Input{Session: "s1", Kind: InputConnected}
	net.waitFor(t, "s1", "이름을 입력하세요")

	inputs <- Input{Session: "s1", Kind: InputLine, Line: "x"}
	net.waitFor(t, "s1", "2~12자")

	inputs <- Input{Session: "s1", Kind: InputLine, Line: "영희"}
	net.waitFor(t, "s1", "생성할까요")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "맞아"}
	net.waitFor(t, "s1", "'네' 또는 '아니오'")

	inputs <- Input{Session: "s1", Kind: InputLine, Line: "아니오"}
	net.waitFor(t, "s1", "이름을 입력하세요")
	cancel()
	<-done
}

func TestLoopReusesSavedCharacter(t *testing.T) {
	server, net, repo, inputs := newLoopServer(t)
	// Pre-populate the repository with a character that already moved.
	existing := domain.NewCharacter("철수", "alley")
	if err := repo.Save(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	inputs <- Input{Session: "s1", Kind: InputConnected}
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "철수"}
	net.waitFor(t, "s1", "뒷골목") // restored room, no creation prompt
	cancel()
	<-done
}

func TestLoopAdminShutdownSavesAndStops(t *testing.T) {
	net := newFakeNet()
	repo := newFakeRepo()
	inputs := make(chan Input, 16)
	game := domain.NewGame(testWorld(), SystemClock{}, SystemRandom{})
	server := NewServer(Config{TickInterval: 5 * time.Millisecond, Operators: []string{"영희"}}, game, repo, net, inputs, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	// Login the operator and request shutdown.
	inputs <- Input{Session: "s1", Kind: InputConnected}
	net.waitFor(t, "s1", "이름을 입력하세요")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "영희"}
	net.waitFor(t, "s1", "생성할까요")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "네"}
	net.waitFor(t, "s1", "마을 광장")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "북쪽"}
	net.waitFor(t, "s1", "뒷골목")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "@shutdown"}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop on @shutdown")
	}
	if repo.savesFor("영희") == 0 {
		t.Fatal("shutdown must save characters")
	}
	saved, ok := repo.character("영희")
	if !ok || saved.Room != "alley" {
		t.Fatalf("shutdown save wrong: %+v", saved)
	}
	net.mu.Lock()
	stopped := net.stopped
	net.mu.Unlock()
	if !stopped {
		t.Fatal("shutdown must stop the gateway")
	}
}

func TestLoopNonOperatorCannotShutdown(t *testing.T) {
	net := newFakeNet()
	repo := newFakeRepo()
	inputs := make(chan Input, 16)
	game := domain.NewGame(testWorld(), SystemClock{}, SystemRandom{})
	server := NewServer(Config{TickInterval: 5 * time.Millisecond}, game, repo, net, inputs, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	inputs <- Input{Session: "s1", Kind: InputConnected}
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "철수"}
	net.waitFor(t, "s1", "생성할까요")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "네"}
	net.waitFor(t, "s1", "마을 광장")
	inputs <- Input{Session: "s1", Kind: InputLine, Line: "@shutdown"}
	net.waitFor(t, "s1", "운영자 권한")

	select {
	case <-done:
		t.Fatal("server must keep running for non-operators")
	case <-time.After(300 * time.Millisecond):
		// still running: success
	}
	cancel()
	<-done
}
