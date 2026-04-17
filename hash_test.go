package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHashAlgo(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want hashAlgo
	}{
		{"xxh64", hashAlgoXXH64},
		{"XXHASH", hashAlgoXXH64},
		{"sha256", hashAlgoSHA256},
	} {
		got, err := resolveHashAlgo(tc.in)
		if err != nil {
			t.Fatalf("resolveHashAlgo(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("resolveHashAlgo(%q) = %v want %v", tc.in, got, tc.want)
		}
	}
	if _, err := resolveHashAlgo(""); err == nil {
		t.Fatal("expected error for empty algorithm string")
	}
	if _, err := resolveHashAlgo("nope"); err == nil {
		t.Fatal("expected error for unknown algorithm")
	}
}

func TestHashFileXXH64Known(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "blob")
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := hashFile(p, hashAlgoXXH64)
	if err != nil {
		t.Fatal(err)
	}
	// Deterministic cross-check: same bytes hashed again must match.
	got2, err := hashFile(p, hashAlgoXXH64)
	if err != nil {
		t.Fatal(err)
	}
	if got != got2 {
		t.Fatalf("digest mismatch: %q vs %q", got, got2)
	}
}

func TestFindHashDuplicates(t *testing.T) {
	dir := t.TempDir()
	content := []byte("shared-bytes")
	mustWrite(t, filepath.Join(dir, "a", "one.bin"), content)
	mustWrite(t, filepath.Join(dir, "b", "two.bin"), content)
	mustWrite(t, filepath.Join(dir, "c", "other.txt"), []byte("unique"))

	cfg := RunConfig{Root: dir}
	files, err := ScanFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := FindHashDuplicates(files, hashAlgoXXH64)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("want 1 duplicate group, got %d", len(groups))
	}
	if len(groups[0].files) != 2 {
		t.Fatalf("want 2 files in group, got %d", len(groups[0].files))
	}
}

func TestFindHashDuplicatesSizeSkipsRead(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "small-a.txt"), []byte("x"))
	mustWrite(t, filepath.Join(dir, "small-b.txt"), []byte("y"))
	// Same size (1 byte) but different content — should not cluster as dupes.
	cfg := RunConfig{Root: dir}
	files, err := ScanFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := FindHashDuplicates(files, hashAlgoXXH64)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("want no groups, got %d", len(groups))
	}
}

func TestRefineClustersByHash(t *testing.T) {
	dir := t.TempDir()
	data := []byte("same")
	mustWrite(t, filepath.Join(dir, "a", "dup.txt"), data)
	mustWrite(t, filepath.Join(dir, "b", "dup.txt"), data)
	mustWrite(t, filepath.Join(dir, "c", "other.txt"), []byte("other"))

	cfg := RunConfig{Root: dir, Exact: true}
	files, err := ScanFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	clusters := FindDuplicates(files, cfg)
	if len(clusters) != 1 {
		t.Fatalf("want 1 exact-name cluster, got %d", len(clusters))
	}

	refined, err := refineClustersByHash(clusters, hashAlgoXXH64, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(refined) != 1 || len(refined[0].files) != 2 {
		t.Fatalf("want 1 hash group of 2 files, got %+v", refined)
	}

	refined256, err := refineClustersByHash(clusters, hashAlgoSHA256, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(refined256) != 1 {
		t.Fatalf("sha256 refine: want 1 group, got %d", len(refined256))
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
