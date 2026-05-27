package conformance

import (
	"fmt"
	"regexp"
	"strings"
)

// ---------- Invariant 1 — Stateless w.r.t. business state ----------

// statelessImports lists Go imports that strongly suggest the surface owns
// business state (a database / KV / persistent backend). Any of these in
// non-test surface code triggers a SeverityWarn (per §5.1 exception path).
var statelessGoImports = []string{
	"database/sql",
	"gorm.io/gorm",
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"go.mongodb.org/mongo-driver",
	"github.com/redis/go-redis",
	"github.com/go-redis/redis",
	"github.com/dgraph-io/badger",
	"github.com/etcd-io/bbolt",
	"go.etcd.io/bbolt",
	"github.com/syndtr/goleveldb",
}

// statelessTSDeps mirrors statelessGoImports for the TS / SPA side.
var statelessTSDeps = []string{
	"pg", "pg-promise",
	"mysql", "mysql2",
	"mongodb", "mongoose",
	"ioredis", "redis",
	"typeorm", "sequelize", "prisma",
}

// statelessLocalStoragePattern detects business-shaped localStorage / IndexedDB
// writes. The contract permits storage for UX-only state (form drafts, expanded
// panels) but business state is §5.1 territory.
var statelessLocalStoragePattern = regexp.MustCompile(`(localStorage|sessionStorage|indexedDB|IndexedDB)\.(setItem|put|add)`)

// uxOnlyTokens are common UX-only keys; presence next to a storage write
// suppresses the warning.
var uxOnlyTokens = []string{"draft", "ui_", "ui-", "lastVisited", "preferred", "expanded", "collapsed", "theme", "locale", "i18n"}

func (l *Linter) checkStateless(w *repoWalker) []Finding {
	var findings []Finding

	// Go imports
	for _, f := range w.goFiles {
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		for i, line := range lines {
			for _, pkg := range statelessGoImports {
				if !strings.Contains(line, pkg) {
					continue
				}
				if !looksLikeImportLine(line) {
					continue
				}
				findings = append(findings, Finding{
					Invariant: InvariantStateless,
					Severity:  SeverityWarn,
					File:      f.path,
					Line:      i + 1,
					Message:   fmt.Sprintf("surface imports persistent-store package %q — surfaces are stateless w.r.t. business state (SURFACE_CONTRACT.md §3.1)", pkg),
					Suggestion: "Move durable state to an integration (ensure_*/observe_*) or operator backend. If genuinely UX-only, document the ADR per §5.1.",
				})
			}
		}
	}

	// TS / SPA deps via package.json
	for _, f := range w.cfgFiles {
		if filepathBase(f.path) != "package.json" {
			continue
		}
		data, err := f.read()
		if err != nil {
			continue
		}
		text := string(data)
		for _, dep := range statelessTSDeps {
			needle := `"` + dep + `"`
			if !strings.Contains(text, needle) {
				continue
			}
			line := findFirstLine(text, needle)
			findings = append(findings, Finding{
				Invariant: InvariantStateless,
				Severity:  SeverityWarn,
				File:      f.path,
				Line:      line,
				Message:   fmt.Sprintf("surface package.json declares persistent-store dependency %q — surfaces are stateless w.r.t. business state (SURFACE_CONTRACT.md §3.1)", dep),
				Suggestion: "Remove the dependency and call yggdrasil-core /api/v1/* + integrations instead. If genuinely UX-only, document the ADR per §5.1.",
			})
		}
	}

	// localStorage / IndexedDB business writes. We look at the call line + the
	// file name + the surrounding ~6 lines for a UX-only token (draft / theme /
	// lastVisited / etc). If any of those carries a token, we treat the write
	// as UX-state and exempt it.
	for _, f := range w.tsFiles {
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		for i, line := range lines {
			loc := statelessLocalStoragePattern.FindStringIndex(line)
			if loc == nil {
				continue
			}
			window := strings.Join(linesSlice(lines, max0(i-3), 7), "\n") + "\n" + f.path
			if containsAnyToken(window, uxOnlyTokens) {
				continue
			}
			findings = append(findings, Finding{
				Invariant: InvariantStateless,
				Severity:  SeverityWarn,
				File:      f.path,
				Line:      i + 1,
				Message:   "surface writes to client-side storage outside the UX-only allow list (SURFACE_CONTRACT.md §3.1 + §4)",
				Suggestion: "Either rename the key to a UX-only token (draft, lastVisited, ui_*, preferred, expanded, theme, locale) or move the state to yggdrasil-core / integration.",
			})
		}
	}

	return findings
}

