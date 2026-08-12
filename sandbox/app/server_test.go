package main

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func getPage(t *testing.T, path string) (int, string) {
	t.Helper()
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + path)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

// getPageWithClient is getPage for a caller-supplied client, so a session
// cookie obtained via signInViaDezi carries over to the request.
func getPageWithClient(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	res, err := client.Get(url)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

// signInViaDezi drives the real POST /demo/login -> GET /demo/auth/callback
// round trip against a fakeDezi backend and returns a client carrying the
// resulting session cookie. srv's NewMux must have been constructed after
// DEZI_INTERNAL_BASE_URL was pointed at a fakeDezi server, so the callback's
// token and userinfo calls land there instead of the real Dezi mock.
func signInViaDezi(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	res, err := client.PostForm(srv.URL+"/demo/login", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	location, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)
	state := location.Query().Get("state")
	require.NotEmpty(t, state)

	res, err = client.Get(srv.URL + "/demo/auth/callback?code=the-code&state=" + state)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/ehr", res.Header.Get("Location"), "a successful callback lands on the EHR home")

	return client
}

// startLogin drives POST /demo/login and returns the state it issued, so a
// caller can complete or replay the callback under precise control.
func startLogin(t *testing.T, client *http.Client, srv string) string {
	t.Helper()
	res, err := client.PostForm(srv+"/demo/login", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	location, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)
	state := location.Query().Get("state")
	require.NotEmpty(t, state)
	return state
}

// sessionCookieValue returns the raw session cookie value client is holding
// for target. A test can then replay it directly on a fresh request, which
// is the only way to prove the server itself, not just the client-side
// cookie jar, has forgotten a session.
func sessionCookieValue(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	u, err := url.Parse(target)
	require.NoError(t, err)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == sessionCookie {
			return c.Value
		}
	}
	t.Fatalf("no %s cookie held for %s", sessionCookie, target)
	return ""
}

// replaySessionCookie sends value as the session cookie on a fresh,
// jar-less request to /demo/ehr, bypassing whatever the client-side cookie
// jar believes, and reports where the server sends it.
func replaySessionCookie(t *testing.T, srv *httptest.Server, value string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/demo/ehr", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: value})
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	return res.StatusCode, res.Header.Get("Location")
}

func TestHealthz(t *testing.T) {
	status, body := getPage(t, "/healthz")
	require.Equal(t, http.StatusOK, status)
	require.JSONEq(t, `{"ok": true}`, body)
}

func TestEhrRedirectsWhenNotSignedIn(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.Get(srv.URL + "/demo/ehr")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/login", res.Header.Get("Location"))
}

func TestLoginRedirectsToDezi(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.PostForm(srv.URL+"/demo/login", nil)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	location, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)
	require.NotEmpty(t, location.Query().Get("state"))
	require.Equal(t, "S256", location.Query().Get("code_challenge_method"))
}

func TestCallbackRejectsUnknownState(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.Get(srv.URL + "/demo/auth/callback?code=x&state=never-issued")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestCallbackSurfacesOAuthError(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// state=x is never issued, so a bare status check here would also pass if
	// the error branch were deleted: the unknown-state check below it returns
	// 400 too. The body must name the OAuth error to prove this is the error
	// branch, not the unrelated unknown-state rejection.
	res, err := client.Get(srv.URL + "/demo/auth/callback?error=access_denied&state=x")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "access_denied")
}

