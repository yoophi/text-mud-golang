package domain

import (
	"strings"
	"testing"
	"time"
)

func newAdminGame(t *testing.T) *Game {
	t.Helper()
	rooms := []*Room{
		{ID: "plaza", Name: "마을 광장", Description: "광장이다.", Exits: map[Direction]RoomID{DirNorth: "alley"}},
		{ID: "alley", Name: "뒷골목", Description: "골목이다.", Exits: map[Direction]RoomID{DirSouth: "plaza"}},
		{ID: "cellar", Name: "지하 저장고", Description: "저장고다.", Exits: nil},
	}
	w, err := NewWorld(rooms, "plaza", "plaza")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.SetNPCs([]*NPCDef{
		{ID: "rat", Name: "쥐", Room: "alley", HP: 4, DamageMin: 1, DamageMax: 2, RespawnDelay: 30 * time.Second},
	}); err != nil {
		t.Fatal(err)
	}
	return NewGame(w, &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}, &fakeRandom{vals: []int{0}})
}

func operatorCharacter(name string, room RoomID) *Character {
	c := NewCharacter(name, room)
	c.Operator = true
	return c
}

func TestParseAdminCommands(t *testing.T) {
	cmd, err := Parse("@goto cellar")
	if err != nil || cmd.Verb != VerbAdmin || cmd.Admin != AdminGoto || cmd.AdminArg != "cellar" {
		t.Fatalf("cmd = %+v err = %v", cmd, err)
	}
	cmd, err = Parse("운영 종료")
	if err != nil || cmd.Admin != AdminShutdown {
		t.Fatalf("cmd = %+v err = %v", cmd, err)
	}
	cmd, err = Parse("@spawn rat cellar")
	if err != nil || cmd.Admin != AdminSpawn || cmd.AdminArg != "rat cellar" {
		t.Fatalf("cmd = %+v err = %v", cmd, err)
	}
	if _, err := Parse("@fly plaza"); err == nil {
		t.Fatal("unknown admin action must fail")
	}
	if _, err := Parse("@goto"); err == nil {
		t.Fatal("goto without argument must fail")
	}
}

func TestAdminRequiresOperator(t *testing.T) {
	g := newAdminGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "@goto cellar")
	outs := outputs(effects)
	if len(outs) != 1 || !strings.Contains(outs[0].Text, "운영자 권한") {
		t.Fatalf("non-operator must be rejected: %+v", outs)
	}
	if hasSave(effects) {
		t.Fatal("rejected admin command must not save")
	}
	snap, _ := g.Character("영희")
	if snap.Room != "plaza" {
		t.Fatal("rejected goto must not move")
	}
}

func TestAdminGotoTeleports(t *testing.T) {
	g := newAdminGame(t)
	g.Connect("s1", operatorCharacter("영희", "plaza"))
	g.Connect("s2", NewCharacter("철수", "cellar"))

	effects := g.ExecuteLine("s1", "@goto cellar")
	if !strings.Contains(outputs(effects)[0].Text, "지하 저장고") {
		t.Fatalf("goto should show the target room: %+v", outputs(effects))
	}
	if !hasSave(effects) {
		t.Fatal("goto should save")
	}
	snap, _ := g.Character("영희")
	if snap.Room != "cellar" {
		t.Fatalf("room = %s", snap.Room)
	}
	var told bool
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok && containsSession(b, "s2") && strings.Contains(b.Text, "영희") {
			told = true
		}
	}
	if !told {
		t.Fatal("players in the target room should see the arrival")
	}

	if out := outputs(g.ExecuteLine("s1", "@goto nowhere"))[0].Text; !strings.Contains(out, "그런 방이 없습니다") {
		t.Fatalf("unknown room = %q", out)
	}
}

func TestAdminSpawnCreatesNPC(t *testing.T) {
	g := newAdminGame(t)
	g.Connect("s1", operatorCharacter("영희", "plaza"))

	// Spawn by definition ID into the current room.
	effects := g.ExecuteLine("s1", "@spawn rat")
	if !strings.Contains(outputs(effects)[0].Text, "생성했습니다") {
		t.Fatalf("spawn output: %+v", outputs(effects))
	}
	if got := g.npcNames("plaza"); len(got) != 1 || got[0] != "쥐" {
		t.Fatalf("plaza npcs = %v", got)
	}

	// Spawn by display name into an explicit room.
	effects = g.ExecuteLine("s1", "@spawn 쥐 cellar")
	if got := g.npcNames("cellar"); len(got) != 1 {
		t.Fatalf("cellar npcs = %v", got)
	}

	if out := outputs(g.ExecuteLine("s1", "@spawn 드래곤"))[0].Text; !strings.Contains(out, "그런 NPC 정의") {
		t.Fatalf("unknown def = %q", out)
	}
	if out := outputs(g.ExecuteLine("s1", "@spawn rat void"))[0].Text; !strings.Contains(out, "그런 방") {
		t.Fatalf("unknown room = %q", out)
	}
}

func TestAdminShutdownEmitsEffect(t *testing.T) {
	g := newAdminGame(t)
	g.Connect("s1", operatorCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "@shutdown")
	var saw bool
	for _, e := range effects {
		if s, ok := e.(Shutdown); ok && s.Reason == "운영자 종료 명령" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("shutdown effect missing: %+v", effects)
	}
}

func TestAdminBreaksCombatOnTeleport(t *testing.T) {
	g := newAdminGame(t)
	g.Connect("s1", operatorCharacter("영희", "alley"))
	g.ExecuteLine("s1", "공격 쥐")

	g.ExecuteLine("s1", "@goto cellar")
	snap, _ := g.Character("영희")
	if snap.InCombat() {
		t.Fatal("teleport must break combat")
	}
}
