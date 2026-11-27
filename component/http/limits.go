package http

import (
	"net/http"
	"time"
)

// Fixed limits for both HTTP servers. They are deliberately not configurable, to keep the config
// surface small; only the body size differs per interface (InterfaceConfig.MaxBodySize).
const (
	// readHeaderTimeout bounds reading the request line and headers (slowloris).
	readHeaderTimeout = 10 * time.Second
	// readTimeout bounds reading the whole request, including a 10 MiB body over a slow link.
	readTimeout = 60 * time.Second
	// idleTimeout bounds how long a keep-alive connection waits for the next request.
	idleTimeout = 120 * time.Second
	// maxHeaderBytes bounds the request line and headers, so the URL and query string too.
	// A request over it gets 431.
	maxHeaderBytes = 64 << 10
	// publicWriteTimeout bounds writing the response on the public interface.
	publicWriteTimeout = 60 * time.Second
	// internalWriteTimeout is 0 (no limit): POST /lrza/update and POST /mcsd/update run a full
	// sync before they respond, which a server-wide write timeout would cut off. The LRZA sync
	// bounds itself instead (syncTimeout and pageTimeout in component/lrza).
	internalWriteTimeout = 0
)

// limitBody caps every request body at maxBytes. A handler that reads past it gets an
// *http.MaxBytesError and turns that into a response in its own format (e.g. a 413
// OperationOutcome); the server closes the connection afterwards. Requests are not rejected
// up front on Content-Length, so each handler keeps control over its error format.
//
// This is a ceiling: a route can apply a tighter limit of its own, but cannot raise it.
func limitBody(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}
