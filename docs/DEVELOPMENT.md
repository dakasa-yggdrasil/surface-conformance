# Development

Build, test, add a rule, and release `surface-conformance`. Ground truth: the code in `pkg/conformance/` and `cmd/`.

Back to the [README](../README.md) · running the linter in [USAGE.md](./USAGE.md) · rule catalog in [RULES.md](./RULES.md).

---

## Prerequisites

- Go **1.25** (`go.mod`). No third-party dependencies — standard library only.

---

## Repo layout

| Path | What it is |
|---|---|
| `cmd/surface-conformance-lint/main.go` | CLI: flag parsing, runs the linter, renders text/JSON, maps severity → exit code. |
| `pkg/conformance/conformance.go` | Public API: `Linter`, `Config`, `Finding`, `Severity`, `Invariant`, `NewLinter`, `Lint`, `MaxSeverity`, `ExitCode`, `FormatText`, `FormatJSON`. Orchestrates the six checks. |
| `pkg/conformance/checks.go` | The six invariant checks (`checkStateless`, `checkAuth`, `checkBackendAgnostic`, `checkMultiTenant`, `checkLego`, `checkNoBusinessDecisions`) + shared helpers and the pattern/allow-list tables. |
| `pkg/conformance/walker.go` | `repoWalker`: walks the surface tree, classifies files (`goFiles`, `tsFiles`, `cfgFiles`, `manifestYAML`, …), applies skip-dirs + allow globs, lazy file reads, the tiny `**`/`*` glob matcher. |
| `pkg/conformance/json.go` | `jsonFinding` projection so JSON `severity` is a stable string, not the Go iota. |
| `pkg/conformance/conformance_test.go` | Table of tests over the fixtures + format/exit-code/skip behavior. |
| `testdata/good-surface/` | Clean fixture (zero findings). |
| `testdata/bad-surface/` | Fixture that trips every invariant + at least one ERROR. |
| `.github/workflows/ci.yml` | This repo's own CI: vet, test, build, smoke-run both fixtures. |
| `.github/workflows/surface-conformance-reusable.yml` | The reusable workflow surface repos call. |

---

## Build, vet, test

```bash
go vet ./...
go test ./... -count=1
go build ./cmd/surface-conformance-lint
```

Smoke-test the built binary against the fixtures (mirrors CI):

```bash
./surface-conformance-lint ./testdata/good-surface   # expect: no output, exit 0
./surface-conformance-lint ./testdata/bad-surface    # expect: findings, exit 1
```

> If your checkout sits inside a multi-module `go.work`, prefix commands with `GOWORK=off` to build this module in isolation.

### What the tests assert

`conformance_test.go` covers:

- `TestLintGoodSurfaceIsClean` — the good fixture yields zero findings and `MaxSeverity == OK`.
- `TestLintBadSurfaceFlagsEveryInvariant` — the bad fixture produces at least one finding for **each** of the six invariants and a `SeverityError` overall.
- `TestExitCodeMapping` — `ExitCode(OK)=0`, `ExitCode(Warn)=0`, `ExitCode(Error)=1`.
- `TestSkipConfigSuppressesInvariant` — `Config.Skip` removes the named invariants' findings.
- `TestFormatJSONShape` — JSON renders `severity`/`invariant` as strings.
- `TestFormatTextEmptyOnNoFindings` / `TestLintErrorsOnMissingPath` — empty render and IO error paths.

---

## Add a rule

A "rule" is either a new pattern inside an existing invariant check or a brand-new invariant. The data-driven shape of `checks.go` makes the common case small.

### Add a pattern to an existing invariant

1. Extend the relevant table in `checks.go` — e.g. add a package to `statelessGoImports`, a dep to `authDangerousTSDeps`, or a host to `providerHostsHardForbidden`.
2. Append a violating sample to `testdata/bad-surface/` (and keep `testdata/good-surface/` clean), then assert the new finding in `conformance_test.go`.
3. Document the addition in [RULES.md](./RULES.md) — one row in the at-a-glance table + a line under the invariant.

### Add a new invariant

1. **Define the id** in `conformance.go`: add an `Invariant` const (e.g. `InvariantFoo Invariant = "foo"`).
2. **Write the check** in `checks.go` as a method `func (l *Linter) checkFoo(w *repoWalker) []Finding`. Read the file buckets you need off `*repoWalker` (`w.goFiles`, `w.tsFiles`, `w.cfgFiles`, `w.manifestYAML`, …); emit `Finding{Invariant, Severity, File, Line, Message, Suggestion}`. Reuse helpers: `looksLikeImportLine`, `filepathBase`, `findFirstLine`, `containsAnyToken`, `linesSlice`.
3. **Register it** in the `checks` slice in `Lint` (`conformance.go`) so `-skip` and ordering pick it up:
   ```go
   {InvariantFoo, l.checkFoo},
   ```
4. **Update the CLI help** string for `-skip` in `cmd/surface-conformance-lint/main.go` so the new id is listed.
5. **Choose severity deliberately:** `SeverityError` only for hard-line contract §6 violations (fails CI); `SeverityWarn` for §5 "strongly discouraged".
6. **Add fixtures + a test** asserting the new invariant fires on `bad-surface` and stays silent on `good-surface`.
7. **Document** the rule in [RULES.md](./RULES.md) and the summary table in the [README](../README.md).

### Conventions for low false positives

- Gate Go-import matches behind `looksLikeImportLine` so a package string in a slice literal isn't mistaken for an import.
- Prefer scoping a check to the surface package (`isUnderSurfacePkg`) when a pattern is only a violation there.
- Exempt comment lines, test paths (`isUnderTestPath`), and build/CD config where appropriate.
- Keep heuristics conservative — a clean run should never lull a reviewer into skipping the §8 manual self-test for `no-business-decisions`.

---

## CI

`.github/workflows/ci.yml` runs on push to `main` and on every PR:

1. `go vet ./...`
2. `go test ./... -count=1`
3. `go build ./cmd/surface-conformance-lint`
4. Smoke-run `good-surface` — fails if it emits any output.
5. Smoke-run `bad-surface` — fails unless it exits `1`.

Keep these green before merging; the smoke runs are the end-to-end guard that the binary still behaves as documented.

---

## Release

There is no compiled-binary release artifact. Consumers install at a git ref:

- **CLI:** `go install github.com/dakasa-yggdrasil/surface-conformance/cmd/surface-conformance-lint@latest` (or `@<tag>` / `@<sha>`).
- **Library:** `import "github.com/dakasa-yggdrasil/surface-conformance/pkg/conformance"`, pinned in the consumer's `go.mod`.
- **Reusable workflow:** surface repos reference `...@main` (or a pinned ref via the `linter-ref` input) — see [USAGE.md](./USAGE.md#wire-it-into-a-surface-repos-ci).

To cut a versioned release, tag the commit (`git tag vX.Y.Z && git push --tags`); `go install @vX.Y.Z` and `linter-ref: vX.Y.Z` then resolve to it. Update the [Compatibility](../README.md#compatibility) note if the Go version or contract baseline changes.

---

## Relationship to the integration-side lint

`surface-conformance` mirrors [`integration-template/pkg/contractcheck`](https://github.com/dakasa-yggdrasil/integration-template/tree/main/pkg/contractcheck): same "static lint + reusable CI gate" shape, applied to the other side of the contract. When changing the shared conventions (severity ladder, finding shape, exit-code mapping), keep the two siblings aligned so surface and integration repos read as one product.
