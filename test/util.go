package test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ReadJSON(t *testing.T, fs fs.ReadFileFS, fileName string) []byte {
	t.Helper()
	data, err := fs.ReadFile(fileName)
	require.NoError(t, err)
	return data
}

func ParseJSON[T any](t *testing.T, fs fs.ReadFileFS, fileName string) T {
	data := ReadJSON(t, fs, fileName)
	var result T
	err := json.Unmarshal(data, &result)
	require.NoError(t, err)
	return result
}

func WaitForHTTPStatus(testURL string, statusCode int) (chan struct{}, chan error) {
	done := make(chan struct{})
	errChan := make(chan error)

	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			resp, err := http.Get(testURL)
			if err == nil && resp.StatusCode == statusCode {
				return
			}
			if i < 9 {
				time.Sleep(1 * time.Second)
			} else {
				errChan <- err
				return
			}
		}
	}()

	return done, errChan
}

// TempDir creates a temporary directory and changes the working directory to it for the duration of the test.
func TempDir(t *testing.T) string {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})
	return tmpDir
}
