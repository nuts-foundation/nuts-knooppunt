# GF Sandbox

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

For the containerized variant:

```shell
docker compose --profile sandbox up
```

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

Datastar's event-attribute grammar is `data-on:click` (colon-separated), not `data-on-click` — verified against the
vendored v1.0.2 bundle. Keep this form when adding interactivity.

## Integration points

- `page.Session` and the `topbar` partial (E2).
- The reset stub, `POST /demo/reset` (E5).
- The `#gf-viewer-steps` container and `window.GFJourney.apply(stepEvent)` / `.reset()`, consuming the DESIGN.md §7
  step-event schema (E6).
- The `GF_STEP` mapping in `static/js/journey-model.js` is provisional and owned by E6 from here.

**E6 carry-forward**, so the gap between what is wired today and what animates is explicit:

1. The `GF_STEP` mapping in `static/js/journey-model.js` is provisional — E6 owns live journey semantics from here.
2. The `.js-lock.unlocked` rules (`viewer.css`) and the `body[data-s="4"]` lock stagger delays are intentionally
   inert until E6 lands lock-unlock semantics: no JS shipped today toggles the `unlocked` class.
3. All `body[data-s="6"]` withdrawal-scene rules (key-hand dimming, retrieved-record removal, the Mitz shield
   flipping to deny) are unreachable today because `GF_STEP` in `static/js/journey-model.js` tops out at 5
   (`exchange`), so `document.body.dataset.s` never reaches `"6"`. The consent-withdrawal scene mapping is E6/E8
   work.

## Provenance

- Datastar v1.0.2, vendored at `static/vendor/datastar.js`, from
  `https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js` (MIT license). The vendored file
  retains its trailing `//# sourceMappingURL=datastar.js.map` line; the map file itself is intentionally not
  vendored, so devtools may log a harmless 404 for it when a browser requests the source map.
- Fonts Fraunces, Inter, Fira Sans and Fira Mono are vendored from Google Fonts (SIL Open Font License).
