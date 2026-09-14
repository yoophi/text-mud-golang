package domain

import (
	"strings"
	"testing"
	"time"
)

func newCombatGame(t *testing.T, ratHP, ratDmgMin, ratDmgMax int, aggressive bool) (*Game, *fakeClock) {
	t.Helper()
	rooms := []*Room{
		{ID: "plaza", Name: "마을 광장", Description: "광장이다.", Exits: map[Direction]RoomID{DirNorth: "alley"}},
		{ID: "alley", Name: "뒷골목", Description: "골목이다.", Exits: map[Direction]RoomID{DirSouth: "plaza", DirNorth: "cellar"}},
		{ID: "cellar", Name: "지하 저장고", Description: "축축한 저장고다.", Exits: map[Direction]RoomID{DirSouth: "alley"}},
	}
	w, err := NewWorld(rooms, "plaza", "plaza")
	if err != nil {
		t.Fatal(err)
	}
	err = w.SetNPCs([]*NPCDef{
		{ID: "rat", Name: "쥐", Aliases: []string{"생쥐"}, Room: "alley", HP: ratHP, DamageMin: ratDmgMin, DamageMax: ratDmgMax, Aggressive: aggressive, RespawnDelay: 30 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	g := NewGame(w, clock, &fakeRandom{vals: []int{0}})
	return g, clock
}

func TestAttackFirstStrikeAndScheduledRounds(t *testing.T) {
	g, clock := newCombatGame(t, 50, 1, 3, false)
	g.Connect("s1", NewCharacter("영희", "alley"))

	effects := g.ExecuteLine("s1", "공격 쥐")

	// Immediate first strike with the minimum roll (fake random = 0).
	found := false
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "당신은 쥐을(를) 공격했습니다") && strings.Contains(o.Text, "피해 2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("first strike output missing: %+v", outputs(effects))
	}
	if inst, _ := g.npcByName("쥐"); inst.HP != 50-2 {
		t.Fatalf("npc hp = %d, want %d", inst.HP, 50-2)
	}

	// One round later both the player and the NPC strike (player first).
	g.Connect("s2", NewCharacter("철수", "alley"))
	clock.Advance(attackInterval)
	effects = g.Tick(clock.Now())

	var playerHit, npcHit bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "당신은 쥐을(를) 공격했습니다") {
			playerHit = true
		}
		if strings.Contains(o.Text, "쥐이(가) 당신을 공격했습니다") {
			npcHit = true
		}
	}
	if !playerHit || !npcHit {
		t.Fatalf("scheduled round missing (player=%v npc=%v): %+v", playerHit, npcHit, outputs(effects))
	}
	snap, _ := g.Character("영희")
	if snap.HP != DefaultMaxHP-1 { // npc min damage 1
		t.Fatalf("player hp = %d, want %d", snap.HP, DefaultMaxHP-1)
	}

	// Rounds keep firing on the next interval.
	clock.Advance(attackInterval)
	effects = g.Tick(clock.Now())
	if len(outputs(effects)) == 0 {
		t.Fatal("combat should continue automatically")
	}
}

func TestAttackUnknownAndAmbiguousTargets(t *testing.T) {
	g, _ := newCombatGame(t, 5, 1, 1, false)
	g.Connect("s1", NewCharacter("영희", "alley"))

	if out := outputs(g.ExecuteLine("s1", "공격"))[0].Text; !strings.Contains(out, "누구를") {
		t.Fatalf("usage = %q", out)
	}
	if out := outputs(g.ExecuteLine("s1", "공격 드래곤"))[0].Text; !strings.Contains(out, "그런 대상") {
		t.Fatalf("missing target = %q", out)
	}
	if out := outputs(g.ExecuteLine("s1", "공격 쥐를"))[0].Text; !strings.Contains(out, "공격했습니다") {
		t.Fatalf("particle target should hit: %q", out)
	}
}

func TestAttackWhileInCombatRejected(t *testing.T) {
	g, _ := newCombatGame(t, 50, 1, 1, false)
	g.Connect("s1", NewCharacter("영희", "alley"))
	g.ExecuteLine("s1", "공격 쥐")

	if out := outputs(g.ExecuteLine("s1", "공격 쥐"))[0].Text; !strings.Contains(out, "이미 전투 중") {
		t.Fatalf("double engage = %q", out)
	}
}

