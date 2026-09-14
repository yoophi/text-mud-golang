package domain

import (
	"strings"
	"testing"
)

func TestParseLookAliases(t *testing.T) {
	for _, in := range []string{"보기", " 뷰 ", "look", "L", "주변"} {
		cmd, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if cmd.Verb != VerbLook {
			t.Fatalf("Parse(%q) verb = %s, want 보기", in, cmd.Verb)
		}
	}
}

func TestParseDirectionAliases(t *testing.T) {
	cases := map[string]Direction{
		"북": DirNorth, "북쪽": DirNorth, "N": DirNorth, "north": DirNorth,
		"남": DirSouth, "남쪽": DirSouth, "s": DirSouth,
		"동": DirEast, "동쪽": DirEast, "e": DirEast,
		"서": DirWest, "서쪽": DirWest, "w": DirWest,
		"위": DirUp, "u": DirUp,
		"아래": DirDown, "밑": DirDown, "d": DirDown,
	}
	for in, want := range cases {
		cmd, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if cmd.Verb != VerbMove || cmd.Direction != want {
			t.Fatalf("Parse(%q) = %+v, want move %s", in, cmd, want)
		}
	}
}

func TestParseDirectionWithParticle(t *testing.T) {
	for _, in := range []string{"북쪽으로", "북으로", "위로"} {
		cmd, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if cmd.Verb != VerbMove {
			t.Fatalf("Parse(%q) verb = %s", in, cmd.Verb)
		}
	}
	if cmd, _ := Parse("위로"); cmd.Direction != DirUp {
		t.Fatalf("위로 direction = %s", cmd.Direction)
	}
}

func TestParseExplicitMoveVerb(t *testing.T) {
	cmd, err := Parse("이동 남쪽")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Verb != VerbMove || cmd.Direction != DirSouth {
		t.Fatalf("Parse = %+v", cmd)
	}
	if _, err := Parse("이동 북동쪽"); err == nil {
		t.Fatal("unknown direction argument must fail")
	}
	if _, err := Parse("이동"); err == nil {
		t.Fatal("move without direction must fail")
	}
}

func TestParseRejectsEmptyAndUnknown(t *testing.T) {
	for _, in := range []string{"", "   ", "춤추기", "dance"} {
		if _, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) should fail", in)
		}
	}
}

func TestStripParticle(t *testing.T) {
	cases := map[string]string{
		"모자를":   "모자",
		"쥐가":    "쥐",
		"쥐":     "쥐",
		"모래에서":  "모래",
		"빵은":    "빵",
		"동전으로":  "동전",
		"칼과":    "칼",
		"bread": "bread",
	}
	for in, want := range cases {
		if got := StripParticle(in); got != want {
			t.Fatalf("StripParticle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTargetCandidates(t *testing.T) {
	got := TargetCandidates("칼에게도")
	want := []string{"칼에게도", "칼에게", "칼"}
	if len(got) != len(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidates = %v, want %v", got, want)
		}
	}
	if got := TargetCandidates("모래에서는"); len(got) != 2 || got[1] != "모래" {
		t.Fatalf("compound particle should strip at once: %v", got)
	}
}

func TestExecuteLineMovesAndBlocks(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	g.ExecuteLine("s1", "북")
	if c, _ := g.Character("영희"); c.Room != "alley" {
		t.Fatalf("room = %s, want alley", c.Room)
	}

	effects := g.ExecuteLine("s1", "동쪽")
	outs := outputs(effects)
	if len(outs) != 1 || !strings.Contains(outs[0].Text, "갈 수 없습니다") {
		t.Fatalf("blocked move output = %+v", outs)
	}
	if c, _ := g.Character("영희"); c.Room != "alley" {
		t.Fatal("blocked move must not change the room")
	}
}

func TestExecuteLineUnknownKeepsState(t *testing.T) {
	g, _ := newTestGame(t)
	g.Connect("s1", NewCharacter("영희", "plaza"))

	effects := g.ExecuteLine("s1", "  춤추기  ")
	outs := outputs(effects)
	if len(outs) != 1 || !strings.Contains(outs[0].Text, "알 수 없는 명령") {
		t.Fatalf("unknown input output = %+v", outs)
	}
	if hasSave(effects) {
		t.Fatal("unknown input must not save")
	}
	if c, _ := g.Character("영희"); c.Room != "plaza" {
		t.Fatal("unknown input must not move the character")
	}
}
