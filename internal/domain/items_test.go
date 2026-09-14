package domain

import (
	"strings"
	"testing"
	"time"
)

func newItemWorld(t *testing.T) *Game {
	t.Helper()
	rooms := []*Room{
		{ID: "plaza", Name: "마을 광장", Description: "광장이다.", Exits: map[Direction]RoomID{DirNorth: "alley"}},
		{ID: "alley", Name: "뒷골목", Description: "골목이다.", Exits: map[Direction]RoomID{DirSouth: "plaza"}},
	}
	w, err := NewWorld(rooms, "plaza", "")
	if err != nil {
		t.Fatal(err)
	}
	err = w.SetItems([]*ItemProto{
		{ID: "bread", Name: "빵", Aliases: []string{"식빵"}},
		{ID: "coin", Name: "동전"},
	}, []ItemSpawn{
		{Room: "plaza", Item: "bread", Count: 2},
		{Room: "plaza", Item: "coin", Count: 1},
		{Room: "alley", Item: "coin", Count: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	return NewGame(w, clock, &fakeRandom{vals: []int{1}})
}

func TestBootSpawnsFloorItemsVisibleInLook(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "보기")
	text := outputs(effects)[0].Text
	for _, want := range []string{"빵, 빵, 동전"} {
		if !strings.Contains(text, want) {
			t.Fatalf("look should show floor items, got %q", text)
		}
	}
}

func TestPickUpMovesItemToInventory(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))
	g.Connect("s2", NewCharacter("철수", "plaza"))

	effects := g.ExecuteLine("s1", "줍기 동전")

	if !strings.Contains(outputs(effects)[0].Text, "동전을(를) 주웠") {
		t.Fatalf("pickup output: %+v", outputs(effects))
	}
	if !hasSave(effects) {
		t.Fatal("pickup must save")
	}
	var sawBroadcast bool
	for _, e := range effects {
		if b, ok := e.(Broadcast); ok {
			if containsSession(b, "s2") && strings.Contains(b.Text, "동전을(를) 주웠다") {
				sawBroadcast = true
			}
		}
	}
	if !sawBroadcast {
		t.Fatalf("roommate should see the pickup: %+v", effects)
	}

	snap, _ := g.Character("영희")
	if len(snap.Inventory) != 1 || snap.Inventory[0].Proto != "coin" {
		t.Fatalf("inventory = %+v", snap.Inventory)
	}
	// Item must no longer be on the floor: single location invariant.
	look := outputs(g.ExecuteLine("s1", "보기"))[0].Text
	if strings.Contains(look, "동전") {
		t.Fatal("picked up item must leave the floor")
	}
}

func TestPickUpAmbiguousRequiresIndex(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "줍기 빵")
	text := outputs(effects)[0].Text
	if !strings.Contains(text, "여러 개") || !strings.Contains(text, "줍기 빵 2") {
		t.Fatalf("ambiguity prompt = %q", text)
	}
	if hasSave(effects) {
		t.Fatal("ambiguous pickup must not save")
	}

	// Numbered selection picks exactly one.
	effects = g.ExecuteLine("s1", "줍기 빵 2")
	if !strings.Contains(outputs(effects)[0].Text, "주웠") {
		t.Fatalf("indexed pickup failed: %+v", effects)
	}
	snap, _ := g.Character("영희")
	if len(snap.Inventory) != 1 {
		t.Fatalf("inventory = %+v", snap.Inventory)
	}
	// One bread remains on the floor.
	look := outputs(g.ExecuteLine("s1", "보기"))[0].Text
	if !strings.Contains(look, "빵") {
		t.Fatal("one bread should remain on the floor")
	}
}

func TestPickUpUnknownTarget(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "줍기 검")
	if !strings.Contains(outputs(effects)[0].Text, "그런 물건이 바닥에 없습니다") {
		t.Fatalf("output = %+v", outputs(effects))
	}
}

func TestPickUpWithParticle(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "줍기 동전을")
	if !strings.Contains(outputs(effects)[0].Text, "주웠") {
		t.Fatalf("particle target should match: %+v", outputs(effects))
	}
}

func TestDropRoundTrip(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))
	g.ExecuteLine("s1", "줍기 동전")

	effects := g.ExecuteLine("s1", "버리기 동전")
	if !strings.Contains(outputs(effects)[0].Text, "바닥에 놓았") {
		t.Fatalf("drop output: %+v", outputs(effects))
	}
	if !hasSave(effects) {
		t.Fatal("drop must save")
	}
	snap, _ := g.Character("영희")
	if len(snap.Inventory) != 0 {
		t.Fatalf("inventory after drop = %+v", snap.Inventory)
	}
	// Item back on the floor exactly once.
	look := outputs(g.ExecuteLine("s1", "보기"))[0].Text
	if strings.Count(look, "동전") != 1 {
		t.Fatalf("floor after drop: %q", look)
	}
}

func TestInventoryListing(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	if out := outputs(g.ExecuteLine("s1", "인벤토리"))[0].Text; !strings.Contains(out, "가진 물건이 없습니다") {
		t.Fatalf("empty inventory = %q", out)
	}
	g.ExecuteLine("s1", "줍기 동전")
	out := outputs(g.ExecuteLine("s1", "i"))[0].Text
	if !strings.Contains(out, "1) 동전") {
		t.Fatalf("inventory listing = %q", out)
	}
}

func TestItemsInOtherRoomsNotVisible(t *testing.T) {
	g := newItemWorld(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	look := outputs(g.ExecuteLine("s1", "보기"))[0].Text
	count := strings.Count(look, "동전")
	if count != 1 {
		t.Fatalf("plaza should show exactly one coin, got %d in %q", count, look)
	}
}

func TestItemInstanceIDsDoNotCollideAcrossRestarts(t *testing.T) {
	// Boot 1: pick up the coin and persist the character.
	g1 := newItemWorld(t)
	g1.Connect("s1", NewCharacter("영희", "plaza"))
	g1.ExecuteLine("s1", "줍기 동전")
	saved, _ := g1.Character("영희")
	if len(saved.Inventory) != 1 {
		t.Fatalf("inventory = %+v", saved.Inventory)
	}

	// Boot 2: same world definition, fresh process counter.
	g2 := newItemWorld(t)
	seen := map[string]int{}
	for _, it := range g2.rooms["plaza"].items {
		seen[it.ID]++
	}
	for _, it := range saved.Inventory {
		seen[it.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("item instance ID collides across restarts: %s", id)
		}
	}

	// Restore the character, drop the old coin among fresh floor items,
	// and pick one back: exactly-once invariants must hold.
	g2.Connect("s1", saved)
	g2.ExecuteLine("s1", "버리기 동전")
	look := outputs(g2.ExecuteLine("s1", "보기"))[0].Text
	if strings.Count(look, "동전") != 2 {
		t.Fatalf("floor should hold two distinct coins: %q", look)
	}
	g2.ExecuteLine("s1", "줍기 동전 2")
	snap, _ := g2.Character("영희")
	if len(snap.Inventory) != 1 {
		t.Fatalf("inventory after re-pick = %+v", snap.Inventory)
	}

	ids := map[string]bool{}
	for _, it := range snap.Inventory {
		if ids[it.ID] {
			t.Fatalf("duplicate instance id in inventory: %s", it.ID)
		}
		ids[it.ID] = true
	}
	for _, state := range g2.rooms {
		for _, it := range state.items {
			if ids[it.ID] {
				t.Fatalf("instance id in two places at once: %s", it.ID)
			}
			ids[it.ID] = true
		}
	}
}
