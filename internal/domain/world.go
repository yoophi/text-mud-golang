package domain

import (
	"fmt"
	"sort"
	"strings"
)

// Room is a single location in the world.
type Room struct {
	ID          RoomID
	Name        string
	Description string
	Exits       map[Direction]RoomID
}

// ExitNames returns the room's exit directions in display order.
func (r *Room) ExitNames() []string {
	names := make([]string, 0, len(r.Exits))
	for _, d := range Directions() {
		if _, ok := r.Exits[d]; ok {
			names = append(names, string(d))
		}
	}
	return names
}

// World is the validated, immutable room graph plus spawn metadata.
type World struct {
	rooms   map[RoomID]*Room
	start   RoomID
	respawn RoomID
}

// NewWorld validates a set of rooms and returns a World. Duplicate room
// IDs, unknown exit directions, exits pointing at missing rooms, and a
// missing start or respawn room are rejected with explicit errors.
func NewWorld(rooms []*Room, start, respawn RoomID) (*World, error) {
	if len(rooms) == 0 {
		return nil, fmt.Errorf("세계에 방이 하나도 없습니다")
	}
	w := &World{rooms: make(map[RoomID]*Room, len(rooms))}
	for _, r := range rooms {
		if r.ID == "" {
			return nil, fmt.Errorf("빈 방 ID가 있습니다")
		}
		if _, dup := w.rooms[r.ID]; dup {
			return nil, fmt.Errorf("방 ID가 중복되었습니다: %s", r.ID)
		}
		w.rooms[r.ID] = r
	}
	for _, r := range rooms {
		for d, to := range r.Exits {
			if !ValidDirection(d) {
				return nil, fmt.Errorf("방 %s: 알 수 없는 출구 방향 %q", r.ID, d)
			}
			if _, ok := w.rooms[to]; !ok {
				return nil, fmt.Errorf("방 %s: 출구 %s이(가) 존재하지 않는 방 %s을(를) 가리킵니다", r.ID, d, to)
			}
		}
	}
	if _, ok := w.rooms[start]; !ok {
		return nil, fmt.Errorf("시작 방 %s이(가) 정의에 없습니다", start)
	}
	if respawn == "" {
		respawn = start
	}
	if _, ok := w.rooms[respawn]; !ok {
		return nil, fmt.Errorf("부활 방 %s이(가) 정의에 없습니다", respawn)
	}
	w.start = start
	w.respawn = respawn
	return w, nil
}

// Start returns the room where new characters begin.
func (w *World) Start() RoomID { return w.start }

// Respawn returns the room where dead characters revive.
func (w *World) Respawn() RoomID { return w.respawn }

// Room looks up a room by ID.
func (w *World) Room(id RoomID) (*Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

// Rooms returns all rooms sorted by ID for deterministic iteration.
func (w *World) Rooms() []*Room {
	out := make([]*Room, 0, len(w.rooms))
	for _, r := range w.rooms {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Describe renders the look output for a room.
func (w *World) Describe(id RoomID) string {
	r, ok := w.rooms[id]
	if !ok {
		return "알 수 없는 장소입니다."
	}
	var b strings.Builder
	b.WriteString("[" + r.Name + "]\n")
	b.WriteString(r.Description + "\n")
	exits := r.ExitNames()
	if len(exits) == 0 {
		b.WriteString("출구: 없음")
	} else {
		b.WriteString("출구: " + strings.Join(exits, ", "))
	}
	return b.String()
}
