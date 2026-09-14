package tcp

import (
	"log/slog"
	"net"
	"strings"
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
