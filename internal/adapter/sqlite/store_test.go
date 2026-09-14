package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yoophi/text-mud-golang/internal/application"
	"github.com/yoophi/text-mud-golang/internal/domain"
)

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "mud.db"))
	ctx := context.Background()

	c := domain.NewCharacter("영희", "plaza")
	c.Inventory = []domain.ItemInstance{{ID: "i1", Proto: "coin"}, {ID: "i2", Proto: "bread"}}
	if err := store.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load(ctx, "영희")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "영희" || loaded.Room != "plaza" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(loaded.Inventory) != 2 || loaded.Inventory[0].Proto != "coin" || loaded.Inventory[1].ID != "i2" {
		t.Fatalf("inventory = %+v", loaded.Inventory)
	}
}

func TestLoadMissingCharacter(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "mud.db"))
	if _, err := store.Load(context.Background(), "없는사람"); err != application.ErrCharacterNotFound {
		t.Fatalf("err = %v, want ErrCharacterNotFound", err)
	}
}

func TestMovePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mud.db")
	ctx := context.Background()

	store := openTestStore(t, path)
	c := domain.NewCharacter("철수", "plaza")
	if err := store.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Simulate a move and a save, then a brand new process opening the db.
	c.Room = "alley"
	if err := store.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Close()

	reopened := openTestStore(t, path)
	loaded, err := reopened.Load(ctx, "철수")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Room != "alley" {
		t.Fatalf("room = %s, want alley", loaded.Room)
	}
}

func TestHPPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mud.db")
	ctx := context.Background()

	store := openTestStore(t, path)
	c := domain.NewCharacter("영희", "plaza")
	c.HP = 13
	if err := store.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Close()

	reopened := openTestStore(t, path)
	loaded, err := reopened.Load(ctx, "영희")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.HP != 13 {
		t.Fatalf("hp = %d, want 13", loaded.HP)
	}
}

func TestLegacyRowWithoutHPHealsToFull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mud.db")
	ctx := context.Background()
	store := openTestStore(t, path)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO characters (name, room_id, inventory, hp) VALUES ('옛날', 'plaza', '[]', 0)`); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, "옛날")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.HP != domain.DefaultMaxHP {
		t.Fatalf("legacy hp = %d, want full", loaded.HP)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mud.db")
	openTestStore(t, path).Close()
	store := openTestStore(t, path) // reopening must not re-apply migrations
	if _, err := store.Load(context.Background(), "x"); err != application.ErrCharacterNotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}
