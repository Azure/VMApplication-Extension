// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package requesthelper_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/stretchr/testify/require"
)

func (*badDownloader) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

func (*testUrlRequest) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

func (*errorDownloader) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

func (*eofDownloader) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

func (*unexpectedEOFDownloader) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

func (*connResetDownloader) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

func (*serverClosedIdleDownloader) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

func (*readLoopPeekFailDownloader) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.ErrUnsupported
}

type fallbackTestFactory struct {
	url           string
	primaryErr    error
	fallbackErr   error
	primaryCalls  int
	fallbackCalls int
}

func (f *fallbackTestFactory) GetRequest() (*http.Request, error) {
	f.primaryCalls++
	if f.primaryErr != nil {
		return nil, f.primaryErr
	}
	return http.NewRequest(http.MethodGet, f.url, nil)
}

func (f *fallbackTestFactory) GetFallbackRequest() (*http.Request, error) {
	f.fallbackCalls++
	if f.fallbackErr != nil {
		return nil, f.fallbackErr
	}
	return http.NewRequest(http.MethodGet, f.url, nil)
}

func TestWithRetries_fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	primaryErr := errors.New("primary request failed")
	fallbackErr := errors.New("fallback request failed")
	for _, tt := range []struct {
		name          string
		primaryErr    error
		fallbackErr   error
		wantErr       error
		fallbackCalls int
	}{
		{"unsupported", primaryErr, errors.ErrUnsupported, primaryErr, 1},
		{"wrapped unsupported", primaryErr, fmt.Errorf("fallback: %w", errors.ErrUnsupported), primaryErr, 1},
		{"fallback succeeds", primaryErr, nil, nil, 1},
		{"fallback fails", primaryErr, fallbackErr, fallbackErr, 1},
		{"primary succeeds", nil, errors.ErrUnsupported, nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			factory := &fallbackTestFactory{
				url: srv.URL, primaryErr: tt.primaryErr, fallbackErr: tt.fallbackErr,
			}

			rm := requesthelper.GetRequestManager(factory, testRequestTimeout)
			sr := new(sleepRecorder)
			resp, err := requesthelper.WithRetries(nopLog(), rm, sr.Sleep)
			require.Equal(t, 1, factory.primaryCalls)
			require.Equal(t, tt.fallbackCalls, factory.fallbackCalls)
			require.Empty(t, *sr)
			if tt.wantErr != nil {
				require.Same(t, tt.wantErr, err)
				require.Nil(t, resp)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestWithRetries_unsupportedFallbackPreservesErrorAfterRetries(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"unsupported", errors.ErrUnsupported},
		{"wrapped unsupported", fmt.Errorf("fallback: %w", errors.ErrUnsupported)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			primaryErr := &requestError{timeout: true}
			factory := &fallbackTestFactory{
				primaryErr:  primaryErr,
				fallbackErr: tt.err,
			}
			rm := requesthelper.GetRequestManager(factory, testRequestTimeout)
			sr := new(sleepRecorder)

			resp, err := requesthelper.WithRetries(nopLog(), rm, sr.Sleep)

			require.Same(t, primaryErr, err)
			require.False(t, errors.Is(err, errors.ErrUnsupported))
			require.Nil(t, resp)
			require.Equal(t, 7, factory.primaryCalls)
			require.Equal(t, 1, factory.fallbackCalls)
			require.Equal(t, sleepSchedule, []time.Duration(*sr))
		})
	}
}
