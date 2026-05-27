// Package conformance provides a static linter that enforces the six
// SURFACE_CONTRACT.md §3 invariants across a Yggdrasil surface repository.
//
// The contract treats surfaces as "slime" — shape-free (UI / API / MCP / bot /
// hybrid) — but anchored by six invariants:
//
//  1. Stateless w.r.t. business — no surface-owned DB / KV / persistent business
//     state.
//  2. Auth via Yggdrasil session — surface MUST NOT reimplement OIDC/SAML/login.
//  3. Backend-agnostic — surface calls /api/v1/* on yggdrasil-core; it does NOT
//     call Stripe/AWS/GitHub/EFI providers directly.
//  4. Multi-tenant aware — surface respects integration_instance scoping.
//  5. Federated deployable / Lego — surface does NOT hardcode a specific cloud
//     / CDN / auth provider URL at runtime.
//  6. No business decision authority — surface dispatches and visualizes; it
//     does NOT decide business rules. Hard to lint statically — flagged as
//     "manual review required" by default.
//
// Severity ladder:
//
//   - SeverityOK     — no finding emitted.
//   - SeverityWarn   — strongly discouraged pattern present (SURFACE_CONTRACT.md
//     §5). Exit code 0 + warning entries in output. Caller can still ship after
//     authoring an ADR and adding the path to the allow-list.
//   - SeverityError  — hard-line violation (SURFACE_CONTRACT.md §6). Exit code 1.
//
// Usage from a unit test:
//
//	import "github.com/dakasa-yggdrasil/surface-conformance/pkg/conformance"
//
//	linter := conformance.NewLinter(conformance.Config{})
//	findings, err := linter.Lint("/path/to/surface")
//
// Usage from the CLI:
//
//	go run github.com/dakasa-yggdrasil/surface-conformance/cmd/surface-conformance-lint /path/to/surface
package conformance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Severity classifies a finding on the SURFACE_CONTRACT.md ladder.
type Severity int

const (
	// SeverityOK is reserved as the zero value and never emitted as a Finding.
	SeverityOK Severity = iota
	// SeverityWarn flags a pattern that is "strongly discouraged" (contract §5).
	// Exit code remains 0; caller may add the path to the per-repo allow-list
	// after writing an ADR.
	SeverityWarn
	// SeverityError flags a hard-line violation (contract §6). Exit code 1.
	SeverityError
)

