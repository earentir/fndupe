package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/network-plane/textscore"
)

func TestParseCSVList(t *testing.T) {
	got := parseCSVList(" .JPG, png , ,TXT,.md ")
	want := []string{"jpg", "png", "txt", "md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCSVList mismatch: got=%v want=%v", got, want)
	}
}

func TestResolveCommonExts(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got, err := resolveCommonExts("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil map for empty input, got=%v", got)
		}
	})

	t.Run("preset", func(t *testing.T) {
		got, err := resolveCommonExts("image")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got["jpg"]; !ok {
			t.Fatalf("expected preset to include jpg")
		}
		if _, ok := got["bmp"]; !ok {
			t.Fatalf("expected preset to include bmp")
		}
	})

	t.Run("csv", func(t *testing.T) {
		got, err := resolveCommonExts("jpg,.bmp,png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, ext := range []string{"jpg", "bmp", "png"} {
			if _, ok := got[ext]; !ok {
				t.Fatalf("expected ext %q in set", ext)
			}
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := resolveCommonExts(" , , ")
		if err == nil {
			t.Fatalf("expected error for invalid --common-ext value")
		}
	})
}

func TestMatchKeyModes(t *testing.T) {
	f := fileEntry{name: "photo.jpg", base: "photo", ext: "jpg"}

	if got := matchKey(f, nil); got != "photo.jpg" {
		t.Fatalf("nil set should keep full name, got=%q", got)
	}

	union := UnionAllPresetExtensions()
	if got := matchKey(f, union); got != "photo" {
		t.Fatalf("jpg in preset union should use basename, got=%q", got)
	}

	if got := matchKey(f, map[string]struct{}{"jpg": {}}); got != "photo" {
		t.Fatalf("jpg in set should use basename, got=%q", got)
	}

	if got := matchKey(f, map[string]struct{}{"png": {}}); got != "photo.jpg" {
		t.Fatalf("jpg not in set should keep full name, got=%q", got)
	}
}

func TestFindExactCommonExtModes(t *testing.T) {
	files := []fileEntry{
		{name: "photo.jpg", base: "photo", ext: "jpg", path: "/a/photo.jpg"},
		{name: "photo.bmp", base: "photo", ext: "bmp", path: "/b/photo.bmp"},
		{name: "notes.txt", base: "notes", ext: "txt", path: "/c/notes.txt"},
	}

	// Default behavior: compare full name (no groups here).
	defaultCfg := RunConfig{Exact: true, CommonExts: nil}
	if got := FindDuplicates(files, defaultCfg); len(got) != 0 {
		t.Fatalf("expected no groups by default, got=%d", len(got))
	}

	// Bare --common-ext behavior: union of all presets (jpg and bmp both in image).
	allExtCfg := RunConfig{Exact: true, CommonExts: UnionAllPresetExtensions()}
	gotAll := FindDuplicates(files, allExtCfg)
	if len(gotAll) != 1 {
		t.Fatalf("expected 1 group in all-ext mode, got=%d", len(gotAll))
	}
	if len(gotAll[0]) != 2 {
		t.Fatalf("expected 2 files in grouped result, got=%d", len(gotAll[0]))
	}

	// Preset-like behavior: only selected extensions are basename-compared.
	imageOnlyCfg := RunConfig{Exact: true, CommonExts: map[string]struct{}{"jpg": {}, "bmp": {}}}
	gotImage := FindDuplicates(files, imageOnlyCfg)
	if len(gotImage) != 1 || len(gotImage[0]) != 2 {
		t.Fatalf("expected 1 image-only group of size 2, got=%v", len(gotImage))
	}
}

func TestScanFilesFilters(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "keep.txt"))
	mustTouch(t, filepath.Join(root, "skip.nfo"))
	mustTouch(t, filepath.Join(root, "movie_sample.mp4"))
	mustTouch(t, filepath.Join(root, "nested", "other.md"))

	cfg := RunConfig{
		Root:        root,
		ExcludeExts: map[string]struct{}{"nfo": {}},
		ExcludeStrs: []string{"sample"},
	}
	files, err := ScanFiles(cfg)
	if err != nil {
		t.Fatalf("ScanFiles error: %v", err)
	}

	var names []string
	for _, f := range files {
		names = append(names, f.name)
	}
	sort.Strings(names)
	want := []string{"keep.txt", "other.md"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("filtered names mismatch: got=%v want=%v", names, want)
	}
}

func TestUnionAllPresetsDoesNotExactGroupNfoWithMkv(t *testing.T) {
	files := []fileEntry{
		{name: "Show (2025).mkv", base: "Show (2025)", ext: "mkv", path: "/a/Show (2025).mkv"},
		{name: "Show (2025).nfo", base: "Show (2025)", ext: "nfo", path: "/b/Show (2025).nfo"},
	}
	cfg := RunConfig{Exact: true, CommonExts: UnionAllPresetExtensions()}
	if got := FindDuplicates(files, cfg); len(got) != 0 {
		t.Fatalf("expected no exact group for mkv+nfo with union presets, got %d", len(got))
	}
}

