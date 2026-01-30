// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

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

// retryRequest is a shared function for retrying HTTP requests with exponential backoff.
func retryRequest(
	el *logging.ExtensionLogger,
	sf SleepFunc,
	requestFunc func() (*http.Response, error),
	warnMsg string,
	infoPrefix string,
) (*http.Response, error) {
	var lastErr error

	for n := range expRetryN {
		resp, err := requestFunc()
		if err == nil {
			return resp, nil
		}

		lastErr = err
		el.Warn(warnMsg, err)

		status := -1
		if resp != nil {
			if resp.Body != nil { // we are not going to read this response body
				resp.Body.Close()
			}

			status = resp.StatusCode
		}

		// status == -1 the value when there was no http request
		if status == -1 {
			te, haste := lastErr.(interface {
				Temporary() bool
			})
			to, hasto := lastErr.(interface {
				Timeout() bool
			})

			if haste || hasto {
				if haste && te.Temporary() {
					el.Info("%sTemporary error occurred. Retrying: %v", infoPrefix, lastErr)
				} else if hasto && to.Timeout() {
					el.Info("%sTimeout error occurred. Retrying: %v", infoPrefix, lastErr)
				} else {
					el.Info("%sNon-timeout, non-temporary error occurred, skipping retries: %v", infoPrefix, lastErr)
					break
				}
			} else {
				el.Info("%sNo response returned and unexpected error, skipping retries", infoPrefix)
				break
			}
		} else if !isTransientHTTPStatusCode(status) {
			el.Info("%sRequest returned %v, skipping retries", infoPrefix, status)
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

// WithRetries retrieves a response body using the specified downloader. Any
// error returned from d will be retried (and retrieved response bodies will be
// closed on failures). If the retries do not succeed, the last error is returned.
//
// It sleeps in exponentially increasing durations between retries.
func WithRetries(el *logging.ExtensionLogger, rm *RequestManager, sf SleepFunc) (*http.Response, error) {
	return retryRequest(
		el,
		sf,
		rm.MakeRequest,
		"error: %v",
		"",
	)
}

func isTransientHTTPStatusCode(statusCode int) bool {
	switch statusCode {
	case
		http.StatusNotFound,            // 404. This will occur if there is a timing issue with the NodeService CCF cache
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

// WithRetriesArc retrieves a response body using the Arc authentication handler with retries.
// It handles the Arc challenge-response flow and retries on transient errors.
func WithRetriesArc(el *logging.ExtensionLogger, arcHandler *ArcAuthHandler, sf SleepFunc) (*http.Response, error) {
	return retryRequest(
		el,
		sf,
		func() (*http.Response, error) { return arcHandler.MakeArcRequest(el) },
		"Arc request error: %v",
		"Arc request: ",
	)
}
