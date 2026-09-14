package e2e

import (
	"testing"
	"time"
)

func TestVerticalSliceCreateMoveReconnect(t *testing.T) {
	ts := startServer(t, defaultWorld)
	defer ts.stop()

	// Fresh character creation over real TCP.
	c1 := dial(t, ts.addr)
	c1.login("영희", true, "마을 광장")

	// Look and move.
	c1.send("보기")
	c1.readUntil("분수", 5*time.Second)
	c1.send("북")
	c1.readUntil("뒷골목", 5*time.Second)
	c1.send("동쪽") // no exit that way
	c1.readUntil("갈 수 없습니다", 5*time.Second)

	// Disconnect and reconnect: the saved room must be restored.
	c1.conn.Close()
	time.Sleep(200 * time.Millisecond)

	c2 := dial(t, ts.addr)
	c2.login("영희", false, "뒷골목")
}

func TestReconnectAfterServerRestart(t *testing.T) {
	ts := startServer(t, defaultWorld)
	c := dial(t, ts.addr)
	c.login("철수", true, "마을 광장")
	c.send("동쪽")
	c.readUntil("시장", 5*time.Second)
	c.conn.Close()

	ts.restart()

	c2 := dial(t, ts.addr)
	c2.login("철수", false, "시장")
}

func TestSecondCharacterIndependentMovement(t *testing.T) {
	ts := startServer(t, defaultWorld)
	a := dial(t, ts.addr)
	a.login("영희", true, "마을 광장")
	b := dial(t, ts.addr)
	b.login("철수", true, "마을 광장")

	b.send("북")
	a.readUntil("철수이(가) 북쪽(으)로 이동했다", 5*time.Second)

	a.send("보기")
	a.readUntil("출구", 5*time.Second)
}
