package nutsnode

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
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

func Test_createProxy_BodyLimit(t *testing.T) {
	const limit = 1024
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	proxy := createProxy(target, nil)
	// Stand-in for the body ceiling the HTTP component puts in front of the mux.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		proxy.ServeHTTP(w, r)
	})
	post := func(size int) int {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/foo", strings.NewReader(strings.Repeat("a", size))))
		return recorder.Code
	}

	t.Run("body at the limit is proxied", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, post(limit))
	})
	t.Run("body over the limit is a 413, not a 502", func(t *testing.T) {
		// The MaxBytesError must survive http.Transport and the otelhttp transport around it.
		assert.Equal(t, http.StatusRequestEntityTooLarge, post(limit+1))
	})
	t.Run("an unreachable upstream is still a 502", func(t *testing.T) {
		unreachable, _ := url.Parse("http://127.0.0.1:1")
		recorder := httptest.NewRecorder()
		createProxy(unreachable, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/foo", nil))
		assert.Equal(t, http.StatusBadGateway, recorder.Code)
	})
}
