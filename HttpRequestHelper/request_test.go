package requesthelper_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	requesthelper "github.com/Azure/VMApplication-Extension/HttpRequestHelper"
	"github.com/ahmetalpbalkan/go-httpbin"
	"github.com/stretchr/testify/require"
)

type badDownloader struct{ calls int }

func (b *badDownloader) GetRequest() (*http.Request, error) {
	b.calls++
	return nil, errors.New("expected error")
}

func TestMakeRequest_wrapsGetRequestError(t *testing.T) {
	rm := requesthelper.GetMetadataRequestManager(new(badDownloader))
	_, _, err := rm.MakeRequest()
	require.NotNil(t, err)
	require.EqualError(t, err, "failed to create http request: expected error")
}

func TestMakeRequest_wrapsHTTPError(t *testing.T) {
	rm := requesthelper.GetMetadataRequestManager(requesthelper.NewURLRequest("bad url"))
	_, _, err := rm.MakeRequest()
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "http request failed:")
}

func TestMakeRequest_requestTimeout(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetRequestManagerWithTimeout(
		requesthelper.NewURLRequest(fmt.Sprintf("%s/delay/%d", srv.URL, 600)),
		500*time.Second)
	_, _, err := rm.MakeRequest()
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "timeout")

	rm = requesthelper.GetRequestManagerWithTimeout(
		requesthelper.NewURLRequest(fmt.Sprintf("%s/delay/%d", srv.URL, 400)),
		500*time.Second)
	_, body, err := rm.MakeRequest()
	require.Nil(t, err)
	defer body.Close()
	require.NotNil(t, body)
}

func TestMakeRequest_badStatusCodeFails(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	for _, code := range []int{
		http.StatusNotFound,
		http.StatusForbidden,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusBadRequest,
		http.StatusUnauthorized,
	} {
		rm := requesthelper.GetMetadataRequestManager(requesthelper.NewURLRequest(fmt.Sprintf("%s/status/%d", srv.URL, code)))
		_, _, err := rm.MakeRequest()
		require.NotNil(t, err, "not failed for code:%d", code)
		require.Contains(t, err.Error(), "unexpected status code", "wrong message for code %d", code)
	}
}

func TestMakeRequest_statusOKSucceeds(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetMetadataRequestManager(requesthelper.NewURLRequest(srv.URL + "/status/200"))
	_, body, err := rm.MakeRequest()
	require.Nil(t, err)
	defer body.Close()
	require.NotNil(t, body)
}

func TestMakeRequest_retrievesBody(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetMetadataRequestManager(requesthelper.NewURLRequest(srv.URL + "/bytes/65536"))
	_, body, err := rm.MakeRequest()
	require.Nil(t, err)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	require.Nil(t, err)
	require.EqualValues(t, 65536, len(b))
}

func TestMakeRequest_bodyClosesWithoutError(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetMetadataRequestManager(requesthelper.NewURLRequest(srv.URL + "/get"))
	_, body, err := rm.MakeRequest()
	require.Nil(t, err)
	require.Nil(t, body.Close(), "body should close fine")
}
