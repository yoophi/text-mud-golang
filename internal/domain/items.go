package domain

import (
	"fmt"
	"strings"
)

// itemAliases map accepted verbs for item handling.
var (
	getAliases  = []string{"줍기", "획득", "줍자", "get", "take", "pick"}
	dropAliases = []string{"버리기", "버려", "버리자", "drop"}
	invAliases  = []string{"인벤토리", "소지품", "i", "inv", "inventory"}
)

// spawnFloorItems places the world's boot-time items on room floors.
func (g *Game) spawnFloorItems() {
	for _, sp := range g.world.ItemSpawns() {
		state, ok := g.rooms[sp.Room]
		if !ok {
			continue
		}
		for i := 0; i < sp.Count; i++ {
			state.items = append(state.items, g.newItemInstance(sp.Item))
		}
	}
}

func (g *Game) newItemInstance(proto ItemProtoID) *ItemInstance {
	g.nextInst++
	return &ItemInstance{ID: fmt.Sprintf("item-%s-%d", g.bootID, g.nextInst), Proto: proto}
}

// matchFloor returns floor items in the room whose prototype matches the
// target (particle-stripped), in stable order.
func (g *Game) matchFloor(room RoomID, target string) []*ItemInstance {
	state, ok := g.rooms[room]
	if !ok {
		return nil
	}
	var out []*ItemInstance
	for _, it := range state.items {
		if g.itemMatches(it, target) {
			out = append(out, it)
		}
	}
	return out
}

// matchInventory returns inventory items matching the target.
func (g *Game) matchInventory(c *Character, target string) []*ItemInstance {
	var out []*ItemInstance
	for _, it := range c.Inventory {
		if g.itemMatches(&it, target) {
			out = append(out, &it)
		}
	}
	return out
}

func (g *Game) itemMatches(item *ItemInstance, target string) bool {
	proto, ok := g.world.ItemProto(item.Proto)
	if !ok {
		return false
	}
	for _, cand := range TargetCandidates(target) {
		if proto.Matches(cand) {
			return true
		}
	}
	return false
}

// itemName resolves an instance's display name.
func (g *Game) itemName(item ItemInstance) string {
	if proto, ok := g.world.ItemProto(item.Proto); ok {
		return proto.Name
	}
	return string(item.Proto)
}

// pickUp moves a floor item into the character's inventory.
func (g *Game) pickUp(session SessionID, c *Character, target string, index int) []Effect {
	if target == "" {
		return []Effect{Output{Session: session, Text: "무엇을 줍겠습니까? 예: 줍기 빵"}}
	}
	state := g.rooms[c.Room]
	matches := g.matchFloor(c.Room, target)
	if len(matches) == 0 {
		return []Effect{Output{Session: session, Text: "그런 물건이 바닥에 없습니다."}}
	}
	if index == 0 && len(matches) > 1 {
		return []Effect{Output{Session: session, Text: ambiguousTarget("줍기", target, g.itemNames(matches))}}
	}
	if index == 0 {
		index = 1
	}
	if index < 1 || index > len(matches) {
		return []Effect{Output{Session: session, Text: fmt.Sprintf("번호는 1~%d 사이로 입력하세요.", len(matches))}}
	}
	picked := matches[index-1]
	state.items = removeInstancePtrs(state.items, picked.ID)
	c.Inventory = append(c.Inventory, *picked)

	name := g.itemName(*picked)
	effects := []Effect{
		Output{Session: session, Text: fmt.Sprintf("당신은 %s을(를) 주웠습니다.", name)},
	}
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) %s을(를) 주웠다.", c.Name, name))...)
	effects = append(effects, g.save(session))
	return effects
}

// dropItem moves an inventory item onto the room floor.
func (g *Game) dropItem(session SessionID, c *Character, target string, index int) []Effect {
	if target == "" {
		return []Effect{Output{Session: session, Text: "무엇을 버리겠습니까? 예: 버리기 빵"}}
	}
	matches := g.matchInventory(c, target)
	if len(matches) == 0 {
		return []Effect{Output{Session: session, Text: "그런 물건을 가지고 있지 않습니다."}}
	}
	if index == 0 && len(matches) > 1 {
		return []Effect{Output{Session: session, Text: ambiguousTarget("버리기", target, g.itemNames(matches))}}
	}
	if index == 0 {
		index = 1
	}
	if index < 1 || index > len(matches) {
		return []Effect{Output{Session: session, Text: fmt.Sprintf("번호는 1~%d 사이로 입력하세요.", len(matches))}}
	}
	dropped := *matches[index-1]
	c.Inventory = removeInstance(c.Inventory, dropped.ID)
	g.rooms[c.Room].items = append(g.rooms[c.Room].items, &dropped)

	name := g.itemName(dropped)
	effects := []Effect{
		Output{Session: session, Text: fmt.Sprintf("당신은 %s을(를) 바닥에 놓았습니다.", name)},
	}
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) %s을(를) 바닥에 놓았다.", c.Name, name))...)
	effects = append(effects, g.save(session))
	return effects
}

// inventory lists what the character carries.
func (g *Game) inventory(session SessionID, c *Character) []Effect {
	if len(c.Inventory) == 0 {
		return []Effect{Output{Session: session, Text: "가진 물건이 없습니다."}}
	}
	names := make([]string, 0, len(c.Inventory))
	for i := range c.Inventory {
		names = append(names, fmt.Sprintf("%d) %s", i+1, g.itemName(c.Inventory[i])))
	}
	return []Effect{Output{Session: session, Text: "소지품:\n" + strings.Join(names, "\n")}}
}

func (g *Game) itemNames(items []*ItemInstance) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, g.itemName(*it))
	}
	return out
}

// removeInstance returns a copy without the given instance ID.
func removeInstance(items []ItemInstance, id string) []ItemInstance {
	out := items[:0:0]
	for _, it := range items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	return out
}

func removeInstancePtrs(items []*ItemInstance, id string) []*ItemInstance {
	out := items[:0:0]
	for _, it := range items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	return out
}

// ambiguousTarget asks the player to disambiguate same-named targets.
func ambiguousTarget(verb, target string, names []string) string {
	listed := make([]string, 0, len(names))
	for i, name := range names {
		listed = append(listed, fmt.Sprintf("%d) %s", i+1, name))
	}
	return fmt.Sprintf("같은 이름의 대상이 여러 개입니다. 번호로 선택하세요.\n%s\n예: %s %s 2",
		strings.Join(listed, "\n"), verb, target)
}
