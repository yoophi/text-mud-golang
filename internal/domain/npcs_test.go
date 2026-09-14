package domain

import (
	"strings"
	"testing"
	"time"
)

func newNPCGame(t *testing.T) (*Game, *fakeClock) {
	t.Helper()
	rooms := []*Room{
		{ID: "plaza", Name: "마을 광장", Description: "광장이다.", Exits: map[Direction]RoomID{DirNorth: "alley"}},
		{ID: "alley", Name: "뒷골목", Description: "골목이다.", Exits: map[Direction]RoomID{DirSouth: "plaza"}},
	}
	w, err := NewWorld(rooms, "plaza", "")
	if err != nil {
		t.Fatal(err)
	}
	err = w.SetNPCs([]*NPCDef{
		{ID: "cat", Name: "고양이", Aliases: []string{"냥"}, Room: "plaza", HP: 10, DamageMin: 1, DamageMax: 2, RespawnDelay: 30 * time.Second, WanderInterval: 5 * time.Second},
		{ID: "rat", Name: "쥐", Room: "alley", HP: 4, DamageMin: 1, DamageMax: 1, RespawnDelay: 10 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	g := NewGame(w, clock, &fakeRandom{vals: []int{0}})
	return g, clock
}

func TestNPCsSpawnAtBootAndShowInLook(t *testing.T) {
	g, _ := newNPCGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	look := outputs(g.ExecuteLine("s1", "보기"))[0].Text
	if !strings.Contains(look, "이곳에 있는 존재: 고양이") {
		t.Fatalf("look should list NPCs: %q", look)
	}
}

func TestWanderMovesNPCOnSchedule(t *testing.T) {
	g, clock := newNPCGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	// Before the timer nothing happens.
	clock.Advance(4 * time.Second)
	if effects := g.Tick(clock.Now()); len(effects) != 0 {
		t.Fatalf("early tick produced effects: %+v", effects)
	}
	if len(g.npcNames("alley")) != 1 { // 쥐 still there
		t.Fatal("npc list changed early")
	}

	// At the timer the cat wanders north (fakeRandom picks index 0).
	g.Connect("s2", NewCharacter("철수", "alley"))
	clock.Advance(1 * time.Second)
	effects := g.Tick(clock.Now())

	var sawLeave, sawArrive bool
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok {
			if containsSession(b, "s1") && strings.Contains(b.Text, "고양이") && strings.Contains(b.Text, "사라졌다") {
				sawLeave = true
			}
			if containsSession(b, "s2") && strings.Contains(b.Text, "고양이") && strings.Contains(b.Text, "모습을 드러냈다") {
				sawArrive = true
			}
		}
	}
	if !sawLeave || !sawArrive {
		t.Fatalf("wander broadcasts missing (leave=%v arrive=%v): %+v", sawLeave, sawArrive, effects)
	}
	if got := g.npcNames("alley"); len(got) != 2 {
		t.Fatalf("alley should now hold 쥐 and 고양이: %v", got)
	}
	if got := g.npcNames("plaza"); len(got) != 0 {
		t.Fatalf("plaza should be empty of NPCs: %v", got)
	}
}

func TestWanderEventRunsExactlyOnce(t *testing.T) {
	g, clock := newNPCGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	clock.Advance(5 * time.Second)
	first := g.Tick(clock.Now())
	if len(first) == 0 {
		t.Fatal("wander should fire at its scheduled time")
	}
	// Re-ticking at the same instant must not repeat the event.
	if again := g.Tick(clock.Now()); len(again) != 0 {
		t.Fatalf("duplicate wander execution: %+v", again)
	}
	// The next wander is 5s later, not before.
	clock.Advance(4 * time.Second)
	if effects := g.Tick(clock.Now()); len(effects) != 0 {
		t.Fatalf("wander fired early: %+v", effects)
	}
	clock.Advance(1 * time.Second)
	if effects := g.Tick(clock.Now()); len(effects) == 0 {
		t.Fatal("rescheduled wander should fire")
	}
}

func TestStationaryNPDoesNotWander(t *testing.T) {
	g, clock := newNPCGame(t)
	g.Connect("s1", NewCharacter("영희", "alley"))

	// Run many wander cycles; the wandering cat may come and go, but the
	// stationary rat must never move.
	for i := 0; i < 12; i++ {
		clock.Advance(5 * time.Second)
		g.Tick(clock.Now())
		if inst, ok := g.npcByName("쥐"); !ok || inst.Room != "alley" {
			t.Fatalf("step %d: 쥐 moved to %v", i, inst.Room)
		}
	}
}

func TestRespawnAfterDespawn(t *testing.T) {
	g, clock := newNPCGame(t)
	g.Connect("s1", NewCharacter("영희", "alley"))

	// White-box: simulate the NPC dying (public death path arrives with
	// combat in #11) and confirm the respawn rule. The wandering cat is
	// excluded from assertions by tracking the rat specifically.
	rat, ok := g.npcByName("쥐")
	if !ok {
		t.Fatal("rat not spawned")
	}
	oldID := rat.InstID
	g.despawnNPC(rat)

	if _, still := g.npcByID(oldID); still {
		t.Fatal("despawned NPC should leave the room")
	}
	// Respawn not before the delay.
	clock.Advance(9 * time.Second)
	for _, e := range g.Tick(clock.Now()) {
		if b, ok := e.(Broadcast); ok && strings.Contains(b.Text, "쥐") {
			t.Fatalf("respawn fired early: %+v", e)
		}
	}
	clock.Advance(1*time.Second + time.Millisecond)
	effects := g.Tick(clock.Now())
	var saw bool
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok && strings.Contains(b.Text, "쥐") && strings.Contains(b.Text, "모습을 드러냈다") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("respawn broadcast missing: %+v", effects)
	}
	if again := g.Tick(clock.Now()); len(again) != 0 {
		t.Fatalf("respawn must run exactly once: %+v", again)
	}
	inst2, ok := g.npcByName("쥐")
	if !ok || inst2.InstID == oldID {
		t.Fatal("respawned NPC should be a new instance")
	}
	if inst2.Room != "alley" {
		t.Fatalf("respawn must be at the defined room: %s", inst2.Room)
	}
}

func TestRespawnSkipsWhenInstanceAlive(t *testing.T) {
	g, _ := newNPCGame(t)
	def := g.world.NPCDefs()[1] // 쥐 in alley

	// A live 쥐 exists; a stray respawn event must be ignored.
	if effects := g.respawnNPC(def, "alley"); len(effects) != 0 {
		t.Fatalf("duplicate respawn should be skipped: %+v", effects)
	}
	if got := g.npcNames("alley"); len(got) != 1 {
		t.Fatalf("duplicate npc created: %v", got)
	}
}

func TestMatchNPCsByAliasAndParticle(t *testing.T) {
	g, _ := newNPCGame(t)
	if got := g.matchNPCs("plaza", "고양이를"); len(got) != 1 || got[0].Def.ID != "cat" {
		t.Fatalf("particle target should match: %+v", got)
	}
	if got := g.matchNPCs("plaza", "냥"); len(got) != 1 {
		t.Fatalf("alias target should match: %+v", got)
	}
	if got := g.matchNPCs("plaza", "쥐"); len(got) != 0 {
		t.Fatalf("npc in other room must not match: %+v", got)
	}
}

func TestSetNPCsValidation(t *testing.T) {
	w, err := NewWorld([]*Room{{ID: "a", Name: "A"}}, "a", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.SetNPCs([]*NPCDef{{ID: "x", Name: "X", Room: "void", HP: 1, RespawnDelay: time.Second}}); err == nil {
		t.Fatal("npc in unknown room must be rejected")
	}
	if err := w.SetNPCs([]*NPCDef{{ID: "x", Name: "X", Room: "a", HP: 0, RespawnDelay: time.Second}}); err == nil {
		t.Fatal("zero hp must be rejected")
	}
	if err := w.SetNPCs([]*NPCDef{
		{ID: "x", Name: "X", Room: "a", HP: 1, RespawnDelay: time.Second},
		{ID: "x", Name: "X", Room: "a", HP: 1, RespawnDelay: time.Second},
	}); err == nil {
		t.Fatal("duplicate npc ids must be rejected")
	}
}

func firstKey(m map[string]*NPCInstance) string {
	for k := range m {
		return k
	}
	return ""
}