// ---------- Invariant 2 — Auth via Yggdrasil session ----------

var authDangerousImports = []string{
	"github.com/coreos/go-oidc",
	"github.com/go-oidc",
	"github.com/crewjam/saml",
	"github.com/auth0/go-jwt-middleware",
	"golang.org/x/oauth2",
	"gopkg.in/auth0",
}

var authDangerousTSDeps = []string{
	"@auth0/auth0-react",
	"@auth0/auth0-spa-js",
	"oidc-client-ts",
	"oidc-client",
	"react-oidc-context",
	"keycloak-js",
	"firebase/auth",
	"@clerk/clerk-react",
	"@clerk/clerk-js",
	"next-auth",
}

func (l *Linter) checkAuth(w *repoWalker) []Finding {
	var findings []Finding

	for _, f := range w.goFiles {
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		for i, line := range lines {
			for _, pkg := range authDangerousImports {
				if !strings.Contains(line, pkg) {
					continue
				}
				if !looksLikeImportLine(line) {
					continue
				}
				findings = append(findings, Finding{
					Invariant: InvariantAuth,
					Severity:  SeverityError,
					File:      f.path,
					Line:      i + 1,
					Message:   fmt.Sprintf("surface imports upstream auth provider library %q — surfaces delegate auth to Yggdrasil session and MUST NOT reimplement OIDC/SAML (SURFACE_CONTRACT.md §3.2 + §6.4)", pkg),
					Suggestion: "Drop the library; rely on yggdrasil-core session cookie (read via /api/v1/auth/session).",
				})
			}
		}
	}

	for _, f := range w.cfgFiles {
		if filepathBase(f.path) != "package.json" {
			continue
		}
		data, err := f.read()
		if err != nil {
			continue
		}
		text := string(data)
		for _, dep := range authDangerousTSDeps {
			needle := `"` + dep + `"`
			if !strings.Contains(text, needle) {
				continue
			}
			line := findFirstLine(text, needle)
			findings = append(findings, Finding{
				Invariant: InvariantAuth,
				Severity:  SeverityError,
				File:      f.path,
				Line:      line,
				Message:   fmt.Sprintf("surface package.json declares upstream auth provider %q — surfaces delegate auth to Yggdrasil session (SURFACE_CONTRACT.md §3.2 + §6.4)", dep),
				Suggestion: "Drop the dependency. Use credentials: 'include' against /api/v1/auth/session (see surface-toolkit useYggdrasilAPI).",
			})
		}
	}

	// Heuristic: explicit "login form + password" patterns in TS code.
	loginPattern := regexp.MustCompile(`(?i)<form[^>]*>[^<]*password|new\s+Strategy\(|passport-local`)
	for _, f := range w.tsFiles {
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		for i, line := range lines {
			if loginPattern.MatchString(line) {
				findings = append(findings, Finding{
					Invariant: InvariantAuth,
					Severity:  SeverityWarn,
					File:      f.path,
					Line:      i + 1,
					Message:   "surface contains a login form / passport-local pattern — surfaces MUST NOT reimplement auth (SURFACE_CONTRACT.md §3.2)",
					Suggestion: "Remove the form; rely on yggdrasil-core /api/v1/auth/session.",
				})
			}
		}
	}

	return findings
}

// ---------- Invariant 3 — Backend-agnostic (via Yggdrasil core) ----------

// providerHostsHardForbidden lists provider hostnames whose direct call from a
// surface is a hard-line violation (SURFACE_CONTRACT.md §3.3 + §6.3).
var providerHostsHardForbidden = []string{
	"api.stripe.com",
	"api.github.com",
	"slack.com/api",
	"api.nfe.io",
	"api.efipay.com.br",
	"api.openai.com",
	"api.anthropic.com",
}

// fetchPattern detects fetch( / axios. / XMLHttpRequest call sites with URL
// literals, including template strings starting with `https://...` or `http://`.
var fetchPattern = regexp.MustCompile("(?i)(fetch\\(|axios(?:\\.[a-z]+)?\\(|XMLHttpRequest\\(\\)|new\\s+XMLHttpRequest|\\$http\\.|\\$\\.ajax)")
var urlLiteralPattern = regexp.MustCompile(`https?://[^\s"'\x60)]+`)

