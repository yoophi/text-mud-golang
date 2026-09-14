package e2e

import (
	"testing"
	"time"
)

// mvpWorld is a compact world for the full MVP scenario: a weak rat to
// kill, an overwhelming wolf to die to, and items to carry.
const mvpWorld = `{
  "startRoom": "plaza",
  "respawnRoom": "plaza",
  "rooms": [
    {"id": "plaza", "name": "마을 광장", "description": "오래된 분수가 있는 광장이다.", "exits": {"북쪽": "alley", "동쪽": "market"}},
    {"id": "market", "name": "시장", "description": "활기찬 시장이다.", "exits": {"서쪽": "plaza"}},
    {"id": "alley", "name": "뒷골목", "description": "좁고 어두운 골목이다.", "exits": {"남쪽": "plaza", "아래": "cellar"}},
    {"id": "cellar", "name": "지하 저장고", "description": "축축한 저장고다.", "exits": {"위": "alley"}}
  ],
  "items": [
    {"id": "coin", "name": "동전", "description": "반짝이는 동전이다."},
    {"id": "dagger", "name": "낡은 단검", "aliases": ["단검"], "description": "무뎌진 단검이다."}
  ],
  "itemSpawns": [
    {"room": "plaza", "item": "coin", "count": 1},
    {"room": "cellar", "item": "dagger", "count": 1}
  ],
  "npcs": [
    {"id": "rat", "name": "쥐", "aliases": ["생쥐"], "room": "alley", "hp": 2, "damageMin": 1, "damageMax": 1, "aggressive": false, "respawnSeconds": 2},
    {"id": "dire_wolf", "name": "굶주린 늑대", "aliases": ["늑대"], "room": "cellar", "hp": 100, "damageMin": 40, "damageMax": 50, "aggressive": true, "respawnSeconds": 30}
  ]
}`

// TestFullMVPScenario walks the whole first-completion criteria over real
// TCP with two concurrent clients: login, meet, talk, move apart, pick up
// items, fight, die, revive, operator shutdown, restart restore.
func TestFullMVPScenario(t *testing.T) {
	ts := startServer(t, mvpWorld, "영희") // 영희 is the operator (for shutdown)

	a := dial(t, ts.addr)
	a.login("영희", true, "마을 광장")
	b := dial(t, ts.addr)
	b.login("철수", true, "마을 광장")

	// Two clients in the same room see each other and hear each other.
	a.send("보기")
	a.readUntil("함께 있는 사람: 철수", 5*time.Second)
	a.send("말하기 다 같이 모험을 떠나자!")
	b.readUntil("영희: 다 같이 모험을 떠나자!", 5*time.Second)

	// A picks up an item before leaving.
	a.send("줍기 동전")
	a.readUntil("주웠", 5*time.Second)

	// Independent movement in opposite directions.
	a.send("북쪽")
	a.readUntil("뒷골목", 5*time.Second)
	b.readUntil("영희이(가) 북쪽(으)로 이동했다", 5*time.Second) // B still in plaza, sees A leave
	b.send("동쪽")
	b.readUntil("시장", 5*time.Second)

	// A slays the weak rat in a single strike.
	a.send("공격 쥐")
	a.readUntil("쓰러뜨렸습니다", 5*time.Second)

	// A descends and is mauled by the aggressive wolf, dies, revives.
	a.send("아래")
	a.readUntil("굶주린 늑대이(가) 당신을 노려봅니다", 5*time.Second)
	a.readUntil("굶주린 늑대이(가) 당신을 공격했습니다", 10*time.Second)
	a.readUntil("당신은 쓰러졌습니다", 15*time.Second)
	a.readUntil("부활 지점", 5*time.Second)

	// Death keeps the inventory.
	a.send("인벤토리")
	a.readUntil("1) 동전", 5*time.Second)

	// Operator shutdown, then restart: positions and inventory restore.
	a.send("@shutdown")
	ts.waitStopped()

	ts.restart()

	a2 := dial(t, ts.addr)
	a2.login("영희", false, "마을 광장")
	a2.send("인벤토리")
	a2.readUntil("1) 동전", 5*time.Second)

	b2 := dial(t, ts.addr)
	b2.login("철수", false, "시장")
}
