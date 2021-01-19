package hostgacommunicator_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/require"
)

const (
	myAppName       = "chipmunkdetector"
	nonExistentFile = "blarf"
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

func TestGetVmAppInfo_NoEnvironmentVariable(t *testing.T) {
	os.Setenv(hostgacommunicator.WireProtocolAddress, "")
	hgc := &hostgacommunicator.HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "WireProtocolAddress not present in environment", "Wrong message for non-existent environment variable")
}

func TestGetVmAppInfo_InvalidUri(t *testing.T) {
	os.Setenv(hostgacommunicator.WireProtocolAddress, "h%t!p:notgoingtohappen!")
	hgc := &hostgacommunicator.HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "Could not parse the HostGA URI", "Wrong message for invalid uri")
}

func TestGetVmAppInfo_RequestFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "Metadata request failed with retries.", "Wrong message for failed request")
}

func TestGetVmAppInfo_CouldNotDecodeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		b := []byte(`"Type":"Chipmunk","Age":6,"FavoriteFoods":["Acorns","Cookies"]}`)
		w.Write(b)
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "failed to decode response body", "Wrong message for invalid response")
}

func TestGetVmAppInfo_MissingProperties(t *testing.T) {
	expected := hostgacommunicator.VMAppMetadata{
		ApplicationName: "chipmunk",
		Version:         "42",
		Operation:       "install",
		InstallCommand:  "installchipmunk.bat",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		output, err := json.Marshal(expected)
		require.Nil(t, err, "Marshal failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(output)
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	actual, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.Nil(t, err, "request failed")
	require.Equal(t, expected.ApplicationName, actual.ApplicationName)
	require.Equal(t, expected.Version, actual.Version)
	require.Equal(t, expected.Operation, actual.Operation)
	require.Equal(t, expected.InstallCommand, actual.InstallCommand)
}

func TestGetVmAppInfo_ValidResponse(t *testing.T) {
	expected := hostgacommunicator.VMAppMetadata{
		ApplicationName:    "chipmunk",
		Version:            "42",
		Operation:          "install",
		InstallCommand:     "installchipmunk.bat",
		UpdateCommand:      "updatechipmunk.bat",
		RemoveCommand:      "removechipmunk.bat",
		DirectDownloadOnly: false,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		output, err := json.Marshal(expected)
		require.Nil(t, err, "Marshal failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(output)
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	actual, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.Nil(t, err, "request failed")
	require.Equal(t, expected.ApplicationName, actual.ApplicationName)
	require.Equal(t, expected.Version, actual.Version)
	require.Equal(t, expected.Operation, actual.Operation)
	require.Equal(t, expected.InstallCommand, actual.InstallCommand)
	require.Equal(t, expected.UpdateCommand, actual.UpdateCommand)
	require.Equal(t, expected.RemoveCommand, actual.RemoveCommand)
	require.Equal(t, expected.DirectDownloadOnly, actual.DirectDownloadOnly)
}

func TestDownloadPackage_NoEnvironmentVariable(t *testing.T) {
	os.Setenv(hostgacommunicator.WireProtocolAddress, "")
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, nonExistentFile)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "WireProtocolAddress not present in environment", "Wrong message for non-existent environment variable")
}

func TestDownloadPackage_CannotRemoveExistingFile(t *testing.T) {
	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "LockedFile")
	f, err := os.Create(filePath)
	defer f.Close()
	require.Nil(t, err, "File creation failed")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err = hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "Could not remove the existing file", "Wrong message for failing to remove locked file")
}

func TestDownloadPackage_InvalidPath(t *testing.T) {
	filePath := "aaargh!$(*&#$%)($"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
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
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
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
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
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

		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
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
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.Nil(t, err, "Download failed")
	require.Equal(t, expectedCallCount, callCount)
	verifyFileContents(t, filePath, expected)
}

func TestDownloadConfig_NoEnvironmentVariable(t *testing.T) {
	os.Setenv(hostgacommunicator.WireProtocolAddress, "")
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err := hgc.DownloadConfig(nopLog(), myAppName, nonExistentFile)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "WireProtocolAddress not present in environment", "Wrong message for non-existent environment variable")
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
		return
	}))
	defer srv.Close()

	os.Setenv(hostgacommunicator.WireProtocolAddress, srv.URL)
	hgc := &hostgacommunicator.HostGaCommunicator{}
	err := hgc.DownloadConfig(nopLog(), myAppName, filePath)
	require.Nil(t, err, "Download failed")
	verifyFileContents(t, filePath, expected)
}

func verifyFileContents(t *testing.T, file string, expected string) {
	content, err := ioutil.ReadFile(file)
	require.Nil(t, err, "File does not exist")
	actual := string(content)
	require.Equal(t, expected, actual)
}

func nopLog() *logging.ExtensionLogger {
	return logging.New(nil)
}
