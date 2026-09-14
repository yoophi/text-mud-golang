package domain

// SessionID identifies a connected client inside the game engine.
type SessionID string

// RoomID is a unique room identifier within a world.
type RoomID string

// Direction names an exit direction. Values are Korean display names.
type Direction string

const (
	DirNorth Direction = "북쪽"
	DirSouth Direction = "남쪽"
	DirEast  Direction = "동쪽"
	DirWest  Direction = "서쪽"
	DirUp    Direction = "위"
	DirDown  Direction = "아래"
)

var directions = map[Direction]bool{
	DirNorth: true,
	DirSouth: true,
	DirEast:  true,
	DirWest:  true,
	DirUp:    true,
	DirDown:  true,
}

// ValidDirection reports whether d is a known exit direction.
func ValidDirection(d Direction) bool { return directions[d] }

// Directions returns all exit directions in display order.
func Directions() []Direction {
	return []Direction{DirNorth, DirSouth, DirEast, DirWest, DirUp, DirDown}
}

// ItemProtoID identifies an item prototype defined by the world.
type ItemProtoID string

// NPCDefID identifies an NPC prototype defined by the world.
type NPCDefID string
