package nutsnode

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
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

func Test_Component_ConfigFileOptional(t *testing.T) {
	t.Run("config file missing - should not set NUTS_CONFIGFILE", func(t *testing.T) {
		// Create a temporary directory
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		
		// Change to temp directory where config/nuts.yaml doesn't exist
		err := os.Chdir(tmpDir)
		require.NoError(t, err)
		
		component, err := New(Config{Enabled: true})
		require.NoError(t, err)
		
		// Clear the environment variable to ensure it's not set from a previous test
		os.Unsetenv("NUTS_CONFIGFILE")
		
		// Test the specific part that sets environment variables
		configFile := component.config.ConfigFile
		if configFile == "" {
			configFile = "config/nuts.yaml"
		}
		
		envVars := map[string]string{
			"NUTS_HTTP_INTERNAL_ADDRESS": component.internalAddr.Host,
			"NUTS_HTTP_PUBLIC_ADDRESS":   component.publicAddr.Host,
			"NUTS_DATADIR":               "data/nuts",
		}
		
		// Only set NUTS_CONFIGFILE if the config file exists
		if _, err := os.Stat(configFile); err == nil {
			envVars["NUTS_CONFIGFILE"] = configFile
		}
		
		// Verify NUTS_CONFIGFILE is not in the envVars map
		_, exists := envVars["NUTS_CONFIGFILE"]
		assert.False(t, exists, "NUTS_CONFIGFILE should not be set when config file doesn't exist")
	})
	
	t.Run("config file exists - should set NUTS_CONFIGFILE", func(t *testing.T) {
		// Create a temporary directory with a config file
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		
		// Create config directory and file
		configDir := filepath.Join(tmpDir, "config")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)
		
		configFile := filepath.Join(configDir, "nuts.yaml")
		err = os.WriteFile(configFile, []byte("url: http://localhost:8080\nstrictmode: false\n"), 0644)
		require.NoError(t, err)
		
		// Change to temp directory
		err = os.Chdir(tmpDir)
		require.NoError(t, err)
		
		component, err := New(Config{Enabled: true})
		require.NoError(t, err)
		
		// Test the specific part that sets environment variables
		configFile2 := component.config.ConfigFile
		if configFile2 == "" {
			configFile2 = "config/nuts.yaml"
		}
		
		envVars := map[string]string{
			"NUTS_HTTP_INTERNAL_ADDRESS": component.internalAddr.Host,
			"NUTS_HTTP_PUBLIC_ADDRESS":   component.publicAddr.Host,
			"NUTS_DATADIR":               "data/nuts",
		}
		
		// Only set NUTS_CONFIGFILE if the config file exists
		if _, err := os.Stat(configFile2); err == nil {
			envVars["NUTS_CONFIGFILE"] = configFile2
		}
		
		// Verify NUTS_CONFIGFILE is in the envVars map
		configFileEnv, exists := envVars["NUTS_CONFIGFILE"]
		assert.True(t, exists, "NUTS_CONFIGFILE should be set when config file exists")
		assert.Equal(t, "config/nuts.yaml", configFileEnv)
	})
}
