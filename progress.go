package main

import (
	"fmt"
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

func useProgress() bool {
	return !flagNoProgress && term.IsTerminal(int(os.Stderr.Fd()))
}

func progressStatusLine() string {
	if flagNoProgress {
		return "disabled (--no-progress)"
	}
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return "stderr bars + ETA (scan / fuzzy pairs / hash)"
	}
	return "off (stderr not a TTY)"
}

func newScanBar() *progressbar.ProgressBar {
	return progressbar.NewOptions64(-1,
		progressbar.OptionSetDescription("Scanning files"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(12),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("files"),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionOnCompletion(func() { fmt.Fprint(os.Stderr, "\n") }),
		progressbar.OptionSetRenderBlankState(true),
	)
}

func newCompareBar(total int64) *progressbar.ProgressBar {
	return progressbar.NewOptions64(total,
		progressbar.OptionSetDescription("Comparing names (fuzzy pairs)"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(12),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("pairs"),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionOnCompletion(func() { fmt.Fprint(os.Stderr, "\n") }),
		progressbar.OptionSetRenderBlankState(true),
	)
}

func newHashBar(total int64, algo string) *progressbar.ProgressBar {
	return progressbar.NewOptions64(total,
		progressbar.OptionSetDescription(fmt.Sprintf("Hashing contents (%s)", algo)),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(12),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("files"),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionOnCompletion(func() { fmt.Fprint(os.Stderr, "\n") }),
		progressbar.OptionSetRenderBlankState(true),
	)
}
