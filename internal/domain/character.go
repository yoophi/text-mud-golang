package domain

// DefaultMaxHP is the starting and maximum HP of a player character.
const DefaultMaxHP = 50

// Character is a player avatar present in the world while connected.
// Persistence adapters store the exported fields; combat bookkeeping is
// runtime-only.
type Character struct {
	Name      string
	Room      RoomID
	HP        int
	MaxHP     int
	Operator  bool
	Inventory []ItemInstance

	// combatNPCInst is the NPC instance the character is fighting, or "".
	combatNPCInst string
}

// NewCharacter creates a fresh character at room with full HP.
func NewCharacter(name string, room RoomID) *Character {
	return &Character{
		Name:  name,
		Room:  room,
		HP:    DefaultMaxHP,
		MaxHP: DefaultMaxHP,
	}
}

// Copy returns a deep copy safe to hand to persistence adapters.
func (c *Character) Copy() *Character {
	if c == nil {
		return nil
	}
	dup := *c
	if c.Inventory != nil {
		dup.Inventory = append([]ItemInstance(nil), c.Inventory...)
	}
	return &dup
}

// InCombat reports whether the character is currently engaged in combat.
func (c *Character) InCombat() bool { return c.combatNPCInst != "" }
