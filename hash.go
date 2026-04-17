package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// hashReadChunkSize is the buffer size for streaming reads. Memory stays bounded
// regardless of file size (multi-gigabyte safe).
const hashReadChunkSize = 8 << 20 // 8 MiB

type hashAlgo int

const (
	hashAlgoXXH64 hashAlgo = iota
	hashAlgoSHA256
)

func resolveHashAlgo(s string) (hashAlgo, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "xxh64", "xxhash":
		return hashAlgoXXH64, nil
	case "sha256":
		return hashAlgoSHA256, nil
	default:
		return 0, fmt.Errorf("unknown hash algorithm %q — valid: xxh64, sha256", s)
	}
}

func hashAlgoDisplayName(a hashAlgo) string {
	switch a {
	case hashAlgoXXH64:
		return "xxh64"
	case hashAlgoSHA256:
		return "sha256"
	default:
		return "?"
	}
}

// nameHashCluster is a name-matching group further split by identical content.
type nameHashCluster struct {
	nameKey string
	digest  string
	files   []fileEntry
}

// fileEntriesBySize groups entries by file size (bytes).
func fileEntriesBySize(entries []fileEntry) map[int64][]fileEntry {
	m := make(map[int64][]fileEntry)
	for _, f := range entries {
		m[f.size] = append(m[f.size], f)
	}
	return m
}

// countRefineHashOperations returns how many hashFile calls refineClustersByHash will perform.
func countRefineHashOperations(clusters [][]fileEntry) int64 {
	var n int64
	for _, cluster := range clusters {
		if len(cluster) < 2 {
			continue
		}
		for _, sameSize := range fileEntriesBySize(cluster) {
			if len(sameSize) >= 2 {
				n += int64(len(sameSize))
			}
		}
	}
	return n
}

// refineClustersByHash keeps only files that share the same name-based key AND the same
// streamed digest. Buckets by size within each name cluster so unlike sizes are not read.
// onHash is optional; when non-nil it receives (done, total) after each successful hash (done in [1,total]).
func refineClustersByHash(clusters [][]fileEntry, algo hashAlgo, commonExts map[string]struct{}, onHash func(done, total int64)) ([]nameHashCluster, error) {
	totalHashes := countRefineHashOperations(clusters)
	var doneHashes int64

	var out []nameHashCluster
	for _, cluster := range clusters {
		if len(cluster) < 2 {
			continue
		}
		nameKey := matchKey(cluster[0], commonExts)

		for _, sameSize := range fileEntriesBySize(cluster) {
			if len(sameSize) < 2 {
				continue
			}
			byDigest := make(map[string][]fileEntry)
			for _, f := range sameSize {
				digest, err := hashFile(f.path, algo)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", f.path, err)
				}
				doneHashes++
				if onHash != nil && totalHashes > 0 {
					onHash(doneHashes, totalHashes)
				}
				byDigest[digest] = append(byDigest[digest], f)
			}
			for digest, files := range byDigest {
				if len(files) < 2 {
					continue
				}
				sorted := make([]fileEntry, len(files))
				copy(sorted, files)
				sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
				out = append(out, nameHashCluster{nameKey: nameKey, digest: digest, files: sorted})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].nameKey != out[j].nameKey {
			return out[i].nameKey < out[j].nameKey
		}
		if out[i].digest != out[j].digest {
			return out[i].digest < out[j].digest
		}
		return out[i].files[0].path < out[j].files[0].path
	})
	return out, nil
}

// hashFile streams file content through the hasher; the full file is never loaded into memory.
func hashFile(path string, algo hashAlgo) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, hashReadChunkSize)

	switch algo {
	case hashAlgoXXH64:
		h := xxhash.New()
		if _, err := io.CopyBuffer(h, f, buf); err != nil {
			return "", err
		}
		return fmt.Sprintf("%016x", h.Sum64()), nil

	case hashAlgoSHA256:
		s := sha256.New()
		if _, err := io.CopyBuffer(s, f, buf); err != nil {
			return "", err
		}
		return hex.EncodeToString(s.Sum(nil)), nil

	default:
		return "", fmt.Errorf("internal: unknown hash algorithm %d", algo)
	}
}

// hashDupGroup is one cluster of byte-identical files (same digest).
type hashDupGroup struct {
	digest string
	files  []fileEntry
}

// FindHashDuplicates groups files with identical content.
// It first buckets by file size (no reads for unique sizes), then hashes remaining files
// using a streaming digest so large files are safe.
func FindHashDuplicates(files []fileEntry, algo hashAlgo) ([]hashDupGroup, error) {
	bySize := make(map[int64][]fileEntry)
	for _, f := range files {
		bySize[f.size] = append(bySize[f.size], f)
	}

	var out []hashDupGroup
	for _, group := range bySize {
		if len(group) < 2 {
			continue
		}
		byDigest := make(map[string][]fileEntry)
		for _, f := range group {
			digest, err := hashFile(f.path, algo)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.path, err)
			}
			byDigest[digest] = append(byDigest[digest], f)
		}
		for digest, g := range byDigest {
			if len(g) < 2 {
				continue
			}
			sorted := make([]fileEntry, len(g))
			copy(sorted, g)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
			out = append(out, hashDupGroup{digest: digest, files: sorted})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].files[0].path < out[j].files[0].path
	})
	return out, nil
}
