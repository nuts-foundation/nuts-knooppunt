package nutsnode

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_createProxy(t *testing.T) {
	target, _ := url.Parse("http://example.com:8080")
	inURL, _ := url.Parse("http://localhost/foo")

	t.Run("without rewriter", func(t *testing.T) {
		proxy := createProxy(target, nil)
		require.NotNil(t, proxy)

		inReq := &http.Request{URL: inURL, Host: "localhost"}
		outReq := &http.Request{URL: &url.URL{}}
		pr := &httputil.ProxyRequest{In: inReq, Out: outReq}
		proxy.Rewrite(pr)
		assert.Equal(t, target.Scheme, pr.Out.URL.Scheme)
		assert.Equal(t, target.Host, pr.Out.URL.Host)
		assert.Equal(t, inReq.Host, pr.Out.Host)
	})

	t.Run("with rewriter", func(t *testing.T) {
		inReq := &http.Request{URL: inURL, Host: "localhost"}
		rewriter := func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Path = "/bar"
		}
		proxyWithRewriter := createProxy(target, rewriter)
		pr := &httputil.ProxyRequest{In: inReq, Out: &http.Request{URL: &url.URL{}}}
		proxyWithRewriter.Rewrite(pr)
		assert.Equal(t, "/bar", pr.Out.URL.Path)
	})

}

func Test_createProxy_URLRewriting(t *testing.T) {
	target, _ := url.Parse("http://example.com")
	rewriter := RemovePrefixRewriter("/api")
	proxy := createProxy(target, rewriter)

	tests := []struct {
		name     string
		inPath   string
		wantPath string
	}{
		{"prefix present", "/api/resource", "/resource"},
		{"prefix not present", "/other/resource", "/other/resource"},
		{"exact prefix", "/api", "/"},
		{"prefix substring", "/apix/resource", "/apix/resource"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inURL, _ := url.Parse("http://localhost" + tt.inPath)
			inReq := &http.Request{URL: inURL}
			outReq := &http.Request{URL: &url.URL{}}
			pr := &httputil.ProxyRequest{
				In:  inReq,
				Out: outReq,
			}
			proxy.Rewrite(pr)
			assert.Equal(t, tt.wantPath, pr.Out.URL.Path)
		})
	}
}