func TestNPCDeathStopsCombatAndSchedulesRespawn(t *testing.T) {
	// fakeRandom returns 0: player deals min damage 2, npc deals min 1.
	g, clock := newCombatGame(t, 4, 1, 1, false)
	g.Connect("s1", NewCharacter("영희", "alley"))

	g.ExecuteLine("s1", "공격 쥐") // first hit: 2 damage

	clock.Advance(attackInterval)
	effects := g.Tick(clock.Now())

	var defeated bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "쓰러뜨렸습니다") {
			defeated = true
		}
	}
	if !defeated {
		t.Fatalf("npc should die on the second hit: %+v", outputs(effects))
	}
	if _, alive := g.npcByName("쥐"); alive {
		t.Fatal("dead npc must despawn")
	}
	snap, _ := g.Character("영희")
	if snap.InCombat() {
		t.Fatal("combat must end when the target dies")
	}
	if !hasSave(effects) {
		t.Fatal("npc kill should save")
	}

	// No further combat rounds fire.
	clock.Advance(attackInterval)
	if effects := g.Tick(clock.Now()); len(outputs(effects)) != 0 {
		t.Fatalf("combat continued after death: %+v", outputs(effects))
	}

	// Respawn after the configured delay.
	clock.Advance(30 * time.Second)
	effects = g.Tick(clock.Now())
	var respawned bool
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok && strings.Contains(b.Text, "쥐") && strings.Contains(b.Text, "모습을 드러냈다") {
			respawned = true
		}
	}
	if !respawned {
		t.Fatalf("npc should respawn: %+v", effects)
	}
	if inst, ok := g.npcByName("쥐"); !ok || inst.HP != 4 || inst.engaged != "" {
		t.Fatalf("respawned npc invalid: %+v", inst)
	}
}

func TestPlayerDeathRevivesAtRespawnRoom(t *testing.T) {
	// Rat hits for 999 to guarantee a one-shot kill.
	g, clock := newCombatGame(t, 500, 999, 999, false)
	g.Connect("s1", NewCharacter("영희", "alley"))
	g.Connect("s2", NewCharacter("철수", "alley"))

	g.ExecuteLine("s1", "공격 쥐")
	clock.Advance(attackInterval)
	effects := g.Tick(clock.Now())

	var revived, roommateSaw bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "부활 지점") {
			revived = true
		}
	}
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok && containsSession(b, "s2") && strings.Contains(b.Text, "쓰러졌다") {
			roommateSaw = true
		}
	}
	if !revived || !roommateSaw {
		t.Fatalf("death effects missing (revived=%v seen=%v): %+v", revived, roommateSaw, effects)
	}
	snap, _ := g.Character("영희")
	if snap.Room != "plaza" || snap.HP != snap.MaxHP || snap.InCombat() {
		t.Fatalf("dead player should revive at respawn room with full hp: %+v", snap)
	}
	if !hasSave(effects) {
		t.Fatal("death should save")
	}

	// The rat disengages.
	if inst, _ := g.npcByName("쥐"); inst.engaged != "" {
		t.Fatal("npc must disengage when the player dies")
	}

	// The dead player's rounds no longer fire.
	clock.Advance(attackInterval)
	if effects := g.Tick(clock.Now()); len(outputs(effects)) != 0 {
		t.Fatalf("no rounds should fire for the dead player: %+v", outputs(effects))
	}
}

func TestMovingAwayBreaksCombat(t *testing.T) {
	g, clock := newCombatGame(t, 500, 10, 10, false)
	g.Connect("s1", NewCharacter("영희", "alley"))

	g.ExecuteLine("s1", "공격 쥐")
	g.ExecuteLine("s1", "남쪽") // flee to plaza

	snap, _ := g.Character("영희")
	if snap.Room != "plaza" || snap.InCombat() {
		t.Fatalf("player should have fled and disengaged: %+v", snap)
	}
	if inst, _ := g.npcByName("쥐"); inst.engaged != "" {
		t.Fatal("npc should be free after the player flees")
	}

	// Scheduled rounds are cancelled.
	clock.Advance(attackInterval)
	effects := g.Tick(clock.Now())
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "공격") {
			t.Fatalf("combat round fired after fleeing: %q", o.Text)
		}
	}
}

func TestDisconnectDuringCombatCancelsRounds(t *testing.T) {
	g, clock := newCombatGame(t, 500, 10, 10, false)
	g.Connect("s1", NewCharacter("영희", "alley"))
	g.ExecuteLine("s1", "공격 쥐")

	g.Disconnect("s1")
	clock.Advance(attackInterval)
	if effects := g.Tick(clock.Now()); len(effects) != 0 {
		t.Fatalf("rounds must stop after disconnect: %+v", effects)
	}
	if inst, _ := g.npcByName("쥐"); inst.engaged != "" {
		t.Fatal("npc should disengage on disconnect")
	}
}

func TestAggressiveNPCAttacksOnEntry(t *testing.T) {
	g, clock := newCombatGame(t, 50, 5, 5, true)

	// Player walks into the alley; the rat notices them.
	g.Connect("s1", NewCharacter("영희", "plaza"))
	g.ExecuteLine("s1", "북쪽")

	snap, _ := g.Character("영희")
	if !snap.InCombat() {
		t.Fatal("aggressive npc should engage on entry")
	}

	clock.Advance(attackInterval)
	effects := g.Tick(clock.Now())
	var hit bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "쥐이(가) 당신을 공격했습니다") {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("aggressive npc should strike: %+v", outputs(effects))
	}
}

