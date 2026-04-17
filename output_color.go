package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/logrusorgru/aurora/v4"
	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/term"
)

// Colors (via Aurora): Green = exact match, BrightCyan = chars only on this stem vs ref (fuzzy diff),
// BrightYellow gap marker (·) where the sibling has extra text you do not, Red = extension / mismatch, BrightBlack = directory.
func colorAur() *aurora.Aurora {
	return aurora.New(aurora.WithColors(useColor()))
}

func useColor() bool {
	if flagNoColor || flagNoColour {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func colorSummary() string {
	if flagNoColor || flagNoColour {
		return "off (--no-color / --no-colour)"
	}
	if os.Getenv("NO_COLOR") != "" {
		return "off (NO_COLOR)"
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return "Aurora colors when listing duplicates"
	}
	return "off (stdout not a TTY)"
}

// fileBaseAndExt splits filepath.Base(name) into stem and extension (ext includes dot, may be empty).
func fileBaseAndExt(filename string) (base, ext string) {
	ext = filepath.Ext(filename)
	base = strings.TrimSuffix(filename, ext)
	return base, ext
}

// diffGapMarker is printed on this line when the sibling stem has characters here that this stem omits.
const diffGapMarker = "·"

// cyclicNeighborStem returns the stem of basename(files[(i+1) % n]).
// Each row diffs against a neighbor so inserts (e.g. "-" vs "+") show as BrightCyan on both filenames;
// where one stem omits text the other has, a yellow gap marker shows the other side of the diff.
func cyclicNeighborStem(files []fileEntry, i int) string {
	n := len(files)
	if n == 0 {
		return ""
	}
	j := (i + 1) % n
	stem, _ := fileBaseAndExt(filepath.Base(files[j].path))
	return stem
}

func colorizeBasenameAgainstRef(refBase, thisBase string, fuzzyCluster bool) string {
	if !useColor() {
		return thisBase
	}
	return colorizeStemDiff(colorAur(), refBase, thisBase, fuzzyCluster)
}

// colorizeStemDiff paints thisBase against refBase (DiffMain(refBase, thisBase)).
// Inserts unique to thisBase are BrightCyan (fuzzy) or Red (exact); pure deletions render as a gap marker on this line.
func colorizeStemDiff(a *aurora.Aurora, refBase, thisBase string, fuzzyCluster bool) string {
	if refBase == thisBase {
		return a.Green(thisBase).String()
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(refBase, thisBase, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var b strings.Builder
	for i := 0; i < len(diffs); i++ {
		d := diffs[i]
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			b.WriteString(a.Green(d.Text).String())
		case diffmatchpatch.DiffInsert:
			if fuzzyCluster {
				b.WriteString(a.BrightCyan(d.Text).String())
			} else {
				b.WriteString(a.Red(d.Text).String())
			}
		case diffmatchpatch.DiffDelete:
			// Substitution (e.g. "-" vs "+"): only highlight the text on *this* stem; do not add a gap marker.
			if i+1 < len(diffs) && diffs[i+1].Type == diffmatchpatch.DiffInsert {
				ins := diffs[i+1].Text
				if fuzzyCluster {
					b.WriteString(a.BrightCyan(ins).String())
				} else {
					b.WriteString(a.Red(ins).String())
				}
				i++
				continue
			}
			// ref has text here that this stem omits — gap marker on this line so both sides show a diff.
			if fuzzyCluster {
				b.WriteString(a.BrightYellow(diffGapMarker).String())
			} else {
				b.WriteString(a.Red(diffGapMarker).String())
			}
		}
	}
	return b.String()
}

// formatClusterPathLine prints one full path: dim directory prefix, colored basename stem, red extension.
// refBase is the stem to diff against (typically cyclicNeighborStem for this index).
func formatClusterPathLine(refBase, fullPath string, fuzzyCluster bool) string {
	baseName := filepath.Base(fullPath)
	if !useColor() {
		return fullPath
	}

	a := colorAur()
	thisBase, ext := fileBaseAndExt(baseName)
	stemColored := colorizeBasenameAgainstRef(refBase, thisBase, fuzzyCluster)
	var extOut string
	if ext != "" {
		extOut = a.Red(ext).String()
	}
	prefix := strings.TrimSuffix(fullPath, baseName)
	return a.BrightBlack(prefix).String() + stemColored + extOut
}

// formatClusterHeaderLabel formats the match key shown in brackets for the first line of a group.
// When the key is the full filename (e.g. .nfo sidecars), stem is exact green and extension red.
func formatClusterHeaderLabel(label, firstFilePath string) string {
	if !useColor() {
		return label
	}
	a := colorAur()
	base := filepath.Base(firstFilePath)
	st, ex := fileBaseAndExt(base)
	switch {
	case label == st:
		return a.Green(label).String()
	case label == base && ex != "":
		return a.Green(st).String() + a.Red(ex).String()
	default:
		return a.Green(label).String()
	}
}
