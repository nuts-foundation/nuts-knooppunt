# GF viewer experiment: Datastar vs htmx vs buildless SPA

The sandbox's signature UI element, the GF viewer side panel, built three times with the
same scripted run: the two hypermedia candidates (Datastar, htmx) on the thing we are
unsure about, a server-driven SSE-updated panel, plus a buildless zero-npm SPA (vendored
Preact + htm, JSON over SSE, client-side rendering) to answer the wider question of
server rendering versus SPA. See the framework discussion in Slack and the design in
`../DESIGN.md` (section 4, cross-cutting).

All variants are deliberately identical except for the delivery mechanism:

- `shared/scenario.js` — the scripted run as plain data (steps, cards, timings), the
  server-side `renderItem()` used by both hypermedia variants, and the panel CSS.
- `shared/map.svg.html` + `shared/map.css` — the journey strip, extracted from `../wireframe.html`.
- `shared/map.js` — the framework-free animation driver: it watches exactly one attribute
  (`data-s` on `#viewer`) and translates it into the CSS classes the strip animates on.
  How `data-s` gets set is the only framework-specific part.

Zero npm dependencies in all three variants: the servers use only Node core modules
(`http`, `fs`, `path`), and every framework is vendored as a static file, the way
`component/mcsdadmin` vendors htmx today.

## Run

```
node datastar/server.js   →  http://localhost:3010
node htmx/server.js       →  http://localhost:3011
node spa/server.js        →  http://localhost:3012
```

Each page load replays the ~25s run: share → localize → address → authorize (locks) →
retrieve (vault). "Replay run" is a plain page reload: the server drives the replay. In the
hypermedia variants the browser only holds the projected DOM; the SPA additionally holds
the step and item state in its component tree and derives the DOM from it.

## How each variant works

**Datastar** (`datastar/`, v1.0.2 vendored, 34 KB): the panel subscribes with
`data-init="@get('/run')"`. The server pushes two event types from the
[SSE reference](https://data-star.dev/reference/sse_events):
`datastar-patch-signals` with `{step: n}` (bound to the `data-s` attribute via
`data-attr:data-s="$step"`) and `datastar-patch-elements` with `mode append` for the
server-rendered cards. Framework surface used: 3 attributes, 2 event types.

**htmx** (`htmx/`, htmx 2.0.10 + htmx-ext-sse 2.2.4 vendored, 60 KB): the panel carries
`hx-ext="sse" sse-connect="/run" sse-close="finished"` per the
[SSE extension docs](https://htmx.org/extensions/sse/). Named `card` events append into
`#cards` (`sse-swap="card" hx-swap="beforeend"`). The `step` event needs a hidden sink
element, because the extension discards events without an `sse-swap` target; an
`htmx:sseBeforeMessage` listener then routes the value to the `data-s` attribute.
`sse-close` stops the EventSource from auto-reconnecting and replaying the run.

**SPA** (`spa/`, Preact + htm standalone vendored, 13 KB, buildless): the server is an
API: it streams JSON over plain SSE `message` events and serves static files. The client
(`spa/app.js`) owns the state (step, received items) in a Preact component tree and
renders the cards itself, with `EventSource` wired by hand and a `finished` message to
stop the auto-reconnect. The card markup exists twice in the repo by construction: once
server-side in `shared/scenario.js` (used by the hypermedia variants) and once as a
Preact component; the two must be kept in sync. The journey strip stays vanilla and
outside the component tree; a `useEffect` projects the step state onto its `data-s`.

## Scorecard (first pass, add your own findings)

There are no points or winners in this table: every variant produces the exact same
panel, pixel for pixel. The table compares what that costs. The first four rows are
counted lines of first-party code (lower means less code to maintain for identical
output; the vendored frameworks and the shared map driver are excluded everywhere).
The remaining rows describe how each variant solves the same recurring problems, so
you can judge which trade-offs you prefer.

| Aspect | Datastar | htmx | SPA (Preact + htm) |
| --- | --- | --- | --- |
| Server LoC (file) | 93 | 105 | 90 |
| Server-side renderer used from `shared/scenario.js` | 11 | 11 | 0 |
| Client LoC beyond the shared map driver | 0 | 8 (step listener, inline in the server file) | 54 (`app.js`: components, state, EventSource) |
| Implementation total (unique lines) | 104 | 116 | 144 |
| Framework payload, vendored | 34 KB (1 file) | 60 KB (2 files: core + SSE extension) | 13 KB (1 file) |
| npm dependencies | 0 | 0 | 0 |
| SSE support | core model | extension | hand-wired `EventSource` |
| Signal → attribute | native (`data-attr`) | hidden sink + JS event listener | `useEffect` |
| Append fragments | `mode append` data line | `hx-swap="beforeend"`, works but is not documented for `sse-swap` | client re-render from state |
| Wire contract to keep in sync | names only: the `step` signal and `#cards` selector must match the page bindings | names only: the `step`/`card`/`finished` event names must match `sse-swap`/`sse-close` and the listener | full JSON event schema: the shape of every item, agreed between server and client |
| Reconnect semantics | fetch-based; reopens when a hidden tab becomes visible again (`openWhenHidden` default), replaying this stateless endpoint | EventSource auto-reconnects; needs `sse-close` | EventSource auto-reconnects; needs a `finished` message + `es.close()` |
| Gotchas hit while building | `@get` appends `?datastar=...` query param; match on pathname | events without a DOM target are silently discarded | none new; the reconnect footgun is the same as htmx's |

All three were built the same AI-assisted way: docs fetched first, then generated against them.
Author's log of gotchas hit (anecdote, not a metric): the Datastar `?datastar=` query
param, the htmx discarded-event rule, htmx's undocumented `hx-swap` composition, and one
of our own in Node land (`req.on('close')` never fires for a disconnecting SSE client;
cleanup belongs on the response). A stateless replay endpoint also means any reconnect
or visibility-reopen restarts the run in every variant; a real backend would resume by run id.

Not part of this comparison: forms/modals and the journey map itself (vanilla by design,
identical in all three variants).