// String implements fmt.Stringer for human-readable severity labels.
func (s Severity) String() string {
	switch s {
	case SeverityOK:
		return "OK"
	case SeverityWarn:
		return "WARN"
	case SeverityError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Invariant identifies which of the six §3 invariants a Finding pertains to.
type Invariant string

const (
	InvariantStateless        Invariant = "stateless"
	InvariantAuth             Invariant = "auth"
	InvariantBackendAgnostic  Invariant = "backend-agnostic"
	InvariantMultiTenant      Invariant = "multi-tenant"
	InvariantLego             Invariant = "lego"
	InvariantNoBusinessDecide Invariant = "no-business-decisions"
)

// Finding is one rule violation, emitted to the caller for rendering.
type Finding struct {
	Invariant  Invariant `json:"invariant"`
	Severity   Severity  `json:"severity"`
	File       string    `json:"file"`
	Line       int       `json:"line,omitempty"`
	Message    string    `json:"message"`
	Suggestion string    `json:"suggestion,omitempty"`
}

// Config tunes which checks the linter runs and how it filters results.
// A zero-value Config runs every check; this is the recommended default.
type Config struct {
	// Skip lists invariant identifiers (e.g. "stateless") that should not run.
	Skip []Invariant
	// AllowPaths lists path globs (relative to the surface repo root) that are
	// exempt from every check. Useful for vendored code, generated files,
	// dev/sandbox fixtures.
	AllowPaths []string
	// AllowedURLPrefixes are extra URL prefixes treated as "core-canonical"
	// in addition to the built-in /api/v1/* allow-list (rare; use sparingly).
	AllowedURLPrefixes []string
}

// Linter applies the §3 invariant checks against a surface repository.
type Linter struct {
	cfg Config
}

// NewLinter returns a Linter ready to inspect a surface repo.
func NewLinter(cfg Config) *Linter {
	return &Linter{cfg: cfg}
}

// Lint runs every configured invariant check against surfacePath and returns
// the sorted list of findings. An error is returned only when the directory
// cannot be walked; per-check violations are returned as Finding entries.
func (l *Linter) Lint(surfacePath string) ([]Finding, error) {
	abs, err := filepath.Abs(surfacePath)
	if err != nil {
		return nil, fmt.Errorf("resolve surface path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat surface path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("surface path is not a directory: %s", abs)
	}

	skip := make(map[Invariant]struct{}, len(l.cfg.Skip))
	for _, s := range l.cfg.Skip {
		skip[s] = struct{}{}
	}

	allowGlobs := append([]string{}, defaultAllowGlobs...)
	allowGlobs = append(allowGlobs, l.cfg.AllowPaths...)

	walker, err := newRepoWalker(abs, allowGlobs)
	if err != nil {
		return nil, fmt.Errorf("scan repo: %w", err)
	}

	var findings []Finding
	checks := []struct {
		id Invariant
		fn func(*repoWalker) []Finding
	}{
		{InvariantStateless, l.checkStateless},
		{InvariantAuth, l.checkAuth},
		{InvariantBackendAgnostic, l.checkBackendAgnostic},
		{InvariantMultiTenant, l.checkMultiTenant},
		{InvariantLego, l.checkLego},
		{InvariantNoBusinessDecide, l.checkNoBusinessDecisions},
	}
	for _, check := range checks {
		if _, skipped := skip[check.id]; skipped {
			continue
		}
		findings = append(findings, check.fn(walker)...)
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity > findings[j].Severity
		}
		if findings[i].Invariant != findings[j].Invariant {
			return findings[i].Invariant < findings[j].Invariant
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// MaxSeverity returns the highest severity among the findings, or SeverityOK
// when the slice is empty.
func MaxSeverity(findings []Finding) Severity {
	max := SeverityOK
	for _, f := range findings {
		if f.Severity > max {
			max = f.Severity
		}
	}
	return max
}

// ExitCode maps a finding severity to a recommended process exit code (0 for
// warn / ok, 1 for error).
func ExitCode(severity Severity) int {
	if severity >= SeverityError {
		return 1
	}
	return 0
}

// FormatText renders the findings as a single human-readable string suitable
// for CI log output. An empty slice yields the empty string.
func FormatText(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range findings {
		icon := "[WARN]"
		if f.Severity == SeverityError {
			icon = "[ERROR]"
		}
		fmt.Fprintf(&b, "%s %s — %s", icon, f.Invariant, f.Message)
		if f.File != "" {
			fmt.Fprintf(&b, "\n  file: %s", f.File)
			if f.Line > 0 {
				fmt.Fprintf(&b, ":%d", f.Line)
			}
		}
		if f.Suggestion != "" {
			fmt.Fprintf(&b, "\n  suggestion: %s", f.Suggestion)
		}
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormatJSON renders the findings as a JSON array (one object per finding).
// Returned bytes are appropriate for `-format=json` mode.
func FormatJSON(findings []Finding) ([]byte, error) {
	if findings == nil {
		findings = []Finding{}
	}
	return marshalIndentJSON(findings)
}

// ErrInvariantImpossible is the sentinel returned by the no-business-decisions
// check when it concludes that static lint cannot reach a verdict and
// recommends manual review. The error itself is not surfaced as a Finding;
// instead the check emits a SeverityWarn entry pointing at the checklist
// item in SURFACE_CONTRACT.md §8.
var ErrInvariantImpossible = errors.New("surface-conformance: invariant requires manual review")
