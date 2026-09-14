package domain

import "time"

// NPCDef is an NPC prototype loaded from the world definition.
type NPCDef struct {
	ID           NPCDefID
	Name         string
	Aliases      []string
	Room         RoomID // spawn room
	HP           int
	DamageMin    int
	DamageMax    int
	Aggressive   bool
	RespawnDelay time.Duration
}

// Matches reports whether the given name refers to this NPC definition.
func (d *NPCDef) Matches(name string) bool {
	if name == "" {
		return false
	}
	if name == d.Name {
		return true
	}
	for _, alias := range d.Aliases {
		if name == alias {
			return true
		}
	}
	return false
}

// NPCInstance is a live NPC in a room. InstID is unique per instance so
// respawns create a distinct identity.
type NPCInstance struct {
	InstID string
	Def    *NPCDef
	Room   RoomID
	HP     int
	// engaged is the session currently fighting this NPC, or "".
	engaged SessionID
}

// Alive reports whether the instance can act or be attacked.
func (n *NPCInstance) Alive() bool { return n.HP > 0 }
