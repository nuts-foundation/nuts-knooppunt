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

	// Replay the cookie value the browser held before logout. If the server
	// still honours it, clearing the cookie client-side was cosmetic and the
	// old session is still live.
	status, location := replaySessionCookie(t, srv, raw)
	require.Equal(t, http.StatusSeeOther, status, "a logged-out session must not still resolve server-side")
	require.Equal(t, "/demo/login", location)

	status, _ = replaySessionCookie(t, srv, bystander)
	require.Equal(t, http.StatusOK, status, "logout must end only the caller's session")
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

	status, location := replaySessionCookie(t, srv, raw)
	require.Equal(t, http.StatusSeeOther, status, "reset must drop every session, not just clear the caller's cookie")
	require.Equal(t, "/demo/login", location)

	status, _ = replaySessionCookie(t, srv, other)
	require.Equal(t, http.StatusSeeOther, status, "reset must also drop a session that never made the request")
}

func TestResetRejectsCrossSiteRequest(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/reset", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "a cross-site Sec-Fetch-Site must be rejected")
	for _, c := range res.Cookies() {
		require.NotEqual(t, sessionCookie, c.Name, "a rejected reset must not touch the session cookie")
	}
}

func TestResetRejectsSameSiteRequestAndKeepsSessions(t *testing.T) {
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	first := sessionCookieValue(t, signInViaDezi(t, srv), srv.URL)
	second := sessionCookieValue(t, signInViaDezi(t, srv), srv.URL)

	// Reset needs no session and clears everyone's, so a sibling origin being
	// able to reach it is the damaging case.
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/reset", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "same-site")
	res, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "a sibling origin must not be able to reset")
	for _, c := range res.Cookies() {
		require.NotEqual(t, sessionCookie, c.Name, "a rejected reset must not sign the browser out either")
	}

	for _, raw := range []string{first, second} {
		status, _ := replaySessionCookie(t, srv, raw)
		require.Equal(t, http.StatusOK, status, "a rejected reset must leave every session intact")
	}
}

func TestResetAcceptsSameOriginRequest(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/reset", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode, "a same-origin Sec-Fetch-Site must be allowed")
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
		"logout mutates session state and must be guarded like reset")

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

