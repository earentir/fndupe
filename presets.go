package main

// ExtPreset defines a named group of extensions to treat as the same "kind".
// When --common-ext is set to a preset name, these extensions are used.
// The tool considers two files a match when their base names are similar
// AND both extensions appear in the same preset group.
//
// Bare --common-ext (NoOptDefVal "*") uses UnionAllPresetExtensions: every built-in
// preset merged into one extension set (still excludes types like .nfo unless listed).
var ExtPresets = map[string][]string{
	"image": {"jpg", "jpeg", "png", "gif", "bmp", "webp", "tiff", "tif", "heic", "heif", "avif", "svg", "raw", "cr2", "nef", "arw"},
	"video": {"mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "mpg", "mpeg", "m4v", "3gp", "ts", "m2ts", "vob", "ogv", "divx", "xvid"},
	"audio": {"mp3", "flac", "aac", "ogg", "wav", "m4a", "wma", "opus", "aiff", "ape", "mka"},
	"doc":   {"pdf", "doc", "docx", "odt", "rtf", "txt", "md", "tex", "pages"},
	"code":  {"go", "py", "js", "ts", "java", "c", "cpp", "h", "hpp", "rs", "rb", "php", "swift", "kt", "cs", "scala"},
	"arch":  {"zip", "tar", "gz", "bz2", "xz", "7z", "rar", "zst", "lz4"},
}

// UnionAllPresetExtensions returns the union of extensions from every entry in ExtPresets.
// Used when --common-ext is given with no value (same idea as “all groups”, not “every ext on earth”).
func UnionAllPresetExtensions() map[string]struct{} {
	m := make(map[string]struct{})
	for _, exts := range ExtPresets {
		for _, e := range exts {
			m[e] = struct{}{}
		}
	}
	return m
}
