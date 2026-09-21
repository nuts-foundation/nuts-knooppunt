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
# has to have run before the sandbox starts. Start the node and trace collector first.
docker compose -f docker-compose.yml -f docker-compose.sandbox.yml --profile sandbox up -d sandbox-otel-collector knooppunt
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

### Reloading Knooppunt while the demo is running

After recreating the Knooppunt container, reload both PEP proxies once the node is ready.
Their [upstreams](../../pep/nginx/conf.d/knooppunt.conf) can retain the previous container address.
This makes token introspection fail with `connect() failed (111: Connection refused)` and turns
the source's patient search into HTTP 500 even though each container's health check passes.

```shell
docker compose -f docker-compose.yml -f docker-compose.sandbox.yml --profile sandbox exec -T pep-zonnebloem nginx -s reload
docker compose -f docker-compose.yml -f docker-compose.sandbox.yml --profile sandbox exec -T pep-plataan nginx -s reload
```

Verify an actual source retrieval after the reload; share and lookup do not exercise the PEP.

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
| `SANDBOX_EVENT_IDENTIFIERS` | `masked` | BSN visibility in step events. `synthetic` reveals only identifiers in the fixed demo pool, and startup refuses it unless `SANDBOX_PUBLIC_URL` names localhost or a loopback IP. Shared deployments must use `masked`. Other values are rejected. This setting applies to event capture, not the existing synthetic patient record screens. |
| `KNOOPPUNT_INTERNAL_URL` | `http://localhost:8081` | the knooppunt's internal mux. The backend reaches the Nuts node through it (proxied under `/nuts`) for the token request, and the same address enables reset/recycle when `HAPI_BASE_URL` is set too. One variable because it is one address: it was spelled `NUTS_INTERNAL_BASE_URL` here and `KNOOPPUNT_INTERNAL_URL` for reset, and `NUTS_` is the node's own configuration prefix, so that name read as node config the node never sees |
| `SANDBOX_NUTS_SUBJECT` | `plataan` | the Nuts subject the token is requested for. Must name the subject `sandbox/bootstrap-nuts.sh` creates, whose wallet holds the `X509Credential` |
| `SANDBOX_BGZ_SCOPE` | `bgz` | the scope requested. Must be a key in the definition the node loads, which the sandbox renders to `sandbox/.certs/policy/bgz.json` from `sandbox/policy/bgz.json.template`, or the node answers `invalid_scope` |
| `SANDBOX_FACILITY_TYPE` | `Z3` | the facility type asserted in the organization context credential, the only thing on this path that carries one |
| `SANDBOX_NVI_CLIENT_ID` | `gf-sandbox-plataan` | `List.source.identifier` on published localization records. Synthetic: no NVI OAuth client is registered for the demo, and this is not the `gf-sandbox` client id used on the Dezi flow. The client id is the scope a registration is deleted by, so recycle and the global reset receive this same value in `vectors.SandboxTarget` rather than reaching for the compiled-in default; overriding it here therefore also moves what those clean up. |
| `MITZMOCK_URL` | unset | Base URL of the mock Mitz. Enables subscription reconciliation and the consent-subscription cleanup in reset and recycle. Unset means the share flow reports an unreconciled Mitz error as unknown rather than failed, and both cleanup paths report a partial restore rather than a clean one. The sandbox compose overlay sets it; the base `--profile sandbox` invocation does not. |

The public and internal URLs are separate on purpose. Under compose the browser cannot resolve the
`mock-dezi` service name, and the sandbox container resolving `localhost` would reach itself.

Of the four Nuts settings, `docker-compose.yml` overrides only `KNOOPPUNT_INTERNAL_URL`, to
`http://knooppunt:8081`. It is the only one this topology changes; the other three already default to
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

