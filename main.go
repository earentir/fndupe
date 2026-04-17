package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/network-plane/textscore"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	flagThreshold  float64
	flagExact      bool
	flagMetric     string
	flagExcludeExt string
	flagExcludeStr string
	flagCommonExt  string
	flagHash       string
	flagNoProgress bool
	flagNoColor    bool
	flagNoColour   bool // British spelling; same as --no-color
	appVersion     = "0.1.13"
)

var rootCmd = &cobra.Command{
	Use:   "fndupe [dir]",
	Short: "Find files with duplicate or similar names recursively",
	Long: `fndupe scans a directory tree and reports files whose names are
identical or similar to each other.

Fuzzy mode (default) compares the full filename string but only between files
with the same extension, so movie.mp4 and movie.nfo are never grouped. Use bare
--exclude-ext to opt into fuzzy matching across extensions (legacy behavior).
Use --common-ext with a preset (image, video, …) or bare --common-ext for the
union of all presets: only files whose extension is in that set compare by basename;
other extensions keep full-filename comparison (so e.g. .nfo is not lumped with .mkv
unless you add nfo to a preset).

Exact mode (--exact) still groups by full name or basename per --common-ext as before.

Similarity metrics (--metric):
  hybrid      weighted average of Jaccard + Dice + Levenshtein (default)
  levenshtein normalized edit distance
  jaccard     token-set Jaccard index
  dice        token-set Sørensen-Dice coefficient

Extension presets for --common-ext:
  image   jpg, jpeg, png, gif, bmp, webp, tiff, heic, heif, avif, svg, raw, ...
  video   mp4, mkv, avi, mov, wmv, flv, webm, mpg, mpeg, m4v, ...
  audio   mp3, flac, aac, ogg, wav, m4a, wma, opus, aiff, ape, mka
  doc     pdf, doc, docx, odt, rtf, txt, md, tex, pages
  code    go, py, js, ts, java, c, cpp, h, rs, rb, php, swift, kt, cs, ...
  arch    zip, tar, gz, bz2, xz, 7z, rar, zst, lz4

Content verification (--hash):
  After name-based matching, require identical file contents. Reads are streamed in
  chunks (safe for multi-gigabyte files). Within each name group, files are bucketed
  by size first so unlike sizes are not hashed.
  Default algorithm is xxh64 (very fast). sha256 is optional and much slower — use it
  only if you want a stronger fingerprint, not for speed.

Flag order: put the directory last, or use "=" so the next token is not swallowed, e.g.
  fndupe /path/to/dir --common-ext video
  fndupe --common-ext=video /path/to/dir
If a string flag value looks like a filesystem path, fndupe errors (applies to --metric, --exclude-ext,
--exclude-str, --common-ext, --hash). (Avoid: fndupe --common-ext /path/to/dir  →  /path is parsed as the preset value, not the scan root.)

Bare --common-ext (no value) is the same as merging every preset: image ∪ video ∪ audio ∪ doc ∪ code ∪ arch.

Examples:
  fndupe
  fndupe -t 0.7 /some/path
  fndupe --exact
  fndupe --common-ext image
  fndupe --common-ext jpg,png,bmp
  fndupe --exclude-ext nfo,txt --exclude-str sample
  fndupe --exclude-ext   # allow fuzzy matches across different extensions (optional)
  fndupe --metric levenshtein -t 0.8
  fndupe --hash --exact testdata/namefixtures
  fndupe --hash sha256 .
  fndupe --common-ext=video /home/media/Video/mv/Movies
  fndupe --no-progress .`,

	Args: cobra.MaximumNArgs(1),
	RunE: runRoot,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func main() {
	Execute()
}

func init() {
	rootCmd.Version = appVersion

	rootCmd.Flags().Float64VarP(&flagThreshold, "threshold", "t", 0.85,
		"similarity threshold 0.0–1.0 (higher = stricter)")
	rootCmd.Flags().BoolVar(&flagExact, "exact", false,
		"match exact names only, no fuzzy scoring")
	rootCmd.Flags().StringVar(&flagMetric, "metric", "hybrid",
		"similarity metric: hybrid, levenshtein, jaccard, dice")
	rootCmd.Flags().StringVar(&flagExcludeExt, "exclude-ext", "",
		"comma-separated extensions to skip during the scan, e.g. nfo,txt,srt.\n"+
			"Bare --exclude-ext (no value) does not filter extensions but allows fuzzy matching across different extensions.")
	rootCmd.Flags().Lookup("exclude-ext").NoOptDefVal = "*"
	rootCmd.Flags().StringVar(&flagExcludeStr, "exclude-str", "",
		"comma-separated substrings — files whose names contain any of these are ignored")
	rootCmd.Flags().StringVar(&flagCommonExt, "common-ext", "",
		"preset (image/video/audio/doc/code/arch), comma-separated extensions, or bare --common-ext\n"+
			"for union of all presets. Basename comparison applies only to files whose extension is in that set.")
	rootCmd.Flags().Lookup("common-ext").NoOptDefVal = "*"

	rootCmd.Flags().StringVar(&flagHash, "hash", "",
		"after name matching, require same file contents (streaming). "+
			"Optional value: xxh64 (default with bare --hash) or sha256 (slower, stronger fingerprint)")
	rootCmd.Flags().Lookup("hash").NoOptDefVal = "xxh64"

	rootCmd.Flags().BoolVar(&flagNoProgress, "no-progress", false,
		"disable progress bars and ETA (useful when stderr is not a TTY or for scripts)")
	rootCmd.Flags().BoolVar(&flagNoColor, "no-color", false,
		"disable ANSI colors in duplicate listing (also respects NO_COLOR=1)")
	rootCmd.Flags().BoolVar(&flagNoColour, "no-colour", false,
		"same as --no-color")
}

func runRoot(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	if err := validateFlagsAgainstMistakenPath(); err != nil {
		return err
	}

	var hashAlgo hashAlgo
	if flagHash != "" {
		var err error
		hashAlgo, err = resolveHashAlgo(flagHash)
		if err != nil {
			return err
		}
	}

	metric, err := resolveMetric(flagMetric)
	if err != nil {
		return err
	}

	// Resolve --exclude-ext. Bare --exclude-ext → NoOptDefVal "*": cross-extension fuzzy, no scan filter.
	var excludeExts map[string]struct{}
	crossExtFuzzy := false
	if flagExcludeExt == "*" {
		excludeExts = parseCSVSet("")
		crossExtFuzzy = true
	} else {
		excludeExts = parseCSVSet(flagExcludeExt)
	}

	// Resolve --exclude-str
	excludeStrs := parseCSVList(flagExcludeStr)

	// Resolve --common-ext.
	// A bare "--common-ext" (no value) uses NoOptDefVal="*" → union of all ExtPresets.
	var commonExts map[string]struct{}
	if flagCommonExt == "*" {
		commonExts = UnionAllPresetExtensions()
	} else {
		commonExts, err = resolveCommonExts(flagCommonExt)
		if err != nil {
			return err
		}
	}

	printRunSummary(root, commonExts)

	cfg := RunConfig{
		Root:          root,
		Threshold:     flagThreshold,
		Exact:         flagExact,
		Metric:        metric,
		ExcludeExts:   excludeExts,
		ExcludeStrs:   excludeStrs,
		CommonExts:    commonExts,
		CrossExtFuzzy: crossExtFuzzy,
	}

	var scanBar *progressbar.ProgressBar
	if useProgress() {
		scanBar = newScanBar()
		cfg.OnScanTick = func() { _ = scanBar.Add(1) }
	}
	files, err := ScanFiles(cfg)
	cfg.OnScanTick = nil
	if scanBar != nil {
		_ = scanBar.Finish()
	}
	if err != nil {
		return fmt.Errorf("scan error: %w", err)
	}

	var cmpBar *progressbar.ProgressBar
	if useProgress() && !cfg.Exact && len(files) > 1 {
		totalPairs := int64(len(files)) * int64(len(files)-1) / 2
		if totalPairs > 0 {
			cmpBar = newCompareBar(totalPairs)
			cfg.OnCompareStep = func(done, total int64) {
				_ = cmpBar.Set64(done)
			}
		}
	}
	clusters := FindDuplicates(files, cfg)
	cfg.OnCompareStep = nil
	if cmpBar != nil {
		_ = cmpBar.Finish()
	}

	if len(clusters) == 0 {
		if flagExact {
			fmt.Println("No exact duplicate names found.")
		} else {
			fmt.Printf("No similar names found (threshold: %.2f, metric: %s).\n", flagThreshold, flagMetric)
		}
		return nil
	}

	if flagHash != "" {
		var hBar *progressbar.ProgressBar
		if useProgress() {
			if tot := countRefineHashOperations(clusters); tot > 0 {
				hBar = newHashBar(tot, hashAlgoDisplayName(hashAlgo))
			}
		}
		refined, err := refineClustersByHash(clusters, hashAlgo, commonExts, func(done, total int64) {
			if hBar != nil {
				_ = hBar.Set64(done)
			}
		})
		if hBar != nil {
			_ = hBar.Finish()
		}
		if err != nil {
			return err
		}
		algoLabel := hashAlgoDisplayName(hashAlgo)
		if len(refined) == 0 {
			fmt.Printf("No groups left after content check (%s).\n", algoLabel)
			return nil
		}
		fmt.Printf("Found %d group(s) with matching names and identical content (hash: %s):\n",
			len(refined), algoLabel)
		fuzzyOut := !flagExact
		for _, g := range refined {
			fmt.Printf("\n  [%s]  %s:%s\n", formatClusterHeaderLabel(g.nameKey, g.files[0].path), algoLabel, g.digest)
			for i := range g.files {
				fmt.Printf("    %s\n", formatClusterPathLine(cyclicNeighborStem(g.files, i), g.files[i].path, fuzzyOut))
			}
		}
		return nil
	}

	if flagExact {
		fmt.Printf("Found %d group(s) with exact duplicate names:\n", len(clusters))
	} else {
		fmt.Printf("Found %d group(s) of similar names (threshold: %.2f, metric: %s):\n",
			len(clusters), flagThreshold, flagMetric)
	}

	fuzzyOut := !flagExact
	for _, group := range clusters {
		label := matchKey(group[0], commonExts)
		fmt.Printf("\n  [%s]\n", formatClusterHeaderLabel(label, group[0].path))
		for i := range group {
			fmt.Printf("    %s\n", formatClusterPathLine(cyclicNeighborStem(group, i), group[i].path, fuzzyOut))
		}
	}

	return nil
}

func printRunSummary(root string, commonExts map[string]struct{}) {
	fmt.Printf("fndupe %s\n\n", appVersion)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "option\tvalue")
	fmt.Fprintf(w, "root\t%s\n", root)
	fmt.Fprintf(w, "threshold\t%g\n", flagThreshold)
	fmt.Fprintf(w, "exact\t%v\n", flagExact)
	fmt.Fprintf(w, "metric\t%s\n", flagMetric)
	fmt.Fprintf(w, "exclude-ext\t%s\n", describeExcludeExtFlag(flagExcludeExt))
	fmt.Fprintf(w, "exclude-str\t%s\n", displayStringFlag(flagExcludeStr))
	fmt.Fprintf(w, "common-ext\t%s\n", describeCommonExtSetting(flagCommonExt, commonExts))
	fmt.Fprintf(w, "hash\t%s\n", displayHashFlag())
	fmt.Fprintf(w, "progress\t%s\n", progressStatusLine())
	fmt.Fprintf(w, "color\t%s\n", colorSummary())
	w.Flush()
	fmt.Println()
}

