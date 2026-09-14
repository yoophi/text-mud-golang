package e2e

import (
	"testing"
	"time"
)

func TestItemPickUpDropInventoryOverTCP(t *testing.T) {
	ts := startServer(t, defaultWorld)
	defer ts.stop()

	c := dial(t, ts.addr)
	c.login("영희", true, "마을 광장")

	// Floor items are visible.
	c.send("보기")
	c.readUntil("바닥에 놓인 물건: 빵, 빵, 동전", 5*time.Second)

	// Ambiguous pickup demands an index.
	c.send("줍기 빵")
	c.readUntil("여러 개", 5*time.Second)

	// Numbered pickup, particle target, inventory.
	c.send("줍기 빵 1")
	c.readUntil("주웠", 5*time.Second)
	c.send("줍기 동전을")
	c.readUntil("주웠", 5*time.Second)
	c.send("인벤토리")
	c.readUntil("1) 빵", 5*time.Second)
	c.readUntil("2) 동전", 5*time.Second)

	// Drop puts it back on the floor.
	c.send("버리기 동전")
	c.readUntil("바닥에 놓았", 5*time.Second)

	// Persistence across reconnect.
	c.conn.Close()
	time.Sleep(200 * time.Millisecond)
	c2 := dial(t, ts.addr)
	c2.login("영희", false, "마을 광장")
	c2.send("인벤토리")
	c2.readUntil("1) 빵", 5*time.Second)
}
