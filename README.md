# surface-conformance

> Static linter for the six invariants defined in
> [SURFACE_CONTRACT.md](https://github.com/dakasa-yggdrasil/surface-template/blob/main/SURFACE_CONTRACT.md)
> section 3.

`surface-conformance` is to surfaces what
[`integration-template/pkg/contractcheck`](https://github.com/dakasa-yggdrasil/integration-template/tree/main/pkg/contractcheck)
is to integrations: a small, importable Go package and a CLI binary that
walks a surface repository and emits warnings or errors for any pattern that
violates the surface contract.

## Why

The 2026-05-27 audit (23 surfaces walked) found 20 clean / 2 strongly
discouraged / 0 hard-line violations. The audit is a snapshot — new code
can regress at any time. This linter codifies the audit so a CI gate
catches the regressions before they reach `main`.

## What the six invariants are

| # | Name | Severity rule |
|---|------|---|
| 1 | **Stateless w.r.t. business** | Warn if surface owns a DB / KV / persistent business state. |
| 2 | **Auth via Yggdrasil session** | Error if surface imports an upstream OIDC/SAML/auth library. |
| 3 | **Backend-agnostic** | Error if surface calls a provider API (Stripe / GitHub / etc.) directly; warn if it fetches a non-`/api/v1/*` URL. |
| 4 | **Multi-tenant aware** | Warn if a `/api/v1/*` call lacks an `integration_instance` / `instanceId` scope marker. |
| 5 | **Federated deployable / Lego** | Warn if runtime code or manifest hardcodes a cloud-specific host (`*.amazonaws.com`, `*.googleapis.com`, `*.azure.com`, etc.). |
| 6 | **No business decision authority** | Warn (only narrow heuristic — manual review required) when local control-flow gates a monetary verb. |

See [SURFACE_CONTRACT.md](https://github.com/dakasa-yggdrasil/surface-template/blob/main/SURFACE_CONTRACT.md)
section 3 for the canonical definitions and section 5 for the exception path
(when a warning is acceptable with an ADR).

## Usage

### As a CLI

```bash
go install github.com/dakasa-yggdrasil/surface-conformance/cmd/surface-conformance-lint@latest

surface-conformance-lint /path/to/surface
# exit 0 + no output         → fully ok
# exit 0 + WARN lines        → strongly discouraged patterns (section 5)
# exit 1 + ERROR lines       → hard-line violations (section 6)
```

Flags:

```text
-format=json                  JSON output instead of text.
-skip=auth,multi-tenant       Comma-separated invariant ids to skip.
-allow=legacy/**              Comma-separated glob paths to exclude from scan.
-allow-url-prefix=/api/v2/    Extra URL prefixes treated as core-canonical.
```

### As a library

```go
import "github.com/dakasa-yggdrasil/surface-conformance/pkg/conformance"

linter := conformance.NewLinter(conformance.Config{})
findings, err := linter.Lint("/path/to/surface")
if err != nil { return err }
fmt.Println(conformance.FormatText(findings))
os.Exit(conformance.ExitCode(conformance.MaxSeverity(findings)))
```

### In CI (reusable workflow)

```yaml
# .github/workflows/surface-conformance.yml
name: Surface Conformance

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  conformance:
    uses: dakasa-yggdrasil/surface-conformance/.github/workflows/surface-conformance-reusable.yml@main
    with:
      surface-path: "."
```

## Fixtures

The repo ships two fixtures under `testdata/`:

- `good-surface/` — passes every check; a regression target.
- `bad-surface/` — deliberately violates each invariant; verifies the linter
  emits at least one finding per invariant.

## What the linter is NOT

- Not a substitute for the section 8 self-test (manual review required for
  the no-business-decisions invariant).
- Not a replacement for the integration-side
  [contractcheck](https://github.com/dakasa-yggdrasil/integration-template/tree/main/pkg/contractcheck);
  the two are sibling lints, one for surfaces, one for integrations.
- Not a hard policy enforcer: warnings can be allow-listed when a documented
  ADR (per `SURFACE_CONTRACT.md` section 5) signs off the exception.

## Related

- Surface contract: <https://github.com/dakasa-yggdrasil/surface-template/blob/main/SURFACE_CONTRACT.md>
- Integration contract: <https://github.com/dakasa-yggdrasil/integration-template/blob/main/INTEGRATION_CONTRACT.md>
- Integration-side lint (mirror pattern): <https://github.com/dakasa-yggdrasil/integration-template/tree/main/pkg/contractcheck>

## License

Apache 2.0 — see [LICENSE](./LICENSE).
