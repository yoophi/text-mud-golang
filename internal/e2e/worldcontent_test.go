package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yoophi/text-mud-golang/internal/adapter/worldfile"
	"github.com/yoophi/text-mud-golang/internal/domain"
)

// TestWorldContentMeetsMVP verifies the shipped world satisfies the MVP
// completion criteria: 10-20 connected rooms, obtainable items, and at
// least one hostile NPC.
func TestWorldContentMeetsMVP(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "world.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("world file: %v", err)
	}
	world, err := worldfile.Load(path)
	if err != nil {
		t.Fatalf("world load: %v", err)
	}

	rooms := world.Rooms()
	if len(rooms) < 10 || len(rooms) > 20 {
		t.Fatalf("MVP world should have 10-20 rooms, has %d", len(rooms))
	}

	// Every room must be reachable from the start room.
	visited := map[domain.RoomID]bool{world.Start(): true}
	queue := []domain.RoomID{world.Start()}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		room, _ := world.Room(cur)
		for _, to := range room.Exits {
			if !visited[to] {
				visited[to] = true
				queue = append(queue, to)
			}
		}
	}
	if len(visited) != len(rooms) {
		t.Fatalf("disconnected world: %d of %d rooms reachable", len(visited), len(rooms))
	}

	if len(world.ItemSpawns()) == 0 {
		t.Fatal("world must offer obtainable items")
	}
	aggressive := false
	for _, def := range world.NPCDefs() {
		if def.Aggressive {
			aggressive = true
		}
	}
	if !aggressive {
		t.Fatal("world must contain at least one hostile NPC")
	}
}
