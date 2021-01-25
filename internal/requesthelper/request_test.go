package requesthelper_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/ahmetalpbalkan/go-httpbin"
	"github.com/stretchr/testify/require"
)

var (
	testRequestTimeout = 2 * time.Second
)

type requestError struct {
	timeout   bool
	temporary bool
}

func (u requestError) Error() string {
	return "something happened"
}

func (u requestError) Temporary() bool {
	return u.temporary
}

func (u requestError) Timeout() bool {
	return u.timeout
}

type testUrlRequest struct {
	calls int
	url   string
}

type badDownloader struct{ calls int }

type errorDownloader struct {
	calls int
	err   requestError
}

func (b *badDownloader) GetRequest() (*http.Request, error) {
	b.calls++
	return nil, errors.New("expected error")
}

func NewTestURLRequest(url string) *testUrlRequest {
	return &testUrlRequest{0, url}
}

func NewErrorRequest(isTemporary bool, isTimeout bool) *errorDownloader {
	err := requestError{isTimeout, isTemporary}
	return &errorDownloader{0, err}
}

func (u *testUrlRequest) GetRequest() (*http.Request, error) {
	u.calls++
	return http.NewRequest("GET", u.url, nil)
}

func (e *errorDownloader) GetRequest() (*http.Request, error) {
	e.calls++
	return nil, e.err
}

func TestMakeRequest_wrapsGetRequestError(t *testing.T) {
	rm := requesthelper.GetRequestManager(new(badDownloader), testRequestTimeout)
	_, err := rm.MakeRequest()
	require.NotNil(t, err)
	require.EqualError(t, err, "expected error")
}

func TestMakeRequest_wrapsHTTPError(t *testing.T) {
	rm := requesthelper.GetRequestManager(NewTestURLRequest("bad url"), testRequestTimeout)
	_, err := rm.MakeRequest()
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "unsupported protocol scheme")
}

func TestMakeRequest_requestTimeout(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetRequestManager(
		NewTestURLRequest(fmt.Sprintf("%s/delay/%d", srv.URL, 3)),
		testRequestTimeout)
	_, err := rm.MakeRequest()
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "Timeout")

	rm = requesthelper.GetRequestManager(
		NewTestURLRequest(fmt.Sprintf("%s/delay/%d", srv.URL, 1)),
		testRequestTimeout)
	resp, err := rm.MakeRequest()
	require.Nil(t, err)
	defer resp.Body.Close()
	require.NotNil(t, resp.Body)
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
		rm := requesthelper.GetRequestManager(
			NewTestURLRequest(fmt.Sprintf("%s/status/%d", srv.URL, code)),
			testRequestTimeout)
		_, err := rm.MakeRequest()
		require.NotNil(t, err, "not failed for code:%d", code)
		require.Contains(t, err.Error(), "unexpected status code", "wrong message for code %d", code)
	}
}

func TestMakeRequest_statusOKSucceeds(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetRequestManager(NewTestURLRequest(srv.URL+"/status/200"), testRequestTimeout)
	resp, err := rm.MakeRequest()
	require.Nil(t, err)
	defer resp.Body.Close()
	require.NotNil(t, resp.Body)
}

func TestMakeRequest_statusPartialResultsSucceeds(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetRequestManager(NewTestURLRequest(srv.URL+"/status/206"), testRequestTimeout)
	resp, err := rm.MakeRequest()
	require.Nil(t, err)
	defer resp.Body.Close()
	require.NotNil(t, resp.Body)
}

func TestMakeRequest_retrievesBody(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetRequestManager(NewTestURLRequest(srv.URL+"/bytes/65536"), testRequestTimeout)
	resp, err := rm.MakeRequest()
	require.Nil(t, err)
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err)
	require.EqualValues(t, 65536, len(b))
}

func TestMakeRequest_bodyClosesWithoutError(t *testing.T) {
	srv := httptest.NewServer(httpbin.GetMux())
	defer srv.Close()

	rm := requesthelper.GetRequestManager(NewTestURLRequest(srv.URL+"/get"), testRequestTimeout)
	resp, err := rm.MakeRequest()
	require.Nil(t, err)
	require.Nil(t, resp.Body.Close(), "body should close fine")
}
