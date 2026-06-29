// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package hostgacommunicator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	myAppName = "chipmunkdetector"
)

var testDirPath string

func createTestDir(t *testing.T) {
	wd, err := os.Getwd()
	require.Nil(t, err, "Couldn't get working directory")
	testDirPath = path.Join(wd, "TestArtifacts")

	_, err = os.Stat(testDirPath)
	if os.IsNotExist(err) {
		err = os.MkdirAll(testDirPath, 0755)
		require.Nil(t, err, "Could not create the test directory")
	}
}

func cleanupTestDir() {
	os.RemoveAll(testDirPath)
}

func TestGetVmAppInfo_InvalidUri(t *testing.T) {
	os.Setenv(WireProtocolAddress, "h%t!p:notgoingtohappen!")
	hgc := &HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*HostGaCommunicatorGetVMAppInfoError)
	require.True(t, ok, "expected error to be of type *HostGaCommunicatorGetVMAppInfoError")
	require.Contains(t, err.Error(), InitializationError.ToString(), "Wrong error code")
	require.Contains(t, err.Error(), "Could not parse the HostGA URI", "Wrong message for invalid uri")
}

func TestGetVmAppInfo_RequestFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*HostGaCommunicatorGetVMAppInfoError)
	require.True(t, ok, "expected error to be of type *HostGaCommunicatorGetVMAppInfoError")
	require.Contains(t, err.Error(), MetadataRequestFailedWithRetries.ToString(), "Wrong error code")
	require.Contains(t, err.Error(), "Metadata request failed after retries:", "Wrong message for failed request")
}

func TestGetVmAppInfo_CouldNotDecodeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		b := []byte(`"Type":"Chipmunk","Age":6,"FavoriteFoods":["Acorns","Cookies"]}`)
		w.Write(b)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*HostGaCommunicatorGetVMAppInfoError)
	require.True(t, ok, "expected error to be of type *HostGaCommunicatorGetVMAppInfoError")
	require.Contains(t, err.Error(), MetadataRequestFailedInvalidResponseBody.ToString(), "Wrong error code")
	require.Contains(t, err.Error(), "Failed to decode response body:", "Wrong message for invalid response")
}

