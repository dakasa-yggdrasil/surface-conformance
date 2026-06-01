# Rules

Every rule `surface-conformance` enforces, mapped to the [SURFACE_CONTRACT.md §3](https://github.com/dakasa-yggdrasil/surface-template/blob/main/SURFACE_CONTRACT.md) invariant it protects, with the exact trigger and how to fix a violation. Ground truth: `pkg/conformance/checks.go`.

Back to the [README](../README.md) · running the linter in [USAGE.md](./USAGE.md).

---

## Severity ladder

| Severity | Exit code | Contract clause | Meaning |
|---|---|---|---|
| `WARN` | 0 | §5 | Strongly discouraged. Reported but does not block; allow-listable after an ADR. |
| `ERROR` | 1 | §6 | Hard-line violation. Fails the CI gate; must be fixed before merge. |

The six invariant ids (for `-skip`): `stateless`, `auth`, `backend-agnostic`, `multi-tenant`, `lego`, `no-business-decisions`.

---

## Rules at a glance

| Rule | Invariant id | Worst severity | Contract | Trigger |
|---|---|---|---|---|
| Persistent-store Go import | `stateless` | WARN | §3.1 | `database/sql`, `gorm`, `pgx`, `lib/pq`, `mongo-driver`, `go-redis`, `badger`, `bbolt`, `goleveldb` in an import line |
| Persistent-store npm dep | `stateless` | WARN | §3.1 | `pg`, `mysql(2)`, `mongodb`, `mongoose`, `ioredis`/`redis`, `typeorm`, `sequelize`, `prisma` in `package.json` |
| Business-state client storage | `stateless` | WARN | §3.1, §4 | `localStorage`/`sessionStorage`/`indexedDB` `.setItem`/`.put`/`.add` with no UX-only token nearby |
| Upstream auth Go library | `auth` | ERROR | §3.2, §6.4 | `go-oidc`, `crewjam/saml`, `oauth2`, `auth0`, `go-jwt-middleware` import |
| Upstream auth npm dep | `auth` | ERROR | §3.2, §6.4 | `@auth0/*`, `oidc-client(-ts)`, `react-oidc-context`, `keycloak-js`, `firebase/auth`, `@clerk/*`, `next-auth` in `package.json` |
| Hand-rolled login | `auth` | WARN | §3.2 | `<form>…password`, `new Strategy(`, or `passport-local` in TS/JS |
| Direct provider fetch | `backend-agnostic` | ERROR | §3.3, §6.3 | `fetch`/`axios`/XHR to a known provider host (Stripe/GitHub/Slack/nfe.io/EFI/OpenAI/Anthropic) |
| Non-core URL fetch | `backend-agnostic` | WARN | §3.3 | `fetch`/`axios`/XHR to an absolute URL that is not `/api/v1/*` or localhost |
| Cloud SDK in surface pkg | `backend-agnostic` | ERROR | §3.3, §6.3 | AWS/GCP/Azure/Stripe/GitHub/Slack SDK import under `surface/` |
| Unscoped core fetch | `multi-tenant` | WARN | §3.4 | `/api/v1/*` reference with no instance scope marker and not a read-only core path |
| Hardcoded cloud host | `lego` | WARN | §3.5, §6.3 | Cloud-specific host (`*.amazonaws.com`, `*.googleapis.com`, `cloudfront.net`, …) in runtime code or a manifest |
| Local monetary decision | `no-business-decisions` | WARN | §3.6, §8 | `if (…comparison…) { …charge/refund/… }` or a `canCharge`-style boolean |

---

## Invariant 1 — Stateless w.r.t. business state (`stateless`)

> **Contract §3.1:** A surface owns no business DB / KV / persistent state. Durable data lives in integrations or the operator backend; surfaces hold only UX-only state.

### 1a · Persistent-store Go import — WARN

- **Detects:** an import line containing one of `database/sql`, `gorm.io/gorm`, `github.com/jackc/pgx`, `github.com/lib/pq`, `go.mongodb.org/mongo-driver`, `github.com/redis/go-redis`, `github.com/go-redis/redis`, `github.com/dgraph-io/badger`, `github.com/etcd-io/bbolt`, `go.etcd.io/bbolt`, `github.com/syndtr/goleveldb`. The `looksLikeImportLine` heuristic avoids flagging the same strings inside a slice literal.
- **Fix:** move durable state into an integration (`ensure_*` / `observe_*` capabilities) or the operator backend. If genuinely UX-only, write the ADR per §5.1 and `-allow` the path.

### 1b · Persistent-store npm dependency — WARN

- **Detects:** `package.json` declaring `pg`, `pg-promise`, `mysql`, `mysql2`, `mongodb`, `mongoose`, `ioredis`, `redis`, `typeorm`, `sequelize`, or `prisma` (matched as a quoted JSON key).
- **Fix:** remove the dependency; read data over `yggdrasil-core /api/v1/*` + integrations instead.

### 1c · Business-state client storage — WARN

- **Detects:** a `localStorage` / `sessionStorage` / `indexedDB` / `IndexedDB` write (`.setItem` / `.put` / `.add`) where neither the call line, surrounding ~6 lines, nor the file path contain a **UX-only token**: `draft`, `ui_`, `ui-`, `lastVisited`, `preferred`, `expanded`, `collapsed`, `theme`, `locale`, `i18n`.
- **Fix:** rename the key to one of the UX-only tokens if it really is UX state; otherwise move the value to core / an integration. Example from the `bad-surface` fixture: `localStorage.setItem(\`order_total_${orderId}\`, "1000.00")` is business state, not UX.

---

## Invariant 2 — Auth via Yggdrasil session (`auth`)

> **Contract §3.2 / §6.4:** A surface delegates authentication to the Yggdrasil session. It MUST NOT reimplement OIDC / SAML / login.

### 2a · Upstream auth Go library — ERROR

- **Detects:** an import line with `github.com/coreos/go-oidc`, `github.com/go-oidc`, `github.com/crewjam/saml`, `github.com/auth0/go-jwt-middleware`, `golang.org/x/oauth2`, or `gopkg.in/auth0`.
- **Fix:** drop the library; rely on the yggdrasil-core session cookie, read via `/api/v1/auth/session`.

### 2b · Upstream auth npm dependency — ERROR

- **Detects:** `package.json` declaring `@auth0/auth0-react`, `@auth0/auth0-spa-js`, `oidc-client-ts`, `oidc-client`, `react-oidc-context`, `keycloak-js`, `firebase/auth`, `@clerk/clerk-react`, `@clerk/clerk-js`, or `next-auth`.
- **Fix:** remove the dependency; use `credentials: 'include'` against `/api/v1/auth/session` (see `surface-toolkit` `useYggdrasilAPI`).

### 2c · Hand-rolled login — WARN

- **Detects:** a `<form>…password` element, `new Strategy(`, or `passport-local` in TS/JS.
- **Fix:** remove the form/strategy; rely on `yggdrasil-core /api/v1/auth/session`.

---

## Invariant 3 — Backend-agnostic, via Yggdrasil core (`backend-agnostic`)

> **Contract §3.3 / §6.3:** A surface calls `/api/v1/*` on yggdrasil-core. It does NOT call Stripe / AWS / GitHub / provider APIs directly — integrations own provider calls.

### 3a · Direct provider fetch — ERROR

- **Detects:** a `fetch(` / `axios(.method)?(` / `XMLHttpRequest` / `$http.` / `$.ajax` call whose URL literal (on the call line or the next two) contains a hard-forbidden provider host: `api.stripe.com`, `api.github.com`, `slack.com/api`, `api.nfe.io`, `api.efipay.com.br`, `api.openai.com`, `api.anthropic.com`.
- **Fix:** route through yggdrasil-core — `/api/v1/integrations/{instanceId}/surface-query` or a workflow dispatch (see `surface-toolkit` `useSurfaceQuery`). The provider call belongs in an integration.

### 3b · Non-core absolute-URL fetch — WARN

- **Detects:** the same call shapes, but to an absolute `http(s)://` URL that is **not** core-canonical. Core-canonical = contains `/api/v1/`, or is `localhost` / `127.0.0.1`, or matches a `-allow-url-prefix`. Relative `/api/v1/*` calls are clean and never flagged.
- **Fix:** prefer a relative `/api/v1/*` call, or add the prefix to `-allow-url-prefix` if it is genuinely core.

### 3c · Cloud SDK in a surface package — ERROR

- **Detects:** an import under a `surface/` or `surface-ui/` path of `github.com/aws/aws-sdk-go`, `cloud.google.com/go`, `github.com/Azure/azure-sdk-for-go`, `github.com/stripe/stripe-go`, `github.com/google/go-github`, or `github.com/slack-go/slack`.
- **Fix:** move the call to `integration-<provider>`; expose results via `observe_` / `discover_` capabilities; the surface consumes that data over `/api/v1/*`.

---

## Invariant 4 — Multi-tenant aware (`multi-tenant`)

> **Contract §3.4:** A surface respects `integration_instance` scoping; tenant boundaries are honored on every data call.

### 4 · Unscoped core fetch — WARN

- **Detects:** a non-config TS/JS line referencing `/api/v1/*` that is **not** scoped and **not** exempt. A line is scoped if it contains `/api/v1/integrations/`, `integration_instance`, `instanceId`, `instance_id`, `useInstance`, `useSurfaceQuery`, or `useYggdrasilAPI`. Comment lines, Vite/build configs, and read-only core paths are skipped.
- **Exempt read-only core paths** (RBAC-scoped at the handler, not the URL): `/api/v1/auth/`, `/console/`, `/heimdall`, `/readyz`, `/healthz`, `/ops/`, `/integration-surfaces`, `/integration-catalog`, `/manifests`, `/manifest-`, `/workflow`, `/teams`, `/tenant/`, `/me/`.
- **Fix:** use `surface-toolkit` `useSurfaceQuery` / `useInstance`, or include `instanceId` (or `integration_instance`) in the path or query.

---

## Invariant 5 — Federated deployable / Lego (`lego`)

> **Contract §3.5 / §6.3:** A surface deploys into any federation. It does NOT hardcode a specific cloud / CDN / auth-provider URL at runtime.

### 5 · Hardcoded cloud host — WARN

- **Detects:** a cloud-specific host matched by the lego pattern — `*.amazonaws.com`, `*.googleapis.com`, `*.azure.com`, `cloudfront.net`, `cloudflare.com`, `akamai.net`, `fastly.net`, and the `s3.*.amazonaws` / `ecr.*.amazonaws` / `dkr.ecr.*.amazonaws` forms — in TS, Go, or a manifest.
- **Exemptions:** files under `deploy/`, `.github/`, or named `Dockerfile*` / `docker-compose*` / `buildspec*` are build/CD artifacts and skipped — **except** `*.manifest.json` / `*.manifest.yaml` / `*.manifest.yml`, which are runtime artifacts and are always checked. A line carrying `// ok:` is suppressed.
- **Fix:** make the URL operator-configurable (env var or an operator-supplied manifest field). Keep cloud pins in `deploy/` or `.github/`.

---

## Invariant 6 — No business decision authority (`no-business-decisions`)

> **Contract §3.6 / §8:** A surface dispatches and visualizes; it does NOT decide business rules. This is hard to lint statically — the check is deliberately narrow and manual review (§8) is still required.

### 6 · Local monetary decision — WARN

- **Detects (narrow heuristic):** within a 3-line window in a non-test TS/JS file, either a control-flow statement gating a monetary verb — `if (…<>=!… ) { … charge | refund | debit | credit | chargeback | capture | deduct … }` — or a hand-rolled boolean `canCharge` / `canRefund` / `shouldCharge` / `shouldRefund` / `isAllowedTo*` / `authorize(Charge|Refund|Debit)`. Comment lines and **canonical dispatch lines** (a `/api/v1/*` `fetch`/`axios`/`request`/`useQuery`/`useMutation`) are skipped. At most one finding per file.
- **Fix:** move the rule into an integration capability or workflow; the surface should POST the request and render the returned outcome. Example from the `bad-surface` fixture: `if (balance > amount) { return "charge"; }` decides locally — dispatch instead.

> The heuristic is intentionally conservative to keep false positives near zero. A clean run here is **not** proof of compliance — complete the §8 self-test by hand.

---

## How exemptions compose

| Mechanism | Scope | Use when |
|---|---|---|
| `-skip=<id>` | Disables an entire invariant for the run | The invariant is N/A for this surface (document why). |
| `-allow=<glob>` | Excludes paths from all checks | Vendored / generated / sandbox code. |
| `-allow-url-prefix=<prefix>` | Adds a core-canonical URL prefix (invariant 3) | A genuinely-core API outside `/api/v1/*`. |
| UX-only token (invariant 1c) | Suppresses a single storage write | The key is UX state (rename to `draft`, `theme`, …). |
| `// ok:` comment (invariant 5) | Suppresses a single line | A cloud host that is provably build-time only. |
| Build/CD path (invariant 5) | `deploy/`, `.github/`, `Dockerfile*`, `docker-compose*`, `buildspec*` | Cloud pins that never run in the surface. |
| Read-only core path (invariant 4) | The exempt-segment list above | Endpoints scoped by RBAC at the handler, not the URL. |
