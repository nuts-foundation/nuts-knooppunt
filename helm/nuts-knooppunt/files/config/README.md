Copies of `config/nuts.yml`, `config/discovery/*.json`, and `config/policy/*.json`
from the repo root - the embedded Nuts node's own config (separate from
`knooppunt.yml`), loaded via `component/nutsnode/component.go`'s hardcoded
`config/nuts.yml` path.

Copied rather than referenced because a Helm chart's `.Files.Get` can only
read files inside the chart's own directory. If you change one of the repo
root files, change the matching file here too.
