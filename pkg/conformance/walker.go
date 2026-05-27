package conformance

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// defaultAllowGlobs lists path patterns the linter never inspects. These cover
// vendored code, build output, dependency caches, and well-known ignore-paths.
var defaultAllowGlobs = []string{
	"node_modules/**",
	"vendor/**",
	"dist/**",
	"build/**",
	".git/**",
	"**/*.min.js",
	"**/*.min.css",
}

// fileEntry holds one source file's path and lazily-loaded contents.
type fileEntry struct {
	path     string // path relative to the repo root
	abs      string // absolute path on disk
	contents []byte // populated on first read
	lines    []string
}

// repoWalker materializes every relevant source file in a surface repo,
// classified by file extension. Tests use the same walker, so heavy
// reads happen at most once per repo.
type repoWalker struct {
	root   string
	allows []string
	all    []*fileEntry

	// Buckets, keyed by relevance.
	goFiles      []*fileEntry // *.go (excluding *_test.go)
	goTestFiles  []*fileEntry // *_test.go
	tsFiles      []*fileEntry // *.ts, *.tsx (excluding *.test.*)
	tsTestFiles  []*fileEntry // *.test.ts, *.test.tsx, *.spec.*
	jsonFiles    []*fileEntry // *.json
	manifestYAML []*fileEntry // surface.manifest.json + *.manifest.yaml/yml
	dockerFiles  []*fileEntry // Dockerfile, Dockerfile.*
	yamlFiles    []*fileEntry // *.yaml, *.yml
	cfgFiles     []*fileEntry // package.json, vite.config.ts, etc.
}

func newRepoWalker(root string, allows []string) (*repoWalker, error) {
	w := &repoWalker{root: root, allows: allows}
	if err := w.walk(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *repoWalker) walk() error {
	return filepath.WalkDir(w.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(w.root, p)
		if rel == "." || rel == "" {
			return nil
		}
		if d.IsDir() {
			if w.skipDir(rel) {
				return fs.SkipDir
			}
			return nil
		}
		if w.skipFile(rel) {
			return nil
		}
		entry := &fileEntry{path: rel, abs: p}
		w.all = append(w.all, entry)
		w.classify(entry)
		return nil
	})
}

func (w *repoWalker) skipDir(rel string) bool {
	name := filepath.Base(rel)
	skipNames := map[string]struct{}{
		"node_modules": {}, "vendor": {}, "dist": {}, "build": {},
		".git": {}, ".idea": {}, ".vscode": {}, ".turbo": {}, ".next": {},
		"coverage": {}, "out": {}, "tmp": {}, "testdata": {},
		// The reusable workflow clones surface-conformance into this dir;
		// belt-and-suspenders so the linter never scans its own source.
		".surface-conformance": {}, "surface-conformance-tool": {},
	}
	if _, hit := skipNames[name]; hit {
		return true
	}
	for _, glob := range w.allows {
		if matchGlob(glob, rel) {
			return true
		}
	}
	return false
}

func (w *repoWalker) skipFile(rel string) bool {
	for _, glob := range w.allows {
		if matchGlob(glob, rel) {
			return true
		}
	}
	return false
}

func (w *repoWalker) classify(entry *fileEntry) {
	base := filepath.Base(entry.path)
	switch {
	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
		w.dockerFiles = append(w.dockerFiles, entry)
	}
	ext := strings.ToLower(filepath.Ext(entry.path))
	switch ext {
	case ".go":
		if strings.HasSuffix(entry.path, "_test.go") {
			w.goTestFiles = append(w.goTestFiles, entry)
		} else {
			w.goFiles = append(w.goFiles, entry)
		}
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		lower := strings.ToLower(base)
		if strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") {
			w.tsTestFiles = append(w.tsTestFiles, entry)
		} else {
			w.tsFiles = append(w.tsFiles, entry)
		}
	case ".json":
		w.jsonFiles = append(w.jsonFiles, entry)
		if strings.HasSuffix(base, "manifest.json") || base == "surface.manifest.json" {
			w.manifestYAML = append(w.manifestYAML, entry)
		}
		if base == "package.json" || strings.HasSuffix(base, ".config.json") {
			w.cfgFiles = append(w.cfgFiles, entry)
		}
	case ".yaml", ".yml":
		w.yamlFiles = append(w.yamlFiles, entry)
		if strings.HasSuffix(base, "manifest.yaml") || strings.HasSuffix(base, "manifest.yml") {
			w.manifestYAML = append(w.manifestYAML, entry)
		}
	}
}

// read loads the file's contents lazily and caches them.
func (e *fileEntry) read() ([]byte, error) {
	if e.contents != nil {
		return e.contents, nil
	}
	data, err := os.ReadFile(e.abs)
	if err != nil {
		return nil, err
	}
	e.contents = data
	return data, nil
}

// linesOf returns the file's contents split by '\n'. Useful for line-number
// reporting in findings.
func (e *fileEntry) linesOf() ([]string, error) {
	if e.lines != nil {
		return e.lines, nil
	}
	data, err := e.read()
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var out []string
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	e.lines = out
	return out, nil
}

// matchGlob implements a tiny glob with `**/` (any depth) and `*` (any segment
// of one directory) support. Enough for our default allow patterns.
func matchGlob(glob, path string) bool {
	path = filepath.ToSlash(path)
	glob = filepath.ToSlash(glob)
	if strings.HasPrefix(glob, "**/") {
		return matchGlobBody(glob[3:], path) || strings.Contains(path, "/") && matchGlob(glob, lastSegmentDrop(path))
	}
	return matchGlobBody(glob, path)
}

func lastSegmentDrop(p string) string {
	idx := strings.Index(p, "/")
	if idx < 0 {
		return ""
	}
	return p[idx+1:]
}

func matchGlobBody(glob, path string) bool {
	for {
		if glob == "" {
			return path == ""
		}
		if strings.HasPrefix(glob, "**/") {
			rest := glob[3:]
			// any depth
			for trial := path; ; trial = lastSegmentDrop(trial) {
				if matchGlobBody(rest, trial) {
					return true
				}
				if trial == "" {
					return false
				}
			}
		}
		if glob == "**" {
			return true
		}
		// process one segment at a time
		gi := strings.IndexByte(glob, '/')
		var gseg string
		if gi < 0 {
			gseg = glob
			glob = ""
		} else {
			gseg = glob[:gi]
			glob = glob[gi+1:]
		}
		pi := strings.IndexByte(path, '/')
		var pseg string
		if pi < 0 {
			pseg = path
			path = ""
		} else {
			pseg = path[:pi]
			path = path[pi+1:]
		}
		if !segmentMatch(gseg, pseg) {
			return false
		}
		if gi < 0 && pi >= 0 {
			return false
		}
		if gi >= 0 && pi < 0 {
			return false
		}
	}
}

// segmentMatch handles a single path segment with `*` wildcards.
func segmentMatch(pattern, seg string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == seg
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(seg, parts[0]) {
		return false
	}
	if !strings.HasSuffix(seg, parts[len(parts)-1]) {
		return false
	}
	cursor := len(parts[0])
	for _, mid := range parts[1 : len(parts)-1] {
		idx := strings.Index(seg[cursor:], mid)
		if idx < 0 {
			return false
		}
		cursor += idx + len(mid)
	}
	return true
}