func (l *Linter) checkBackendAgnostic(w *repoWalker) []Finding {
	var findings []Finding

	// TS / SPA: scan for fetch/axios with URL literal not under /api/v1/*
	for _, f := range w.tsFiles {
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		for i, line := range lines {
			if !fetchPattern.MatchString(line) {
				continue
			}
			// Skip if the call uses a relative /api/v1/* URL.
			if hasCoreCanonicalReference(line, l.cfg.AllowedURLPrefixes) {
				continue
			}
			// Look for an absolute URL on this line, or the next two.
			urls := collectURLs(lines, i)
			if len(urls) == 0 {
				// fetch with template var — flag as info-level warn for review.
				continue
			}
			for _, url := range urls {
				if isCoreCanonicalURL(url) {
					continue
				}
				severity := SeverityWarn
				message := fmt.Sprintf("surface fetches %q directly — surfaces are backend-agnostic and should call yggdrasil-core /api/v1/* (SURFACE_CONTRACT.md §3.3)", url)
				for _, host := range providerHostsHardForbidden {
					if strings.Contains(strings.ToLower(url), strings.ToLower(host)) {
						severity = SeverityError
						message = fmt.Sprintf("surface calls upstream provider directly: %q — hard-line violation (SURFACE_CONTRACT.md §3.3 + §6.3)", url)
						break
					}
				}
				findings = append(findings, Finding{
					Invariant: InvariantBackendAgnostic,
					Severity:  severity,
					File:      f.path,
					Line:      i + 1,
					Message:   message,
					Suggestion: "Route the call through yggdrasil-core (`/api/v1/integrations/{instanceId}/surface-query` or a workflow dispatch). See surface-toolkit useSurfaceQuery.",
				})
			}
		}
	}

	// Go surface side: forbid direct cloud SDK clients in surface/* package.
	cloudSDKMarkers := []string{
		"github.com/aws/aws-sdk-go",
		"cloud.google.com/go",
		"github.com/Azure/azure-sdk-for-go",
		"github.com/stripe/stripe-go",
		"github.com/google/go-github",
		"github.com/slack-go/slack",
	}
	for _, f := range w.goFiles {
		if !isUnderSurfacePkg(f.path) {
			continue
		}
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		for i, line := range lines {
			if !looksLikeImportLine(line) {
				continue
			}
			for _, marker := range cloudSDKMarkers {
				if strings.Contains(line, marker) {
					findings = append(findings, Finding{
						Invariant: InvariantBackendAgnostic,
						Severity:  SeverityError,
						File:      f.path,
						Line:      i + 1,
						Message:   fmt.Sprintf("surface package imports cloud SDK %q — surfaces dispatch through yggdrasil-core; integrations own the provider call (SURFACE_CONTRACT.md §3.3 + §6.3)", marker),
						Suggestion: "Move the call to integration-<provider>; expose the result via observe_/discover_ capabilities; surface consumes the data over /api/v1/*.",
					})
				}
			}
		}
	}

	return findings
}

func isCoreCanonicalURL(url string) bool {
	lower := strings.ToLower(url)
	// Relative URLs (start with /api/v1) handled by hasCoreCanonicalReference.
	switch {
	case strings.HasPrefix(lower, "http://localhost"),
		strings.HasPrefix(lower, "https://localhost"),
		strings.HasPrefix(lower, "http://127.0.0.1"),
		strings.HasPrefix(lower, "https://127.0.0.1"):
		return true
	case strings.Contains(lower, "/api/v1/"):
		return true
	}
	return false
}

func hasCoreCanonicalReference(line string, extraPrefixes []string) bool {
	if strings.Contains(line, "/api/v1/") || strings.Contains(line, "`/api/v1/") || strings.Contains(line, `"/api/v1/`) || strings.Contains(line, "'/api/v1/") {
		return true
	}
	for _, prefix := range extraPrefixes {
		if strings.Contains(line, prefix) {
			return true
		}
	}
	return false
}

func collectURLs(lines []string, idx int) []string {
	var urls []string
	for off := 0; off < 3 && idx+off < len(lines); off++ {
		matches := urlLiteralPattern.FindAllString(lines[idx+off], -1)
		urls = append(urls, matches...)
	}
	return urls
}

