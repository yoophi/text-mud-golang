package domain

import (
	"strings"
	"testing"
	"time"
)

// fakeClock is a manually advanced Clock for deterministic tests.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// fakeRandom replays a fixed sequence of values.
type fakeRandom struct {
	vals []int
	i    int
}

func (r *fakeRandom) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	v := r.vals[r.i%len(r.vals)]
	r.i++
	return v % n
}

func testWorld(t *testing.T) *World {
	t.Helper()
	w, err := NewWorld(
		[]*Room{
			{ID: "plaza", Name: "마을 광장", Description: "오래된 분수가 있는 광장이다.", Exits: map[Direction]RoomID{DirNorth: "alley"}},
			{ID: "alley", Name: "뒷골목", Description: "좁고 어두운 골목이다.", Exits: map[Direction]RoomID{DirSouth: "plaza"}},
		},
		"plaza", "",
	)
	if err != nil {
		t.Fatalf("NewWorld: %v", err)
	}
	return w
}

func newTestGame(t *testing.T) (*Game, *fakeClock) {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	g := NewGame(testWorld(t), clock, &fakeRandom{vals: []int{1}})
	return g, clock
}

func outputs(effects []Effect) []Output {
	var out []Output
	for _, e := range effects {
		if o, ok := e.(Output); ok {
			out = append(out, o)
		}
	}
	return out
}

func hasSave(effects []Effect) bool {
	for _, e := range effects {
		if _, ok := e.(SaveCharacter); ok {
			return true
		}
	}
	return false
}

func TestConnectEmitsLookAndSave(t *testing.T) {
	g, _ := newTestGame(t)
	effects := g.Connect("s1", NewCharacter("영희", "plaza"))

	var gotLook, gotSave bool
	for _, e := range effects {
		switch e := e.(type) {
		case Output:
			if strings.Contains(e.Text, "마을 광장") {
				gotLook = true
			}
		case SaveCharacter:
			gotSave = true
			if e.Character.Name != "영희" || e.Character.Room != "plaza" {
				t.Fatalf("save effect has wrong character: %+v", e.Character)
			}
		}
	}
	if !gotLook || !gotSave {
		t.Fatalf("Connect effects missing look(%v) or save(%v): %+v", gotLook, gotSave, effects)
	}
	if !g.IsOnline("영희") {
		t.Fatal("character should be online after connect")
	}
}

func TestMoveUpdatesRoomAndEffects(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.Execute("s1", Command{Verb: VerbMove, Direction: DirNorth})

	sawAlley := false
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "뒷골목") {
			sawAlley = true
		}
	}
	if !sawAlley {
		t.Fatalf("move output should show the new room: %+v", outputs(effects))
	}
	if !hasSave(effects) {
		t.Fatal("move should emit a save effect")
	}
	if c, _ := g.Character("영희"); c.Room != "alley" {
		t.Fatalf("character room = %s, want alley", c.Room)
	}
}

func TestMoveBlockedKeepsState(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.Execute("s1", Command{Verb: VerbMove, Direction: DirEast})

	outs := outputs(effects)
	if len(outs) != 1 || !strings.Contains(outs[0].Text, "갈 수 없습니다") {
		t.Fatalf("blocked move should explain itself once: %+v", outs)
	}
	if hasSave(effects) {
		t.Fatal("blocked move must not save")
	}
	if c, _ := g.Character("영희"); c.Room != "plaza" {
		t.Fatalf("blocked move changed room to %s", c.Room)
	}
}

func TestUnknownCommandDoesNotChangeState(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.Execute("s1", Command{Verb: Verb("춤추기")})

	outs := outputs(effects)
	if len(outs) != 1 || !strings.Contains(outs[0].Text, "알 수 없는 명령") {
		t.Fatalf("unknown command should return guidance: %+v", outs)
	}
	if hasSave(effects) {
		t.Fatal("unknown command must not save")
	}
	if c, _ := g.Character("영희"); c.Room != "plaza" || c.HP != DefaultMaxHP {
		t.Fatal("unknown command changed state")
	}
}

func TestDisconnectSavesAndIsIdempotent(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.Disconnect("s1")
	if !hasSave(effects) {
		t.Fatal("disconnect should save the character")
	}
	if g.IsOnline("영희") {
		t.Fatal("character should be offline after disconnect")
	}
	if again := g.Disconnect("s1"); again != nil {
		t.Fatalf("second disconnect should be a no-op, got %+v", again)
	}
}

func TestConnectSameCharacterNameKicksPreviousSession(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))
	g.Connect("s2", NewCharacter("영희", "plaza"))

	if s, _ := g.Session("영희"); s != "s2" {
		t.Fatalf("latest session should own the name, got %s", s)
	}
	if _, online := g.sessions["s1"]; online {
		t.Fatal("old session should be disconnected")
	}
}

func TestTickWithoutEventsIsNoop(t *testing.T) {
	g, clock := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	clock.Advance(time.Minute)
	if effects := g.Tick(clock.Now()); len(effects) != 0 {
		t.Fatalf("tick with no scheduled events should return no effects, got %+v", effects)
	}
}

func TestSayBroadcastsToRoom(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))
	g.Connect("s2", NewCharacter("철수", "plaza"))

	effects := g.Execute("s1", Command{Verb: VerbSay, Text: "안녕!"})

	var heard bool
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok {
			for _, s := range b.Sessions {
				if s == "s2" && strings.Contains(b.Text, "안녕!") {
					heard = true
				}
			}
		}
	}
	if !heard {
		t.Fatalf("s2 should hear the message: %+v", effects)
	}
}
