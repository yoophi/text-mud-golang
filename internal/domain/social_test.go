package domain

import (
	"strings"
	"testing"
)

func TestParseSay(t *testing.T) {
	cmd, err := Parse("말하기 안녕하세요 여러분")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Verb != VerbSay || cmd.Text != "안녕하세요 여러분" {
		t.Fatalf("cmd = %+v", cmd)
	}
	cmd, err = Parse("say hello world")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Text != "hello world" {
		t.Fatalf("text = %q", cmd.Text)
	}
	cmd, err = Parse("말 반갑다")
	if err != nil || cmd.Verb != VerbSay || cmd.Text != "반갑다" {
		t.Fatalf("cmd = %+v, err = %v", cmd, err)
	}
}

func TestSayEmptyTextUsage(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "말하기")
	outs := outputs(effects)
	if len(outs) != 1 || !strings.Contains(outs[0].Text, "무엇을 말") {
		t.Fatalf("empty say should print usage: %+v", outs)
	}
	if hasSave(effects) {
		t.Fatal("empty say must not save")
	}
}

func TestLookShowsOthersInRoom(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))
	g.Connect("s2", NewCharacter("철수", "plaza"))
	g.Connect("s3", NewCharacter("민수", "alley"))

	effects := g.ExecuteLine("s1", "보기")
	outs := outputs(effects)
	if len(outs) != 1 {
		t.Fatalf("want one output, got %+v", outs)
	}
	if !strings.Contains(outs[0].Text, "함께 있는 사람: 철수") {
		t.Fatalf("look should list roommates: %q", outs[0].Text)
	}
	if strings.Contains(outs[0].Text, "민수") {
		t.Fatalf("look must not list characters in other rooms: %q", outs[0].Text)
	}
}

func TestConnectAndDisconnectBroadcastToRoommates(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	var arrive, leave string
	for _, e := range g.Connect("s2", NewCharacter("철수", "plaza")) {
		if b, ok := e.(Broadcast); ok {
			arrive = b.Text
		}
	}
	if !strings.Contains(arrive, "철수") || !strings.Contains(arrive, "모습을 드러냈다") {
		t.Fatalf("arrive broadcast = %q", arrive)
	}
	for _, e := range g.Disconnect("s2") {
		if b, ok := e.(Broadcast); ok {
			leave = b.Text
		}
	}
	if !strings.Contains(leave, "철수") || !strings.Contains(leave, "자취를 감췄다") {
		t.Fatalf("leave broadcast = %q", leave)
	}
}

func TestMoveBroadcastsToBothRooms(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza")) // mover
	g.Connect("s2", NewCharacter("철수", "plaza")) // stays behind
	g.Connect("s3", NewCharacter("민수", "alley")) // waits ahead

	effects := g.ExecuteLine("s1", "북쪽")

	var fromMsg, toMsg string
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok {
			if containsSession(b, "s2") {
				fromMsg = b.Text
			}
			if containsSession(b, "s3") {
				toMsg = b.Text
			}
		}
	}
	if !strings.Contains(fromMsg, "북쪽(으)로 이동했다") {
		t.Fatalf("departure broadcast = %q", fromMsg)
	}
	if !strings.Contains(toMsg, "모습을 드러냈다") {
		t.Fatalf("arrival broadcast = %q", toMsg)
	}
}

func containsSession(b Broadcast, id SessionID) bool {
	for _, s := range b.Sessions {
		if s == id {
			return true
		}
	}
	return false
}
