package domain

// ItemProto is an immutable item prototype loaded from the world
// definition. Instances reference a prototype by ID.
type ItemProto struct {
	ID          ItemProtoID
	Name        string
	Aliases     []string
	Description string
}

// Matches reports whether the given (particle-stripped) name refers to
// this prototype.
func (p *ItemProto) Matches(name string) bool {
	if name == "" {
		return false
	}
	if name == p.Name {
		return true
	}
	for _, alias := range p.Aliases {
		if name == alias {
			return true
		}
	}
	return false
}

// ItemInstance is a concrete item located in a room or an inventory.
type ItemInstance struct {
	ID     string
	Proto  ItemProtoID
	holder string // "room:<id>" or "char:<name>"; empty means unplaced
}

// PlacedAt reports where the instance currently resides.
func (i ItemInstance) PlacedAt() (room RoomID, character string, ok bool) {
	if len(i.holder) > 5 && i.holder[:5] == "room:" {
		return RoomID(i.holder[5:]), "", true
	}
	if len(i.holder) > 5 && i.holder[:5] == "char:" {
		return "", i.holder[5:], true
	}
	return "", "", false
}

// place is used by the engine to track exactly one location per instance.
func (i *ItemInstance) placeInRoom(room RoomID)      { i.holder = "room:" + string(room) }
func (i *ItemInstance) placeInInventory(name string) { i.holder = "char:" + name }
func (i *ItemInstance) remove()                      { i.holder = "" }
