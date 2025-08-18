package nutsnode

import (
	"net/http/httputil"
	"net/url"
	"strings"
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
		},
	}
}

func RemovePrefixRewriter(prefix string) ProxyRequestRewriter {
	return func(request *httputil.ProxyRequest) {
		if strings.HasPrefix(request.In.URL.Path, prefix) {
			request.Out.URL.Path = strings.TrimPrefix(request.In.URL.Path, prefix)
			if request.Out.URL.Path == "" || !strings.HasPrefix(request.Out.URL.Path, "/") {
				request.Out.URL.Path = "/"
			}
		}
	}
}

type ProxyRequestRewriter func(request *httputil.ProxyRequest)
