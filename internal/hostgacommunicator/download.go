package hostgacommunicator

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-extension-platform/pkg/constants"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
)

const (
	configOperation  = "config"
	packageOperation = "package"

	rangeHeaderName        = "x-ms-range"
	contentRangeHeaderName = "Content-Range"
	rangeHeaderFormat      = "bytes=%d-"

	maxDownloadAttempts = 10
)

var (
	downloadRequestTimeout = 1 * time.Hour
	removeFileFunc         = removeFile
)

type downloadRequestFactory struct {
	url             string
	downloadedBytes int64
}

func newPackageDownloadRequestFactory(el *logging.ExtensionLogger, appName string, appVersion string) (*downloadRequestFactory, error) {
	downloadURL, err := getOperationURI(el, appName, appVersion, packageOperation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain operationURI")
	}

	drf := downloadRequestFactory{
		url:             downloadURL,
		downloadedBytes: 0,
	}

	return &drf, nil
}

func newConfigDownloadRequestFactory(el *logging.ExtensionLogger, appName string, appVersion string) (*downloadRequestFactory, error) {
	downloadURL, err := getOperationURI(el, appName, appVersion, configOperation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain operationURI")
	}

	drf := downloadRequestFactory{
		url:             downloadURL,
		downloadedBytes: 0,
	}

	return &drf, nil
}

func (u downloadRequestFactory) downloadFile(el *logging.ExtensionLogger, filename string) error {
	// Delete the file if it already exists
	err := removeFileFunc(filename)
	if err != nil {
		return errors.Wrapf(err, "Could not remove the existing file")
	}

	finished := false
	attempts := 0

	for finished == false && err == nil && attempts < maxDownloadAttempts {
		finished, err = u.downloadAttempt(el, filename)
		if err != nil {
			return errors.Wrapf(err, "Unrecoverable error while downloading the file")
		}

		attempts++
	}

	if finished == false {
		return errors.New("Failed to completely download the file")
	}

	return nil
}

// downloadAttempt tries once to download as much of the file as it can
// It returns true if the download is now complete, or false if it isn't yet
// complete. It returns an error if a non-recoverable error occurred
func (u downloadRequestFactory) downloadAttempt(el *logging.ExtensionLogger, filename string) (bool, error) {
	requestManager := requesthelper.GetRequestManager(u, downloadRequestTimeout)

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		return true, err
	}
	defer f.Close()

	// Find out how much of the file we've downloaded, if any
	fi, err := f.Stat()
	if err != nil {
		return true, errors.Wrapf(err, "Could not retrieve stats for file")
	}

	// Start writing at the end of the file
	_, err = f.Seek(0, io.SeekEnd)
	if err != nil {
		return true, errors.Wrapf(err, "Could not seek to end of file")
	}

	u.downloadedBytes = fi.Size()
	resp, err := requesthelper.WithRetries(el, requestManager, requesthelper.ActualSleep)
	if err != nil {
		return true, errors.Wrapf(err, "Download request failed with retries.")
	}

	body := resp.Body
	defer body.Close()

	// Copy will call the reader and writer multiple times to write by chunk instead of placing
	// the entire file in memory
	_, err = io.Copy(f, body)
	if err != nil {
		return true, errors.Wrapf(err, "Could not copy response data to file")
	}

	if resp.StatusCode == http.StatusOK {
		// The download completed
		return true, nil
	}

	if resp.StatusCode == http.StatusPartialContent {
		// HostGA was only able to download part of the file.
		return false, nil
	}

	return true, errors.New("Unexpected status code")
}

func removeFile(filename string) error {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return errors.Wrapf(err, "Cannot retrieve file information")
	}

	err = os.Remove(filename)
	return err
}

// GetRequest returns a new request to download the URL
func (u downloadRequestFactory) GetRequest() (*http.Request, error) {
	r, err := http.NewRequest("GET", u.url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create request")
	}

	// Set the x-ms-range header to what we've downloaded so far
	rangeHeaderValue := fmt.Sprintf(rangeHeaderFormat, u.downloadedBytes)
	r.Header.Set(rangeHeaderName, rangeHeaderValue)

	return r, nil
}
