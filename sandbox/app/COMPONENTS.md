# EHR components

The visual reference is [the wireframe](../wireframe.html). Keep its tokens, serif
headings, rounded cards, pill controls, and source/status colors. There is no
frontend build step. The stylesheets and templates are embedded in the Go binary;
rebuild `gf-sandbox` before checking changes in the compose application.

## Assemble a screen

Each EHR template defines `content` as `{{template "ehr-layout" .}}`, then defines
`ehr-content` with its own `<main class="content">`. The shared
[layout](templates/_ehr-layout.html) supplies the demo bar, sidebar, top bar, and
viewer exactly once. Use `content-narrow` for lists and forms (880px maximum);
record and authorization overviews use the full available width (1460px maximum).

[components.css](static/css/components.css) owns content components;
[ehr.css](static/css/ehr.css) owns EHR chrome and login;
[viewer.css](static/css/viewer.css) owns only the dock and journey visualization.
Component rules must not depend on the current route or viewer state.

- `content`: a vertical stack with a 20px gap between page blocks. Add blocks
  directly to it; do not add individual top/bottom margins to imitate page flow.
- `page-header`: one `h1`, with optional `eyebrow` and `h-sub` copy. Combine it with
  `pat-head` and a `pav` avatar for patient identity, or `auth-hero` for a verdict.
- `card`: a neutral surface with 18px/20px padding and 12px internal flow spacing.
  Use a section heading and optional `h-sub`. `card-flush` removes padding and
  flow spacing for a list whose rows supply them.
- `rec-grid`, `confirm-grid`, `action-grid`: grids with 16px gaps. Each column
  prefers at least 240px, wraps when needed, and can shrink below that on phones.
- `actions`: a wrapping row of related controls. `action-group`: one control or
  form with its `btn-sub` explanation, separated by 8px. Put groups in
  `action-grid` for separate actions. `card-action` adds 14px before an action
  inside a card; it is not a page spacer.
- `btn`: shared link/button styling, with `solid`, `ghost`, and optional `big`.
  Text wraps; submit controls remain inside real POST forms.
- `chip-row`: a wrapping group (a `ul` or `div`). `chip` labels categories and
  identifiers; `src` identifies provenance; `stat-chip` identifies status. All
  accept full labels. Color modifiers retain the existing wireframe vocabulary.
- `facts` / `kv`: a semantic `dl` containing `div.kv` rows with `dt` and `dd`.
  Values may contain chips or `mono` text. The label column is 112px; on the
  narrowest layouts the value moves below its label. The shared
  [partials](templates/_ehr-components.html) render patient facts, generic
  result rows, and share result cards.
- `kv-table`: tabular query results with row headers (`th scope="row"`). Long
  request URLs wrap. Below 380px of content width, each value follows its label
  vertically. `claims` retains the same treatment for the existing token screen.
- `prow`: the patient-opening submit button, with avatar, identity, status and
  chevron. At narrow widths status moves below identity. `data-item` is a clinical
  row; its provenance chip wraps with the row. `hl` retains the marker treatment.
- `src-card`: a source summary composed from `src-top`, `chip-row`, `src-line`,
  and `action-group`. `confirm-card` has `failed`, `unknown`, and `skipped` states.
- `auth-hero`: the source verdict, with the `denied` modifier for refusal.
  `check-card` / `chk` holds the narrated checks; narrow rows move the status
  below the explanation. `disclaimer` remains separate from the real verdict.

## Responsive behavior and deliberate deviations

The `main-col` container measures space left after the sidebar and viewer. The
practitioner pill keeps the full name, role, UZI and organization visible. The top
bar has a minimum height instead of a fixed height; at narrow container widths,
its session controls move to a second row. Decorative search appears only where
there is room. Below a 700px viewport, the sidebar becomes a compact brand strip.

The `content` container controls row/table reflow. Grids use available space
rather than viewport breakpoints. This deliberately departs from the wireframe's
fixed four-column record grid and short provenance labels: real rendered source
names and identifiers are longer. The record no longer inherits the form's 880px
limit. The 1400px dock reservation/default-open breakpoint remains unchanged.

The viewer tab toggles both ways and exposes its expanded state. Collapsed panel
contents are inert while its tab remains focusable. Viewer expressions stay in
HTML `data-*` attributes; no application JavaScript or route logic is added.

## Check changes

Run `go test ./sandbox/... -skip TestAcceptance`, `go vet ./sandbox/...`, and
`gofmt -l sandbox/app/`. Acceptance tests need their fixed ports and cannot run
alongside the human's compose stack.

For browser regression checks, sign in at `http://localhost:8091/demo`, then open
an unused pool patient. Pass the absolute path of
[the browser check](./layout-check.js) to Playwright MCP's
`browser_run_code_unsafe` tool as `filename`. It uses the supplied browser page,
without npm or an app build step. It shares and retrieves that synthetic patient,
so choose the patient deliberately. It checks all five screens, both POST results,
and the enriched record at 768, 1024, 1280, 1440, 1920 and 390px, with the viewer
open and closed. Checks cover horizontal containment, document scroll width,
vertical top-bar containment, page gaps, tab toggling and collapsed-panel focus.