func TestGetVmAppInfo_MissingProperties(t *testing.T) {
	expected := VMAppMetadataReceiver{
		ApplicationName: "chipmunk",
		Version:         "42",
		InstallCommand:  "installchipmunk.bat",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		output, err := json.Marshal(expected)
		require.Nil(t, err, "Marshal failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(output)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	actual, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.Nil(t, err, "request failed")
	require.Equal(t, expected.ApplicationName, actual.ApplicationName)
	require.Equal(t, expected.Version, actual.Version)
	require.Equal(t, expected.InstallCommand, actual.InstallCommand)
}

func TestGetVmAppInfo_ValidResponse(t *testing.T) {
	expected := VMAppMetadataReceiver{
		ApplicationName:    "chipmunk",
		Version:            "42",
		InstallCommand:     "installchipmunk.bat",
		UpdateCommand:      "updatechipmunk.bat",
		RemoveCommand:      "removechipmunk.bat",
		DirectDownloadOnly: "false",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		output, err := json.Marshal(expected)
		require.Nil(t, err, "Marshal failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(output)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	actual, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.Nil(t, err, "request failed")
	require.Equal(t, expected.ApplicationName, actual.ApplicationName)
	require.Equal(t, expected.Version, actual.Version)
	require.Equal(t, expected.InstallCommand, actual.InstallCommand)
	require.Equal(t, expected.UpdateCommand, actual.UpdateCommand)
	require.Equal(t, expected.RemoveCommand, actual.RemoveCommand)
	require.Equal(t, expected.DirectDownloadOnly, fmt.Sprintf("%v", actual.DirectDownloadOnly))
}

func TestOperations_ImmediateCloseTransportError_RetriedByRequestHelper(t *testing.T) {
	createTestDir(t)
	defer cleanupTestDir()

	origSleep := requesthelper.ActualSleep
	requesthelper.ActualSleep = func(time.Duration) {}
	defer func() { requesthelper.ActualSleep = origSleep }()

	runWithImmediateClose := func(t *testing.T, expectedAttempts int32, op func(hgc *HostGaCommunicator) error) {
		t.Helper()

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		var attempts atomic.Int32
		go func() {
			for {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				attempts.Add(1)
				_ = conn.Close()
			}
		}()

		t.Setenv(WireProtocolAddress, listener.Addr().String())
		hgc := &HostGaCommunicator{}

		err = op(hgc)
		require.Error(t, err)
		require.True(
			t,
			isExpectedImmediateCloseTransportError(err),
			"expected EOF-family, idle-close, or connection reset error, got: %s",
			err.Error(),
		)
		require.Equal(t, expectedAttempts, attempts.Load(), "unexpected number of HTTP attempts")
	}

	t.Run("metadata", func(t *testing.T) {
		runWithImmediateClose(t, 7, func(hgc *HostGaCommunicator) error {
			_, opErr := hgc.GetVMAppInfo(nopLog(), myAppName)
			_, ok := opErr.(*HostGaCommunicatorGetVMAppInfoError)
			require.True(t, ok)
			require.Contains(t, opErr.Error(), MetadataRequestFailedWithRetries.ToString())
			return opErr
		})
	})

	t.Run("package", func(t *testing.T) {
		runWithImmediateClose(t, 7, func(hgc *HostGaCommunicator) error {
			dst := path.Join(testDirPath, "ImmediateClosePackage.bin")
			opErr := hgc.DownloadPackage(nopLog(), myAppName, dst)
			_, ok := opErr.(*DownloadPackageError)
			require.True(t, ok)
			require.Contains(t, opErr.Error(), DownloadPackageFileError.ToString())
			require.Contains(t, opErr.Error(), "Download request failed with retries")
			return opErr
		})
	})

	t.Run("config", func(t *testing.T) {
		runWithImmediateClose(t, 7, func(hgc *HostGaCommunicator) error {
			dst := path.Join(testDirPath, "ImmediateCloseConfig.bin")
			opErr := hgc.DownloadConfig(nopLog(), myAppName, dst)
			_, ok := opErr.(*DownloadConfigError)
			require.True(t, ok)
			require.Contains(t, opErr.Error(), DownloadConfigFileError.ToString())
			require.Contains(t, opErr.Error(), "Download request failed with retries")
			return opErr
		})
	})
}

func TestOperations_TruncatedTransportResponse_RetriedByRequestHelper(t *testing.T) {
	createTestDir(t)
	defer cleanupTestDir()

	origSleep := requesthelper.ActualSleep
	requesthelper.ActualSleep = func(time.Duration) {}
	defer func() { requesthelper.ActualSleep = origSleep }()

	// Simulate a transport-level truncation (partial HTTP response headers, then close).
	// This should fail during http.Client.Do() and be retried by requesthelper.retryRequest.
	runWithTruncatedResponse := func(t *testing.T, expectedAttempts int32, op func(hgc *HostGaCommunicator) error) {
		t.Helper()

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		var attempts atomic.Int32
		go func() {
			for {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				attempts.Add(1)

				_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\n"))
				_, _ = conn.Write([]byte("Content-Length: 128\r\n"))
				_, _ = conn.Write([]byte("Content-Type: application/octet-stream\r\n"))
				// Close before completing headers/body to trigger Do()-time truncated response.
				_ = conn.Close()
			}
		}()

		t.Setenv(WireProtocolAddress, listener.Addr().String())
		hgc := &HostGaCommunicator{}

		err = op(hgc)
		require.Error(t, err)
		require.Equal(t, expectedAttempts, attempts.Load(), "unexpected number of HTTP attempts")
	}

	t.Run("metadata", func(t *testing.T) {
		runWithTruncatedResponse(t, 7, func(hgc *HostGaCommunicator) error {
			_, opErr := hgc.GetVMAppInfo(nopLog(), myAppName)
			_, ok := opErr.(*HostGaCommunicatorGetVMAppInfoError)
			require.True(t, ok)
			require.Contains(t, opErr.Error(), MetadataRequestFailedWithRetries.ToString())
			require.Contains(t, opErr.Error(), "unexpected EOF")
			return opErr
		})
	})

	t.Run("package", func(t *testing.T) {
		runWithTruncatedResponse(t, 7, func(hgc *HostGaCommunicator) error {
			dst := path.Join(testDirPath, "TruncatedPackage.bin")
			opErr := hgc.DownloadPackage(nopLog(), myAppName, dst)
			_, ok := opErr.(*DownloadPackageError)
			require.True(t, ok)
			require.Contains(t, opErr.Error(), DownloadPackageFileError.ToString())
			require.Contains(t, opErr.Error(), "Download request failed with retries")
			require.Contains(t, opErr.Error(), "unexpected EOF")
			return opErr
		})
	})

	t.Run("config", func(t *testing.T) {
		runWithTruncatedResponse(t, 7, func(hgc *HostGaCommunicator) error {
			dst := path.Join(testDirPath, "TruncatedConfig.bin")
			opErr := hgc.DownloadConfig(nopLog(), myAppName, dst)
			_, ok := opErr.(*DownloadConfigError)
			require.True(t, ok)
			require.Contains(t, opErr.Error(), DownloadConfigFileError.ToString())
			require.Contains(t, opErr.Error(), "Download request failed with retries")
			require.Contains(t, opErr.Error(), "unexpected EOF")
			return opErr
		})
	})
}

func TestDownloadPackage_CannotRemoveExistingFile(t *testing.T) {
	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "LockedFile")
	lf, err := os.Create(filePath)
	require.Nil(t, err, "File creation failed")
	defer lf.Close()
	removeFileFunc = func(filename string) error {
		return errors.New("Could not remove the existing file")
	}
	defer func() { removeFileFunc = removeFile }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err = hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*DownloadPackageError)
	require.True(t, ok, "expected error to be of type *DownloadPackageError")
	require.Contains(t, err.Error(), "DownloadPackageFileError", "Wrong error code")
	require.Contains(t, err.Error(), "Could not remove the existing file", "Wrong message for failing to remove locked file")
}

func TestDownloadPackage_InvalidUri(t *testing.T) {
	os.Setenv(WireProtocolAddress, "htt!p:notgoingtohappen!")
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, "somepath")
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*DownloadPackageError)
	require.True(t, ok, "expected error to be of type *DownloadPackageError")
	require.Contains(t, err.Error(), DownloadPackageRequestFactoryError.ToString(), "Wrong error type")
}

