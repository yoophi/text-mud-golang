package domain

import "time"

// eventKind enumerates scheduled engine events.
type eventKind int

const (
	evNPCRespawn eventKind = iota
	evPlayerRound
	evNPCRound
	evRegen
)

// gameEvent is a scheduled event. seq preserves FIFO order for events
// scheduled at the same instant.
type gameEvent struct {
	at      time.Time
	kind    eventKind
	session SessionID // player involved (may be empty)
	npcInst string    // NPC instance involved (may be empty)
	def     *NPCDef   // for respawn events
	room    RoomID    // for respawn events
	seq     uint64
}

// eventQueue is a time-ordered queue drained by Game.Tick.
type eventQueue struct {
	events []gameEvent
	seq    uint64
}

func (q *eventQueue) push(at time.Time, kind eventKind, session SessionID, npcInst string) {
	q.seq++
	q.insert(gameEvent{at: at, kind: kind, session: session, npcInst: npcInst, seq: q.seq})
}

func (q *eventQueue) pushRespawn(at time.Time, def *NPCDef, room RoomID) {
	q.seq++
	q.insert(gameEvent{at: at, kind: evNPCRespawn, def: def, room: room, seq: q.seq})
}

func (q *eventQueue) insert(ev gameEvent) {
	i := len(q.events)
	for i > 0 && before(ev, q.events[i-1]) {
		i--
	}
	q.events = append(q.events, gameEvent{})
	copy(q.events[i+1:], q.events[i:])
	q.events[i] = ev
}

func before(a, b gameEvent) bool {
	if !a.at.Equal(b.at) {
		return a.at.Before(b.at)
	}
	return a.seq < b.seq
}

// due pops every event scheduled at or before now.
func (q *eventQueue) due(now time.Time) []gameEvent {
	var out []gameEvent
	for len(q.events) > 0 && !q.events[0].at.After(now) {
		out = append(out, q.events[0])
		q.events = q.events[1:]
	}
	return out
}

// dropFor removes every event involving the given session or NPC
// instance, so disengaged combat stops cleanly.
func (q *eventQueue) dropFor(session SessionID, npcInst string) {
	kept := q.events[:0]
	for _, ev := range q.events {
		if session != "" && ev.session == session {
			continue
		}
		if npcInst != "" && ev.npcInst == npcInst {
			continue
		}
		kept = append(kept, ev)
	}
	q.events = kept
}