func isUnderSurfacePkg(p string) bool {
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.HasPrefix(p, "surface/") || strings.Contains(p, "/surface/") ||
		strings.HasPrefix(p, "surface-ui/") || strings.Contains(p, "/surface-ui/")
}

// ---------- Invariant 4 — Multi-tenant aware ----------

// surfaceQueryPath is the canonical core endpoint that already enforces
// integration_instance scoping; if every fetch goes through here we consider
// the multi-tenant invariant satisfied.
const surfaceQueryPath = "/api/v1/integrations/"

func (l *Linter) checkMultiTenant(w *repoWalker) []Finding {
	var findings []Finding

	// Scan SPA TS files: a fetch to /api/v1/* without `integration_instance`
	// or `instanceId` or `integrations/{id}` segment is suspicious.
	for _, f := range w.tsFiles {
		// Skip Vite / build config files — references to /api/v1/* there are
		// dev-server proxy config, not runtime fetch.
		base := filepathBase(f.path)
		if isViteOrBuildConfig(base) {
			continue
		}
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		for i, line := range lines {
			if !strings.Contains(line, "/api/v1/") {
				continue
			}
			if strings.Contains(line, surfaceQueryPath) {
				continue
			}
			// Skip pure comment lines (the actual fetch site comes later).
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
				continue
			}
			if strings.Contains(line, "integration_instance") ||
				strings.Contains(line, "instanceId") ||
				strings.Contains(line, "instance_id") ||
				strings.Contains(line, "useInstance") ||
				strings.Contains(line, "useSurfaceQuery") ||
				strings.Contains(line, "useYggdrasilAPI") {
				continue
			}
			// Read-only console / auth endpoints are exempt: they are tenant-aware
			// upstream in core, not via path scoping.
			if isCoreReadOnlyPath(line) {
				continue
			}
			findings = append(findings, Finding{
				Invariant: InvariantMultiTenant,
				Severity:  SeverityWarn,
				File:      f.path,
				Line:      i + 1,
				Message:   "surface fetch against /api/v1/* lacks an integration_instance / instanceId scope marker (SURFACE_CONTRACT.md §3.4)",
				Suggestion: "Use surface-toolkit useSurfaceQuery / useInstance, or include instanceId in the path / query.",
			})
		}
	}

	return findings
}

func isCoreReadOnlyPath(line string) bool {
	// Endpoint families that are NOT tenant-scoped at the URL level — core
	// enforces RBAC / platform-team scope at handler time, so the surface
	// does not need to embed integration_instance in the path.
	exemptSegments := []string{
		"/api/v1/auth/", // /api/v1/auth/session, /providers, /sso, /logout, /passwords
		"/api/v1/console/",
		"/api/v1/heimdall",
		"/api/v1/readyz",
		"/api/v1/healthz",
		"/api/v1/ops/",
		"/api/v1/integration-surfaces",
		"/api/v1/integration-catalog",
		"/api/v1/manifests",
		"/api/v1/manifest-",
		"/api/v1/workflow",
		"/api/v1/teams",
		"/api/v1/tenant/",
		"/api/v1/me/",
	}
	for _, seg := range exemptSegments {
		if strings.Contains(line, seg) {
			return true
		}
	}
	return false
}

// ---------- Invariant 5 — Federated deployable / Lego ----------

var legoCloudHostPattern = regexp.MustCompile(`(?i)([0-9a-z\-]+\.)?(amazonaws\.com|googleapis\.com|azure\.com|cloudfront\.net|cloudflare\.com|akamai\.net|fastly\.net|s3\.[a-z\-0-9]+\.amazonaws|ecr\.[a-z\-0-9]+\.amazonaws|dkr\.ecr\.[a-z\-0-9]+\.amazonaws)`)

// legoAllowedFilePrefixes are paths where cloud-specific URLs are acceptable
// because they are build/CD artifacts, not runtime source.
var legoAllowedFileSegments = []string{
	"/deploy/",
	"deploy/",
	"/.github/",
	".github/",
	"Dockerfile",
	"docker-compose",
	"buildspec",
}

