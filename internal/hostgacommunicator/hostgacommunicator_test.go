package hostgacommunicator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

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
	require.Contains(t, err.Error(), "Could not parse the HostGA URI", "Wrong message for invalid uri")
}

func TestGetVmAppInfo_RequestFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		return
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
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

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	_, err := hgc.GetVMAppInfo(nopLog(), myAppName)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "failed to decode response body", "Wrong message for invalid response")
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
		return
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
		return
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

func TestDownloadPackage_CannotRemoveExistingFile(t *testing.T) {
	createTestDir(t)
	defer cleanupTestDir()
	filePath := path.Join(testDirPath, "LockedFile")
	lf, err := os.Create(filePath)
	defer lf.Close()
	removeFileFunc = func(filename string) error {
		return errors.New("Could not remove the existing file")
	}
	defer func() { removeFileFunc = removeFile }()
	require.Nil(t, err, "File creation failed")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		return
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err = hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.NotNil(t, err, "did not fail")
	require.Contains(t, err.Error(), "Could not remove the existing file", "Wrong message for failing to remove locked file")
}

func TestDownloadPackage_InvalidPath(t *testing.T) {
	filePath := string(make([]byte, 5)) // null characters in file names are invalid in both windows and linux

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		return
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
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
		return
	}))
	defer srv.Close()

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
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

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
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

	os.Setenv(WireProtocolAddress, srv.URL)
	hgc := &HostGaCommunicator{}
	err := hgc.DownloadPackage(nopLog(), myAppName, filePath)
	require.Nil(t, err, "Download failed")
	require.Equal(t, expectedCallCount, callCount)
	verifyFileContents(t, filePath, expected)
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
	assert.Equal(t, fmt.Sprintf("%s/applications/%s/%s", wireServerFallbackAddress, appName, operation), uri)
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
		return
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
	content, err := ioutil.ReadFile(file)
	require.Nil(t, err, "File does not exist")
	actual := string(content)
	require.Equal(t, expected, actual)
}

func nopLog() *logging.ExtensionLogger {
	return logging.New(nil)
}
