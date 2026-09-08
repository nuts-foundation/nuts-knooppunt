# GF Sandbox

> [!WARNING]
> **Use at your own risk. This sandbox is not held to the standard of the rest of this repository.**
>
> Most of the code, tests and documentation under `sandbox/`, together with the mock components this
> demo runs against, was written by AI models and has had only light human review. It exists to show
> the flow end to end, not to be depended on.
>
> Concretely: the demo PKI is throwaway, generated on your machine and never committed, and two of its
> private keys are left readable by every user account on that machine, because the containers that
> need them run as fixed non-root UIDs and cannot read them otherwise. Several trust decisions are
> weaker than production would allow, including an organization context credential that is self
> asserted and bound to nothing, and a practitioner authenticated for one organization that is not
> rejected when the token names another. And the documentation may be wrong in places nobody has
> checked yet.
>
> The primary knooppunt code, meaning everything outside `sandbox/` and `mock-components/`, is reviewed
> to the project's normal standard. Do not read the quality of one as evidence about the other, and do
> not copy anything from here into a real deployment.

## What this is

The GF Sandbox application (shell + Plataan EHR) for the `/demo` release: it demonstrates the Generieke Functies
(GF) for Dutch healthcare data exchange on top of the Nuts Knooppunt. `sandbox/DESIGN.md` is the design;
`sandbox/wireframe.html` is the visual source of truth for style, layout and copy.

## Run

```shell
go run ./sandbox/app
```

From the repo root, serves on `http://localhost:8091` (the `PORT` environment variable overrides the port).

```shell
go test ./sandbox/...
```

For the containerized variant, generate the demo certificates first. Without this step, Docker
creates empty directories at the bind-mount paths and mock-dezi fails to start
(`mock-components/dezi/README.md` documents why the TLS listener exists).

```shell
./sandbox/generate-demo-certs.sh     # once, writes to the gitignored sandbox/.certs/

# The node has to be listening before the bootstrap can reach it, and the bootstrap
# has to have run before the sandbox starts, so the node comes up on its own first.
docker compose -f docker-compose.yml -f docker-compose.sandbox.yml --profile sandbox up -d knooppunt
./sandbox/bootstrap-nuts.sh
docker compose -f docker-compose.yml -f docker-compose.sandbox.yml --profile sandbox up
```

`sandbox/bootstrap-nuts.sh` runs on the host, not in a container: it needs bash, python3 and the
didx509 toolkit through Docker, with the certificate paths resolved on the host. The overlay's
`nuts-bootstrap-healthcheck` service enforces the ordering rather than trusting it. It waits for the
knooppunt, then for the `plataan` wallet to hold a credential, and `gf-sandbox` starts only once it
has exited successfully. Skip the bootstrap and that service fails after a minute with the command
to run, which `docker compose logs nuts-bootstrap-healthcheck` shows. Without the gate the sandbox
would start and the authorization route would fail on an empty wallet, which reads as a policy or
certificate problem rather than a missing step.

### Rotating the demo PKI

Regenerating the certificates means clearing the Nuts volume as well, and neither the bootstrap nor
the healthcheck will tell you so. Both check only that the wallet holds *an* `X509Credential`, not
that it holds one issued by the CA on disk now. So a rerun after a rotation reports that there is
nothing to do while the wallet still presents a credential from the chain that no longer exists, and
the presentation definition has meanwhile been re-rendered to pin the new CA. The demo then fails at
the token request, naming the node rather than this.

```shell
docker compose -f docker-compose.yml -f docker-compose.sandbox.yml --profile sandbox down -v
rm -rf sandbox/.certs
```

Then run the four commands above again. `down -v` is what removes the credential; without it the
stale one survives and nothing between here and the token request notices. Making the bootstrap
compare the wallet credential's own CA fingerprint against `sandbox/.certs/ca.pem`, so that it
re-issues instead of reporting success, is the durable fix and is not implemented.

