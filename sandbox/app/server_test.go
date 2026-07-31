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
