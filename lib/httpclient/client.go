// Package httpclient builds HTTP clients for outbound calls with timeouts for every phase of a request.
// Without them, a server that accepts a connection and then stops answering holds the caller forever:
// http.DefaultTransport has no ResponseHeaderTimeout, and a zero http.Client has no Timeout.
//
// Start every client for an outbound call here, and wrap the transport in whatever the component needs
// (tracing, retries, OAuth2):
//
//	client := httpclient.New(tracing.WrapTransport(httpclient.NewTransport(tlsConfig)))
//
// Both functions return the concrete type, so a component sets its own limits by overriding fields. The
// defaults suit a background job; a call on the path of an incoming request, such as an authorization
// decision, wants shorter ones:
//
//	transport := httpclient.NewTransport(nil)
//	transport.ResponseHeaderTimeout = 5 * time.Second
//	client := httpclient.New(transport)
//	client.Timeout = 10 * time.Second
package httpclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	// DefaultDialTimeout bounds setting up the TCP connection.
	DefaultDialTimeout = 5 * time.Second
	// DefaultTLSHandshakeTimeout bounds the TLS handshake, including an mTLS client certificate exchange.
	DefaultTLSHandshakeTimeout = 5 * time.Second
	// DefaultResponseHeaderTimeout bounds the wait for the response headers of a single attempt, after the
	// request has been written. It covers the time the server needs to process the request.
	DefaultResponseHeaderTimeout = 30 * time.Second
	// DefaultRequestTimeout bounds one request as the caller sees it: every attempt of a retrying
	// transport and its backoff, an OAuth2 token request made along the way, and reading the response
	// body.
	DefaultRequestTimeout = 2 * time.Minute
)

// NewTransport returns a transport with timeouts for every phase of an attempt. It clones
// http.DefaultTransport, so it keeps the proxy from the environment, HTTP/2 and the idle connection
// settings. tlsConfig carries an mTLS client certificate or a custom trust store; nil means the default
// TLS settings. tlsConfig itself is not modified.
//
// At least TLS 1.2 is required. The Generieke Functies require all connections to follow the TLS
// guidelines of the NCSC ("Transport and certificates" in
// https://minvws.github.io/generiekefuncties-docs/en/authentication.html). Those guidelines (version
// 2025-05, table 1) rate TLS 1.3 as Good and TLS 1.2 as Sufficient, and older versions as Insufficient.
// TLS 1.2 stays allowed, so a server or proxy that doesn't do TLS 1.3 yet can still be reached; Go
// prefers TLS 1.3 whenever the server supports it.
func NewTransport(tlsConfig *tls.Config) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig.Clone()
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.MinVersion = max(transport.TLSClientConfig.MinVersion, tls.VersionTLS12)
	transport.DialContext = (&net.Dialer{Timeout: DefaultDialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = DefaultTLSHandshakeTimeout
	transport.ResponseHeaderTimeout = DefaultResponseHeaderTimeout
	return transport
}

// New returns a client over transport with DefaultRequestTimeout and a redirect policy that refuses a
// downgrade from HTTPS to HTTP (see RefuseDowngrade).
//
// A client built by another library doesn't get these unless that library takes them from a client you
// pass it, as x/oauth2 does (see httpauth.NewOAuth2HTTPClient). Otherwise set them on it yourself:
//
//	client.Timeout = httpclient.DefaultRequestTimeout
//	client.CheckRedirect = httpclient.RefuseDowngrade
func New(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport:     transport,
		Timeout:       DefaultRequestTimeout,
		CheckRedirect: RefuseDowngrade,
	}
}

// maxRedirects matches the standard library's default policy, which CheckRedirect replaces.
const maxRedirects = 10

// RefuseDowngrade is an http.Client.CheckRedirect policy. It refuses a redirect from HTTPS to plain HTTP,
// which would send the request, a bearer token included, unencrypted. The standard library follows such a
// redirect without complaint. Like the default policy, it stops after 10 redirects.
func RefuseDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errors.New("refusing redirect from HTTPS to " + req.URL.Scheme)
	}
	return nil
}
