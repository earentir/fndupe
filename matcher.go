package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/network-plane/textscore"
)

// fileEntry holds a discovered file's metadata.
type fileEntry struct {
	name    string // full filename including extension e.g. "photo.jpg"
	base    string // name without extension e.g. "photo"
	ext     string // lowercased extension without dot e.g. "jpg"
	path    string // full path
	size    int64  // file size in bytes (used for hash-mode bucketing)
}

// RunConfig holds all the resolved options for a scan.
type RunConfig struct {
	Root        string
	Threshold   float64
	Exact       bool
	Metric      textscore.Metric
	ExcludeExts map[string]struct{} // lowercased, no dot
	ExcludeStrs []string            // substrings to exclude (case-insensitive)
	CommonExts  map[string]struct{} // when set, compare only base name (ext-agnostic within group)
	// CrossExtFuzzy (bare --exclude-ext): when CommonExts is nil, allow fuzzy pairs with different extensions (legacy).
	CrossExtFuzzy bool

	// Optional progress callbacks (stderr bars); nil when disabled.
	OnScanTick    func()
	OnCompareStep func(done, total int64) // fuzzy mode only; done in [1,total]
}

// ScanFiles walks the root directory and returns all matching file entries.
func ScanFiles(cfg RunConfig) ([]fileEntry, error) {
	var files []fileEntry

	err := filepath.WalkDir(cfg.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil // skip unreadable entries
		}

		name := d.Name()
		rawExt := filepath.Ext(name)
		ext := strings.ToLower(strings.TrimPrefix(rawExt, "."))
		base := strings.TrimSuffix(name, rawExt)

		// Apply --exclude-ext filter
		if _, excluded := cfg.ExcludeExts[ext]; excluded {
			return nil
		}

		// Apply --exclude-str filter (case-insensitive match against full name)
		nameLower := strings.ToLower(name)
		for _, s := range cfg.ExcludeStrs {
			if strings.Contains(nameLower, strings.ToLower(s)) {
				return nil
			}
		}

		files = append(files, fileEntry{
			name: name,
			base: base,
			ext:  ext,
			path: path,
			size: info.Size(),
		})
		if cfg.OnScanTick != nil {
			cfg.OnScanTick()
		}
		return nil
	})

	return files, err
}

// matchKey returns the string used for similarity comparison.
// When CommonExts is non-nil and the file's extension is in the set,
// we compare only the base name (so photo.jpg vs photo.png match when both are in-set).
// Otherwise we compare the full filename (so test123.jpg != test123.png, and sidecars
// like .nfo stay on full-name keys when not listed in the set).
func matchKey(f fileEntry, commonExts map[string]struct{}) string {
	if commonExts == nil {
		return f.name
	}

	if len(commonExts) > 0 {
		if _, ok := commonExts[f.ext]; ok {
			return f.base
		}
	}
	return f.name
}

// fuzzyPairAllowed returns whether two files may be compared in fuzzy mode.
// When CommonExts is nil, pairs must share the same extension unless CrossExtFuzzy is set (bare --exclude-ext).
// When CommonExts is non-nil, we only link pairs where both files are in the preset, or both are outside it.
// That stops e.g. movie.mp4 (basename key) from fuzzy-matching movie.nfo (full-name key).
func fuzzyPairAllowed(a, b fileEntry, commonExts map[string]struct{}, crossExtFuzzy bool) bool {
	if commonExts == nil {
		if crossExtFuzzy {
			return true
		}
		return a.ext == b.ext
	}
	_, aIn := commonExts[a.ext]
	_, bIn := commonExts[b.ext]
	if aIn && bIn {
		return true
	}
	if !aIn && !bIn {
		return true // neither in preset: both compared by full filename
	}
	return false
}

// FindDuplicates returns clusters of files with duplicate or similar names.
func FindDuplicates(files []fileEntry, cfg RunConfig) [][]fileEntry {
	if cfg.Exact {
		return findExact(files, cfg)
	}
	return findFuzzy(files, cfg)
}

// findExact groups files by their match key (full name or base name).
func findExact(files []fileEntry, cfg RunConfig) [][]fileEntry {
	groups := make(map[string][]fileEntry)
	for _, f := range files {
		key := matchKey(f, cfg.CommonExts)
		groups[key] = append(groups[key], f)
	}

	var result [][]fileEntry
	for _, members := range groups {
		if len(members) > 1 {
			sorted := make([]fileEntry, len(members))
			copy(sorted, members)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
			result = append(result, sorted)
		}
	}

	// Sort clusters by first path for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return result[i][0].path < result[j][0].path
	})
	return result
}

// findFuzzy clusters files whose match keys are above the similarity threshold.
// Uses union-find so transitive matches end up in the same group.
func findFuzzy(files []fileEntry, cfg RunConfig) [][]fileEntry {
	n := len(files)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}

	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		pa, pb := find(a), find(b)
		if pa != pb {
			parent[pa] = pb
		}
	}

	opts := textscore.Options{
		Normalize:     true,
		CaseFold:      true,
		StripPunct:    false,
		CollapseSpace: true,
	}

	totalPairs := int64(n) * int64(n-1) / 2
	var donePairs int64

	for i := 0; i < n; i++ {
		ki := matchKey(files[i], cfg.CommonExts)
		for j := i + 1; j < n; j++ {
			donePairs++
			if cfg.OnCompareStep != nil {
				cfg.OnCompareStep(donePairs, totalPairs)
			}
			if !fuzzyPairAllowed(files[i], files[j], cfg.CommonExts, cfg.CrossExtFuzzy) {
				continue
			}
			kj := matchKey(files[j], cfg.CommonExts)
			score := textscore.Similarity(ki, kj, cfg.Metric, opts)
			if score >= cfg.Threshold {
				union(i, j)
			}
		}
	}

	// Collect clusters
	clusters := make(map[int][]fileEntry)
	for i, f := range files {
		root := find(i)
		clusters[root] = append(clusters[root], f)
	}

	var result [][]fileEntry
	for _, members := range clusters {
		if len(members) > 1 {
			sorted := make([]fileEntry, len(members))
			copy(sorted, members)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
			result = append(result, sorted)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i][0].path < result[j][0].path
	})
	return result
}