func TestAggroPicksSingleVictim(t *testing.T) {
	g, clock := newCombatGame(t, 50, 3, 3, true)
	g.Connect("s1", NewCharacter("영희", "alley"))
	g.Connect("s2", NewCharacter("철수", "alley"))

	// Both connected into the rat's room; it engages exactly one.
	clock.Advance(attackInterval)
	g.Tick(clock.Now())

	engagedCount := 0
	for _, name := range []string{"영희", "철수"} {
		if snap, ok := g.Character(name); ok && snap.InCombat() {
			engagedCount++
		}
	}
	if engagedCount != 1 {
		t.Fatalf("exactly one player should be engaged, got %d", engagedCount)
	}
}

func TestRegenHealsOutOfCombat(t *testing.T) {
	g, clock := newCombatGame(t, 500, 3, 3, false)
	g.Connect("s1", NewCharacter("영희", "alley"))

	// Hurt the player (white-box; damage normally comes from combat).
	c := g.sessions["s1"]
	c.HP = 10

	clock.Advance(regenInterval)
	effects := g.Tick(clock.Now())
	var healed bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "몸이 회복") {
			healed = true
		}
	}
	if !healed {
		t.Fatalf("regen should fire: %+v", outputs(effects))
	}
	snap, _ := g.Character("영희")
	if snap.HP != 10+regenAmount {
		t.Fatalf("hp = %d", snap.HP)
	}

	// Full HP: no message, but the timer keeps rescheduling harmlessly.
	for i := 0; i < 20; i++ {
		clock.Advance(regenInterval)
		g.Tick(clock.Now())
	}
	snap, _ = g.Character("영희")
	if snap.HP != snap.MaxHP {
		t.Fatalf("hp should cap at max: %d", snap.HP)
	}
}

func TestDeterministicCombatReproducible(t *testing.T) {
	run := func() (int, int) {
		g, clock := newCombatGame(t, 500, 1, 4, false)
		g.Connect("s1", NewCharacter("영희", "alley"))
		g.ExecuteLine("s1", "공격 쥐")
		for i := 0; i < 3; i++ {
			clock.Advance(attackInterval)
			g.Tick(clock.Now())
		}
		npc, _ := g.npcByName("쥐")
		player, _ := g.Character("영희")
		return npc.HP, player.HP
	}
	a1, b1 := run()
	a2, b2 := run()
	if a1 != a2 || b1 != b2 {
		t.Fatalf("combat not reproducible: (%d,%d) vs (%d,%d)", a1, b1, a2, b2)
	}
}

func TestAggroedPlayerRetaliatesAutomatically(t *testing.T) {
	g, clock := newCombatGame(t, 50, 5, 5, true)
	g.Connect("s1", NewCharacter("영희", "alley"))

	wolf, ok := g.npcByName("쥐")
	if !ok {
		t.Fatal("npc missing")
	}
	before := wolf.HP

	clock.Advance(attackInterval)
	effects := g.Tick(clock.Now())

	var playerStruck, npcStruck bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "당신은 쥐을(를) 공격했습니다") {
			playerStruck = true
		}
		if strings.Contains(o.Text, "쥐이(가) 당신을 공격했습니다") {
			npcStruck = true
		}
	}
	if !playerStruck || !npcStruck {
		t.Fatalf("ambushed player must fight back (player=%v npc=%v): %+v", playerStruck, npcStruck, outputs(effects))
	}
	if wolf.HP >= before {
		t.Fatalf("retaliation should damage the ambusher: %d -> %d", before, wolf.HP)
	}
}

func TestRegenResumesAfterFleeingCombat(t *testing.T) {
	g, clock := newCombatGame(t, 500, 3, 3, false)
	g.Connect("s1", NewCharacter("영희", "alley"))

	g.ExecuteLine("s1", "공격 쥐")
	g.ExecuteLine("s1", "남쪽") // flee breaks combat

	// White-box wound: regen must still be scheduled after disengaging.
	g.sessions["s1"].HP = 10

	clock.Advance(attackInterval)
	if effects := g.Tick(clock.Now()); len(outputs(effects)) != 0 {
		t.Fatalf("fled combat must not keep fighting: %+v", outputs(effects))
	}
	clock.Advance(regenInterval - attackInterval)
	effects := g.Tick(clock.Now())

	var healed bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "몸이 회복") {
			healed = true
		}
	}
	if !healed {
		t.Fatalf("regeneration must survive combat break: %+v", outputs(effects))
	}
	snap, _ := g.Character("영희")
	if snap.HP != 10+regenAmount {
		t.Fatalf("hp = %d, want %d", snap.HP, 10+regenAmount)
	}
}

func TestRegenResumesAfterNPCKill(t *testing.T) {
	g, clock := newCombatGame(t, 2, 1, 1, false)
	g.Connect("s1", NewCharacter("영희", "alley"))
	g.ExecuteLine("s1", "공격 쥐") // kills the rat, ending combat

	g.sessions["s1"].HP = 10
	clock.Advance(regenInterval)
	effects := g.Tick(clock.Now())

	var healed bool
	for _, o := range outputs(effects) {
		if strings.Contains(o.Text, "몸이 회복") {
			healed = true
		}
	}
	if !healed {
		t.Fatalf("regeneration must survive npc death: %+v", outputs(effects))
	}
}