func displayStringFlag(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

func describeExcludeExtFlag(raw string) string {
	switch raw {
	case "":
		return "(empty)"
	case "*":
		return "* (fuzzy may match across extensions; no scan filter)"
	default:
		return raw
	}
}

func displayHashFlag() string {
	if flagHash == "" {
		return "off"
	}
	return flagHash
}

func describeCommonExtSetting(raw string, resolved map[string]struct{}) string {
	switch raw {
	case "":
		return "(none — compare full filename)"
	case "*":
		return "* (union of all presets: image ∪ video ∪ audio ∪ doc ∪ code ∪ arch)"
	default:
		if resolved == nil {
			return raw
		}
		if len(resolved) == 0 {
			return raw
		}
		return raw + " (basename only for files whose extension is in this set)"
	}
}

// validateFlagsAgainstMistakenPath rejects string flag values that look like a directory/file path.
// Cobra binds the next argv token to the flag, so users often accidentally pass [dir] as the flag value.
func validateFlagsAgainstMistakenPath() error {
	checks := []struct {
		name string
		val  string
	}{
		{"--metric", flagMetric},
		{"--exclude-ext", flagExcludeExt},
		{"--exclude-str", flagExcludeStr},
		{"--common-ext", flagCommonExt},
		{"--hash", flagHash},
	}
	for _, c := range checks {
		if c.val == "" {
			continue
		}
		if (c.name == "--common-ext" || c.name == "--exclude-ext") && c.val == "*" {
			continue
		}
		if looksLikeMistakenPathArg(c.val) {
			return fmt.Errorf(
				"%s value %q looks like a filesystem path.\n"+
					"If that was meant to be the scan directory, put [dir] last or use flag=value, e.g.:\n"+
					"  %s --metric=hybrid /path/to/dir\n"+
					"  %s /path/to/dir --common-ext video",
				c.name, c.val, os.Args[0], os.Args[0])
		}
	}
	return nil
}

// looksLikeMistakenPathArg reports whether a flag value is almost certainly a path meant as [dir].
// Comma-separated flags are checked per segment (e.g. exclude-ext=nfo,txt).
func looksLikeMistakenPathArg(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "*" {
		return false
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if segmentLooksLikeMistakenPath(part) {
			return true
		}
	}
	return false
}

func segmentLooksLikeMistakenPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if isKnownNonPathFlagToken(p) {
		return false
	}
	if filepath.IsAbs(p) {
		return true
	}
	if p == "." || p == ".." {
		return true
	}
	if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		return true
	}
	if strings.HasPrefix(p, "~") {
		return true
	}
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	if strings.Contains(p, `\`) {
		return true
	}
	if len(p) >= 3 && isWindowsDrivePathPrefix(p) {
		return true
	}
	// Relative path with a slash: foo/bar (scan root was swallowed as a flag value)
	if strings.Contains(p, "/") {
		return true
	}
	return false
}

func isKnownNonPathFlagToken(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hybrid", "levenshtein", "jaccard", "dice":
		return true
	case "xxh64", "xxhash", "sha256":
		return true
	case "image", "video", "audio", "doc", "code", "arch":
		return true
	default:
		return false
	}
}

func isWindowsDrivePathPrefix(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
		return false
	}
	if s[1] != ':' {
		return false
	}
	return s[2] == '/' || s[2] == '\\'
}

// resolveMetric maps the --metric flag string to a textscore.Metric constant.
func resolveMetric(s string) (textscore.Metric, error) {
	switch strings.ToLower(s) {
	case "hybrid":
		return textscore.MetricHybrid, nil
	case "levenshtein":
		return textscore.MetricLevenshtein, nil
	case "jaccard":
		return textscore.MetricJaccard, nil
	case "dice":
		return textscore.MetricDice, nil
	default:
		return "", fmt.Errorf("unknown metric %q — valid values: hybrid, levenshtein, jaccard, dice", s)
	}
}

// resolveCommonExts resolves --common-ext to a set of lowercase extensions.
// Accepts a preset name OR a comma-separated list of extensions.
func resolveCommonExts(s string) (map[string]struct{}, error) {
	if s == "" {
		return nil, nil
	}

	// Check if it's a preset name
	if preset, ok := ExtPresets[strings.ToLower(s)]; ok {
		return toSet(preset), nil
	}

	// Otherwise treat as comma-separated extension list
	exts := parseCSVList(s)
	if len(exts) == 0 {
		return nil, fmt.Errorf("--common-ext %q is not a known preset and contains no extensions", s)
	}
	return toSet(exts), nil
}

// parseCSVSet splits a comma-separated string into a lowercase set, ignoring empty entries.
func parseCSVSet(s string) map[string]struct{} {
	return toSet(parseCSVList(s))
}

// parseCSVList splits a comma-separated string into a slice of trimmed lowercase non-empty strings.
func parseCSVList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		// Strip a leading dot if someone writes ".jpg" instead of "jpg"
		part = strings.TrimPrefix(part, ".")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func toSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, v := range items {
		m[v] = struct{}{}
	}
	return m
}