func TestDownloadPackage_InvalidPath(t *testing.T) {
	filePath := string(make([]byte, 5)) // null characters in file names are invalid in both windows and linux

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*DownloadPackageError)
	require.True(t, ok, "expected error to be of type *DownloadPackageError")
	require.Contains(t, err.Error(), "DownloadPackageFileError", "Wrong error code")
	require.Contains(t, err.Error(), "Cannot retrieve file information", "Wrong message for invalid file path")
}

func TestDownloadPackage_SingeCallDownload(t *testing.T) {
	expected := "file contents don't matter"
	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "SingleCallFile")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := []byte(expected)
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.Nil(t, err, "Download failed")
	verifyFileContents(t, filePath, expected)
}

func TestDownloadPackage_TooManyTries(t *testing.T) {
	totalCallCount := 11
	chunk := "This will fail after too many attempts."

	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "TooManyTriesFile")

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := []byte(chunk)
		callCount++

		if callCount < totalCallCount {
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		w.Write(b)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*DownloadPackageError)
	require.True(t, ok, "expected error to be of type *DownloadPackageError")
	require.Contains(t, err.Error(), "DownloadPackageFileError", "Wrong error code")
	require.Contains(t, err.Error(), "Failed to completely download the file", "Wrong message for incomplete file")
}

func TestDownloadPackage_IntermediateCallFails(t *testing.T) {
	failAtCallCount := 5
	chunk := "This is doomed to fail."

	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "TooManyTriesFile")

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := []byte(chunk)
		callCount++

		if callCount < failAtCallCount {
			w.WriteHeader(http.StatusPartialContent)
			w.Write(b)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*DownloadPackageError)
	require.True(t, ok, "expected error to be of type *DownloadPackageError")
	require.Contains(t, err.Error(), "DownloadPackageFileError", "Wrong error code")
	require.Contains(t, err.Error(), "Unrecoverable error while downloading the file", "Wrong message for failure mid-retries")
}

