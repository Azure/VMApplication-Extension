package requesthelper

import (
	"math"
	"net/http"
	"time"

	"github.com/Azure/azure-extension-platform/pkg/logging"
)

// SleepFunc pauses the execution for at least duration d.
type SleepFunc func(d time.Duration)

var (
	// ActualSleep uses actual time to pause the execution.
	ActualSleep SleepFunc = time.Sleep
)

const (
	// time to sleep between retries is an exponential backoff formula:
	//   t(n) = k * m^n
	expRetryN = 7 // how many times we retry the Download
	expRetryK = time.Second * 3
	expRetryM = 2
)

// WithRetries retrieves a response body using the specified downloader. Any
// error returned from d will be retried (and retrieved response bodies will be
// closed on failures). If the retries do not succeed, the last error is returned.
//
// It sleeps in exponentially increasing durations between retries.
func WithRetries(el *logging.ExtensionLogger, rm *RequestManager, sf SleepFunc) (*http.Response, error) {
	var lastErr error

	for n := 0; n < expRetryN; n++ {
		resp, err := rm.MakeRequest()
		if err == nil {
			return resp, nil
		}

		lastErr = err
		el.Warn("error: %v", err)

		status := -1
		if resp != nil {
			if resp.Body != nil { // we are not going to read this response body
				resp.Body.Close()
			}

			status = resp.StatusCode
		}

		// status == -1 the value when there was no http request
		if status == -1 {
			el.Info("No response returned, skipping retries")
			break
		}

		if !isTransientHTTPStatusCode(status) {
			el.Info("RequestManager returned %v, skipping retries", status)
			break
		}

		if n != expRetryN-1 {
			// have more retries to go, sleep before retrying
			slp := expRetryK * time.Duration(int(math.Pow(float64(expRetryM), float64(n))))
			sf(slp)
		}
	}

	return nil, lastErr
}

func isTransientHTTPStatusCode(statusCode int) bool {
	switch statusCode {
	case
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true // timeout and too many requests
	default:
		return false
	}
}
