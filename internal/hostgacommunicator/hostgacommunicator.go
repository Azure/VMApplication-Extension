package hostgacommunicator

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
	"net/url"
	"os"
)

const hostGaPluginPort = "32526"

type IHostGaCommunicator interface {
	DownloadPackage(el *logging.ExtensionLogger, appName string, dst string) error
	DownloadConfig(el *logging.ExtensionLogger, appName string, dst string) error
	GetVMAppInfo(el *logging.ExtensionLogger, appName string) (*VMAppMetadata, error)
}

// HostGaCommunicator provides methods for retrieving application metadata and packages
// from the HostGaPlugin
type HostGaCommunicator struct{}

// GetVMAppInfo returns the metadata for the application
func (*HostGaCommunicator) GetVMAppInfo(el *logging.ExtensionLogger, appName string) (*VMAppMetadata, error) {
	requestManager, err := getMetadataRequestManager(appName)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not create the request manager")
	}

	resp, err := requesthelper.WithRetries(el, requestManager, requesthelper.ActualSleep)
	if err != nil {
		return nil, errors.Wrapf(err, "Metadata request failed with retries.")
	}

	body := resp.Body
	defer body.Close()

	var target VMAppMetadataReceiver
	err = json.NewDecoder(body).Decode(&target)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode response body")
	}

	return target.MapToVMAppMetadata(), nil
}

// DownloadPackage downloads the application package through HostGaPlugin to the specified
// file. If the download fails, it automatically retrieves at the last received bytes
// and rebuilds the file from downloaded parts
func (*HostGaCommunicator) DownloadPackage(el *logging.ExtensionLogger, appName string, dst string) error {
	requestFactory, err := newPackageDownloadRequestFactory(appName)
	if err != nil {
		return errors.Wrapf(err, "Could not create the request factory")
	}

	err = requestFactory.downloadFile(el, dst)
	return err
}

// DownloadConfig downloads the application config through HostGaPlugin to the specified
// file. If the download fails, it automatically retrieves at the last received bytes
// and rebuilds the file from downloaded parts
func (*HostGaCommunicator) DownloadConfig(el *logging.ExtensionLogger, appName string, dst string) error {
	requestFactory, err := newConfigDownloadRequestFactory(appName)
	if err != nil {
		return errors.Wrapf(err, "Could not create the request factory")
	}

	err = requestFactory.downloadFile(el, dst)
	return err
}

func getOperationURI(appName string, operation string) (string, error) {
	baseAddress := os.Getenv(WireProtocolAddress)
	if baseAddress == "" {
		return "", errors.New("WireProtocolAddress not present in environment")
	}

	uri, err := url.Parse(baseAddress)
	if err != nil {
		// ip with port 10.0.0.1:1234 will fail otherwise
		uri, err = url.Parse("//" + baseAddress)
		if err != nil {
			return "", errors.Wrap(err, "Could not parse the HostGA URI")
		}
	}
	if uri.Host == "" {
		// takes care of host names without port like foo.bar.com, 10.0.0.1, these need to be prepended with //
		uri, err = url.Parse("//" + baseAddress)
		if err != nil {
			return "", errors.Wrap(err, "Could not parse the HostGA URI")
		}
	}
	// if port is not specified, set default port
	if uri.Port() == "" {
		uri, err = url.Parse("//" + uri.Host + ":" + hostGaPluginPort)
		if err != nil {
			return "", errors.Wrap(err, "failed to add default host ga plugin port")
		}
	}

	uri.Path = fmt.Sprintf("applications/%s/%s", appName, operation)
	if uri.Scheme == "" {
		uri.Scheme = "http"
	}

	return uri.String(), nil
}
