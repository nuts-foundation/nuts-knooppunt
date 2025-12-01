package authn

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd/core"
	httpComponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/lib/netutil"
	"github.com/stretchr/testify/require"
)

func Test_RequestToken(t *testing.T) {
	p1, _ := netutil.FreeTCPPort()
	p2, _ := netutil.FreeTCPPort()
	internalMux := http.NewServeMux()
	publicMux := http.NewServeMux()
	httpConfig := httpComponent.TestConfig()
	httpConfig.InternalInterface = httpComponent.InterfaceConfig{
		Address: ":" + strconv.Itoa(p1),
		BaseURL: "http://localhost:" + strconv.Itoa(p1),
	}
	httpConfig.PublicInterface = httpComponent.InterfaceConfig{
		Address: ":" + strconv.Itoa(p2),
		BaseURL: "http://localhost:" + strconv.Itoa(p2),
	}
	httpService := httpComponent.New(httpConfig, publicMux, internalMux)

	config := Config{
		Clients: []Client{
			{
				ID:     "test-client",
				Secret: "test-secret",
				RedirectURLs: []string{
					"http://localhost/callback",
				},
			},
		},
	}

	component, err := New(config, httpService, core.DefaultConfig())
	require.NoError(t, err)
	component.RegisterHttpHandlers(publicMux, internalMux)
	err = httpService.Start()
	require.NoError(t, err)
	defer httpService.Stop(t.Context())

	t.Run("OpenID Discovery", func(t *testing.T) {
		httpResponse, err := http.Get(httpService.Internal().URL().JoinPath("/.well-known/openid-configuration").String())
		require.NoError(t, err)
		defer httpResponse.Body.Close()
		require.Equal(t, http.StatusOK, httpResponse.StatusCode)
		responseData, _ := io.ReadAll(httpResponse.Body)
		var data map[string]any
		require.NoError(t, json.Unmarshal(responseData, &data))

		require.Equal(t, data["token_endpoint"], httpService.Internal().URL().JoinPath("/auth/token").String())
		require.Equal(t, data["issuer"], httpService.Internal().URL().JoinPath("/auth").String())
	})
	t.Run("Authorization Code Flow", func(t *testing.T) {
		// Step 1: Initiate authorization request
		authURL := httpService.Public().URL().JoinPath("/auth/authorize")
		query := authURL.Query()
		query.Set("client_id", "test-client")
		query.Set("redirect_uri", "http://localhost/callback")
		query.Set("response_type", "code")
		query.Set("scope", "openid profile")
		query.Set("state", "test-state-123")
		authURL.RawQuery = query.Encode()

		// Make authorization request (should redirect to login page)
		httpClient := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		}

		authResponse, err := httpClient.Get(authURL.String())
		require.NoError(t, err)
		defer authResponse.Body.Close()
		data, _ := io.ReadAll(authResponse.Body)
		println(string(data))

		// Should redirect to login page with authRequestID
		require.Equal(t, http.StatusFound, authResponse.StatusCode)
		loginLocation, err := authResponse.Location()
		require.NoError(t, err)
		require.Contains(t, loginLocation.Path, "/auth/login")

		authRequestID := loginLocation.Query().Get("authRequestID")
		require.NotEmpty(t, authRequestID)

		// Step 2: Simulate user authentication by posting to login form
		loginURL := httpService.Public().URL().JoinPath("/auth/login")
		loginResponse, err := httpClient.PostForm(loginURL.String(), map[string][]string{
			"authRequestID":  {authRequestID},
			"action":         {"allow"},
			"loa_dezi":       {"http://eidas.europe.eu/LoA/high"},
			"verklaring_id":  {"8539f75d-634c-47db-bb41-28791dfd1f8d"},
			"dezi_nummer":    {"123456789"},
			"voorletters":    {"A.B."},
			"voorvoegsel":    {"van"},
			"achternaam":     {"Tester"},
			"abonnee_nummer": {"987654321"},
			"abonnee_naam":   {"Test Zorgaanbieder"},
			"rol_code":       {"01.000"},
			"rol_naam":       {"Arts"},
			"rol_code_bron":  {"http://www.dezi.nl/rol_code_bron/big"},
		})
		require.NoError(t, err)
		defer loginResponse.Body.Close()

		// Should redirect to OIDC provider callback first
		require.Equal(t, http.StatusFound, loginResponse.StatusCode)
		providerCallbackLocation, err := loginResponse.Location()
		require.NoError(t, err)
		require.Contains(t, providerCallbackLocation.Path, "/auth/authorize/callback")

		// Follow the redirect to get the authorization code
		providerCallbackResponse, err := httpClient.Get(providerCallbackLocation.String())
		require.NoError(t, err)
		defer providerCallbackResponse.Body.Close()

		// Should now redirect to client callback URL with authorization code
		require.Equal(t, http.StatusFound, providerCallbackResponse.StatusCode)
		callbackLocation, err := providerCallbackResponse.Location()
		require.NoError(t, err)
		require.Equal(t, "localhost", callbackLocation.Host)
		require.Equal(t, "/callback", callbackLocation.Path)

		authCode := callbackLocation.Query().Get("code")
		require.NotEmpty(t, authCode)
		require.Equal(t, "test-state-123", callbackLocation.Query().Get("state"))

		// Step 3: Exchange authorization code for tokens
		tokenResponse, err := httpClient.PostForm(httpService.Internal().URL().JoinPath("/auth/token").String(), map[string][]string{
			"grant_type":    {"authorization_code"},
			"code":          {authCode},
			"redirect_uri":  {"http://localhost/callback"},
			"client_id":     {"test-client"},
			"client_secret": {"test-secret"},
		})
		require.NoError(t, err)
		defer tokenResponse.Body.Close()
		require.Equal(t, http.StatusOK, tokenResponse.StatusCode)

		tokenData, err := io.ReadAll(tokenResponse.Body)
		require.NoError(t, err)
		var tokens map[string]any
		require.NoError(t, json.Unmarshal(tokenData, &tokens))

		// Verify token response
		require.NotEmpty(t, tokens["access_token"])
		require.NotEmpty(t, tokens["id_token"])
		require.NotEmpty(t, tokens["expires_in"])
		require.Equal(t, "Bearer", tokens["token_type"])

		t.Run("Introspect Access Token", func(t *testing.T) {
			introspectURL := httpService.Internal().URL().JoinPath("/auth/introspect").String()

			// Create form data
			form := url.Values{}
			form.Set("token", tokens["access_token"].(string))

			req, err := http.NewRequest("POST", introspectURL, strings.NewReader(form.Encode()))
			require.NoError(t, err)
			req.SetBasicAuth("test-client", "test-secret")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			introspectResponse, err := httpClient.Do(req)
			require.NoError(t, err)
			defer introspectResponse.Body.Close()

			if introspectResponse.StatusCode != http.StatusOK {
				bodyData, _ := io.ReadAll(introspectResponse.Body)
				t.Logf("Introspection failed with status %d: %s", introspectResponse.StatusCode, string(bodyData))
			}
			require.Equal(t, http.StatusOK, introspectResponse.StatusCode)

			introspectData, err := io.ReadAll(introspectResponse.Body)
			require.NoError(t, err)
			var introspection map[string]any
			require.NoError(t, json.Unmarshal(introspectData, &introspection))

			require.Equal(t, true, introspection["active"])
		})
	})
}
