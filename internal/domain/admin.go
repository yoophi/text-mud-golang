package domain

import (
	"fmt"
	"strings"
)

// adminAliases are operator command prefixes.
var adminVerbs = []string{"운영", "운영자"}

// admin handles operator commands. Permission is checked first.
func (g *Game) admin(session SessionID, c *Character, cmd Command) []Effect {
	if !c.Operator {
		return []Effect{Output{Session: session, Text: "운영자 권한이 필요합니다."}}
	}
	switch cmd.Admin {
	case AdminGoto:
		return g.adminGoto(session, c, cmd.AdminArg)
	case AdminSpawn:
		return g.adminSpawn(session, c, cmd.AdminArg)
	case AdminShutdown:
		return []Effect{
			Output{Session: session, Text: "서버를 종료합니다..."},
			Shutdown{Reason: "운영자 종료 명령"},
		}
	default:
		return []Effect{Output{Session: session, Text: "알 수 없는 운영 명령입니다. 사용 가능: @goto <방>, @spawn <NPC> [방], @shutdown"}}
	}
}

// adminGoto teleports the operator to any room.
func (g *Game) adminGoto(session SessionID, c *Character, arg string) []Effect {
	roomID := RoomID(strings.TrimSpace(arg))
	if roomID == "" {
		return []Effect{Output{Session: session, Text: "사용법: @goto <방 ID>"}}
	}
	if _, ok := g.world.Room(roomID); !ok {
		return []Effect{Output{Session: session, Text: "그런 방이 없습니다."}}
	}
	effects := g.breakCombat(session, c)
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) 자취를 감췄다.", c.Name))...)
	c.Room = roomID
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) 모습을 드러냈다.", c.Name))...)
	effects = append(effects, Output{Session: session, Text: g.renderRoom(session, c.Room)})
	effects = append(effects, g.checkAggro(session, c)...)
	effects = append(effects, g.save(session))
	return effects
}

// adminSpawn creates an NPC instance by definition ID or name, optionally
// in a specific room.
func (g *Game) adminSpawn(session SessionID, c *Character, arg string) []Effect {
	fields := strings.Fields(strings.TrimSpace(arg))
	if len(fields) == 0 {
		return []Effect{Output{Session: session, Text: "사용법: @spawn <NPC 정의 ID 또는 이름> [방 ID]"}}
	}
	def := g.findNPCDef(fields[0])
	if def == nil {
		return []Effect{Output{Session: session, Text: "그런 NPC 정의가 없습니다."}}
	}
	room := c.Room
	if len(fields) > 1 {
		room = RoomID(fields[1])
		if _, ok := g.world.Room(room); !ok {
			return []Effect{Output{Session: session, Text: "그런 방이 없습니다."}}
		}
	}
	inst := g.addNPCInstance(def, room)
	effects := []Effect{Output{Session: session, Text: fmt.Sprintf("%s을(를) %s에 생성했습니다.", def.Name, room)}}
	effects = append(effects, g.broadcastRoom(room, session, fmt.Sprintf("%s이(가) 모습을 드러냈다.", def.Name))...)
	effects = append(effects, g.checkAggroAll(inst)...)
	return effects
}

// findNPCDef resolves an NPC definition by ID or display name.
func (g *Game) findNPCDef(key string) *NPCDef {
	for _, def := range g.world.NPCDefs() {
		if string(def.ID) == key || def.Name == key {
			return def
		}
	}
	return nil
}
