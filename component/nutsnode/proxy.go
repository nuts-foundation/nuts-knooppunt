package nutsnode

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/must"
	"net/http/httputil"
	"strings"
)

func createProxy(targetAddress string, routePrefix string) *httputil.ReverseProxy {
	targetURL := must.ParseURL("http://" + targetAddress)
	return &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(targetURL)
			request.Out.Host = request.In.Host
			// Strip routePrefix from the request URL path (e.g. /nuts)
			if routePrefix != "" {
				if strings.HasPrefix(request.In.URL.Path, routePrefix) {
					request.Out.URL.Path = strings.TrimPrefix(request.In.URL.Path, routePrefix)
					if request.Out.URL.Path == "" || !strings.HasPrefix(request.Out.URL.Path, "/") {
						request.Out.URL.Path = "/"
					}
				}
			}
		},
	}
}
