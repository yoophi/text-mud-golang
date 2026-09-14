package domain

import (
	"fmt"
	"sort"
	"strings"
)

// spawnNPCs creates live instances for every NPC definition in its
// defined room and schedules wandering behavior.
func (g *Game) spawnNPCs() {
	for _, def := range g.world.NPCDefs() {
		g.addNPCInstance(def, def.Room)
	}
}

// addNPCInstance places a fresh NPC instance in a room, scheduling its
// wander timer when configured.
func (g *Game) addNPCInstance(def *NPCDef, room RoomID) *NPCInstance {
	g.nextInst++
	inst := &NPCInstance{
		InstID: fmt.Sprintf("npc-%d", g.nextInst),
		Def:    def,
		Room:   room,
		HP:     def.HP,
	}
	g.rooms[room].npcs[inst.InstID] = inst
	if def.WanderInterval > 0 {
		g.events.push(g.clock.Now().Add(def.WanderInterval), evWander, "", inst.InstID)
	}
	return inst
}

// npcInRoom finds an NPC instance by ID in any room.
func (g *Game) npcByID(instID string) (*NPCInstance, bool) {
	for _, state := range g.rooms {
		if inst, ok := state.npcs[instID]; ok {
			return inst, true
		}
	}
	return nil, false
}

// npcByName returns the live instance of the named NPC, if any.
func (g *Game) npcByName(name string) (*NPCInstance, bool) {
	for _, state := range g.rooms {
		for _, inst := range state.npcs {
			if inst.Def.Name == name {
				return inst, true
			}
		}
	}
	return nil, false
}

// npcNames lists the NPCs present in a room, sorted for stable output.
func (g *Game) npcNames(room RoomID) []string {
	state, ok := g.rooms[room]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(state.npcs))
	for _, inst := range state.npcs {
		names = append(names, inst.Def.Name)
	}
	sort.Strings(names)
	return names
}

// matchNPCs returns live NPC instances in the room matching target.
func (g *Game) matchNPCs(room RoomID, target string) []*NPCInstance {
	state, ok := g.rooms[room]
	if !ok {
		return nil
	}
	var out []*NPCInstance
	for _, inst := range state.npcs {
		for _, cand := range TargetCandidates(target) {
			if inst.Def.Matches(cand) {
				out = append(out, inst)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstID < out[j].InstID })
	return out
}

// npcWander moves a wandering NPC through a random exit. It is driven by
// a scheduled event and re-schedules itself while the NPC lives.
func (g *Game) npcWander(instID string) []Effect {
	inst, ok := g.npcByID(instID)
	if !ok || !inst.Alive() {
		return nil
	}
	room, ok := g.world.Room(inst.Room)
	if !ok {
		return nil
	}
	exits := room.ExitNames()
	if len(exits) == 0 {
		g.events.push(g.clock.Now().Add(inst.Def.WanderInterval), evWander, "", instID)
		return nil
	}
	pick := exits[g.random.IntN(len(exits))]
	to := room.Exits[Direction(pick)]

	var effects []Effect
	effects = append(effects, g.broadcastRoom(inst.Room, "", fmt.Sprintf("%s이(가) %s(으)로 사라졌다.", inst.Def.Name, pick))...)
	delete(g.rooms[inst.Room].npcs, instID)
	inst.Room = to
	g.rooms[to].npcs[instID] = inst
	effects = append(effects, g.broadcastRoom(to, "", fmt.Sprintf("%s이(가) 모습을 드러냈다.", inst.Def.Name))...)

	g.events.push(g.clock.Now().Add(inst.Def.WanderInterval), evWander, "", instID)
	return effects
}

// despawnNPC removes a dead NPC and schedules its respawn at the room
// defined by its NPC definition.
func (g *Game) despawnNPC(inst *NPCInstance) []Effect {
	if state, ok := g.rooms[inst.Room]; ok {
		delete(state.npcs, inst.InstID)
	}
	g.events.dropFor("", inst.InstID)
	g.events.pushRespawn(g.clock.Now().Add(inst.Def.RespawnDelay), inst.Def, inst.Def.Room)
	return g.broadcastRoom(inst.Room, "", fmt.Sprintf("%s의 시체가 사라졌다.", inst.Def.Name))
}

// respawnNPC recreates a despawned NPC in its defined room exactly once.
func (g *Game) respawnNPC(def *NPCDef, room RoomID) []Effect {
	if def == nil {
		return nil
	}
	// Never stack duplicates: if any live instance of this definition
	// exists in the room already, skip this respawn.
	if state, ok := g.rooms[room]; ok {
		for _, inst := range state.npcs {
			if inst.Def.ID == def.ID {
				return nil
			}
		}
	}
	inst := g.addNPCInstance(def, room)
	return g.broadcastRoom(room, "", fmt.Sprintf("%s이(가) 모습을 드러냈다.", inst.Def.Name))
}

// npcRoomStateText renders the NPC section of a room description.
func (g *Game) npcRoomStateText(room RoomID) string {
	names := g.npcNames(room)
	if len(names) == 0 {
		return ""
	}
	return "\n이곳에 있는 존재: " + strings.Join(names, ", ")
}
