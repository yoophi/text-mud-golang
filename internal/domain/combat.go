package domain

import (
	"fmt"
	"sort"
	"time"
)

// Combat tuning. Damage ranges are inclusive: IntN(max-min+1) + min.
const (
	attackInterval = 2 * time.Second
	regenInterval  = 20 * time.Second
	regenAmount    = DefaultMaxHP / 10

	playerDamageMin = 2
	playerDamageMax = 6
)

var attackAliases = []string{"공격", "치기", "치다", "attack", "kill", "hit"}

// attack starts combat against an NPC in the current room.
func (g *Game) attack(session SessionID, c *Character, target string, index int) []Effect {
	if target == "" {
		return []Effect{Output{Session: session, Text: "누구를 공격하시겠습니까? 예: 공격 쥐"}}
	}
	if c.InCombat() {
		return []Effect{Output{Session: session, Text: "이미 전투 중입니다."}}
	}
	matches := g.matchNPCs(c.Room, target)
	if len(matches) == 0 {
		return []Effect{Output{Session: session, Text: "그런 대상이 이곳에 없습니다."}}
	}
	if index == 0 && len(matches) > 1 {
		return []Effect{Output{Session: session, Text: ambiguousTarget("공격", target, g.npcNamesFor(matches))}}
	}
	if index == 0 {
		index = 1
	}
	if index < 1 || index > len(matches) {
		return []Effect{Output{Session: session, Text: fmt.Sprintf("번호는 1~%d 사이로 입력하세요.", len(matches))}}
	}
	inst := matches[index-1]
	if inst.engaged != "" && inst.engaged != session {
		return []Effect{Output{Session: session, Text: "다른 모험가가 이미 그 대상과 싸우고 있습니다."}}
	}

	c.combatNPCInst = inst.InstID
	inst.engaged = session
	effects := g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) %s과(와) 싸우기 시작했다!", c.Name, inst.Def.Name))
	effects = append(effects, g.playerStrike(session, c, inst)...)
	// The NPC retaliates on its own schedule, even if the first strike
	// kills it (the round re-validates and no-ops in that case).
	g.events.push(g.clock.Now().Add(attackInterval), evNPCRound, session, inst.InstID)
	return effects
}

// playerStrike resolves one player attack round and schedules the next.
func (g *Game) playerStrike(session SessionID, c *Character, inst *NPCInstance) []Effect {
	dmg := playerDamageMin + g.random.IntN(playerDamageMax-playerDamageMin+1)
	inst.HP -= dmg

	var effects []Effect
	effects = append(effects, Output{Session: session, Text: fmt.Sprintf(
		"당신은 %s을(를) 공격했습니다! (피해 %d, %s 체력 %d/%d)",
		inst.Def.Name, dmg, inst.Def.Name, inst.HP, inst.Def.HP)})
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf(
		"%s이(가) %s을(를) 공격했다!", c.Name, inst.Def.Name))...)

	if !inst.Alive() {
		return append(effects, g.npcDefeated(session, c, inst)...)
	}
	g.events.push(g.clock.Now().Add(attackInterval), evPlayerRound, session, inst.InstID)
	return effects
}

// npcStrike resolves one NPC attack round and schedules the next.
func (g *Game) npcStrike(session SessionID, c *Character, inst *NPCInstance) []Effect {
	if !inst.Alive() || !g.stillEngaged(session, c, inst) {
		return nil
	}
	maxDmg := inst.Def.DamageMax
	if maxDmg < inst.Def.DamageMin {
		maxDmg = inst.Def.DamageMin
	}
	dmg := inst.Def.DamageMin + g.random.IntN(maxDmg-inst.Def.DamageMin+1)
	c.HP -= dmg

	var effects []Effect
	effects = append(effects, Output{Session: session, Text: fmt.Sprintf(
		"%s이(가) 당신을 공격했습니다! (피해 %d, 체력 %d/%d)",
		inst.Def.Name, dmg, c.HP, c.MaxHP)})
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf(
		"%s이(가) %s과(와) 싸우고 있다!", inst.Def.Name, c.Name))...)

	if c.HP <= 0 {
		return append(effects, g.playerDefeated(session, c, inst)...)
	}
	g.events.push(g.clock.Now().Add(attackInterval), evNPCRound, session, inst.InstID)
	return effects
}

// stillEngaged re-validates combat preconditions every round: both sides
// alive, in the same room, and still bound to each other.
func (g *Game) stillEngaged(session SessionID, c *Character, inst *NPCInstance) bool {
	if c == nil || !c.InCombat() || c.combatNPCInst != inst.InstID {
		return false
	}
	if !inst.Alive() || inst.engaged != session {
		return false
	}
	current, ok := g.npcByID(inst.InstID)
	return ok && current == inst && current.Room == c.Room
}

// npcDefeated handles an NPC death: messages, disengage, scheduled respawn.
func (g *Game) npcDefeated(session SessionID, c *Character, inst *NPCInstance) []Effect {
	effects := []Effect{Output{Session: session, Text: fmt.Sprintf("당신은 %s을(를) 쓰러뜨렸습니다!", inst.Def.Name)}}
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) %s을(를) 쓰러뜨렸다!", c.Name, inst.Def.Name))...)
	effects = append(effects, g.breakCombat(session, c)...)
	effects = append(effects, g.despawnNPC(inst)...)
	effects = append(effects, g.save(session))
	return effects
}