func TestDownloadPackage_MultipleCallDownload(t *testing.T) {
	expectedCallCount := 10
	chunk := "I will keep trying until this test succeeds."
	var buffer bytes.Buffer
	for i := 0; i < expectedCallCount; i++ {
		buffer.WriteString(chunk)
	}
	expected := buffer.String()

	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "MultipleCallFile")

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := []byte(chunk)
		callCount++

		if callCount < expectedCallCount {
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		w.Write(b)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.Nil(t, err, "Download failed")
	require.Equal(t, expectedCallCount, callCount)
	verifyFileContents(t, filePath, expected)
}

// TestDownloadPackage_UnexpectedEOFIsRecoverable verifies that a truncated response body
// (surfacing as io.ErrUnexpectedEOF during io.Copy) is treated as recoverable and the
// next attempt completes the download successfully.
func TestDownloadPackage_UnexpectedEOFIsRecoverable(t *testing.T) {
	expected := "This download should succeed after recovering from an unexpected EOF."
	firstChunk := expected[:20]
	secondChunk := expected[20:]

	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "UnexpectedEOFRecoverableFile")

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount == 1 {
			// Hijack the connection so we can write a deliberately truncated HTTP response.
			// We advertise a larger Content-Length than bytes actually sent, then close.
			// The client should surface io.ErrUnexpectedEOF while copying the body.
			hj, ok := w.(http.Hijacker)
			require.True(t, ok, "response writer does not support hijacking")

			conn, rw, err := hj.Hijack()
			require.NoError(t, err, "failed to hijack HTTP connection")
			defer conn.Close()

			_, err = rw.WriteString("HTTP/1.1 200 OK\r\n")
			require.NoError(t, err)
			_, err = rw.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(expected)))
			require.NoError(t, err)
			_, err = rw.WriteString("Content-Type: application/octet-stream\r\n\r\n")
			require.NoError(t, err)
			_, err = rw.WriteString(firstChunk)
			require.NoError(t, err)
			require.NoError(t, rw.Flush())
			return
		}

		// Second request is a normal successful response with the remaining bytes.
		// The test verifies the download logic treats the first failure as recoverable.
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(secondChunk))
		require.NoError(t, err)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NoError(t, err, "Download should recover from io.ErrUnexpectedEOF and succeed")
	require.Equal(t, 2, callCount, "Expected one retry after interrupted download")
	verifyFileContents(t, filePath, expected)
}

func TestDownloadConfig_InvalidUri(t *testing.T) {
	os.Setenv(WireProtocolAddress, "htt!p:notgoingtohappen!")
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadConfig(nopLog(), myAppName, "somepath")
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*DownloadConfigError)
	require.True(t, ok, "expected error to be of type *DownloadConfigError")
	require.Contains(t, err.Error(), DownloadConfigRequestFactoryError.ToString(), "Wrong error code")
}

func TestDownloadConfig_InvalidPath(t *testing.T) {
	filePath := string(make([]byte, 5)) // null characters in file names are invalid in both windows and linux

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadConfig(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
	_, ok := err.(*DownloadConfigError)
	require.True(t, ok, "expected error to be of type *DownloadConfigError")
	require.Contains(t, err.Error(), DownloadConfigFileError.ToString(), "Wrong error code")
	require.Contains(t, err.Error(), "Cannot retrieve file information", "Wrong message for invalid file path")
}

func TestDownloadConfig_SingeCallDownload(t *testing.T) {
	expected := "file contents don't matter"
	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "SingleCallConfig")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := []byte(expected)
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadConfig(nopLog(), myAppName, filePath)
	require.Nil(t, err, "Download failed")
	verifyFileContents(t, filePath, expected)
}