// authorizeSandbox starts a sandbox wired to a fake Dezi and to nodeBaseURL,
// and returns it together with a client holding a live session. The SANDBOX_
// variables are cleared for the same reason testNutsClient clears them: the
// node fakes pin their routes and assertions to nutsConfigFromEnv's defaults,
// so a developer with any of them exported would otherwise get a sandbox those
// fakes reject.
func authorizeSandbox(t *testing.T, nodeBaseURL string) (*httptest.Server, *http.Client) {
	t.Helper()
	for _, key := range []string{
		"SANDBOX_NUTS_SUBJECT",
		"SANDBOX_BGZ_SCOPE",
		"SANDBOX_AUTH_SERVER",
		"SANDBOX_FACILITY_TYPE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("DEZI_INTERNAL_BASE_URL", fakeDezi(t).URL)
	t.Setenv("NUTS_INTERNAL_BASE_URL", nodeBaseURL)
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	return srv, signInViaDezi(t, srv)
}

// nodeIntrospecting issues a usable token and answers introspection with the
// given status and body, which is the one shape neither existing fake can
// produce: fakeNode always succeeds at both steps, and recordingNode answers
// every path alike, so a failing introspection there is preceded by a failing
// token request.
func nodeIntrospecting(t *testing.T, status int, response string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /nuts/internal/auth/v2/plataan/request-service-access-token",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"the-token"}`))
		})
	mux.HandleFunc("POST /nuts/internal/auth/v2/accesstoken/introspect",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(response))
		})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthorizeRendersTheClaims(t *testing.T) {
	srv, client := authorizeSandbox(t, fakeNode(t).URL)

	res, err := client.PostForm(srv.URL+"/demo/authorize", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	body := string(raw)

	// Exact values, not merely present: a wrong persona or a mismatched
	// certificate would satisfy a non-empty check. Name and value are asserted
	// as the row the template renders, because a value alone also passes when
	// it is rendered under the wrong claim name, and because 900001234 is the
	// practitioner's Dezi number, which any page carrying the session view
	// would print regardless of what the node returned.
	for _, row := range []string{
		"<th>user_id</th><td>900001234</td>",
		"<th>user_role</th><td>01.022</td>",
		"<th>organization_ura</th><td>00000010</td>",
		"<th>organization_facility_type</th><td>Z3</td>",
	} {
		require.Contains(t, body, row)
	}
	require.Contains(t, body, "<title>Authorization · Plataan EHR</title>")
	require.Contains(t, body, "Service access token")
	require.Contains(t, body, "The decision itself is not made here",
		"the page must say the authorization decision is not what it shows")
	require.Contains(t, body, scenario, "the demo bar carries the scenario")
	require.Contains(t, body, "Reset", "the demo bar carries the reset control")
}

// Issue #540's second acceptance criterion is that every authenticated screen
// shows the practitioner's name, role, UZI number and organisation. This route
// sits behind requireSession and renders a full page, so the criterion covers
// it, and Session.Description is where the last three come from. This is the
// same assertion TestEhrHomeShowsFullChrome makes for /demo/ehr.
//
// It is also what makes page.Session load-bearing here: with no top bar the
// field was set and never read, so dropping it changed no byte of the response.
func TestAuthorizeShowsThePractitionerChrome(t *testing.T) {
	srv, client := authorizeSandbox(t, fakeNode(t).URL)

	res, err := client.PostForm(srv.URL+"/demo/authorize", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	body := string(raw)

	require.Contains(t, body, `class="side"`, "the EHR sidebar")
	require.Contains(t, body, `class="top"`, "the EHR top bar")
	// .app, .side, .top and .card have rules in ehr.css and nowhere else, so
	// the guise stopped being cosmetic the moment this page grew the chrome:
	// under "shell" the markup below still renders and the layout collapses.
	require.Contains(t, body, "/static/css/ehr.css", "the chrome is styled only there")
	require.Contains(t, body, "Dr. S. el Amrani", "the practitioner's name")
	require.Contains(t, body, "Clinical geriatrician · UZI 900001234 · De Plataan Hospital",
		"role, UZI number and organisation, the rest of criterion 2")
	require.Contains(t, body, "Dezi ✓", "the top bar shows the signed-in badge")
	require.Contains(t, body, "<h2>Authorization</h2>", "the top bar names the screen")
	// Which item is highlighted is the sidebar's business; that one is at all
	// is this page's, and it is the whole of page.Active's effect here.
	require.Contains(t, body, `class="nav-item active"`, "the page marks its section in the sidebar")
}

func TestAuthorizeRequiresASession(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.PostForm(srv.URL+"/demo/authorize", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/login", res.Header.Get("Location"))
}

// The local go run path has no node at all. The route must say so rather than
// the app failing to start or the page rendering an empty table.
func TestAuthorizeNamesTheFailingStepWhenTheNodeIsAbsent(t *testing.T) {
	srv, client := authorizeSandbox(t, "http://127.0.0.1:1")

	res, err := client.PostForm(srv.URL+"/demo/authorize", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, "request service access token")
	// The handler must stop here. Without the return the empty token is
	// introspected anyway, and the response carries the second step's failure
	// on top of the first, naming the wrong step to whoever reads it.
	require.NotContains(t, body, "introspect access token")
}

// The token step succeeding and introspection failing is unreachable from the
// absent-node case above, which never gets past the first call, so the second
// error branch would otherwise be untested.
func TestAuthorizeNamesTheIntrospectionStep(t *testing.T) {
	srv, client := authorizeSandbox(t, nodeIntrospecting(t, http.StatusUnauthorized, `{"error":"unauthorized"}`).URL)

	res, err := client.PostForm(srv.URL+"/demo/authorize", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, "introspect access token")
	// The status the node gave, not just the step: the claimless guard below
	// reports the same step, so without this an ignored introspection error
	// reads identically to an introspection that returned nothing.
	require.Contains(t, body, "status 401")
	require.NotContains(t, body, "no claims", "the response must carry one failure, not the next check's as well")
	require.NotContains(t, body, "user_id", "a failed introspection must not also render the claims table")
}

// A JSON null, or an object without claims, decodes without error and leaves
// the map empty, so nothing in the client reports it. Rendering that as a page
// of empty rows would call a flow successful that produced no claims at all.
//
// {"active": false} is the node's canonical answer to a token it will not
// vouch for: auth/api/iam/api.go returns it for an empty token, for one absent
// from its store and for an expired one, each under the comment "Return 200 +
// 'Active = false' when token is invalid or malformed". It decodes to a map of
// length 1, so a length check waves it through; only looking for the claims
// the page renders catches it.
func TestAuthorizeRejectsAClaimlessIntrospection(t *testing.T) {
	for _, response := range []string{`null`, `{}`, `{"active":false}`} {
		t.Run(response, func(t *testing.T) {
			srv, client := authorizeSandbox(t, nodeIntrospecting(t, http.StatusOK, response).URL)

			res, err := client.PostForm(srv.URL+"/demo/authorize", nil)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusBadGateway, res.StatusCode)
			raw, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			body := string(raw)
			require.Contains(t, body, "introspect access token")
			require.Contains(t, body, "no claims")
			require.NotContains(t, body, "user_id", "an empty introspection must not render a table of blanks")
		})
	}
}

// A response carrying some of the four claims is the same failure one row at a
// time: fmt.Sprint of an absent or null value is "<nil>", so the page would
// print a blank the reader has no way to tell from a claim the node genuinely
// returned as the string "<nil>". The step is named with the claims it did not
// carry, because that is the difference between a misconfigured credential and
// a token the node refused outright.
func TestAuthorizeRejectsAPartialIntrospection(t *testing.T) {
	for name, response := range map[string]string{
		"absent": `{"active":true,"user_id":"900001234","user_role":"01.022","organization_ura":"00000010"}`,
		"null":   `{"active":true,"user_id":"900001234","user_role":"01.022","organization_ura":"00000010","organization_facility_type":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv, client := authorizeSandbox(t, nodeIntrospecting(t, http.StatusOK, response).URL)

			res, err := client.PostForm(srv.URL+"/demo/authorize", nil)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusBadGateway, res.StatusCode)
			raw, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			body := string(raw)
			require.Contains(t, body, "introspect access token")
			require.Contains(t, body, "organization_facility_type", "the failure must name the claim that is missing")
			require.NotContains(t, body, "no claims",
				"three of four claims is not the same failure as none, and must not report as it")
			// The page's own heading, not a claim name: the message above names
			// one, so absence of a claim name no longer proves absence of a page.
			require.NotContains(t, body, "Service access token", "a partial introspection must not render the page")
			require.NotContains(t, body, "<nil>", "the blank this guard exists to keep off the screen")
		})
	}
}

func TestAuthorizeRejectsCrossSiteRequest(t *testing.T) {
	srv, client := authorizeSandbox(t, fakeNode(t).URL)

	// The session is deliberately live. requireSession is the outer guard, so
	// without one this request is turned away at the login redirect and the
	// check below is never reached, leaving the assertion vacuous.
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/authorize", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, "cross-site")
	// Status alone is too weak: without the return the handler goes on to ask
	// the node for a token and appends the page to the rejection.
	require.NotContains(t, body, "user_id", "a rejected request must not reach the node at all")
}

// The guard rejects on evidence of another site rather than on the header
// being present, and only a request that carries same-origin proves it: the
// tests above send no Sec-Fetch-Site at all, as non-browser clients do.
func TestAuthorizeAcceptsSameOriginRequest(t *testing.T) {
	srv, client := authorizeSandbox(t, fakeNode(t).URL)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/authorize", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}