func (l *Linter) checkLego(w *repoWalker) []Finding {
	var findings []Finding

	checkFiles := func(entries []*fileEntry, severity Severity) {
		for _, f := range entries {
			// Manifests are runtime artifacts even though they live alongside CD
			// configs — flag them explicitly.
			isManifest := strings.HasSuffix(f.path, ".manifest.json") ||
				strings.HasSuffix(f.path, ".manifest.yaml") ||
				strings.HasSuffix(f.path, ".manifest.yml")
			if !isManifest && isUnderBuildOrCDPath(f.path) {
				continue
			}
			lines, err := f.linesOf()
			if err != nil {
				continue
			}
			for i, line := range lines {
				matches := legoCloudHostPattern.FindAllString(line, -1)
				for _, host := range matches {
					if strings.Contains(strings.ToLower(line), "// ok:") {
						continue
					}
					findings = append(findings, Finding{
						Invariant: InvariantLego,
						Severity:  severity,
						File:      f.path,
						Line:      i + 1,
						Message:   fmt.Sprintf("surface hardcodes cloud-specific host %q at runtime — violates Lego principle (SURFACE_CONTRACT.md §3.5 + §6.3)", host),
						Suggestion: "Make the URL operator-configurable (env var or operator-supplied manifest field). Build-time pins in /deploy/ or .github/ are exempt.",
					})
				}
			}
		}
	}

	checkFiles(w.tsFiles, SeverityWarn)
	checkFiles(w.goFiles, SeverityWarn)
	checkFiles(w.manifestYAML, SeverityWarn)

	return findings
}

func isUnderBuildOrCDPath(path string) bool {
	lower := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	for _, seg := range legoAllowedFileSegments {
		if strings.Contains(lower, strings.ToLower(seg)) {
			return true
		}
	}
	return false
}

// ---------- Invariant 6 — No business decision authority ----------

// Static lint cannot reliably tell "this if-statement charges the user" from
// "this if-statement chooses a display color". The check is therefore
// deliberately narrow: we only flag local control-flow that gates a clearly
// monetary or permission-shaped action with a numeric threshold. Everything
// else is left to manual review per SURFACE_CONTRACT.md §8.
//
// Pattern we DO flag (genuine local decision):
//
//	if (balance > amount) { return "charge"; }
//
// Patterns we DON'T flag (dispatch, JSX, comments, test files):
//
//	await fetch(`/api/v1/.../approve`)   // dispatch via core
//	<button onClick={approveRequest}>Approve</button>  // UI label
//	// charge will go through workflow                  // comment

// businessDecisionPattern matches a control-flow statement gating a clearly
// monetary keyword. The structure is approximately:
//
//	`if (...numeric-comparison...) ... <monetary-verb>`
//
// We require both an operator and a monetary verb on the same logical line to
// keep the false positive rate close to zero.
var businessDecisionPattern = regexp.MustCompile(`(?is)\bif\s*\(.*[<>=!]+.*\)\s*\{?[^}]*\b(charge|refund|debit|credit|chargeback|capture|deduct)\b`)

// businessExplicitDecisionPattern flags an explicit "decide" / "authorize"
// hand-rolled boolean in a surface file (e.g. `canCharge = ...`).
var businessExplicitDecisionPattern = regexp.MustCompile(`(?i)\b(canCharge|canRefund|shouldCharge|shouldRefund|isAllowedTo[A-Z]|authorize(Charge|Refund|Debit))\b`)

func (l *Linter) checkNoBusinessDecisions(w *repoWalker) []Finding {
	var findings []Finding

	for _, f := range w.tsFiles {
		if isUnderTestPath(f.path) {
			continue
		}
		lines, err := f.linesOf()
		if err != nil {
			continue
		}
		// Join lines into a single scan window — control-flow can span lines
		// but we want to match `if (...) { return "charge"; }` patterns even
		// when split by formatting. We still report the line of first match.
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "#") {
				continue
			}
			// Build a 3-line lookahead window so multi-line `if (...) { ... }`
			// patterns are visible.
			window := strings.Join(linesSlice(lines, i, 3), "\n")
			if businessDecisionPattern.MatchString(window) || businessExplicitDecisionPattern.MatchString(window) {
				if isCanonicalDispatch(line) {
					continue
				}
				findings = append(findings, Finding{
					Invariant: InvariantNoBusinessDecide,
					Severity:  SeverityWarn,
					File:      f.path,
					Line:      i + 1,
					Message:   "surface appears to decide a business outcome locally (control-flow gates a monetary action) — surface MUST dispatch, not decide (SURFACE_CONTRACT.md §3.6 + §8)",
					Suggestion: "Move the rule into an integration capability or workflow; surface should POST the request and render the resulting outcome.",
				})
				// Only flag once per file to keep output manageable.
				break
			}
		}
	}

	return findings
}

