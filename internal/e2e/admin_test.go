package e2e

import (
	"net"
	"testing"
	"time"
)

func TestOperatorCommandsAndShutdownOverTCP(t *testing.T) {
	ts := startServer(t, defaultWorld, "영희")
	op := dial(t, ts.addr)
	op.login("영희", true, "마을 광장")

	// Non-operators are rejected.
	user := dial(t, ts.addr)
	user.login("철수", true, "마을 광장")
	user.send("@goto market")
	user.readUntil("운영자 권한", 5*time.Second)

	// @goto teleports.
	op.send("@goto market")
	op.readUntil("활기찬 시장", 5*time.Second)

	// @spawn creates an NPC by definition.
	op.send("@spawn rat")
	op.readUntil("생성했습니다", 5*time.Second)
	op.send("보기")
	op.readUntil("이곳에 있는 존재: 쥐", 5*time.Second)

	// @shutdown saves and stops the server; the state survives a restart.
	op.send("@shutdown")
	op.readUntil("서버를 종료합니다", 5*time.Second)
	ts.waitStopped()

	// The listener is gone: new connections are refused.
	if _, err := net.DialTimeout("tcp", ts.addr, 2*time.Second); err == nil {
		t.Fatal("server must stop accepting after shutdown")
	}

	ts.restart()
	back := dial(t, ts.addr)
	back.login("영희", false, "활기찬 시장")
}
