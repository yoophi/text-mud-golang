package domain

import "testing"

func TestNewWorldRejectsDuplicateRoomIDs(t *testing.T) {
	_, err := NewWorld([]*Room{
		{ID: "a", Name: "A", Exits: nil},
		{ID: "a", Name: "A", Exits: nil},
	}, "a", "")
	if err == nil {
		t.Fatal("duplicate room IDs must be rejected")
	}
}

func TestNewWorldRejectsMissingExitTarget(t *testing.T) {
	_, err := NewWorld([]*Room{
		{ID: "a", Name: "A", Exits: map[Direction]RoomID{DirNorth: "ghost"}},
	}, "a", "")
	if err == nil {
		t.Fatal("exit to missing room must be rejected")
	}
}

func TestNewWorldRejectsUnknownDirection(t *testing.T) {
	_, err := NewWorld([]*Room{
		{ID: "a", Name: "A", Exits: map[Direction]RoomID{Direction("북동"): "a"}},
	}, "a", "")
	if err == nil {
		t.Fatal("unknown direction must be rejected")
	}
}

func TestNewWorldRejectsMissingStartRoom(t *testing.T) {
	_, err := NewWorld([]*Room{{ID: "a", Name: "A"}}, "missing", "")
	if err == nil {
		t.Fatal("missing start room must be rejected")
	}
}

func TestNewWorldRespawnDefaultsToStart(t *testing.T) {
	w, err := NewWorld([]*Room{{ID: "a", Name: "A"}}, "a", "")
	if err != nil {
		t.Fatalf("NewWorld: %v", err)
	}
	if w.Respawn() != w.Start() {
		t.Fatal("respawn should default to start room")
	}
}

func TestRoomExitNamesOrdered(t *testing.T) {
	r := &Room{ID: "a", Exits: map[Direction]RoomID{
		DirWest: "b", DirNorth: "b", DirUp: "b",
	}}
	got := r.ExitNames()
	want := []string{"북쪽", "서쪽", "위"}
	if len(got) != len(want) {
		t.Fatalf("ExitNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExitNames() = %v, want %v", got, want)
		}
	}
}
