// Package zonnebloem is Zorgcentrum De Zonnebloem's own record system in the GF
// Sandbox: the source side of the marker proof. gf-sandbox serves it on its own
// port; see README.md.
package zonnebloem

import (
	"math/rand/v2"
	"net/http"
	"net/url"
	"time"
)

// NewHandler returns De Zonnebloem's EHR on the FHIR store at fhirBaseURL, the
// organization's own store and the only thing the EHR talks to.
func NewHandler(fhirBaseURL *url.URL) http.Handler {
	st := newFHIRStore(fhirBaseURL, &http.Client{Timeout: 10 * time.Second})
	marker := func() string { return suggestedMarker(rand.IntN) }
	return newMux(st, time.Now, marker)
}
