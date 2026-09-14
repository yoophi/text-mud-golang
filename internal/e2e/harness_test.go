// Package e2e exercises the assembled server over real TCP connections.
package e2e

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoophi/text-mud-golang/internal/adapter/sqlite"
	"github.com/yoophi/text-mud-golang/internal/adapter/tcp"
	"github.com/yoophi/text-mud-golang/internal/adapter/worldfile"
	"github.com/yoophi/text-mud-golang/internal/application"
	"github.com/yoophi/text-mud-golang/internal/domain"
)

const defaultWorld = `{
  "startRoom": "plaza",
  "respawnRoom": "plaza",
  "rooms": [
    {"id": "plaza", "name": "마을 광장", "description": "오래된 분수가 있는 광장이다.", "exits": {"북쪽": "alley", "동쪽": "market"}},
    {"id": "alley", "name": "뒷골목", "description": "좁고 어두운 골목이다.", "exits": {"남쪽": "plaza"}},
    {"id": "market", "name": "시장", "description": "활기찬 시장이다.", "exits": {"서쪽": "plaza"}}
  ],
  "items": [
    {"id": "bread", "name": "빵", "aliases": ["식빵"], "description": "갓 구운 빵이다."},
    {"id": "coin", "name": "동전", "description": "약간 광이 나는 동전이다."}
  ],
  "itemSpawns": [
    {"room": "plaza", "item": "bread", "count": 2},
    {"room": "plaza", "item": "coin", "count": 1}
  ],
  "npcs": [
    {"id": "rat", "name": "쥐", "aliases": ["생쥐"], "room": "alley", "hp": 3, "damageMin": 1, "damageMax": 2, "aggressive": false, "respawnSeconds": 3}
  ]
}`

// testServer runs the full application stack in-process, reachable over
// real TCP on 127.0.0.1.
type testServer struct {
	t         *testing.T
	addr      string
	dbPath    string
	worldDir  string
	world     string
	operators []string
	cancel    context.CancelFunc
	done      chan error
	stopOnce  sync.Once
}

func startServer(t *testing.T, world string, operators ...string) *testServer {
	t.Helper()
	dir := t.TempDir()
	ts := &testServer{t: t, dbPath: filepath.Join(dir, "mud.db"), worldDir: dir, world: world, operators: operators}
	ts.launch()
	return ts
}

func (ts *testServer) launch() {
	ts.t.Helper()
	worldPath := filepath.Join(ts.worldDir, "world.json")
	if err := os.WriteFile(worldPath, []byte(ts.world), 0o644); err != nil {
		ts.t.Fatal(err)
	}
	world, err := worldfile.Load(worldPath)
	if err != nil {
		ts.t.Fatalf("world: %v", err)
	}
	repo, err := sqlite.Open(context.Background(), ts.dbPath)
	if err != nil {
		ts.t.Fatalf("sqlite: %v", err)
	}
	game := domain.NewGame(world, application.SystemClock{}, application.SystemRandom{})
	inputs := make(chan application.Input, 256)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gateway := tcp.NewGateway(logger, inputs)
	addr, err := gateway.Listen("127.0.0.1:0")
	if err != nil {
		ts.t.Fatalf("listen: %v", err)
	}
	server := application.NewServer(application.Config{Operators: ts.operators}, game, repo, gateway, inputs, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		err := server.Run(ctx)
		repo.Close()
		done <- err
	}()
	ts.addr = addr
	ts.cancel = cancel
	ts.done = done
}

// stop shuts the server down and waits for a clean exit. Idempotent.
func (ts *testServer) stop() {
	ts.t.Helper()
	ts.stopOnce.Do(func() {
		ts.cancel()
		select {
		case err := <-ts.done:
			if err != nil {
				ts.t.Fatalf("server exit: %v", err)
			}
		case <-time.After(10 * time.Second):
			ts.t.Fatal("server did not stop in time")
		}
	})
}

// waitStopped waits for the server to exit on its own (e.g. @shutdown)
// and asserts a clean exit.
func (ts *testServer) waitStopped() {
	ts.t.Helper()
	ts.stopOnce.Do(func() {
		select {
		case err := <-ts.done:
			if err != nil {
				ts.t.Fatalf("server exit: %v", err)
			}
		case <-time.After(10 * time.Second):
			ts.t.Fatal("server did not stop in time")
		}
	})
}

// restart stops the server and starts a fresh process-equivalent using
// the same database file and world definition.
func (ts *testServer) restart() {
	ts.t.Helper()
	ts.stop()
	ts.stopOnce = sync.Once{}
	ts.launch()
}

// client is a scripted TCP client.
type client struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
}

func dial(t *testing.T, addr string) *client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &client{t: t, conn: conn, br: bufio.NewReader(conn)}
}

func (c *client) send(line string) {
	c.t.Helper()
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		c.t.Fatalf("send %q: %v", line, err)
	}
}

// readUntil reads lines until one contains want, returning the matched
// line. It fails the test on timeout.
func (c *client) readUntil(want string, timeout time.Duration) string {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		line, err := c.br.ReadString('\n')
		if err != nil {
			c.t.Fatalf("readUntil(%q): %v", want, err)
		}
		if strings.Contains(line, want) {
			return line
		}
	}
}

// login performs the name / (optional create confirm) flow and waits for
// the first room description.
func (c *client) login(name string, create bool, firstRoom string) {
	c.t.Helper()
	c.readUntil("이름을 입력하세요", 5*time.Second)
	c.send(name)
	if create {
		c.readUntil("생성할까요", 5*time.Second)
		c.send("네")
	}
	c.readUntil(firstRoom, 5*time.Second)
}
