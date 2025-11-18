package authn

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
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
	httpConfig := httpComponent.DefaultConfig()
	httpConfig.InternalInterface = httpComponent.InterfaceConfig{
		Address: ":" + strconv.Itoa(p1),
		BaseURL: "http://localhost:" + strconv.Itoa(p1),
	}
	httpConfig.PublicInterface.Address = ":" + strconv.Itoa(p2)
	httpService := httpComponent.New(httpConfig, publicMux, internalMux)

	config := Config{
		Clients: []Client{
			{
				ID:     "test-client",
				Secret: "test-secret",
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
		require.Equal(t, data["issuer"], httpService.Public().URL().JoinPath("/auth").String())
	})
	//t.Run("Client Credentials grant type", func(t *testing.T) {
	//	httpResponse, err := http.PostForm(httpService.Internal().URL().JoinPath("/auth/token").String(), map[string][]string{
	//		"grant_type":    {"client_credentials"},
	//		"client_id":     {"test-client"},
	//		"client_secret": {"test-secret"},
	//		"scope":         {"openid"},
	//	})
	//	require.NoError(t, err)
	//	defer httpResponse.Body.Close()
	//	data, err := from.JSONResponse[map[string]any](httpResponse)
	//	require.NoError(t, err)
	//
	//	require.NotEmpty(t, data["access_token"])
	//	require.NotEmpty(t, data["expires_in"])
	//	require.Equal(t, data["token_type"], "Bearer")
	//	require.Equal(t, data["scope"], "openid")
	//
	//	t.Run("introspect token", func(t *testing.T) {
	//		httpRequest, _ := http.NewRequest(http.MethodPost, httpService.Internal().URL().JoinPath("/auth/introspect").String(), strings.NewReader(url.Values{
	//			"token": {data["access_token"].(string)},
	//		}.Encode()))
	//		httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	//		httpRequest.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("test-client:test-secret")))
	//
	//		httpResponse, err := http.DefaultClient.Do(httpRequest)
	//		require.NoError(t, err)
	//		defer httpResponse.Body.Close()
	//		response, err := from.JSONResponse[map[string]any](httpResponse)
	//
	//		require.NoError(t, err)
	//		require.Equal(t, true, response["active"])
	//		require.Equal(t, "openid", response["scope"])
	//		require.Equal(t, []interface{}{"TODO(audience)"}, response["aud"])
	//	})
	//})
}
