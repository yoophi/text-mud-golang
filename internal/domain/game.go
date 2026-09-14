package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Game is the single authoritative game state machine. All state changes
// flow through Connect, Execute, Disconnect, and Tick, and every external
// consequence is reported as an Effect. The Game performs no I/O.
type Game struct {
	world    *World
	clock    Clock
	random   Random
	sessions map[SessionID]*Character
	byName   map[string]SessionID
	rooms    map[RoomID]*roomState
	events   eventQueue
	nextInst uint64
	// bootID makes generated instance IDs unique across process
	// restarts, so persisted inventory items never collide with freshly
	// spawned floor items.
	bootID string
}

// roomState holds the mutable per-room state: NPCs and floor items.
type roomState struct {
	npcs  map[string]*NPCInstance
	items []*ItemInstance
}

func newRoomState() *roomState {
	return &roomState{npcs: map[string]*NPCInstance{}}
}

// NewGame creates an engine for the given world. clock and random must be
// non-nil; tests inject fakes to keep behavior deterministic.
func NewGame(w *World, clock Clock, random Random) *Game {
	g := &Game{
		world:    w,
		clock:    clock,
		random:   random,
		sessions: map[SessionID]*Character{},
		byName:   map[string]SessionID{},
		rooms:    map[RoomID]*roomState{},
	}
	for _, r := range w.Rooms() {
		g.rooms[r.ID] = newRoomState()
	}
	g.bootID = newBootID()
	g.spawnFloorItems()
	g.spawnNPCs()
	return g
}

// newBootID returns a random per-process prefix for instance IDs. It
// falls back to high-resolution time if the system entropy source is
// unavailable.
func newBootID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// World returns the world definition the engine runs on.
func (g *Game) World() *World { return g.world }

// IsOnline reports whether a character with the given name is connected.
func (g *Game) IsOnline(name string) bool {
	_, ok := g.byName[name]
	return ok
}

// Session returns the session currently playing the named character.
func (g *Game) Session(name string) (SessionID, bool) {
	s, ok := g.byName[name]
	return s, ok
}

// Character returns a snapshot of the named connected character.
func (g *Game) Character(name string) (*Character, bool) {
	s, ok := g.byName[name]
	if !ok {
		return nil, false
	}
	return g.sessions[s].Copy(), true
}

// Playing reports whether the session currently controls a character.
func (g *Game) Playing(session SessionID) bool {
	_, ok := g.sessions[session]
	return ok
}

// Sessions returns every active session ID.
func (g *Game) Sessions() []SessionID {
	out := make([]SessionID, 0, len(g.sessions))
	for s := range g.sessions {
		out = append(out, s)
	}
	return out
}

// Connect brings an already-loaded (or newly created) character into the
// world. If the session was already connected, the previous character is
// disconnected first.
func (g *Game) Connect(session SessionID, c *Character) []Effect {
	var effects []Effect
	if _, exists := g.sessions[session]; exists {
		effects = append(effects, g.Disconnect(session)...)
	}
	// A character name can only be online once.
	if other, ok := g.byName[c.Name]; ok && other != session {
		effects = append(effects, g.Disconnect(other)...)
	}
	g.sessions[session] = c
	g.byName[c.Name] = session
	g.scheduleRegen(session)
	effects = append(effects, Output{Session: session, Text: g.renderRoom(session, c.Room)})
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) 모습을 드러냈다.", c.Name))...)
	effects = append(effects, g.checkAggro(session, c)...)
	effects = append(effects, g.save(session))
	return effects
}

// Disconnect removes the session from the world. It is idempotent.
func (g *Game) Disconnect(session SessionID) []Effect {
	c, ok := g.sessions[session]
	if !ok {
		return nil
	}
	effects := g.breakCombat(session, c)
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) 자취를 감췄다.", c.Name))...)
	delete(g.sessions, session)
	delete(g.byName, c.Name)
	effects = append(effects, g.saveCharacter(c))
	return effects
}

// Execute runs a normalized command for the session. Unknown verbs do not
// change state and return a guidance message.
func (g *Game) Execute(session SessionID, cmd Command) []Effect {
	c, ok := g.sessions[session]
	if !ok {
		return nil
	}
	switch cmd.Verb {
	case VerbLook:
		return []Effect{Output{Session: session, Text: g.renderRoom(session, c.Room)}}
	case VerbMove:
		return g.move(session, c, cmd.Direction)
	case VerbSay:
		return g.say(session, c, cmd.Text)
	case VerbGet:
		return g.pickUp(session, c, cmd.Target, cmd.Index)
	case VerbDrop:
		return g.dropItem(session, c, cmd.Target, cmd.Index)
	case VerbInventory:
		return g.inventory(session, c)
	case VerbAttack:
		return g.attack(session, c, cmd.Target, cmd.Index)
	case VerbAdmin:
		return g.admin(session, c, cmd)
	default:
		return []Effect{Output{Session: session, Text: unknownCommandHelp}}
	}
}

