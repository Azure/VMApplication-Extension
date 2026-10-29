// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package requesthelper

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// RequestFactory describes a method to create HTTP requests.
type RequestFactory interface {
	// GetRequest returns a new GET request for the resource.
	GetRequest() (*http.Request, error)

	// GetFallbackRequest returns a new GET request for the secondary resource.
	GetFallbackRequest() (*http.Request, error)
}

// RequestManager provides an abstraction for http requests
type RequestManager struct {
	httpClient     *http.Client
	requestFactory RequestFactory
}

// GetRequestManager returns a request manager for json requests
func GetRequestManager(rf RequestFactory, timeout time.Duration) *RequestManager {
	return &RequestManager{
		httpClient:     getHTTPClient(timeout),
		requestFactory: rf,
	}
}

// httpClient is the default client to be used in downloading files from
// Internet. http.Get() uses a client without timeouts (http.DefaultClient)
// However, an infinite timeout will cause the deployment to fail.
func getHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 30 * time.Second,
			}).Dial,
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: timeout}
}

// MakeRequest retrieves a response body and checks the response status code to see
// if it is 200 OK and then returns the response body. It issues a new request
// every time called. It is caller's responsibility to close the response body.
func (rm *RequestManager) MakeRequest() (*http.Response, error) {
	return rm.makeRequest(rm.requestFactory.GetRequest)
}

// MakeFallbackRequest issues a new request using a fallback uri.
// For versioned metadata requests, it uses the base URL without adding the version query parameter.
// The caller is responsible for closing the response body.
func (rm *RequestManager) MakeFallbackRequest() (*http.Response, error) {
	fallbackRequest, err := rm.requestFactory.GetFallbackRequest()
	if err != nil {
		// return early if fallback request is not implemented or fails to be created
		return nil, err
	}
	return rm.makeRequest(func() (*http.Request, error) {
		return fallbackRequest, nil
	})
}

func (rm *RequestManager) makeRequest(getRequest func() (*http.Request, error)) (*http.Response, error) {
	req, err := getRequest()
	if err != nil {
		return nil, err
	}

	resp, err := rm.httpClient.Do(req)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		return resp, nil
	}

	err = fmt.Errorf("unexpected status code: actual=%d expected=%d", resp.StatusCode, http.StatusOK)

	return resp, err
}
