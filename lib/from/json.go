package from

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

// maxErrorBodySize caps how much of a non-2xx response body ends up in the error.
const maxErrorBodySize = 512

// JSONResponse decodes a 2xx response's JSON body into T. A non-2xx response becomes an error
// holding the status, the URL without its query and the start of the response body. Only use it
// for services that never receive patient data, since the body may echo the request.
func JSONResponse[T any](httpResponse *http.Response) (T, error) {
	var result T
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		responseData, _ := io.ReadAll(io.LimitReader(httpResponse.Body, maxErrorBodySize))
		return result, fmt.Errorf("non-OK status code (status=%s, url=%s)\nResponse data:\n----------------\n%s\n----------------", httpResponse.Status, logging.RedactURL(httpResponse.Request.URL), strings.TrimSpace(string(responseData)))
	}
	if err := json.NewDecoder(httpResponse.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("failed to decode response body: %w", err)
	}
	return result, nil
}
