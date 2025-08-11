package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
)

// Handler creates an HTTP handler that proxies requests to a backend server
type Handler struct {
	proxy *httputil.ReverseProxy
	routePrefix string
	backendURL *url.URL
}

// NewHandler creates a new proxy handler
func NewHandler(routePrefix, backendURL string) (*Handler, error) {
	backend, err := url.Parse(backendURL)
	if err != nil {
		return nil, fmt.Errorf("invalid backend URL %s: %w", backendURL, err)
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(backend)
			// Remove the route prefix from the path when forwarding
			if strings.HasPrefix(pr.In.URL.Path, routePrefix) {
				pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, routePrefix)
				if pr.Out.URL.Path == "" || !strings.HasPrefix(pr.Out.URL.Path, "/") {
					pr.Out.URL.Path = "/"
				}
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error().Err(err).Str("backend", backendURL).Msg("Proxy error")
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}

	return &Handler{
		proxy: proxy,
		routePrefix: routePrefix,
		backendURL: backend,
	}, nil
}

// ServeHTTP implements the http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("backend", h.backendURL.String()).
		Msg("Proxying request")
	
	h.proxy.ServeHTTP(w, r)
}