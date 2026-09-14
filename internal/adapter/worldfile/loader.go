// Package worldfile loads the world definition (JSON) into domain types.
// File format details stay in this adapter; validation rules live in the
// domain.
package worldfile

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/yoophi/text-mud-golang/internal/domain"
)

// Definition is the JSON schema of a world file.
type Definition struct {
	StartRoom   string `json:"startRoom"`
	RespawnRoom string `json:"respawnRoom"`
	Rooms       []struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Exits       map[string]string `json:"exits"`
	} `json:"rooms"`
	Items []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Aliases     []string `json:"aliases"`
		Description string   `json:"description"`
	} `json:"items"`
	ItemSpawns []struct {
		Room  string `json:"room"`
		Item  string `json:"item"`
		Count int    `json:"count"`
	} `json:"itemSpawns"`
	NPCs []struct {
		ID             string   `json:"id"`
		Name           string   `json:"name"`
		Aliases        []string `json:"aliases"`
		Room           string   `json:"room"`
		HP             int      `json:"hp"`
		DamageMin      int      `json:"damageMin"`
		DamageMax      int      `json:"damageMax"`
		Aggressive     bool     `json:"aggressive"`
		RespawnSeconds float64  `json:"respawnSeconds"`
		WanderSeconds  float64  `json:"wanderSeconds"`
	} `json:"npcs"`
}

// Load reads and validates a world file from disk.
func Load(path string) (*domain.World, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("월드 파일을 읽을 수 없습니다: %w", err)
	}
	return Parse(data)
}

// Parse validates world definition bytes and returns a fully populated
// domain world.
func Parse(data []byte) (*domain.World, error) {
	var def Definition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("월드 정의를 해석할 수 없습니다: %w", err)
	}

	rooms := make([]*domain.Room, 0, len(def.Rooms))
	for _, r := range def.Rooms {
		exits := make(map[domain.Direction]domain.RoomID, len(r.Exits))
		for d, to := range r.Exits {
			exits[domain.Direction(d)] = domain.RoomID(to)
		}
		rooms = append(rooms, &domain.Room{
			ID:          domain.RoomID(r.ID),
			Name:        r.Name,
			Description: r.Description,
			Exits:       exits,
		})
	}
	world, err := domain.NewWorld(rooms, domain.RoomID(def.StartRoom), domain.RoomID(def.RespawnRoom))
	if err != nil {
		return nil, err
	}

	protos := make([]*domain.ItemProto, 0, len(def.Items))
	for _, it := range def.Items {
		protos = append(protos, &domain.ItemProto{
			ID:          domain.ItemProtoID(it.ID),
			Name:        it.Name,
			Aliases:     it.Aliases,
			Description: it.Description,
		})
	}
	spawns := make([]domain.ItemSpawn, 0, len(def.ItemSpawns))
	for _, sp := range def.ItemSpawns {
		spawns = append(spawns, domain.ItemSpawn{
			Room:  domain.RoomID(sp.Room),
			Item:  domain.ItemProtoID(sp.Item),
			Count: sp.Count,
		})
	}
	if err := world.SetItems(protos, spawns); err != nil {
		return nil, err
	}

	npcDefs := make([]*domain.NPCDef, 0, len(def.NPCs))
	for _, n := range def.NPCs {
		def := &domain.NPCDef{
			ID:           domain.NPCDefID(n.ID),
			Name:         n.Name,
			Aliases:      n.Aliases,
			Room:         domain.RoomID(n.Room),
			HP:           n.HP,
			DamageMin:    n.DamageMin,
			DamageMax:    n.DamageMax,
			Aggressive:   n.Aggressive,
			RespawnDelay: respawnDuration(n.RespawnSeconds),
		}
		if n.WanderSeconds > 0 {
			def.WanderInterval = respawnDuration(n.WanderSeconds)
		}
		npcDefs = append(npcDefs, def)
	}
	if err := world.SetNPCs(npcDefs); err != nil {
		return nil, err
	}
	return world, nil
}

// respawnDuration converts fractional seconds into a Duration so tests
// and dense worlds can use sub-second respawn timers.
func respawnDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
