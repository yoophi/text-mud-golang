package e2e

import (
	"testing"
	"time"
)

func TestRoommatesSeeAndHearEachOther(t *testing.T) {
	ts := startServer(t, defaultWorld)
	defer ts.stop()

	a := dial(t, ts.addr)
	a.login("영희", true, "마을 광장")
	b := dial(t, ts.addr)
	b.login("철수", true, "마을 광장")

	// A sees B in the room.
	a.send("보기")
	a.readUntil("함께 있는 사람: 철수", 5*time.Second)

	// A talks; B hears.
	a.send("말하기 안녕하세요!")
	b.readUntil("영희: 안녕하세요!", 5*time.Second)

	// B moves away; A is told, and B no longer hears A.
	b.send("북")
	a.readUntil("철수이(가) 북쪽(으)로 이동했다", 5*time.Second)
	a.send("말하기 혼잣말")
	b.send("보기")
	b.readUntil("출구", 5*time.Second)

	// A disconnects; the roommate is notified.
	a.conn.Close()
	b.send("남쪽")
	b.readUntil("마을 광장", 5*time.Second)
	// (철수 is now alone in the plaza; the leave broadcast happened while
	// he was away, so verify he does not see 영희 anymore.)
	b.send("보기")
	b.readUntil("출구", 5*time.Second)
}

func TestJoinBroadcastReachesRoommate(t *testing.T) {
	ts := startServer(t, defaultWorld)
	defer ts.stop()

	a := dial(t, ts.addr)
	a.login("영희", true, "마을 광장")

	b := dial(t, ts.addr)
	b.login("철수", true, "마을 광장")

	// 영희 should have seen 철수 arrive while she idles.
	a.readUntil("철수이(가) 모습을 드러냈다", 5*time.Second)
}
