package lrza

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Timeouts for the HTTP clients to the LRZA and the query directory. Without them, a server that accepts
// a connection and then stops answering holds the sync forever (see update).
var (
	// dialTimeout bounds setting up the TCP connection.
	dialTimeout = 5 * time.Second
	// tlsHandshakeTimeout bounds the TLS handshake, including the mTLS client certificate exchange.
	tlsHandshakeTimeout = 5 * time.Second
	// responseHeaderTimeout bounds the wait for the response headers of a single attempt, after the
	// request has been written. It covers the time the server needs to process the request, so it must
	// leave room for the query directory applying a transaction of searchPageSize entries.
	responseHeaderTimeout = 30 * time.Second
	// requestTimeout bounds one request as the caller sees it: every retry attempt and its backoff (see
	// retryTransport), an OAuth2 token request, and reading the response body. retryTransport's own
	// backoff adds up to about 31 s, so this leaves room for several slow attempts.
	requestTimeout = 2 * time.Minute
)

// maxConnsPerHost bounds the connections to one host. The sync is sequential, so this is mostly for the
// registration path, where every EHR request makes its own calls: beyond the limit, a request waits for
// a free connection (or for its context to end) instead of opening yet another one to the LRZA.
const maxConnsPerHost = 8

// newTransport returns the base transport for the clients to the LRZA and the query directory. It clones
// http.DefaultTransport, so it keeps the proxy from the environment, HTTP/2 and the idle connection
// settings, and adds timeouts for every phase of an attempt. tlsConfig carries the mTLS client
// certificate; nil means the default TLS settings.
//
// In both cases at least TLS 1.2 is required. The Generieke Functies require all connections to follow
// the TLS guidelines of the NCSC ("Transport and certificates" in
// https://minvws.github.io/generiekefuncties-docs/en/authentication.html). Those guidelines (version
// 2025-05, table 1) rate TLS 1.3 as Good and TLS 1.2 as Sufficient, and older versions as Insufficient.
// TLS 1.2 stays allowed, so a server or proxy that doesn't do TLS 1.3 yet can still be reached; Go
// prefers TLS 1.3 whenever the server supports it.
func newTransport(tlsConfig *tls.Config) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig.Clone()
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.MinVersion = max(transport.TLSClientConfig.MinVersion, tls.VersionTLS12)
	transport.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = tlsHandshakeTimeout
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	transport.MaxConnsPerHost = maxConnsPerHost
	return transport
}

// newHTTPClient returns a client over transport with the overall requestTimeout and a redirect policy that
// refuses a downgrade from HTTPS to HTTP.
func newHTTPClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport:     transport,
		Timeout:       requestTimeout,
		CheckRedirect: refuseDowngrade,
	}
}

// maxRedirects matches the standard library's default policy, which CheckRedirect replaces.
const maxRedirects = 10

// refuseDowngrade is an http.Client.CheckRedirect policy. It refuses a redirect from HTTPS to plain HTTP,
// which would send the request, its bearer token included, unencrypted. The standard library follows such
// a redirect without complaint.
func refuseDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errors.New("refusing redirect from HTTPS to " + req.URL.Scheme)
	}
	return nil
}
