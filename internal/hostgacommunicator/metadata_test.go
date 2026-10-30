// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package hostgacommunicator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVmAppInfo_FallbackSucceedsAfterVersionedRetries(t *testing.T) {
	expected := VMAppMetadataReceiver{
		ApplicationName:    myAppName,
		Version:            myAppVersion,
		InstallCommand:     "install.sh",
		UpdateCommand:      "update.sh",
		RemoveCommand:      "remove.sh",
		DirectDownloadOnly: "true",
		Config:             "config",
		PackageFileName:    "package.zip",
		ConfigFileName:     "config.json",
		RebootBehavior:     "Rerun",
	}
	output, err := json.Marshal(expected)
	require.NoError(t, err)

	var primaryCalls, fallbackCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/applications/"+myAppName+"/metadata", r.URL.Path)
		if r.URL.Query().Has(versionQueryParameterName) {
			primaryCalls.Add(1)
			assert.Equal(t, myAppVersion, r.URL.Query().Get(versionQueryParameterName))
			assert.Zero(t, fallbackCalls.Load())
			w.WriteHeader(http.StatusNotFound) // simulate HGAP returning 404 for versioned request
			return
		}

		fallbackCalls.Add(1)
		assert.EqualValues(t, 7, primaryCalls.Load())
		assert.Empty(t, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write(output)
		assert.NoError(t, writeErr)
	}))
	defer srv.Close()
	t.Setenv(WireProtocolAddress, srv.URL)

	originalSleep := requesthelper.ActualSleep
	var sleeps []time.Duration
	requesthelper.ActualSleep = func(d time.Duration) {
		sleeps = append(sleeps, d)
	}
	t.Cleanup(func() { requesthelper.ActualSleep = originalSleep })

	hgc := &HostGaCommunicator{}
	actual, err := hgc.GetVMAppInfo(nopLog(), myAppName, myAppVersion)

	require.NoError(t, err)
	require.Equal(t, expected.MapToVMAppMetadata(), actual)
	require.EqualValues(t, 7, primaryCalls.Load())
	require.EqualValues(t, 1, fallbackCalls.Load())
	require.Equal(t, []time.Duration{
		3 * time.Second,
		6 * time.Second,
		12 * time.Second,
		24 * time.Second,
		48 * time.Second,
		96 * time.Second,
	}, sleeps)
}

func TestVersionedMetadataRequestFactory_GetFallbackRequest(t *testing.T) {
	const baseURL = "http://127.0.0.1:32526/applications/testapp/metadata"
	for _, tt := range []struct {
		name    string
		version string
		query   string
	}{
		{"version", "1.0.0", ""},
		{"encoded version", "1.0+build &test", ""},
		{"empty version", "", ""},
		{"unrelated query", "1.0.0", "?other=value"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalURL := baseURL + tt.query
			t.Setenv(WireProtocolAddress, "http://127.0.0.1:32526"+tt.query)
			factory, err := newVersionedMetadataRequestFactory(nopLog(), "testapp", tt.version)
			require.NoError(t, err)
			require.Equal(t, originalURL, factory.url)
			require.Equal(t, tt.version, factory.vmAppMetadataVersion)

			primary, err := factory.GetRequest()
			require.NoError(t, err)
			require.Equal(t, http.MethodGet, primary.Method)
			require.Equal(t, tt.version, primary.URL.Query().Get(versionQueryParameterName))
			require.Equal(t, tt.version != "", primary.URL.Query().Has(versionQueryParameterName))
			primaryURL := primary.URL.String()

			fallback, err := factory.GetFallbackRequest()
			require.NoError(t, err)
			require.Equal(t, http.MethodGet, fallback.Method)
			require.NotContains(t, fallback.URL.Query(), versionQueryParameterName)
			require.Equal(t, tt.query != "", fallback.URL.Query().Has("other"))
			require.Equal(t, primary.URL.Query().Get("other"), fallback.URL.Query().Get("other"))
			require.False(t, fallback.URL.ForceQuery)
			require.Equal(t, originalURL, fallback.URL.String())
			require.NotSame(t, primary, fallback)
			require.Equal(t, originalURL, factory.url)
			require.Equal(t, primaryURL, primary.URL.String())

			nextPrimary, err := factory.GetRequest()
			require.NoError(t, err)
			require.Equal(t, primaryURL, nextPrimary.URL.String())
		})
	}
}