The overlay carries the knooppunt settings the sandbox needs: the demo CA and the Dezi JWK Set
allowlist. They live there rather than in `docker-compose.yml` because the knooppunt service is
not profile-gated, so anything set on it would also apply to a plain `docker compose up`. The
allowlist matters in particular: configuring it replaces the built-in production and acceptance
defaults instead of extending them.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8091` | listen port |
| `DEZI_PUBLIC_AUTHORIZE_URL` | `http://localhost:8092/authorize` | where the **browser** is sent |
| `DEZI_INTERNAL_BASE_URL` | `http://localhost:8092` | where the **backend** calls token and userinfo |
| `SANDBOX_PUBLIC_URL` | `http://localhost:8091` | the URL the browser reaches the sandbox on. Builds the redirect URI, and its scheme decides whether the session cookie carries `Secure`. A hosted deployment behind a TLS-terminating proxy must set this to its `https://` URL: the request arriving at this process is plain http, so nothing else here can tell that the browser used TLS |
| `KNOOPPUNT_INTERNAL_URL` | `http://localhost:8081` | the knooppunt's internal mux. The backend reaches the Nuts node through it (proxied under `/nuts`) for the token request, and the same address enables reset/recycle when `HAPI_BASE_URL` is set too. One variable because it is one address: it was spelled `NUTS_INTERNAL_BASE_URL` here and `KNOOPPUNT_INTERNAL_URL` for reset, and `NUTS_` is the node's own configuration prefix, so that name read as node config the node never sees |
| `SANDBOX_NUTS_SUBJECT` | `plataan` | the Nuts subject the token is requested for. Must name the subject `sandbox/bootstrap-nuts.sh` creates, whose wallet holds the `X509Credential` |
| `SANDBOX_BGZ_SCOPE` | `bgz` | the scope requested. Must be a key in the definition the node loads, which the sandbox renders to `sandbox/.certs/policy/bgz.json` from `sandbox/policy/bgz.json.template`, or the node answers `invalid_scope` |
| `SANDBOX_AUTH_SERVER` | `http://localhost:8080/nuts/oauth2/plataan` | the authorization server the token is requested from. It has to satisfy two requirements at once: equal the issuer the node advertises, and be reachable **by the node**, which fetches `/.well-known/oauth-authorization-server` from it before requesting a token (nuts-node `auth/client/iam/openid4vp.go`, `RequestRFC021AccessToken` to `AuthorizationServerMetadata`). Both hold here because the node dials it from inside its own container, where port 8080 is its own public listener. A split deployment has to find one address that satisfies both |
| `SANDBOX_FACILITY_TYPE` | `Z3` | the facility type asserted in the organization context credential, the only thing on this path that carries one |
| `SANDBOX_NVI_CLIENT_ID` | `gf-sandbox-plataan` | `List.source.identifier` on published localization records. Synthetic: no NVI OAuth client is registered for the demo, and this is not the `gf-sandbox` client id used on the Dezi flow. The client id is the scope a registration is deleted by, so recycle and the global reset receive this same value in `vectors.SandboxTarget` rather than reaching for the compiled-in default; overriding it here therefore also moves what those clean up. |
| `MITZMOCK_URL` | unset | Base URL of the mock Mitz. Enables subscription reconciliation and the consent-subscription cleanup in reset and recycle. Unset means the share flow reports an unreconciled Mitz error as unknown rather than failed, and both cleanup paths report a partial restore rather than a clean one. The sandbox compose overlay sets it; the base `--profile sandbox` invocation does not. |

The public and internal URLs are separate on purpose. Under compose the browser cannot resolve the
`mock-dezi` service name, and the sandbox container resolving `localhost` would reach itself.

Of the five Nuts settings, `docker-compose.yml` overrides only `KNOOPPUNT_INTERNAL_URL`, to
`http://knooppunt:8081`. It is the only one this topology changes; the other four already default to
the values that are correct there. This table describes what the application reads, which is not the
same question as what compose sets.

`POST /demo/authorize` needs the compose stack. It asks the Nuts node for a service access token and
introspects it, and the `go run` path has no node to reach, so there it answers 502 naming the step
that failed. Sign-in, session and every other route keep working locally.

## Patient registration (E3)

Sharing a patient publishes one NVI localization record per data category De Plataan holds for them
(`Patient`, `Condition`, `MedicationRequest`), and starts a Mitz consent subscription.

There is no BGZ code. `List.code` is bound to a value set of data categories at FHIR resource
granularity, so a patient summary is the set of categories it contains, and the confirmation card
names those rather than claiming "BGZ".

Three limitations are carried deliberately:

- **Repeat registration converges, it is not atomic.** The NVI exposes no `PUT`, and the conditional
  operations that would make a transaction Bundle atomic cannot be used: the Knooppunt does not
  pseudonymize `entry.request.url`, so a conditional URL would carry a plaintext BSN, and the fake
  NVI was found not to accept the `:identifier` modifier on a conditional delete
  (recorded in `test/testdata/vectors/nvi`; nothing on this branch re-tests it). Registration is
  search-delete-create, serialized per patient within this process and guarded by the demo lock.
  Two sandbox processes registering the same patient at once can still duplicate.
- **The demo lock is advisory and process-local**, with a 15-minute lease. Ownership is checked when
  a share begins, not held for its duration.
- **Reconciliation is a mock-only affordance.** The "was that subscription actually created?" query
  reads the mock directly; neither the Knooppunt nor the national Mitz offers such a lookup.

## Architecture

A standalone Go binary, not a knooppunt component (DESIGN.md standing decision 5: this backend will be the only
caller of the knooppunt internal mux, from E2/E4 on). Server-rendered `html/template` plus `go:embed`, following the
`component/mcsdadmin` pattern. No build step, no npm.

## Frontend implementation and the fallback contract