`POST /demo/ehr/patients/{key}/open` reserves a patient for the session. A successful switch releases
the session's previous holdings; a refused switch preserves them. GET views do not take locks, and
share and subscribe check ownership when each request begins. Opening the same patient resumes its
run and renews its 15-minute lease. Completed calls made by that run also renew the lease; an idle
viewer connection does not. One-patient-per-session describes this open/switch flow: the manual
Lock control can acquire several patients. Removing a session releases its holdings and clears its
events. Run ownership is derived from the signed-in session, never a client-supplied owner field.

One more is inherited rather than chosen. The Knooppunt's `component/mitz` reports a subscription as
created when its request to Mitz fails without a parseable `OperationOutcome` (a dial failure, a
deadline, a gateway 502, an empty 401): it answers 201, the share screen renders "Consent subscription
started", and nothing on this side can tell the difference. `main` records the quirk in a `NOTE` in
`CreateSubscription`; the fix belongs in its own PR against that component.

## Retrieval (E4)

Retrieving a patient summary has four logical steps across two server-rendered actions. Opening
Retrieve data discovers sources (localization and addressing); confirming one source requests access
and retrieves data. The E6 viewer gives each action its own entry in the run's timeline.

1. **Localization.** The NVI is searched for every localization record about this patient, not just De
   Plataan's, and the results are grouped per custodian. Our own registration is dropped: it describes
   the record already on screen.
2. **Addressing.** Each remaining custodian is resolved in the mCSD query directory with
   `Organization?identifier=ura|{URA}&_include=Organization:endpoint`. The Knooppunt syncs that
   directory but exposes no query API, so the sandbox reads the replica directly, which is what the
   Addressing spec has a Query Client do.
3. **Authentication.** An access token is requested from the *source's* authorization server, with the
   session's Dezi attestation as `id_token`. The requester stays De Plataan's subject, which is the
   wallet holding the credential the source's presentation definition asks for.
4. **Retrieval.** The Patient search both opens the BGZ and resolves the source's own id for this
   patient, which every later search is scoped to. Which categories are asked for comes from the NVI;
   which query retrieves one comes from BGZ 2017, which is what `component/pdp/policies/bgz/policy.rego`
   enforces. A refusal is rendered as a refusal, never as an error.

Nothing is retrieved before the practitioner confirms a source. The server keeps one source selection
per patient run for five minutes, including the discovered categories, data address and authorization
server. Confirmation sends an opaque selection ID and URA; both must match that run's saved result.
No addresses or categories from the browser are trusted. Confirmation does not repeat discovery or
query unused sharing status. The source still decides access on each actual data request.

A fresh discovery replaces the old selection, including forms still open in another tab.
Expiry, release, a patient switch, session end or recycle
makes the old selection unusable. An invalid confirmation returns HTTP 409 with a Refresh source list
link and performs no discovery, token request or source query. Discovery finishing after its run has
ended cannot populate a replacement run.

