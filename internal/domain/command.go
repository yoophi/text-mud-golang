package domain

// Verb identifies a parsed player command.
type Verb string

const (
	VerbLook      Verb = "보기"
	VerbMove      Verb = "이동"
	VerbSay       Verb = "말하기"
	VerbGet       Verb = "줍기"
	VerbDrop      Verb = "버리기"
	VerbInventory Verb = "인벤토리"
	VerbAttack    Verb = "공격"
	VerbAdmin     Verb = "운영"
)

// AdminAction identifies an operator subcommand.
type AdminAction string

const (
	AdminGoto     AdminAction = "goto"
	AdminSpawn    AdminAction = "spawn"
	AdminShutdown AdminAction = "shutdown"
)

// Command is a normalized, engine-ready representation of player input.
type Command struct {
	Verb      Verb
	Direction Direction // VerbMove
	Text      string    // VerbSay
	Target    string    // VerbGet/Drop/Attack target name
	Index     int       // 1-based selection among same-named targets
	Admin     AdminAction
	AdminArg  string
}
