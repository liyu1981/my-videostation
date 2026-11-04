// Package scanrules compiles a library's per-folder indexing rules into fast
// path matchers used by the scanner.
//
// Patterns are relative to the library folder they are configured on:
//
//   - a pattern without "/" matches any single path segment, so `sample`
//     skips the folder `Movies/Film/Sample` and the file `x.sample.mkv`
//     (segment matching is glob-aware: `*.sample.mkv` works too);
//   - a pattern with "/" matches the whole relative path, where `**` matches
//     zero or more segments (`**/Extras/**` skips anything under an Extras
//     folder at any depth);
//   - matching is case-insensitive, `\` is treated as `/`, and a leading
//     `./` or a trailing `/` is ignored.
//
// Include patterns are a whitelist: when any are set, a file must match one of
// them to be indexed. They apply to files only, never to directories.
package scanrules

import (
	"path"
	"strings"
)

// Rules is the compiled form of a library's parsing rules.
type Rules struct {
	exclude    []pattern
	include    []pattern
	minSize    int64
	skipExtras bool
}

// Options mirrors the per-library rule columns.
type Options struct {
	ExcludeGlobs  []string
	IncludeGlobs  []string
	MinFileSizeMB int64
	SkipExtras    bool
}

type pattern struct {
	raw      string
	segs     []string
	anywhere bool
}

// New compiles rules. Invalid patterns are ignored (they cannot be entered
// through the API, which validates them with Validate).
func New(o Options) Rules {
	r := Rules{skipExtras: o.SkipExtras}
	if o.MinFileSizeMB > 0 {
		r.minSize = o.MinFileSizeMB * 1024 * 1024
	}
	for _, raw := range o.ExcludeGlobs {
		if p, ok := compile(raw); ok {
			r.exclude = append(r.exclude, p)
		}
	}
	for _, raw := range o.IncludeGlobs {
		if p, ok := compile(raw); ok {
			r.include = append(r.include, p)
		}
	}
	return r
}

// SkipExtras reports whether trailers/featurettes/samples should be ignored.
func (r Rules) SkipExtras() bool { return r.skipExtras }

// Excluded reports whether a directory at rel must not be descended into.
func (r Rules) Excluded(rel string) bool {
	if len(r.exclude) == 0 {
		return false
	}
	segs := splitPath(rel)
	return matchesAny(r.exclude, segs)
}

// SkipFile reports whether a file at rel with the given size must be skipped.
// It returns a short reason for the scan log.
func (r Rules) SkipFile(rel string, size int64) (bool, string) {
	segs := splitPath(rel)
	if matchesAny(r.exclude, segs) {
		return true, "excluded by rule"
	}
	if r.minSize > 0 && size < r.minSize {
		return true, "below minimum size"
	}
	if len(r.include) > 0 && !matchesAny(r.include, segs) {
		return true, "not matched by an include rule"
	}
	return false, ""
}

func matchesAny(patterns []pattern, rel []string) bool {
	for _, p := range patterns {
		if p.match(rel) {
			return true
		}
	}
	return false
}

func (p pattern) match(rel []string) bool {
	if p.anywhere {
		for _, seg := range rel {
			if segMatch(p.segs[0], seg) {
				return true
			}
		}
		return false
	}
	return matchSegs(p.segs, rel)
}

// matchSegs matches a compiled pattern against path segments. `**` matches
// zero or more segments.
func matchSegs(pat, rel []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			// Collapse consecutive wildcards and try every split point.
			for len(pat) > 0 && pat[0] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(rel); i++ {
				if matchSegs(pat, rel[i:]) {
					return true
				}
			}
			return false
		}
		if len(rel) == 0 {
			return false
		}
		if !segMatch(pat[0], rel[0]) {
			return false
		}
		pat, rel = pat[1:], rel[1:]
	}
	return len(rel) == 0
}

// segMatch glob-matches one segment; malformed patterns fall back to equality.
func segMatch(pat, seg string) bool {
	ok, err := path.Match(pat, seg)
	if err != nil {
		return pat == seg
	}
	return ok
}

func splitPath(rel string) []string {
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.Trim(rel, "/")
	rel = strings.ToLower(rel)
	if rel == "" {
		return nil
	}
	return strings.Split(rel, "/")
}

func compile(raw string) (pattern, bool) {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.Trim(p, "/")
	if p == "" {
		return pattern{}, false
	}
	p = strings.ToLower(p)
	return pattern{
		raw:      p,
		segs:     strings.Split(p, "/"),
		anywhere: !strings.Contains(p, "/"),
	}, true
}

// Validate normalises a user-provided pattern list: it trims entries, drops
// empties and duplicates (case-insensitively), and reports the first malformed
// glob. A pattern may not be absolute, contain ".." or be longer than 200 chars.
func Validate(patterns []string) ([]string, error) {
	out := make([]string, 0, len(patterns))
	seen := map[string]bool{}
	for _, raw := range patterns {
		p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		if p == "" {
			continue
		}
		if len(p) > 200 {
			return nil, &InvalidError{Pattern: raw, Reason: "longer than 200 characters"}
		}
		if strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
			return nil, &InvalidError{Pattern: raw, Reason: "must be relative to the library folder"}
		}
		p = strings.TrimPrefix(p, "./")
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		// A pattern that cannot compile is a typo we should report, not ignore.
		for _, seg := range strings.Split(p, "/") {
			if seg == "**" {
				continue
			}
			if _, err := path.Match(seg, "probe"); err != nil {
				return nil, &InvalidError{Pattern: raw, Reason: "invalid glob syntax"}
			}
		}
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out, nil
}

// InvalidError reports a rejected pattern.
type InvalidError struct {
	Pattern string
	Reason  string
}

func (e *InvalidError) Error() string {
	return "invalid pattern " + quote(e.Pattern) + ": " + e.Reason
}

func quote(s string) string {
	return `"` + s + `"`
}