func TestGetOperationUri(t *testing.T) {
	appName := "myApp"
	operation := "metadata"

	el := logging.New(nil)
	os.Setenv(WireProtocolAddress, "10.0.0.1")
	uri, err := getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://10.0.0.1:%s/applications/%s/%s", hostGaPluginPort, appName, operation), uri)

	os.Setenv(WireProtocolAddress, "10.0.0.1:1234")
	uri, err = getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://10.0.0.1:1234/applications/%s/%s", appName, operation), uri)

	os.Setenv(WireProtocolAddress, "foo.bar.com")
	uri, err = getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://foo.bar.com:%s/applications/%s/%s", hostGaPluginPort, appName, operation), uri)

	os.Setenv(WireProtocolAddress, "foo.bar.com:1568")
	uri, err = getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://foo.bar.com:1568/applications/%s/%s", appName, operation), uri)

	os.Setenv(WireProtocolAddress, "https://foo.bar.com:1568")
	uri, err = getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("https://foo.bar.com:1568/applications/%s/%s", appName, operation), uri)

	// test fallback address for Wire Server
	os.Setenv(WireProtocolAddress, "")
	uri, err = getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	if isArcAgentPresent(el) {
		assert.Equal(t, fmt.Sprintf("%s/applications/%s/%s", "https://localhost:40342", appName, operation), uri)
	} else {
		assert.Equal(t, fmt.Sprintf("%s/applications/%s/%s", wireServerFallbackAddress, appName, operation), uri)
	}
}

func TestGetGetVmAppInfo(t *testing.T) {
	metadataToReturn := `
	{
		"name": "advancedsettingsapp",
		"packageBlobLinks": [ "http://localhost/getfile/smallfile" ],
		"version": "3.1415926535897933",
		"operation": "install",
		"install": "doinstall",
		"remove": "doremove",
		"update": "doupdate",
		"configBlobLinks": [ "http://localhost/getfile/smallfile" ],
		"packageFileName": "flarg.exe",
		"configFileName": "flarg.cfg",
		"advancedSettings": {
		  "yurba": "flurba",
		  "snarglesnark": "true"
		}
	}
	`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := []byte(metadataToReturn)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	vmAppMetadata, err := hgc.GetVMAppInfo(nopLog(), "advancedsettingsapp")
	assert.NoError(t, err)
	assert.Equal(t, "flarg.exe", vmAppMetadata.PackageFileName)
	assert.Equal(t, "flarg.cfg", vmAppMetadata.ConfigFileName)
}

func verifyFileContents(t *testing.T, file string, expected string) {
	content, err := os.ReadFile(file)
	require.Nil(t, err, "File does not exist")
	actual := string(content)
	require.Equal(t, expected, actual)
}

func isExpectedImmediateCloseTransportError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "server closed idle connection") ||
		strings.Contains(msg, "forcibly closed by the remote host") ||
		strings.Contains(msg, "aborted by the software in your host machine")
}

func TestIsArcAgentPresent_FileExists(t *testing.T) {
	// Create a temporary file to simulate Arc agent presence
	tempDir := t.TempDir()
	arcFile := path.Join(tempDir, "himds.exe")
	f, err := os.Create(arcFile)
	require.NoError(t, err)
	f.Close()

	el := nopLog()
	result := isArcAgentPresentWithPaths(el, arcFile, arcFile)
	assert.True(t, result, "Arc agent should be detected when file exists")
}

func TestIsArcAgentPresent_FileDoesNotExist(t *testing.T) {
	// Set path to non-existent file
	nonExistentPath := path.Join(t.TempDir(), "non-existent-himds")

	el := nopLog()
	result := isArcAgentPresentWithPaths(el, nonExistentPath, nonExistentPath)
	assert.False(t, result, "Arc agent should not be detected when file does not exist")
}

func TestGetOperationURI_WithEnvironmentVariable(t *testing.T) {
	appName := "testApp"
	operation := "metadata"
	expectedHost := "custom.host.com:1234"

	// Test with environment variable set
	os.Setenv(WireProtocolAddress, expectedHost)
	defer os.Unsetenv(WireProtocolAddress)

	el := nopLog()
	uri, err := getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://%s/applications/%s/%s", expectedHost, appName, operation), uri)
}

func TestGetOperationURI_WithoutEnvironmentVariable(t *testing.T) {
	appName := "testApp"
	operation := "metadata"

	// Ensure no environment variable is set
	os.Unsetenv(WireProtocolAddress)

	el := nopLog()
	uri, err := getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	// Should either use Arc endpoint or fallback to wire server depending on Arc agent presence
	// The exact result depends on the test environment, but it should not error
	assert.Contains(t, uri, fmt.Sprintf("applications/%s/%s", appName, operation))

	if isArcAgentPresent(el) {
		assert.Contains(t, uri, "localhost")
	} else {
		assert.Contains(t, uri, wireServerFallbackAddress)
	}
}