func TestFuzzyDefaultDoesNotLinkMp4ToNfo(t *testing.T) {
	// Long titles: hybrid score for .mp4 vs .nfo can exceed the default threshold; same-extension rule must still block.
	files := []fileEntry{
		{name: "Valerian And The City Of A Thousand Planets (2017).mp4", base: "Valerian And The City Of A Thousand Planets (2017)", ext: "mp4", path: "/a/Valerian And The City Of A Thousand Planets (2017).mp4"},
		{name: "Valerian And The City Of A Thousand Planets (2017).nfo", base: "Valerian And The City Of A Thousand Planets (2017)", ext: "nfo", path: "/b/Valerian And The City Of A Thousand Planets (2017).nfo"},
	}
	cfg := RunConfig{
		Exact:      false,
		Threshold:  0.85,
		Metric:     textscore.MetricHybrid,
		CommonExts: nil,
	}
	clusters := FindDuplicates(files, cfg)
	if len(clusters) != 0 {
		t.Fatalf("expected no cross-extension fuzzy cluster by default, got %d", len(clusters))
	}
}

func TestFuzzyCrossExtBareExcludeExt(t *testing.T) {
	files := []fileEntry{
		{name: "Valerian And The City Of A Thousand Planets (2017).mp4", base: "Valerian And The City Of A Thousand Planets (2017)", ext: "mp4", path: "/a/Valerian And The City Of A Thousand Planets (2017).mp4"},
		{name: "Valerian And The City Of A Thousand Planets (2017).nfo", base: "Valerian And The City Of A Thousand Planets (2017)", ext: "nfo", path: "/b/Valerian And The City Of A Thousand Planets (2017).nfo"},
	}
	cfg := RunConfig{
		Exact:          false,
		Threshold:      0.85,
		Metric:         textscore.MetricHybrid,
		CommonExts:     nil,
		CrossExtFuzzy: true,
	}
	clusters := FindDuplicates(files, cfg)
	if len(clusters) != 1 || len(clusters[0]) != 2 {
		t.Fatalf("expected one cluster of two with CrossExtFuzzy, got %+v", clusters)
	}
}

func TestFuzzyDefaultSameExtStillClusters(t *testing.T) {
	files := []fileEntry{
		{name: "Venom The Last Dance (2024).mkv", base: "Venom The Last Dance (2024)", ext: "mkv", path: "/a/Venom The Last Dance (2024).mkv"},
		{name: "Venom - The Last Dance (2024).mkv", base: "Venom - The Last Dance (2024)", ext: "mkv", path: "/b/Venom - The Last Dance (2024).mkv"},
	}
	cfg := RunConfig{
		Exact:      false,
		Threshold:  0.85,
		Metric:     textscore.MetricHybrid,
		CommonExts: nil,
	}
	clusters := FindDuplicates(files, cfg)
	if len(clusters) != 1 {
		t.Fatalf("expected fuzzy cluster for same extension, got %d clusters", len(clusters))
	}
}

func TestFuzzyVideoPresetDoesNotLinkMp4ToNfo(t *testing.T) {
	files := []fileEntry{
		{name: "Movie (2013).mp4", base: "Movie (2013)", ext: "mp4", path: "/a/Movie (2013).mp4"},
		{name: "Movie (2013).nfo", base: "Movie (2013)", ext: "nfo", path: "/b/Movie (2013).nfo"},
	}
	video, err := resolveCommonExts("video")
	if err != nil {
		t.Fatal(err)
	}
	cfg := RunConfig{
		Exact:      false,
		Threshold:  0.85,
		Metric:     textscore.MetricHybrid,
		CommonExts: video,
	}
	clusters := FindDuplicates(files, cfg)
	if len(clusters) != 0 {
		t.Fatalf("expected no fuzzy cluster between .mp4 and .nfo with video preset, got %d groups", len(clusters))
	}
}

func TestLooksLikeMistakenPathArg(t *testing.T) {
	if !looksLikeMistakenPathArg("/home/media/Movies") {
		t.Fatal("expected unix path")
	}
	if !looksLikeMistakenPathArg(`C:\Users\x`) {
		t.Fatal("expected windows path")
	}
	if !looksLikeMistakenPathArg("foo/bar") {
		t.Fatal("expected relative path with slash")
	}
	if !looksLikeMistakenPathArg(".") {
		t.Fatal("expected lone dot as mistaken cwd")
	}
	if looksLikeMistakenPathArg("video") {
		t.Fatal("preset name should not look like path")
	}
	if looksLikeMistakenPathArg("mp4,mkv") {
		t.Fatal("csv exts should not look like path")
	}
	if looksLikeMistakenPathArg("*") {
		t.Fatal("common-ext bare sentinel")
	}
	if looksLikeMistakenPathArg("hybrid") {
		t.Fatal("metric keyword")
	}
}

func mustTouch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir error for %q: %v", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create error for %q: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close error for %q: %v", path, err)
	}
}
