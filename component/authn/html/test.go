package html

import (
	"io"
	"net/http"
)

func RenderTestStartAuthn(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	httpResponse.Header().Add("Content-Type", "text/html; charset=utf-8")
	_ = testStartTemplate.Execute(httpResponse, nil)
}

func RenderTestCallback(tokenEndpoint string, redirectURL string) func(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	return func(httpResponse http.ResponseWriter, httpRequest *http.Request) {
		errCode := httpRequest.URL.Query().Get("error")
		if errCode != "" {
			httpResponse.Header().Add("Content-Type", "text/html; charset=utf-8")
			_, _ = httpResponse.Write([]byte("<html><body><h1>Authentication failed: " + errCode + "</h1><p>" + httpRequest.URL.Query().Get("error_description") + "</p>" +
				"<p><a href=\"./test\">Try again</a></p>" +
				"</body></html>"))
			return
		}
		httpResponse.Header().Add("Content-Type", "text/html; charset=utf-8")
		tokens, err := retrieveTokens(tokenEndpoint, redirectURL, httpRequest.URL.Query().Get("code"))
		if err != nil {
			_, _ = httpResponse.Write([]byte("<html><body><h1>Token retrieval failed: " + err.Error() + "</h1></body></html>"))
			return
		}
		_, _ = httpResponse.Write([]byte("<html><body><h1>Test Callback Successful</h1><pre style=\"white-space: wrap\">" + tokens + "</pre></body></html>"))
	}
}

func retrieveTokens(tokenEndpoint string, redirectURL string, code string) (string, error) {
	httpResponse, err := http.PostForm(tokenEndpoint, map[string][]string{
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURL},
		"code":          {code},
		"client_id":     {"local"},
		"client_secret": {"local-secret"},
	})
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()
	responseData, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return "", err
	}
	return string(responseData), nil
}
