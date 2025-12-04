package nutsnode

import (
	"net/http/httputil"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// createProxy creates a reverse proxy that forwards requests to the target address.
// It can do the following URL rewriting (in the following order):
// - if removeRoutePrefix is set, it is stripped from the request URL path before forwarding
// - if addRoutePrefix is set, it is prepended to the request URL path before forwarding (this can be useful for /.well-known routes)
func createProxy(targetAddress *url.URL, rewriter ProxyRequestRewriter) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(targetAddress)
			request.Out.Host = request.In.Host
			if rewriter != nil {
				rewriter(request)
			}
			// Propagate trace context (W3C Trace Context headers) to the proxied request
			// This enables distributed tracing across the proxy boundary to nuts-node
			otel.GetTextMapPropagator().Inject(request.In.Context(), propagation.HeaderCarrier(request.Out.Header))
		},
	}
}

func RemovePrefixRewriter(prefix string) ProxyRequestRewriter {
	return func(request *httputil.ProxyRequest) {
		inPath := request.In.URL.Path
		if prefix == "/" {
			// Special case: root always matches
			request.Out.URL.Path = inPath
			return
		}
		if inPath == prefix || (strings.HasPrefix(inPath, prefix) && (len(inPath) == len(prefix) || inPath[len(prefix)] == '/')) {
			request.Out.URL.Path = strings.TrimPrefix(inPath, prefix)
			if request.Out.URL.Path == "" || !strings.HasPrefix(request.Out.URL.Path, "/") {
				request.Out.URL.Path = "/"
			}
		} else {
			request.Out.URL.Path = inPath
		}
	}
}

type ProxyRequestRewriter func(request *httputil.ProxyRequest)