func TestLogoutEndsSessionServerSide(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := signInViaDezi(t, srv)
	raw := sessionCookieValue(t, client, srv.URL)
	// A second practitioner, so this pins which sessions logout ends. With one
	// session a handler that cleared the whole store would pass identically,
	// and signing one person out would sign out everybody.
	bystander := sessionCookieValue(t, signInViaDezi(t, srv), srv.URL)

	res, err := client.PostForm(srv.URL+"/demo/logout", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=signed-out", res.Header.Get("Location"))
	requireDeletionCookie(t, res)

	// Replay the cookie value the browser held before logout. If the server
	// still honours it, clearing the cookie client-side was cosmetic and the
	// old session is still live.
	status, location := replaySessionCookie(t, srv, raw)
	require.Equal(t, http.StatusSeeOther, status, "a logged-out session must not still resolve server-side")
	require.Equal(t, "/demo/login", location)

	status, _ = replaySessionCookie(t, srv, bystander)
	require.Equal(t, http.StatusOK, status, "logout must end only the caller's session")
}

// requireDeletionCookie asserts the response carries a cookie that actually
// deletes the session cookie, rather than merely mentioning its name.
//
// This is the property the two routes share and the one nothing else here
// covers: a browser removes a cookie only when the replacement matches the
// original's Name and Path, so a wrong Path leaves the old cookie in place
// beside a new empty one and the session survives in the browser. Both routes
// go through clearSessionCookie (session.go); before that helper existed each
// route carried its own copy of these attributes, and every test still passed
// while they drifted.
//
// Secure is compared against secureCookies() rather than hardcoded: the set
// cookie's Secure flag is already pinned for http and https by
// TestCallbackSetsSecureCookieAttributes and its https counterpart, and the
// deletion cookie has to agree with whatever those produce.
func requireDeletionCookie(t *testing.T, res *http.Response) {
	t.Helper()
	for _, cookie := range res.Cookies() {
		if cookie.Name != sessionCookie {
			continue
		}
		require.Empty(t, cookie.Value, "the deletion cookie must not carry a session value")
		require.Equal(t, "/", cookie.Path, "a cookie is deleted only when the Path matches the one it was set with")
		require.True(t, cookie.HttpOnly, "the replacement must not be readable from script when the original was not")
		require.Equal(t, secureCookies(), cookie.Secure, "the deletion cookie must match the set cookie's Secure flag")
		require.Equal(t, -1, cookie.MaxAge, "MaxAge -1 is what expires the cookie immediately")
		return
	}
	t.Fatalf("no %s cookie in the response, so the browser keeps the one it has", sessionCookie)
}

func TestResetEndsSessionServerSide(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := signInViaDezi(t, srv)
	raw := sessionCookieValue(t, client, srv.URL)
	// The claim below is about every session, so the fixture needs more than
	// the caller's: dropping only the caller would otherwise satisfy it.
	other := sessionCookieValue(t, signInViaDezi(t, srv), srv.URL)

	res, err := client.PostForm(srv.URL+"/demo/reset", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-pending", res.Header.Get("Location"))
	requireDeletionCookie(t, res)

	status, location := replaySessionCookie(t, srv, raw)
	require.Equal(t, http.StatusSeeOther, status, "reset must drop every session, not just clear the caller's cookie")
	require.Equal(t, "/demo/login", location)

	status, _ = replaySessionCookie(t, srv, other)
	require.Equal(t, http.StatusSeeOther, status, "reset must also drop a session that never made the request")
}

func TestCallbackSetsSecureCookieAttributes(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	state := startLogin(t, client, srv.URL)

	res, err := client.Get(srv.URL + "/demo/auth/callback?code=the-code&state=" + state)
	require.NoError(t, err)
	defer res.Body.Close()

	var sessionCk *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			sessionCk = c
		}
	}
	require.NotNil(t, sessionCk, "the callback must set the session cookie")
	require.True(t, sessionCk.HttpOnly, "the session cookie must be HttpOnly so client script cannot read it")
	require.Equal(t, http.SameSiteLaxMode, sessionCk.SameSite, "the session cookie must be SameSite=Lax")
	require.False(t, sessionCk.Secure, "over a plain-http public URL Secure would make the browser drop the cookie")
}

