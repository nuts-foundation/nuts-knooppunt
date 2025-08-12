package nutsnode

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/must"
	"net/http/httputil"
)

func createProxy(targetAddress string) *httputil.ReverseProxy {
	targetURL := must.ParseURL("http://" + targetAddress)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	return proxy
}
