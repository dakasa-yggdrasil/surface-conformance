package conformance_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/surface-conformance/pkg/conformance"
)

// fixturePath returns the absolute path to the named fixture under testdata/.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("resolve fixture %q: %v", name, err)
	}
	return abs
}

func TestLintGoodSurfaceIsClean(t *testing.T) {
	linter := conformance.NewLinter(conformance.Config{})
	findings, err := linter.Lint(fixturePath(t, "good-surface"))
	if err != nil {
		t.Fatalf("lint good surface: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected good surface to have zero findings; got:\n%s", conformance.FormatText(findings))
	}
	if max := conformance.MaxSeverity(findings); max != conformance.SeverityOK {
		t.Fatalf("expected max severity OK, got %s", max)
	}
}

func TestLintBadSurfaceFlagsEveryInvariant(t *testing.T) {
	linter := conformance.NewLinter(conformance.Config{})
	findings, err := linter.Lint(fixturePath(t, "bad-surface"))
	if err != nil {
		t.Fatalf("lint bad surface: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings for bad surface, got 0")
	}

	expected := []conformance.Invariant{
		conformance.InvariantStateless,
		conformance.InvariantAuth,
		conformance.InvariantBackendAgnostic,
		conformance.InvariantMultiTenant,
		conformance.InvariantLego,
		conformance.InvariantNoBusinessDecide,
	}
	for _, want := range expected {
		if !findingsContainInvariant(findings, want) {
			t.Errorf("expected at least one finding for invariant %s; got:\n%s", want, conformance.FormatText(findings))
		}
	}

	// At least one finding must be a hard-line error (Stripe direct call,
	// auth0 dep, or cloud SDK import).
	if max := conformance.MaxSeverity(findings); max != conformance.SeverityError {
		t.Fatalf("expected SeverityError for bad surface, got %s", max)
	}
}

func TestExitCodeMapping(t *testing.T) {
	if got := conformance.ExitCode(conformance.SeverityOK); got != 0 {
		t.Errorf("ExitCode(OK)=%d, want 0", got)
	}
	if got := conformance.ExitCode(conformance.SeverityWarn); got != 0 {
		t.Errorf("ExitCode(Warn)=%d, want 0", got)
	}
	if got := conformance.ExitCode(conformance.SeverityError); got != 1 {
		t.Errorf("ExitCode(Error)=%d, want 1", got)
	}
}

func TestSkipConfigSuppressesInvariant(t *testing.T) {
	cfg := conformance.Config{Skip: []conformance.Invariant{conformance.InvariantBackendAgnostic, conformance.InvariantAuth, conformance.InvariantLego, conformance.InvariantStateless, conformance.InvariantNoBusinessDecide}}
	linter := conformance.NewLinter(cfg)
	findings, err := linter.Lint(fixturePath(t, "bad-surface"))
	if err != nil {
		t.Fatalf("lint bad surface (skipped): %v", err)
	}
	for _, f := range findings {
		if f.Invariant == conformance.InvariantBackendAgnostic {
			t.Errorf("expected backend-agnostic findings to be suppressed; got %+v", f)
		}
		if f.Invariant == conformance.InvariantAuth {
			t.Errorf("expected auth findings to be suppressed; got %+v", f)
		}
	}
}

func TestFormatJSONShape(t *testing.T) {
	findings := []conformance.Finding{
		{
			Invariant:  conformance.InvariantStateless,
			Severity:   conformance.SeverityWarn,
			File:       "src/store.ts",
			Line:       12,
			Message:    "sample",
			Suggestion: "move state to integration",
		},
	}
	data, err := conformance.FormatJSON(findings)
	if err != nil {
		t.Fatalf("format json: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, `"severity": "WARN"`) {
		t.Errorf("expected severity string in JSON; got %s", out)
	}
	if !strings.Contains(out, `"invariant": "stateless"`) {
		t.Errorf("expected invariant string in JSON; got %s", out)
	}
}

func TestFormatTextEmptyOnNoFindings(t *testing.T) {
	if got := conformance.FormatText(nil); got != "" {
		t.Errorf("FormatText(nil)=%q, want empty", got)
	}
}

func TestLintErrorsOnMissingPath(t *testing.T) {
	linter := conformance.NewLinter(conformance.Config{})
	if _, err := linter.Lint("/nonexistent/path/that/should/not/exist"); err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

// findingsContainInvariant returns true when any Finding carries the given
// invariant id.
func findingsContainInvariant(findings []conformance.Finding, want conformance.Invariant) bool {
	for _, f := range findings {
		if f.Invariant == want {
			return true
		}
	}
	return false
}