// ExecuteLine parses raw player input and runs it. Parse failures do not
// change state and come back as a guidance Output effect.
func (g *Game) ExecuteLine(session SessionID, line string) []Effect {
	cmd, err := Parse(line)
	if err != nil {
		return []Effect{Output{Session: session, Text: err.Error()}}
	}
	return g.Execute(session, cmd)
}

// Tick advances the engine to now, executing every scheduled event whose
// time has come exactly once and returning the resulting effects.
func (g *Game) Tick(now time.Time) []Effect {
	var effects []Effect
	for {
		due := g.events.due(now)
		if len(due) == 0 {
			break
		}
		for _, ev := range due {
			effects = append(effects, g.runEvent(ev)...)
		}
	}
	return effects
}

func (g *Game) runEvent(ev gameEvent) []Effect {
	switch ev.kind {
	case evWander:
		return g.npcWander(ev.npcInst)
	case evNPCRespawn:
		return g.respawnNPC(ev.def, ev.room)
	case evPlayerRound:
		c, ok := g.sessions[ev.session]
		if !ok {
			return nil
		}
		inst, ok := g.npcByID(ev.npcInst)
		if !ok {
			return nil
		}
		if !g.stillEngaged(ev.session, c, inst) {
			return nil
		}
		return g.playerStrike(ev.session, c, inst)
	case evNPCRound:
		c, ok := g.sessions[ev.session]
		if !ok {
			return nil
		}
		inst, ok := g.npcByID(ev.npcInst)
		if !ok {
			return nil
		}
		return g.npcStrike(ev.session, c, inst)
	case evRegen:
		return g.regen(ev.session)
	default:
		return nil
	}
}

// move handles directional movement between rooms.
func (g *Game) move(session SessionID, c *Character, dir Direction) []Effect {
	room, ok := g.world.Room(c.Room)
	if !ok {
		return []Effect{Output{Session: session, Text: "현재 위치를 알 수 없습니다. 운영자에게 문의하세요."}}
	}
	to, ok := room.Exits[dir]
	if !ok {
		return []Effect{Output{Session: session, Text: fmt.Sprintf("%s 방향으로는 갈 수 없습니다.", dir)}}
	}
	effects := g.breakCombat(session, c)
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) %s(으)로 이동했다.", c.Name, dir))...)
	c.Room = to
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s이(가) 모습을 드러냈다.", c.Name))...)
	effects = append(effects, Output{Session: session, Text: g.renderRoom(session, c.Room)})
	effects = append(effects, g.checkAggro(session, c)...)
	effects = append(effects, g.save(session))
	return effects
}

// say broadcasts chat to everyone in the speaker's room.
func (g *Game) say(session SessionID, c *Character, text string) []Effect {
	if strings.TrimSpace(text) == "" {
		return []Effect{Output{Session: session, Text: "무엇을 말하시겠습니까? 예: 말하기 안녕하세요"}}
	}
	effects := []Effect{Output{Session: session, Text: "당신: " + text}}
	effects = append(effects, g.broadcastRoom(c.Room, session, fmt.Sprintf("%s: %s", c.Name, text))...)
	return effects
}

// renderRoom builds the look output for the session's current room,
// including other characters present there.
func (g *Game) renderRoom(session SessionID, room RoomID) string {
	out := g.world.Describe(room)
	out += g.npcRoomStateText(room)
	if state, ok := g.rooms[room]; ok && len(state.items) > 0 {
		names := make([]string, 0, len(state.items))
		for _, it := range state.items {
			names = append(names, g.itemName(*it))
		}
		out += "\n바닥에 놓인 물건: " + strings.Join(names, ", ")
	}
	var others []string
	for s, c := range g.sessions {
		if s != session && c.Room == room {
			others = append(others, c.Name)
		}
	}
	if len(others) > 0 {
		sort.Strings(others)
		out += "\n함께 있는 사람: " + strings.Join(others, ", ")
	}
	return out
}

// save returns a SaveCharacter effect for the session's character.
func (g *Game) save(session SessionID) Effect {
	c, ok := g.sessions[session]
	if !ok {
		return nil
	}
	return g.saveCharacter(c)
}

func (g *Game) saveCharacter(c *Character) Effect {
	return SaveCharacter{Character: c.Copy()}
}

// broadcastRoom sends text to every session in the room except skip.
func (g *Game) broadcastRoom(room RoomID, skip SessionID, text string) []Effect {
	var targets []SessionID
	for s, c := range g.sessions {
		if s != skip && c.Room == room {
			targets = append(targets, s)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	return []Effect{Broadcast{Sessions: targets, Text: text}}
}

const unknownCommandHelp = "알 수 없는 명령입니다. 사용 가능한 명령: 보기, 북쪽/남쪽/동쪽/서쪽/위/아래, 말하기 <내용>, 줍기 <물건>, 버리기 <물건>, 인벤토리, 공격 <대상>"
