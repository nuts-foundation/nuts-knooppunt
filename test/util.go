package test

import (
	"net/http"
	"time"
)

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