Datastar v1.0.2 is the active frontend implementation, vendored at `static/vendor/datastar.js` (source:
`https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js`, MIT license). This is an epic
acceptance criterion, not a style preference.

Datastar may appear only as `data-*` attributes in templates. It must not appear in:

- backend routes
- the step-event schema (DESIGN.md §7)
- `static/css/*`
- `static/js/journey-*.js`

The documented fallback is buildless Preact + htm: server routes, templates, CSS and the journey driver stay, and
the migration replaces the `data-*` interactivity with small Preact + htm islands consuming the same routes and
(from E6) the same SSE contract. Fallback triggers are listed in DESIGN.md §3 (bespoke client JavaScript growth,
duplicated client state, fragile SSE reconnect/resume behavior, or the stateful viewer/forms work slowing down
materially).

Datastar's event-attribute grammar is `data-on:click` (colon-separated), not `data-on-click`, as verified against
the vendored v1.0.2 bundle. Keep this form when adding interactivity.

## Integration points

- Dezi sign-in (E2): `POST /demo/login` starts the flow against `mock-components/dezi`,
  `GET /demo/auth/callback` creates the session, `POST /demo/logout` ends it. `authSession` in
  `session.go` holds the raw attestation that E4 sends to the Nuts node as `id_token`; templates
  only ever see the derived `Session` view model.
- `page.Session` and the `topbar` partial (E2).
- The service access token (E4): `POST /demo/authorize` sends the session's attestation to the Nuts
  node as `id_token` with one self-asserted organization context credential, introspects the token
  it gets back, and requires and renders `user_id`, `user_role`, `organization_ura`,
  `organization_ura_dezi`, `organization_name` and `organization_facility_type`. Compose only; see
  the note under Configuration.

  `organization_ura` comes from the X509 credential in the wallet and `organization_ura_dezi` from
  the Dezi attestation, so the two are independently established and can disagree. Nothing rejects a
  disagreement: a presentation definition filters one path in one credential and cannot compare two,
  and the policy decision point does not make the comparison yet. Both are rendered so the reader can
  make it by eye, which is the only place it currently happens.
- Reset/recycle (E5, landed): `POST /demo/reset` (global, `override=true` past active locks),
  `POST /demo/patients/{key}/recycle` (per-patient, 409 if locked), `POST /demo/patients/{key}/lock` and
  `/release` (manual lock control), and `GET /demo/patients` (pool + lock status JSON). These call
  `vectors.ResetGlobal` / `vectors.RecyclePatient`; they are enabled only when `KNOOPPUNT_INTERNAL_URL` and
  `HAPI_BASE_URL` are set (see `docker-compose.yml`), otherwise reset reports "disabled". The lock trigger on
  scenario-run start is E4/E6; E5 ships the registry, reset-side enforcement and the manual controls.
- The `#gf-viewer-steps` container and `window.GFJourney.apply(stepEvent)` / `.reset()`, consuming the DESIGN.md §7
  step-event schema (E6).

**E6 carry-forward**, so the gap between what is wired today and what animates is explicit:

1. The `GF_STEP` mapping in `static/js/journey-model.js` is provisional: E6 owns live journey semantics from here.
2. The `.js-lock.unlocked` rules (`viewer.css`) and the `body[data-s="4"]` lock stagger delays are intentionally
   inert until E6 lands lock-unlock semantics: no JS shipped today toggles the `unlocked` class.
3. All `body[data-s="6"]` withdrawal-scene rules (key-hand dimming, retrieved-record removal, the Mitz shield
   flipping to deny) are unreachable today because `GF_STEP` in `static/js/journey-model.js` tops out at 5
   (`exchange`), so `document.body.dataset.s` never reaches `"6"`. The consent-withdrawal scene mapping is E6/E8
   work.
4. The wireframe's dock stagger rules (`wireframe.html` lines 679-683: `body[data-s="1"] #js-prs`/`#js-nvi`/`#js-mitz`,
   `body[data-s="3"] #js-pin`, and the `.js-gdelay` transition delays) were not ported with the 354-556 CSS range
   that became `viewer.css`, so step lighting has no choreographed delays until E6 ports them.
5. The driver renders the DESIGN.md §7 `error` outcome the same as `ok` (`journey-strip.js`'s `apply` only
   special-cases `outcome === 'deny'`); the wireframe has no error presentation to port, so E6 must choose a
   deliberate treatment for it.

## Provenance

- Datastar v1.0.2, vendored at `static/vendor/datastar.js`, from
  `https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js` (MIT license). The vendored file
  retains its trailing `//# sourceMappingURL=datastar.js.map` line; the map file itself is intentionally not
  vendored, so devtools may log a harmless 404 for it when a browser requests the source map.
- Fonts Fraunces, Inter, Fira Sans and Fira Mono are vendored from Google Fonts (SIL Open Font License).