func TestCallbackMarksCookieSecureForAnHTTPSDeployment(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	// The hosted sandbox terminates TLS at a proxy, so the request reaching
	// this process is plain http regardless. Only the public URL says what the
	// browser used, which is why the cookie flag is derived from it.
	t.Setenv("SANDBOX_PUBLIC_URL", "https://sandbox.example.com")
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	state := startLogin(t, client, srv.URL)

	res, err := client.Get(srv.URL + "/demo/auth/callback?code=the-code&state=" + state)
	require.NoError(t, err)
	defer res.Body.Close()

	var sessionCk *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			sessionCk = c
		}
	}
	require.NotNil(t, sessionCk, "the callback must set the session cookie")
	require.True(t, sessionCk.Secure, "an https public URL must restrict the session cookie to https")
}

func TestCallbackRejectsReplayedState(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	state := startLogin(t, client, srv.URL)
	callback := srv.URL + "/demo/auth/callback?code=the-code&state=" + state

	res, err := client.Get(callback)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode, "the first use of a genuinely issued state must succeed")

	// Unlike TestCallbackRejectsUnknownState, this state really was issued by
	// POST /demo/login above; it must still be rejected the second time,
	// because a state is single-use.
	res, err = client.Get(callback)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode, "a state must not be usable a second time")
}

// TestRequireSessionGuardsAnySubpath proves the extracted guard, not just the
// one route wired to it today, protects a subpath. No subpath under
// /demo/ehr is registered in NewMux yet, so this builds a minimal mux of its
// own rather than adding an unused route to production code: the next
// epic's screens under /demo/ehr wrap with requireSession the same way.
func TestRequireSessionGuardsAnySubpath(t *testing.T) {
	resolve := func(*http.Request) *authSession { return nil }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /demo/ehr/notes", requireSession(resolve, func(w http.ResponseWriter, _ *http.Request, _ *authSession) {
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.Get(srv.URL + "/demo/ehr/notes")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/login", res.Header.Get("Location"))
}

func TestCallbackWithoutCodeIsARequestError(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	state := startLogin(t, client, srv.URL)

	res, err := client.Get(srv.URL + "/demo/auth/callback?state=" + state)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode,
		"a callback without a code is a client error, not a Dezi failure")

	// The attempt must survive, so the practitioner can follow a correct
	// callback instead of having to restart the sign-in.
	good, err := client.Get(srv.URL + "/demo/auth/callback?code=the-code&state=" + state)
	require.NoError(t, err)
	defer good.Body.Close()
	require.Equal(t, http.StatusSeeOther, good.StatusCode, "the sign-in attempt must not have been consumed")
}

func TestLogoutRejectsCrossSiteRequest(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := signInViaDezi(t, srv)
	raw := sessionCookieValue(t, client, srv.URL)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/logout", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode,
		"a cross-site form post must not be able to sign a practitioner out")

	// Status alone is too weak: a handler that clears the cookie and then
	// returns 403 would pass that check while still signing the victim out.
	for _, c := range res.Cookies() {
		require.NotEqual(t, sessionCookie, c.Name, "a rejected logout must not touch the session cookie")
	}
	status, _ := replaySessionCookie(t, srv, raw)
	require.Equal(t, http.StatusOK, status, "the live session must survive a rejected cross-site logout")
}

func TestLogoutRejectsSameSiteRequestAndKeepsTheSession(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	raw := sessionCookieValue(t, signInViaDezi(t, srv), srv.URL)

	// same-site covers sibling origins under one registrable domain, so on a
	// hosted deployment any other subdomain would qualify. The cookie is sent
	// deliberately: SameSite=Lax withholds it cross-site but attaches it
	// same-site, so this is the one browser vector that hands a live session
	// to a rejected logout. Without it the assertion below is vacuous, and a
	// handler that dropped the session before checking the guard would pass.
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/logout", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	res, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "a sibling origin must not be trusted")

	status, _ := replaySessionCookie(t, srv, raw)
	require.Equal(t, http.StatusOK, status, "a rejected logout must not have dropped the session")
}

func TestLogoutAcceptsSameOriginRequest(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/logout", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode, "a same-origin sign-out must be allowed")
}