// playerDefeated handles a player death: revive at the respawn room with
// full HP, inventory kept.
func (g *Game) playerDefeated(session SessionID, c *Character, inst *NPCInstance) []Effect {
	effects := []Effect{Output{Session: session, Text: fmt.Sprintf("%s에게 당신은 쓰러졌습니다...", inst.Def.Name)}}
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) 쓰러졌다!", c.Name))...)
	effects = append(effects, g.breakCombat(session, c)...)

	c.HP = c.MaxHP
	c.Room = g.world.Respawn()
	effects = append(effects, Output{Session: session, Text: "어둠 속에서 깨어나 보니 부활 지점에 있습니다.\n" + g.renderRoom(session, c.Room)})
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) 정신을 차리고 일어났다.", c.Name))...)
	effects = append(effects, g.save(session))
	return effects
}

// breakCombat cleanly disengages both sides and cancels pending rounds.
func (g *Game) breakCombat(session SessionID, c *Character) []Effect {
	if c == nil || !c.InCombat() {
		return nil
	}
	if inst, ok := g.npcByID(c.combatNPCInst); ok {
		if inst.engaged == session {
			inst.engaged = ""
		}
	}
	c.combatNPCInst = ""
	g.events.dropFor(session, "")
	return nil
}

// checkAggro lets aggressive NPCs engage players entering their room.
func (g *Game) checkAggro(session SessionID, c *Character) []Effect {
	var effects []Effect
	for _, inst := range g.aggressiveFreeNPCs(c.Room) {
		if c.InCombat() || !g.engageNPCTowards(session, c, inst) {
			break
		}
		effects = append(effects, Output{Session: session, Text: fmt.Sprintf("%s이(가) 당신을 노려봅니다!", inst.Def.Name)})
		effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) %s을(를) 노려본다!", inst.Def.Name, c.Name))...)
		g.events.push(g.clock.Now().Add(attackInterval), evNPCRound, session, inst.InstID)
	}
	return effects
}

// aggressiveFreeNPCs returns unengaged aggressive NPCs in a room.
func (g *Game) aggressiveFreeNPCs(room RoomID) []*NPCInstance {
	state, ok := g.rooms[room]
	if !ok {
		return nil
	}
	var out []*NPCInstance
	for _, inst := range state.npcs {
		if inst.Alive() && inst.Def.Aggressive && inst.engaged == "" {
			out = append(out, inst)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstID < out[j].InstID })
	return out
}

// engageNPCTowards binds an NPC to a player unless the player is busy.
func (g *Game) engageNPCTowards(session SessionID, c *Character, inst *NPCInstance) bool {
	if c.InCombat() {
		return false
	}
	c.combatNPCInst = inst.InstID
	inst.engaged = session
	return true
}

// checkAggroAll engages aggressive NPCs against players already in the
// room when the NPC arrives (wander/respawn).
func (g *Game) checkAggroAll(inst *NPCInstance) []Effect {
	var effects []Effect
	if !inst.Alive() || !inst.Def.Aggressive || inst.engaged != "" {
		return nil
	}
	victims := make([]SessionID, 0, len(g.sessions))
	for s, c := range g.sessions {
		if c.Room == inst.Room && c.HP > 0 {
			victims = append(victims, s)
		}
	}
	sort.Slice(victims, func(i, j int) bool { return victims[i] < victims[j] })
	for _, s := range victims {
		c := g.sessions[s]
		if !g.engageNPCTowards(s, c, inst) {
			continue
		}
		effects = append(effects, Output{Session: s, Text: fmt.Sprintf("%s이(가) 당신을 노려봅니다!", inst.Def.Name)})
		effects = append(effects, g.broadcastRoom(inst.Room, s, fmt.Sprintf("%s이(가) %s을(를) 노려본다!", inst.Def.Name, c.Name))...)
		g.events.push(g.clock.Now().Add(attackInterval), evNPCRound, s, inst.InstID)
		break // one victim per NPC
	}
	return effects
}

// scheduleRegen queues periodic out-of-combat healing for a session.
func (g *Game) scheduleRegen(session SessionID) {
	g.events.push(g.clock.Now().Add(regenInterval), evRegen, session, "")
}

// regen heals a resting character by a fixed chunk.
func (g *Game) regen(session SessionID) []Effect {
	c, ok := g.sessions[session]
	if !ok {
		return nil
	}
	var effects []Effect
	if c.HP > 0 && !c.InCombat() && c.HP < c.MaxHP {
		c.HP += regenAmount
		if c.HP > c.MaxHP {
			c.HP = c.MaxHP
		}
		effects = append(effects, Output{Session: session, Text: fmt.Sprintf("몸이 회복되었습니다. (체력 %d/%d)", c.HP, c.MaxHP)})
		effects = append(effects, g.save(session))
	}
	g.scheduleRegen(session)
	return effects
}

func (g *Game) npcNamesFor(insts []*NPCInstance) []string {
	out := make([]string, 0, len(insts))
	for _, inst := range insts {
		out = append(out, inst.Def.Name)
	}
	return out
}
