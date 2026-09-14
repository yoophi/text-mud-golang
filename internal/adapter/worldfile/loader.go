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
	} `json:"npcs"`
}

// Loaded is a fully validated world plus its spawn tables.
type Loaded struct {
	World      *domain.World
	ItemProtos []*domain.ItemProto
	ItemSpawns []ItemSpawn
	NPCDefs    []*domain.NPCDef
}

// ItemSpawn places Count instances of an item prototype in a room at boot.
type ItemSpawn struct {
	Room  domain.RoomID
	Item  domain.ItemProtoID
	Count int
}

// Load reads and validates a world file from disk.
func Load(path string) (*Loaded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("월드 파일을 읽을 수 없습니다: %w", err)
	}
	return Parse(data)
}

// Parse validates world definition bytes.
func Parse(data []byte) (*Loaded, error) {
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

	protos := make(map[domain.ItemProtoID]*domain.ItemProto, len(def.Items))
	loaded := &Loaded{World: world}
	for _, it := range def.Items {
		if it.ID == "" || it.Name == "" {
			return nil, fmt.Errorf("아이템 정의에 id와 name이 필요합니다")
		}
		id := domain.ItemProtoID(it.ID)
		if _, dup := protos[id]; dup {
			return nil, fmt.Errorf("아이템 ID가 중복되었습니다: %s", it.ID)
		}
		protos[id] = &domain.ItemProto{ID: id, Name: it.Name, Aliases: it.Aliases, Description: it.Description}
		loaded.ItemProtos = append(loaded.ItemProtos, protos[id])
	}
	for _, sp := range def.ItemSpawns {
		if _, ok := protos[domain.ItemProtoID(sp.Item)]; !ok {
			return nil, fmt.Errorf("아이템 생성이 정의되지 않은 아이템 %s을(를) 참조합니다", sp.Item)
		}
		if _, ok := world.Room(domain.RoomID(sp.Room)); !ok {
			return nil, fmt.Errorf("아이템 생성이 존재하지 않는 방 %s을(를) 참조합니다", sp.Room)
		}
		if sp.Count < 1 {
			return nil, fmt.Errorf("아이템 %s의 생성 수량은 1 이상이어야 합니다", sp.Item)
		}
		loaded.ItemSpawns = append(loaded.ItemSpawns, ItemSpawn{
			Room: domain.RoomID(sp.Room), Item: domain.ItemProtoID(sp.Item), Count: sp.Count,
		})
	}

	npcIDs := map[domain.NPCDefID]bool{}
	for _, n := range def.NPCs {
		if n.ID == "" || n.Name == "" {
			return nil, fmt.Errorf("NPC 정의에 id와 name이 필요합니다")
		}
		if npcIDs[domain.NPCDefID(n.ID)] {
			return nil, fmt.Errorf("NPC ID가 중복되었습니다: %s", n.ID)
		}
		if _, ok := world.Room(domain.RoomID(n.Room)); !ok {
			return nil, fmt.Errorf("NPC %s이(가) 존재하지 않는 방 %s을(를) 참조합니다", n.ID, n.Room)
		}
		if n.HP < 1 {
			return nil, fmt.Errorf("NPC %s의 hp는 1 이상이어야 합니다", n.ID)
		}
		if n.DamageMin < 0 || n.DamageMax < n.DamageMin {
			return nil, fmt.Errorf("NPC %s의 피해 범위가 잘못되었습니다", n.ID)
		}
		if n.RespawnSeconds <= 0 {
			return nil, fmt.Errorf("NPC %s의 respawnSeconds는 0보다 커야 합니다", n.ID)
		}
		npcIDs[domain.NPCDefID(n.ID)] = true
		loaded.NPCDefs = append(loaded.NPCDefs, &domain.NPCDef{
			ID:           domain.NPCDefID(n.ID),
			Name:         n.Name,
			Aliases:      n.Aliases,
			Room:         domain.RoomID(n.Room),
			HP:           n.HP,
			DamageMin:    n.DamageMin,
			DamageMax:    n.DamageMax,
			Aggressive:   n.Aggressive,
			RespawnDelay: respawnDuration(n.RespawnSeconds),
		})
	}
	return loaded, nil
}

// respawnDuration converts fractional seconds into a Duration so tests
// and dense worlds can use sub-second respawn timers.
func respawnDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
