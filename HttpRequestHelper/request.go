package requesthelper

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

var (
	downloadRequestTimeout = 1 * time.Hour
	metadataRequestTimeout = 30 * time.Second
)

// RequestFactory describes a method to create HTTP requests.
type RequestFactory interface {
	// GetRequest returns a new GET request for the resource.
	GetRequest() (*http.Request, error)
}

// RequestManager provides an abstraction for making HTTP requests
type RequestManager struct {
	httpClient     *http.Client
	requestFactory RequestFactory
}

// GetMetadataRequestManager returns a request manager for json requests
func GetMetadataRequestManager(rf RequestFactory) *RequestManager {
	return GetRequestManagerWithTimeout(rf, metadataRequestTimeout)
}

// GetDownloadRequestManager returns a request manager for downloading files
func GetDownloadRequestManager(rf RequestFactory) *RequestManager {
	return GetRequestManagerWithTimeout(rf, downloadRequestTimeout)
}

// GetRequestManagerWithTimeout returns a request manager with the specified timeout
func GetRequestManagerWithTimeout(rf RequestFactory, timeout time.Duration) *RequestManager {
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
		}}
}

// MakeRequest retrieves a response body and checks the response status code to see
// if it is 200 OK and then returns the response body. It issues a new request
// every time called. It is caller's responsibility to close the response body.
func (rm *RequestManager) MakeRequest() (int, io.ReadCloser, error) {
	req, err := rm.requestFactory.GetRequest()
	if err != nil {
		return -1, nil, errors.Wrapf(err, "failed to create http request")
	}

	resp, err := rm.httpClient.Do(req)
	if err != nil {
		return -1, nil, errors.Wrapf(err, "http request failed")
	}

	if resp.StatusCode == http.StatusOK {
		return resp.StatusCode, resp.Body, nil
	}

	err = fmt.Errorf("unexpected status code: actual=%d expected=%d", resp.StatusCode, http.StatusOK)

	return resp.StatusCode, nil, err
}