The first source query is a `POST /Patient/_search`, a [FHIR search](https://hl7.org/fhir/R4/http.html#search)
with the patient identifier in its body to keep it out of the URL. It establishes the source's local
patient reference. Subsequent GET searches use that reference for the localized clinical categories.
Both the POST and the GETs send the access token in the Authorization header.

Six limitations are carried deliberately:

- **Endpoint selection matches `connectionType` but not `payloadType`.** It honours `status` and
  `period` at the precision FHIR dateTime allows, both SHALLs. An `oauth-nuts` Endpoint is
  the authorization server, and a data endpoint has to carry one of two codings: the spec's
  `http://terminology.hl7.org/CodeSystem/endpoint-connection-type|hl7-fhir-rest`, or
  `http://fhir.nl/fhir/NamingSystem/endpoint-connection-type|fhir`, which is what this repo seeds and
  is in neither GF value set. Every other kind is ignored rather than treated as a FHIR base.
  `payloadType` is not matched at all: the seeded endpoints carry none, although the profile makes it
  `1..*`, so matching on it would reject every data endpoint in this demo. That cuts both ways. It
  recognizes fewer kinds of service than the spec, and it will also accept a FHIR endpoint serving a
  payload this retrieval cannot use, because nothing here reads what an endpoint says it serves. Where
  several endpoints qualify it takes the first, and an unreadable `period` bound reads as no bound.
- **The sub-check breakdown is narration.** The authorization specification makes the decision a single
  `allow` boolean and everything else informational, and the sandbox talks to the source's PEP, which
  answers with a status and nothing else. The verdict and the per-query statuses on that screen are
  real; the four checks above them describe what the chain carried, and the screen says so.
- **Retrieved data lives in memory, scoped to the session that fetched it.** It is discarded when that
  session ends, along with its locks, and a retrieval that finishes after its session ended is not
  stored at all. One window remains: a session that expires and is never looked at again keeps its
  data until the next read or sign-in sweeps it, because the store has no timer of its own.
- **Paged results are followed to a bound.** A search that offers more than twenty pages stops there
  and the row says the result is incomplete, rather than rendering a partial record as a whole one.
- **A searchset's own warning is not surfaced.** FHIR lets a server put an `OperationOutcome` in a
  searchset with `search.mode=outcome` to report that a result is partial or that a parameter was
  ignored (https://hl7.org/fhir/R4/search.html#errors). Those entries are filtered out along with
  everything that is not the resource the search asked for, so a 200 carrying one renders as a complete
  answer. Reading them would mean deciding which outcomes make a result incomplete and which are
  informational, which is a question this build has not settled.
- **Continuation links are bounded, transport errors are not sanitized.** `continuationProblem` refuses
  a next link that cannot be parsed, carries userinfo, points at another origin, or says the BSN in any
  of its decoded components, so a request line this application composes cannot carry the identifier.
  What is not covered is what a source puts in a response: Go parses a `Location` header before
  `CheckRedirect` runs, so an unparsable redirect that echoes the BSN produces an error naming it, and
  that text is stored and rendered. It needs a source that both echoes the identifier into a redirect
  and makes that redirect malformed. Mapping transport errors to fixed messages would close it, at the
  cost of the diagnostics this screen exists to show.

Both token requests read their authorization server from the directory: the retrieval under the
source's URA, `POST /demo/authorize` under De Plataan's own. Which wallet the request is made from is a
separate question, answered by `SANDBOX_NUTS_SUBJECT` in the path of the internal call, so the two
differ here: the wallet is `plataan` while the server published for De Plataan's own data is the one
under `00000010`.

Editing the seeded directory takes a re-seed and a `POST /mcsd/update` before the sandbox sees it. The
sandbox's own reset reloads the fixtures without touching the query directory; compose's `init` does
both. A seed run reports success on HTTP 200 without reading the update report, and a per-directory
failure can sit inside a 200, so "seed complete" is not proof that an endpoint reached
`knpt-mcsd-query`.

## Architecture

A standalone Go binary, not a knooppunt component (DESIGN.md standing decision 5: this backend will be the only
caller of the knooppunt internal mux, from E2/E4 on). Server-rendered `html/template` plus `go:embed`, following the
`component/mcsdadmin` pattern. No build step, no npm.

## Step events (E6)

Opening a patient creates an opaque run ID; reopening it in the same signed-in session resumes that
run. Patient record, share, subscription and retrieval calls carry it in their request context. The
patient list, pre-patient sign-in and standalone authorization demonstration have no patient run, so
their calls do not appear in these streams.

The backend observes each outgoing HTTP call through a transport wrapper. NVI calls use an explicitly
injected client, including the individual search, delete and create requests in registration. Each
completed call produces the framework-neutral JSON shape in [DESIGN.md §7](../DESIGN.md#7-step-events-the-shared-substrate).
Sequence numbers start at 1 and increase in publication order, including calls completing concurrently.
`ts` is the UTC completion time and `durationMs` includes response consumption. HTTP 401/403 mean
`deny`; other unsuccessful statuses or transport/read failures mean `error`. A successful HTTP status
means the call completed; it does not assert a clinical outcome or validate the response's semantics.

Each page action carries an opaque `actionId` and an allowlisted `action` name. Calls additionally
have their own `callId`, a `purpose` (such as `status`, `registration-cleanup` or `registration`),
and, for registration and source queries, `resourceType`. Thus three `POST /nvi/List` requests can
be shown as the three resource categories actually registered. Source query continuation pages keep
their original resource type, even when the next-page URL does not name it. Repeated status reads
remain distinct requests.
PRS spans use `parentCallId` to identify the NVI call that caused them, inherit its action metadata,
and have `actor: "knooppunt"` and `gf: "pseudonym"`. Their timestamps describe the observed call;
their sequence numbers still describe publication, which can occur later due to trace batching.

Capture constructs only allowlisted projections: normalized methods/paths, recognized media types,
bounded counts, known enum values, resource types and selected structured identifiers. Headers other
than `Accept` and `Content-Type` are omitted, as are tokens, cookies, credentials, arbitrary error text,
URL hosts/userinfo, unknown query parameters, resource IDs and clinical free text. BSNs are masked
unless the explicit local synthetic mode applies. Bodies are inspected up to 64 KiB; unknown,
malformed, oversized or incompletely read bodies are null. Request bodies without `GetBody` are also
null: the [current FHIR client recreates its requests](https://github.com/SanteonNL/go-fhir-client/blob/v0.6.1/client.go#L235), so this includes real NVI search and registration
request bodies. Response bodies remain available for sanitized projection. Capture never consumes a
body on behalf of its real caller or changes the bytes that caller receives.

A fully consumed, successful service-access-token response with a nonempty `access_token` sets
the optional `response.tokenReceived` flag. Only this boolean is retained; the token value is never
included in an event. A status of 200 alone does not establish that a key was received.
The token client consumes the complete response before decoding, including chunked responses.
An outgoing source request carrying a nonempty Bearer credential sets `request.tokenAttached`.
Only its presence is retained; the Authorization value never enters event headers or bodies.

The in-memory window contains at most 32 runs, 256 events per run and 16 KiB per serialized event.
An event exceeding that byte cap loses its body projections; if its remaining metadata still exceeds
the cap, it is dropped. Runs expire after 15 minutes without captured activity, or at session expiry,
whichever is earlier. Expired entries are pruned on store access; an open stream wakes at expiry.
Delayed PRS evidence also counts as captured activity and can renew a still-live run's lease.
Its correlation expires five minutes after the originating request starts; it cannot revive an
expired run or extend a session's lifetime.
At the run limit, a new patient open returns 503; switching an existing session's patient still works.
State is process-local and does not survive a restart or move between replicas.

Switching patients, releasing a lock, ending a session, recycling a patient or resetting the sandbox
clears affected runs and wakes their streams. Reset/recycle clear events even if restoring data
reports a failure. Calls already in flight cannot recreate a deleted run. Locks remain advisory;
capture and replay do not add transactional isolation to registration or reset.

`GET /demo/runs/{runId}/events` authorizes the owning session and serves `text/event-stream` with
`Cache-Control: no-store` and proxy buffering disabled. Anonymous requests return 401; a missing,
expired or another session's run returns 404. Cross-origin browser requests are refused.

```text
event: step
id: <runId>:3
data: {"runId":"<runId>","seq":3,"gf":"localization",...}

```

A connection without a cursor replays the retained window. A reconnect's `Last-Event-ID: <runId>:3`
continues after event 3. Malformed, foreign-run and future cursors return 400. If older events were
discarded, `event: replay-gap` with `data: {"firstSeq":17}` precedes the retained window; the viewer
clears its partial history and shows the gap. An initial replay and each batch of new steps end with
`event: snapshot` and `data: {"lastSeq":17}`; this has no SSE ID and marks the delivery boundary for
the viewer. `event: run-ended` closes an active stream when its run
ends. These are transport control records, separate from GF step records. Fifteen-second keepalive
comments do not renew the run lease. Each write has a 10-second deadline, cleared while idle, so a
slow browser does not retain an unbounded event queue. SSE framing and automatic reconnection follow
the [WHATWG EventSource contract](https://html.spec.whatwg.org/multipage/server-sent-events.html).

`static/js/step-events.js` uses native EventSource and text nodes, deduplicates sequences, and rejects
another run's events. Refresh uses the run ID embedded in the server-rendered patient page. The
Functional/Technical toggle remains a Datastar attribute; the event consumer has no Datastar dependency.
The timeline has one entry per user action, with its timestamp and observed outcome. Selecting an
entry replays that action; Replay and Skip stay with that selection even if new evidence arrives.
Follow latest releases the selection and resumes automatic playback of new actions. Evidence that
arrives during a selected replay remains inspectable and is included on its next replay.
Functional mode shows ordered stages. Repeated sharing separates checks, removals and registrations;
a returned Subscription resource is labeled as an existing Mitz subscription. Technical mode nests
all of the action's captured calls, with separate request and response sections. Internal PRS calls
sit under their parent NVI request, and background status reads are collapsed.
When playback advances, the active stage scrolls into view inside the timeline. Switching modes or
reopening the viewer follows the same stage. Ordinary event updates and pause/resume leave the scroll
position alone, so details can be inspected between steps.

The journey map plays observed stages at about 2.5 seconds each, identifies the selected action and
current step, and uses the same anchored path for the outgoing request and returning HTTP response.
An observed error response returns with its status; a transport failure has no invented return.
Missing bodies are labeled as not captured. A Mitz subscription acknowledgement is not a consent
decision. This is a presentation clock: backend calls remain fast and displayed request durations
remain their measured durations. Pause/Resume, Replay and Skip operate on retained events and do not
repeat requests. Playback pauses when the viewer is closed or the document is hidden, respects
reduced motion, and remembers an opaque cursor within the tab. Refreshing therefore does not
automatically animate the entire retained history again.

The functional map places Plataan's access service and Sunflower's authorization server inside
their respective organization boundaries. The access request depicts the configured source issuer;
Technical mode retains the actual HTTP call to Plataan's local Nuts service. A returning key requires
captured token-receipt evidence. The source vault opens separately, only for successful source
queries with captured Bundle responses; refused, failed or unestablished responses keep it closed.
The displayed access requirements describe the demo's contract. They are not individual check
results: the viewer does not receive evidence of each internal policy decision.

Discovery is labeled Find data sources. Confirmed retrieval starts with Request access key, then
Find patient at Sunflower and separate allergy, condition and medication searches as observed.
Each source request with captured key-attachment evidence carries the key icon on its outgoing
packet. Technical mode explains the patient POST search and shows the authorization-presence
metadata alongside the exact method, path and sanitized body. The icon never follows merely from
an earlier token response; its evidence belongs to the outgoing request itself.

PRS motion requires an actual correlated span. The map does not infer individual policy decisions
from successful HTTP responses. PEP/PDP and inbound Mitz notifications still require their own
producers.

### Private PRS trace receiver

The sandbox Compose overlay adds a pinned OpenTelemetry Collector, keeps forwarding logs/traces to
Aspire and also sends traces to the sandbox's private listener. Only recognized PRS client spans from
the configured service and endpoint, with a trace generated for a live patient run, become viewer
events. The retained projection contains method, fixed path, result, status and duration; it excludes
bodies, arbitrary attributes, span errors, tokens and identifiers. Duplicate spans emit once. Missing
or delayed telemetry never changes the actual request result; the viewer allows up to one second for
initial internal evidence and displays unavailable evidence explicitly.

Receiver configuration is separate from the public application listener:

- `SANDBOX_OTLP_LISTEN_ADDR`: private listener, `:4318` in Compose. No host port is published.
- `SANDBOX_OTLP_TOKEN`: shared ingest token, required by the receiver and Collector.
- `SANDBOX_PRS_URL`: PRS base URL to recognize, `http://mock-prs:8080` in offline Compose.
- `SANDBOX_TRACE_SERVICE_NAME`: expected trace service, default `nuts-knooppunt`.

The receiver is disabled when these variables are unset. Partial configuration fails startup. Its
`POST /v1/traces` endpoint accepts authenticated OTLP HTTP protobuf with a 1 MiB request limit and
at most 1,024 spans. It retains up to 512 request correlations for five minutes and up to 128 span
identities per correlation; unrelated or stale spans are ignored. A cleared run cannot be recreated.
The sandbox overlay sets the Go SDK's `OTEL_BSP_SCHEDULE_DELAY=250` to deliver spans promptly.

The default Compose ingest token is for the local demo only. For a hosted deployment, supply a fresh
token through a Kubernetes Secret, set the actual PRS acceptance base URL and match the Knooppunt's
`service.name`. Route the Collector to a private sandbox Service on port 4318; expose only the app's
8091 port through ingress. The current Helm chart does not provision this additional receiver Service
or Collector: configure those private resources and environment variables with the deployment overlay.
Keep the existing Aspire exporter alongside the viewer exporter. The Collector's bounded queues and
short export timeouts keep viewer outages out of the request path.

Run `go test ./sandbox/...` for the sandbox tests (Docker is required for acceptance tests),
`go test -race ./sandbox/app -run 'TestRunStore_|TestEventHTTP_|TestCapture|TestTraceBridge'` for event concurrency,
and `node --test sandbox/app/step-events.test.mjs` for the browser adapter. The NVI helper has a nested
module: run `go test ./vectors/nvi` from `test/testdata`.

The browser adapter tests need Node.js 22.7+ (or 20.19+ on the 20.x release line).
They load the browser's `.js` modules using Node's default syntax detection,
enabled in [22.7](https://nodejs.org/download/release/v22.7.0/docs/api/packages.html#syntax-detection)
and [20.19](https://nodejs.org/download/release/v20.19.0/docs/api/packages.html#syntax-detection).
No npm installation is needed; Node is only used for these tests.

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
the same SSE contract. Fallback triggers are listed in DESIGN.md §3 (bespoke client JavaScript growth,
duplicated client state, fragile SSE reconnect/resume behavior, or the stateful viewer/forms work slowing down
materially).

Datastar's event-attribute grammar is `data-on:click` (colon-separated), not `data-on-click`, as verified against
the vendored v1.0.2 bundle. Keep this form when adding interactivity.

## EHR component library

See [EHR components](COMPONENTS.md) for the shared shell, layout and component
contracts, responsive behavior, and the browser regression check. New EHR screens
should compose these primitives and partials.

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
  `HAPI_BASE_URL` are set (see `docker-compose.yml`), otherwise reset reports "disabled". The E3 patient
  open/switch route reserves the patient; the manual controls remain available for reset demonstrations.
- Retrieval (E4, landed): `GET /demo/ehr/patients/{key}/retrieve` renders where the data can be
  found, `POST` the same path runs the confirmed chain and renders the authorization result. Both
  need a session; the POST additionally needs this session to hold the patient's lock.
- The `#gf-viewer-steps` container and `window.GFJourney.apply(stepEvent)` / `.reset()`, consuming the DESIGN.md §7
  step-event schema (E6).

The wireframe's scene choreography is retained in CSS for later producers. E6 activates only observed
call highlights: refusal is pink and failure amber, applied to the call concerned. It does not infer a
consent refusal from an unrelated 403, animate unobserved interior calls, or advance the withdrawal scene.

## Provenance

- Datastar v1.0.2, vendored at `static/vendor/datastar.js`, from
  `https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js` (MIT license). The vendored file
  retains its trailing `//# sourceMappingURL=datastar.js.map` line; the map file itself is intentionally not
  vendored, so devtools may log a harmless 404 for it when a browser requests the source map.
- Fonts Fraunces, Inter, Fira Sans and Fira Mono are vendored from Google Fonts (SIL Open Font License).