func linesSlice(lines []string, start, span int) []string {
	if start < 0 {
		start = 0
	}
	end := start + span
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return nil
	}
	return lines[start:end]
}

func max0(a int) int {
	if a < 0 {
		return 0
	}
	return a
}

func isViteOrBuildConfig(base string) bool {
	switch base {
	case "vite.config.ts", "vite.config.js", "vitest.config.ts", "vitest.config.js",
		"webpack.config.ts", "webpack.config.js", "rollup.config.ts", "rollup.config.js",
		"tsconfig.json", "esbuild.config.ts", "esbuild.config.js":
		return true
	}
	return false
}

func isCanonicalDispatch(line string) bool {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "/api/v1/") {
		return false
	}
	return strings.Contains(lower, "fetch(") ||
		strings.Contains(lower, "axios") ||
		strings.Contains(lower, "request<") ||
		strings.Contains(lower, "request(") ||
		strings.Contains(lower, "usequery") ||
		strings.Contains(lower, "usemutation")
}

func isUnderTestPath(p string) bool {
	lower := strings.ToLower(p)
	return strings.Contains(lower, "/__tests__/") ||
		strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".test.tsx") ||
		strings.HasSuffix(lower, ".spec.ts") ||
		strings.HasSuffix(lower, ".spec.tsx") ||
		strings.Contains(lower, "/testdata/")
}

func hasAnySuffix(s string, suffixes []string) bool {
	// Scan the first identifier-ish token of s.
	end := 0
	for end < len(s) && (isIdentChar(s[end])) {
		end++
	}
	tok := s[:end]
	for _, suf := range suffixes {
		if strings.HasSuffix(tok, suf) {
			return true
		}
	}
	return false
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// ---------- shared helpers ----------

// looksLikeImportLine returns true when the line *appears* to be part of an
// import declaration. To keep false positives down (a Go string literal in a
// constant slice is NOT an import), we require either an explicit `import`
// keyword on the line or a line of the form:
//
//	\t"github.com/..." [single-token, may have aliasing prefix]
//	\t_ "github.com/..."
//	\timportAlias "github.com/..."
//
// Anything followed by a `,` or `)` or other non-whitespace tail (typical of
// a slice entry) is rejected.
func looksLikeImportLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") {
		return false
	}
	if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "import\t") || trimmed == "import (" {
		return true
	}
	// We want lines that look like:
	//   "github.com/foo/bar"
	//   _ "github.com/foo/bar"
	//   alias "github.com/foo/bar"
	// and reject:
	//   "github.com/foo/bar",   (trailing comma → slice entry)
	//   value: "github.com/foo/bar"
	//
	// Strategy: the line must END at the closing quote of the import path,
	// modulo optional trailing whitespace / comment.
	cleaned := trimmed
	if idx := strings.Index(cleaned, "//"); idx >= 0 {
		cleaned = strings.TrimSpace(cleaned[:idx])
	}
	if !strings.HasSuffix(cleaned, `"`) {
		return false
	}
	// Now the line ends with `"`. Walk back to find the matching opening `"`.
	// Everything before that opening must be import-shaped: empty, `_`, or a
	// single identifier alias.
	openIdx := strings.Index(cleaned, `"`)
	if openIdx < 0 {
		return false
	}
	prefix := strings.TrimSpace(cleaned[:openIdx])
	if prefix == "" || prefix == "_" {
		return true
	}
	// Allow `alias` (single identifier).
	for i := 0; i < len(prefix); i++ {
		if !isIdentChar(prefix[i]) {
			return false
		}
	}
	return true
}

func filepathBase(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func findFirstLine(text, needle string) int {
	idx := strings.Index(text, needle)
	if idx < 0 {
		return 0
	}
	return strings.Count(text[:idx], "\n") + 1
}

func containsAnyToken(s string, tokens []string) bool {
	lower := strings.ToLower(s)
	for _, t := range tokens {
		if strings.Contains(lower, strings.ToLower(t)) {
			return true
		}
	}
	return false
}
