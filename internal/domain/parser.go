package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// directionAliases maps every accepted direction spelling to its
// canonical Direction value.
var directionAliases = map[string]Direction{
	"북쪽": DirNorth, "북": DirNorth, "n": DirNorth, "north": DirNorth,
	"남쪽": DirSouth, "남": DirSouth, "s": DirSouth, "south": DirSouth,
	"동쪽": DirEast, "동": DirEast, "e": DirEast, "east": DirEast,
	"서쪽": DirWest, "서": DirWest, "w": DirWest, "west": DirWest,
	"위": DirUp, "u": DirUp, "up": DirUp, "올라": DirUp,
	"아래": DirDown, "밑": DirDown, "d": DirDown, "down": DirDown, "내려": DirDown,
}

var lookAliases = []string{"보기", "뷰", "주변", "look", "l"}

// sayAliases are chat verbs; everything after the verb is the message.
var sayAliases = []string{"말하기", "말", "say", "'"}

// moveAliases are explicit movement verbs that take a direction argument.
var moveAliases = []string{"이동", "가기", "가자", "go", "move"}

// particles are Korean case/attachment suffixes stripped from command
// targets. Longest suffixes are tried first.
var particles = []string{
	"에게서", "에서는", "에게는", "에게서",
	"에서", "에게", "으로", "로서", "한테", "이랑", "하고", "처럼", "보다",
	"부터", "까지", "마다", "만큼", "이다", "이네", "이야", "로",
	"은", "는", "이", "가", "을", "를", "와", "과", "도", "만", "의", "랑", "야", "여", "께", "뿐", "조차",
}

// Normalize trims the line and collapses inner whitespace.
func Normalize(line string) string {
	return strings.Join(strings.Fields(line), " ")
}

// StripParticle removes one trailing Korean particle from a noun token.
// It returns the token unchanged when no known particle is attached.
func StripParticle(token string) string {
	for _, p := range particles {
		if strings.HasSuffix(token, p) && len(token) > len(p) {
			return strings.TrimSuffix(token, p)
		}
	}
	return token
}

// TargetCandidates returns the token followed by progressively
// particle-stripped forms, most specific first.
func TargetCandidates(token string) []string {
	out := []string{token}
	once := StripParticle(token)
	if once != token {
		out = append(out, once)
		twice := StripParticle(once)
		if twice != once {
			out = append(out, twice)
		}
	}
	return out
}

// Parse converts a normalized input line into an engine-ready Command.
// It returns an error for empty input, unknown verbs, and malformed
// movement arguments; callers must show the error without changing state.
func Parse(line string) (Command, error) {
	words := strings.Fields(Normalize(line))
	if len(words) == 0 {
		return Command{}, fmt.Errorf("명령을 입력하세요. 예: 보기, 북쪽")
	}
	head := words[0]
	rest := words[1:]

	if matchesAlias(head, lookAliases) && len(rest) == 0 {
		return Command{Verb: VerbLook}, nil
	}
	if matchesAlias(head, sayAliases) {
		return Command{Verb: VerbSay, Text: strings.Join(rest, " ")}, nil
	}
	if dir, ok := parseDirection(head); ok && len(rest) == 0 {
		return Command{Verb: VerbMove, Direction: dir}, nil
	}
	if matchesAlias(head, moveAliases) {
		if len(rest) == 0 {
			return Command{}, fmt.Errorf("어느 방향으로 이동할까요? 예: 이동 북쪽")
		}
		dir, ok := parseDirection(rest[0])
		if !ok {
			return Command{}, fmt.Errorf("%s 방향을 알 수 없습니다. 북쪽/남쪽/동쪽/서쪽/위/아래 중 하나를 입력하세요.", rest[0])
		}
		return Command{Verb: VerbMove, Direction: dir}, nil
	}

	// Item and inventory verbs.
	if matchesAlias(head, getAliases) {
		target, index, err := parseTargetArgs(rest)
		if err != nil {
			return Command{}, err
		}
		return Command{Verb: VerbGet, Target: target, Index: index}, nil
	}
	if matchesAlias(head, dropAliases) {
		target, index, err := parseTargetArgs(rest)
		if err != nil {
			return Command{}, err
		}
		return Command{Verb: VerbDrop, Target: target, Index: index}, nil
	}
	if matchesAlias(head, invAliases) && len(rest) == 0 {
		return Command{Verb: VerbInventory}, nil
	}

	// Later issues extend the parser with combat verbs.
	return Command{}, fmt.Errorf(unknownCommandHelp)
}

// parseTargetArgs splits verb arguments into a target name and an
// optional 1-based selection index: "빵 2" -> ("빵", 2).
func parseTargetArgs(rest []string) (string, int, error) {
	if len(rest) == 0 {
		return "", 0, nil
	}
	index := 0
	words := rest
	if n, err := strconv.Atoi(rest[len(rest)-1]); err == nil {
		index = n
		words = rest[:len(rest)-1]
	}
	target := strings.Join(words, " ")
	if target == "" {
		return "", 0, fmt.Errorf("대상 이름을 입력하세요. 예: 빵 2")
	}
	if index < 0 {
		return "", 0, fmt.Errorf("번호는 1 이상이어야 합니다")
	}
	return target, index, nil
}

// parseDirection resolves a bare token to a canonical direction,
// tolerating trailing particles such as 북쪽으로.
func parseDirection(token string) (Direction, bool) {
	for _, cand := range TargetCandidates(token) {
		if d, ok := directionAliases[strings.ToLower(cand)]; ok {
			return d, true
		}
	}
	return "", false
}

func matchesAlias(token string, aliases []string) bool {
	for _, a := range aliases {
		if strings.EqualFold(token, a) {
			return true
		}
	}
	return false
}
