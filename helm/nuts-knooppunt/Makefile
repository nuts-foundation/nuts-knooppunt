# Helm chart dependency management for the nuts-knooppunt umbrella chart.
#
# chart-deps-local packages the local sibling charts that Chart.yaml
# normally pins to an OCI registry (fhir, pep) straight into charts/,
# bypassing `helm dependency update`. This lets you test a change to one of
# those charts immediately, without publishing it first - Chart.yaml is
# never touched, so there's nothing to remember to revert afterward.
#
# Re-run `helm dependency update` (chart-deps) to go back to the pinned,
# published versions - it re-fetches from the OCI registry and overwrites
# whatever chart-deps-local left in charts/.
.PHONY: chart-deps chart-deps-local

chart-deps:
	helm dependency update

chart-deps-local:
	helm package ../fhir -d charts
	helm package ../pep -d charts
