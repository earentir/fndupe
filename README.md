# fndupe

`fndupe` walks a directory tree and reports files whose names are **identical** or **fuzzily similar**. It compares names within the same extension by default (so sidecars like `.nfo` are not lumped with `.mkv` unless you opt in). Optional **content hashing** can require byte-identical files inside each name-based group.

Built with Go 1.26+. From the repository root:

```bash
go build -o fndupe .
./fndupe --help
go test ./...
```

All examples below assume your current working directory is the **repository root**, so paths like `testdata/namefixtures/...` resolve correctly. That tree is small, deterministic, and meant for trying every mode.

---

## Examples

The `testdata/namefixtures` layout:

| Subtree | Purpose |
| --- | --- |
| `similar/` | Fuzzy near-duplicate names (same extension) |
| `exact/` | Same basename in different folders (good for `--exact`) |
| `common-ext/{image,video,audio,doc,code,arch}/` | One scenario per `--common-ext` preset |
| `exclude/` | Files useful for `--exclude-ext` / `--exclude-str` |
| `mixed/` | Odd names (case, dots, no extension, hidden) |

### Meta (no scan)

```bash
fndupe -h
fndupe --help
fndupe -v
fndupe --version
```

### Scan root

```bash
fndupe
fndupe .
fndupe testdata/namefixtures
```

### Similarity threshold (`-t` / `--threshold`)

```bash
fndupe -t 0.7 testdata/namefixtures/similar
fndupe --threshold 0.9 testdata/namefixtures/similar
```

### Exact filename match (no fuzzy scoring)

```bash
fndupe --exact testdata/namefixtures/exact
```

### Similarity metric (`--metric`)

```bash
fndupe --metric hybrid testdata/namefixtures/similar
fndupe --metric levenshtein testdata/namefixtures/similar
fndupe --metric jaccard testdata/namefixtures/similar
fndupe --metric dice testdata/namefixtures/similar
```

### Skip extensions during scan (`--exclude-ext`)

Comma-separated list (no leading dots required):

```bash
fndupe --exclude-ext nfo,srt testdata/namefixtures/exclude
```

Bare flag: do **not** filter extensions, but allow **fuzzy matching across different extensions** (legacy behavior):

```bash
fndupe --exclude-ext testdata/namefixtures
```

### Skip filenames containing substrings (`--exclude-str`)

```bash
fndupe --exclude-str sample testdata/namefixtures/exclude
fndupe --exclude-str sample,srt testdata/namefixtures/exclude
```

### Compare basename only for selected types (`--common-ext`)

**Preset** (built-in extension sets — see `--help` for the full lists):

```bash
fndupe --common-ext image testdata/namefixtures/common-ext/image
fndupe --common-ext video testdata/namefixtures/common-ext/video
fndupe --common-ext audio testdata/namefixtures/common-ext/audio
fndupe --common-ext doc testdata/namefixtures/common-ext/doc
fndupe --common-ext code testdata/namefixtures/common-ext/code
fndupe --common-ext arch testdata/namefixtures/common-ext/arch
```

**Custom extension list**:

```bash
fndupe --common-ext jpg,png,bmp testdata/namefixtures/common-ext/image
```

**Union of every preset** (bare `--common-ext`):

```bash
fndupe testdata/namefixtures --common-ext
```

If the next token after `--common-ext` would be parsed as the preset value, put the directory first (as above) or use `=`:

```bash
fndupe --common-ext=video testdata/namefixtures/common-ext/video
```

### Content verification after name matching (`--hash`)

Streaming hash; default algorithm is **xxh64** when `--hash` is used without a value.

`--hash` does **not** require `--exact`. Without `--exact`, name groups come from **fuzzy** matching first; the hash step then drops files that do not share identical contents. If fuzzy matching finds no groups, nothing is hashed (there is no separate “hash-only” pass).

```bash
fndupe --hash --exact testdata/namefixtures/exact
fndupe --hash testdata/namefixtures/exact
fndupe --hash --no-progress testdata/namefixtures
fndupe --hash xxh64 --exact testdata/namefixtures/exact
fndupe --hash xxhash --exact testdata/namefixtures/exact
fndupe --hash sha256 --exact testdata/namefixtures/exact
fndupe --hash=xxh64 --exact testdata/namefixtures/exact
```

### Progress and color

```bash
fndupe --no-progress testdata/namefixtures
fndupe --no-color testdata/namefixtures/similar
fndupe --no-colour testdata/namefixtures/similar
```

Color is also off when `NO_COLOR` is set in the environment (non-empty).

### Combined runs (realistic)

```bash
fndupe --exact --hash --no-progress testdata/namefixtures/exact
fndupe --common-ext doc -t 0.75 --metric levenshtein testdata/namefixtures/common-ext/doc
fndupe --exclude-ext nfo --exclude-str sample --no-progress testdata/namefixtures/exclude
```

### Flag order pitfall

String flags (`--metric`, `--exclude-ext`, `--exclude-str`, `--common-ext`, `--hash`) must not receive your scan path by mistake. Wrong:

```bash
# BAD: /path is parsed as --common-ext value, not the scan root
fndupe --common-ext testdata/namefixtures/common-ext/video
```

Right:

```bash
fndupe testdata/namefixtures/common-ext/video --common-ext video
fndupe --common-ext=video testdata/namefixtures/common-ext/video
```

If a flag value looks like a filesystem path, `fndupe` exits with an error explaining to use `flag=value` or put `[dir]` last.
