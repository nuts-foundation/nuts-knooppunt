package from

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func JSONResponse[T any](httpResponse *http.Response) (T, error) {
	var result T
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		responseData, _ := io.ReadAll(httpResponse.Body)
		return result, fmt.Errorf("non-OK status code (status=%s, url=%s)\nResponse data:\n----------------\n%s\n----------------", httpResponse.Status, httpResponse.Request.URL, strings.TrimSpace(string(responseData)))
	}
	if err := json.NewDecoder(httpResponse.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("failed to decode response body: %w", err)
	}
	return result, nil
}
