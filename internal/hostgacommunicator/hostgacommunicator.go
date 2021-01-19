package hostgacommunicator

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
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

	var target VMAppMetadata
	err = json.NewDecoder(body).Decode(&target)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode response body")
	}

	return &target, nil
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

func getHostGaAddress() (string, error) {
	baseAddress := os.Getenv(WireProtocolAddress)
	if baseAddress == "" {
		return "", errors.New("WireProtocolAddress not present in environment")
	}

	// The tests will already have a port, so don't add another one
	hostGaURL := baseAddress
	if strings.Contains(hostGaURL, ":") == false {
		hostGaURL = net.JoinHostPort(baseAddress, hostGaPluginPort)
	}

	return hostGaURL, nil
}

func getOperationURI(appName string, operation string) (string, error) {
	baseURI, err := getHostGaAddress()
	if err != nil {
		return "", errors.Wrapf(err, "failed to obtain the base HostGA address")
	}

	rawURI := fmt.Sprintf("%s/applications/%s/%s", baseURI, appName, operation)

	u, err := url.Parse(rawURI)
	if err != nil {
		return "", errors.New("Could not parse the HostGA URI")
	}

	return u.String(), nil
}
