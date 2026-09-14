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
	ID    string
	Proto ItemProtoID
}
