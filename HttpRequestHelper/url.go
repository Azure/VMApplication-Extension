package requesthelper

import (
	"net/http"
)

// urlRequest describes a URL to download.
type urlRequest struct {
	url string
}

// NewURLRequest creates a new  downloader with the provided URL
func NewURLRequest(url string) RequestFactory {
	return urlRequest{url}
}

// GetRequest returns a new request to download the URL
func (u urlRequest) GetRequest() (*http.Request, error) {
	return http.NewRequest("GET", u.url, nil)
}
