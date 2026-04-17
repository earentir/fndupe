package main

import (
	"strings"
	"testing"

	"github.com/logrusorgru/aurora/v4"
)

func TestFileBaseAndExt(t *testing.T) {
	b, e := fileBaseAndExt("Venom - The Last Dance (2024).nfo")
	if b != "Venom - The Last Dance (2024)" || e != ".nfo" {
		t.Fatalf("got base=%q ext=%q", b, e)
	}
	b, e = fileBaseAndExt("README")
	if b != "README" || e != "" {
		t.Fatalf("got base=%q ext=%q", b, e)
	}
}

func TestFormatClusterPathLinePlainWhenNoColor(t *testing.T) {
	flagNoColor = true
	flagNoColour = false
	defer func() {
		flagNoColor = false
		flagNoColour = false
	}()
	p := "/home/movies/Venom The Last Dance (2024).nfo"
	got := formatClusterPathLine("Venom - The Last Dance (2024)", p, true)
	if got != p {
		t.Fatalf("expected plain path, got %q", got)
	}
}

func TestColorizeStemDiffSubstitution(t *testing.T) {
	a := aurora.New(aurora.WithColors(true))
	minus := "Venom - The Last Dance (2024)"
	plus := "Venom + The Last Dance (2024)"
	outMinus := colorizeStemDiff(a, plus, minus, true)
	outPlus := colorizeStemDiff(a, minus, plus, true)
	if strings.Count(outMinus, diffGapMarker) != 0 || strings.Count(outPlus, diffGapMarker) != 0 {
		t.Fatalf("substitution should not emit gap markers: minus=%q plus=%q", outMinus, outPlus)
	}
	if !strings.Contains(outMinus, "-") || !strings.Contains(outPlus, "+") {
		t.Fatalf("expected both differing chars preserved: %q %q", outMinus, outPlus)
	}
}

func TestColorizeStemDiffOmission(t *testing.T) {
	a := aurora.New(aurora.WithColors(true))
	short := "Venom The Last Dance (2024)"
	long := "Venom - The Last Dance (2024)"
	outShort := colorizeStemDiff(a, long, short, true)
	if strings.Count(outShort, diffGapMarker) != 1 {
		t.Fatalf("shorter stem should have one gap marker, got %q", outShort)
	}
	outLong := colorizeStemDiff(a, short, long, true)
	if strings.Count(outLong, diffGapMarker) != 0 {
		t.Fatalf("longer stem should not use gap marker, got %q", outLong)
	}
}

func TestFormatClusterPathLinePlainWhenNoColour(t *testing.T) {
	flagNoColor = false
	flagNoColour = true
	defer func() {
		flagNoColor = false
		flagNoColour = false
	}()
	p := "/a/b/c.txt"
	got := formatClusterPathLine("c", p, true)
	if got != p {
		t.Fatalf("expected plain path, got %q", got)
	}
}
