package worldfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoophi/text-mud-golang/internal/domain"
)

const validWorld = `{
  "startRoom": "plaza",
  "respawnRoom": "plaza",
  "rooms": [
    {"id": "plaza", "name": "마을 광장", "description": "오래된 분수가 있는 광장이다.", "exits": {"북쪽": "alley", "동쪽": "market"}},
    {"id": "alley", "name": "뒷골목", "description": "좁고 어두운 골목이다.", "exits": {"남쪽": "plaza"}},
    {"id": "market", "name": "시장", "description": "활기찬 시장이다.", "exits": {"서쪽": "plaza"}}
  ]
}`

func mustParse(t *testing.T, data string) *domain.World {
	t.Helper()
	world, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return world
}

func TestParseLoadsConnectedRooms(t *testing.T) {
	world := mustParse(t, validWorld)

	if world.Start() != "plaza" {
		t.Fatalf("start = %s, want plaza", world.Start())
	}
	plaza, ok := world.Room("plaza")
	if !ok {
		t.Fatal("plaza room missing")
	}
	if to, ok := plaza.Exits["북쪽"]; !ok || to != "alley" {
		t.Fatalf("plaza north exit = %s (%v), want alley", to, ok)
	}
	alley, _ := world.Room("alley")
	if to, ok := alley.Exits["남쪽"]; !ok || to != "plaza" {
		t.Fatalf("alley south exit should point back at plaza")
	}
	if desc := world.Describe("market"); desc == "" {
		t.Fatal("Describe should render the market")
	}
}

func TestParseRejectsInvalidExitTarget(t *testing.T) {
	bad := `{"startRoom": "a", "rooms": [
		{"id": "a", "name": "A", "exits": {"북쪽": "ghost"}}
	]}`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("invalid exit target must be rejected")
	}
}

func TestParseRejectsDuplicateRoomID(t *testing.T) {
	bad := `{"startRoom": "a", "rooms": [
		{"id": "a", "name": "A"},
		{"id": "a", "name": "A"}
	]}`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("duplicate room ID must be rejected")
	}
}

func TestParseRejectsMissingStartRoom(t *testing.T) {
	bad := `{"startRoom": "nope", "rooms": [{"id": "a", "name": "A"}]}`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("missing start room must be rejected")
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"startRoom": `)); err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
}

func TestParseValidatesItemsAndNPCs(t *testing.T) {
	base := func(items, spawns, npcs string) string {
		return `{"startRoom": "a", "rooms": [{"id": "a", "name": "A"}],` +
			`"items": [` + items + `], "itemSpawns": [` + spawns + `], "npcs": [` + npcs + `]}`
	}

	if _, err := Parse([]byte(base(
		`{"id": "coin", "name": "동전"}`, `{"room": "a", "item": "coin", "count": 2}`, ``))); err != nil {
		t.Fatalf("valid items/npcs should parse: %v", err)
	}
	if _, err := Parse([]byte(base(
		`{"id": "coin", "name": "동전"}`, `{"room": "a", "item": "ghost", "count": 1}`, ``))); err == nil {
		t.Fatal("spawn of unknown item must be rejected")
	}
	if _, err := Parse([]byte(base(
		`{"id": "coin", "name": "동전"}`, `{"room": "void", "item": "coin", "count": 1}`, ``))); err == nil {
		t.Fatal("spawn in unknown room must be rejected")
	}
	if _, err := Parse([]byte(base(
		``, ``, `{"id": "rat", "name": "쥐", "room": "a", "hp": 5, "damageMin": 1, "damageMax": 2, "respawnSeconds": 10}`))); err != nil {
		t.Fatalf("valid npc should parse: %v", err)
	}
	if _, err := Parse([]byte(base(
		``, ``, `{"id": "rat", "name": "쥐", "room": "void", "hp": 5, "respawnSeconds": 10}`))); err == nil {
		t.Fatal("npc in unknown room must be rejected")
	}
	if _, err := Parse([]byte(base(
		``, ``, `{"id": "rat", "name": "쥐", "room": "a", "hp": 0, "respawnSeconds": 10}`))); err == nil {
		t.Fatal("npc with zero hp must be rejected")
	}

	world := mustParse(t, base(``, ``, `{"id": "rat", "name": "쥐", "room": "a", "hp": 5, "damageMin": 1, "damageMax": 3, "aggressive": true, "respawnSeconds": 2.5}`))
	def := world.NPCDefs()[0]
	if !def.Aggressive || def.RespawnDelay != 2500*time.Millisecond {
		t.Fatalf("npc def not loaded correctly: %+v", def)
	}
}

func TestLoadReadsFileFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "world.json")
	if err := os.WriteFile(path, []byte(validWorld), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file must fail")
	}
}