func TestGetOperationURI_PriorityOrder(t *testing.T) {
	appName := "testApp"
	operation := "metadata"

	// Set environment variable (should take priority over Arc detection)
	expectedHost := "env.host.com:5678"
	os.Setenv(WireProtocolAddress, expectedHost)
	defer os.Unsetenv(WireProtocolAddress)

	el := nopLog()
	uri, err := getOperationURI(el, appName, operation)
	assert.NoError(t, err)
	// Environment variable should take priority
	assert.Equal(t, fmt.Sprintf("http://%s/applications/%s/%s", expectedHost, appName, operation), uri)
}

func TestBuildUriUsingWireProtocolAddress_CompleteURL(t *testing.T) {
	baseAddress := "https://complete.example.com:8080"
	appName := "testApp"
	operation := "metadata"

	uri, err := buildUriUsingWireProtocolAddress(baseAddress, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, "https://complete.example.com:8080/applications/testApp/metadata", uri)
}

func TestBuildUriUsingWireProtocolAddress_HostWithPort(t *testing.T) {
	baseAddress := "example.com:9090"
	appName := "testApp"
	operation := "package"

	uri, err := buildUriUsingWireProtocolAddress(baseAddress, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, "http://example.com:9090/applications/testApp/package", uri)
}

func TestBuildUriUsingWireProtocolAddress_HostWithoutPort(t *testing.T) {
	baseAddress := "example.com"
	appName := "testApp"
	operation := "config"

	uri, err := buildUriUsingWireProtocolAddress(baseAddress, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://example.com:%s/applications/testApp/config", hostGaPluginPort), uri)
}

func TestBuildUriUsingWireProtocolAddress_IPWithoutPort(t *testing.T) {
	baseAddress := "192.168.1.100"
	appName := "testApp"
	operation := "metadata"

	uri, err := buildUriUsingWireProtocolAddress(baseAddress, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://192.168.1.100:%s/applications/testApp/metadata", hostGaPluginPort), uri)
}

func TestBuildUriUsingWireProtocolAddress_IPWithPort(t *testing.T) {
	baseAddress := "10.0.0.1:1234"
	appName := "testApp"
	operation := "package"

	uri, err := buildUriUsingWireProtocolAddress(baseAddress, appName, operation)
	assert.NoError(t, err)
	assert.Equal(t, "http://10.0.0.1:1234/applications/testApp/package", uri)
}

func TestBuildUriUsingWireProtocolAddress_InvalidURL(t *testing.T) {
	baseAddress := "h%t!p:invalid-url!"
	appName := "testApp"
	operation := "metadata"

	_, err := buildUriUsingWireProtocolAddress(baseAddress, appName, operation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Could not parse the HostGA URI")
}

func TestIsArcAgentPresentWithPaths_WindowsPath(t *testing.T) {
	// Create a temporary file to simulate Arc agent presence
	tempDir := t.TempDir()
	arcFile := path.Join(tempDir, "himds.exe")
	f, err := os.Create(arcFile)
	require.NoError(t, err)
	f.Close()

	el := nopLog()
	result := isArcAgentPresentWithPaths(el, arcFile, "non-existent-linux-path")

	// Result depends on the OS we're running on
	if runtime.GOOS == "windows" {
		assert.True(t, result, "Arc agent should be detected when Windows file exists and we're on Windows")
	} else {
		assert.False(t, result, "Arc agent should not be detected when we're not on Windows")
	}
}

func TestIsArcAgentPresentWithPaths_LinuxPath(t *testing.T) {
	// Create a temporary file to simulate Arc agent presence
	tempDir := t.TempDir()
	arcFile := path.Join(tempDir, "himds")
	f, err := os.Create(arcFile)
	require.NoError(t, err)
	f.Close()

	el := nopLog()
	result := isArcAgentPresentWithPaths(el, "non-existent-windows-path", arcFile)

	// Result depends on the OS we're running on
	if runtime.GOOS == "linux" {
		assert.True(t, result, "Arc agent should be detected when Linux file exists and we're on Linux")
	} else {
		assert.False(t, result, "Arc agent should not be detected when we're not on Linux")
	}
}

func nopLog() *logging.ExtensionLogger {
	return logging.New(nil)
}
