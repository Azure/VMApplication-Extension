// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package hostgacommunicator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
)

const hostGaPluginPort = "32526"
const WireProtocolAddress = "AZURE_GUEST_AGENT_WIRE_PROTOCOL_ADDRESS"
const wireServerFallbackAddress = "http://168.63.129.16:32526"
const HostGaMetadataErrorPrefix = "HostGaCommunicator GetVMAppInfo error"

type HostGaCommunicatorError int

const (
	InitializationError HostGaCommunicatorError = iota
	MetadataRequestFailedWithRetries
	MetadataRequestFailedInvalidResponseBody
)

func (hostGaCommunicatorError HostGaCommunicatorError) ToString() string {
	switch hostGaCommunicatorError {
	case InitializationError:
		return "InitializationError"
	case MetadataRequestFailedWithRetries:
		return "MetadataRequestFailedWithRetries"
	case MetadataRequestFailedInvalidResponseBody:
		return "MetadataRequestFailedInvalidResponseBody"
	default:
		return "UnknownError"
	}
}

type HostGaCommunicatorGetVMAppInfoError struct {
	errorMessage string
	errorType    HostGaCommunicatorError
}

func (e *HostGaCommunicatorGetVMAppInfoError) Error() string {
	return fmt.Sprintf("%s: %s, error type: %s", HostGaMetadataErrorPrefix, e.errorMessage, e.errorType.ToString())
}

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
	requestManager, isArc, err := getMetadataRequestManager(el, appName)
	if err != nil {
		return nil, &HostGaCommunicatorGetVMAppInfoError{
			errorMessage: fmt.Sprintf("Could not create the request manager: %v", err),
			errorType:    InitializationError,
		}
	}

	var resp *http.Response
	if isArc {
		// Use Arc authentication for Arc endpoints
		arcHandler := requesthelper.NewArcAuthHandler(requestManager)
		resp, err = requesthelper.WithRetriesArc(el, arcHandler, requesthelper.ActualSleep)
	} else {
		// Use standard retry logic for non-Arc endpoints
		resp, err = requesthelper.WithRetries(el, requestManager, requesthelper.ActualSleep)
	}

	if err != nil {
		return nil, &HostGaCommunicatorGetVMAppInfoError{
			errorMessage: fmt.Sprintf("Metadata request failed after retries: %v", err),
			errorType:    MetadataRequestFailedWithRetries,
		}
	}

	body := resp.Body
	defer body.Close()

	var target VMAppMetadataReceiver
	err = json.NewDecoder(body).Decode(&target)
	if err != nil {
		return nil, &HostGaCommunicatorGetVMAppInfoError{
			errorMessage: fmt.Sprintf("Failed to decode response body: %v", err),
			errorType:    MetadataRequestFailedInvalidResponseBody,
		}
	}

	return target.MapToVMAppMetadata(), nil
}

// DownloadPackage downloads the application package through HostGaPlugin to the specified
// file. If the download fails, it automatically retrieves at the last received bytes
// and rebuilds the file from downloaded parts
func (*HostGaCommunicator) DownloadPackage(el *logging.ExtensionLogger, appName string, dst string) error {
	requestFactory, err := newPackageDownloadRequestFactory(el, appName)
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
	requestFactory, err := newConfigDownloadRequestFactory(el, appName)
	if err != nil {
		return errors.Wrapf(err, "Could not create the request factory")
	}

	err = requestFactory.downloadFile(el, dst)
	return err
}

func getOperationURI(el *logging.ExtensionLogger, appName string, operation string) (string, error) {
	baseAddress := os.Getenv(WireProtocolAddress)
	if baseAddress != "" {
		return buildUriUsingWireProtocolAddress(baseAddress, appName, operation)
	}

	var baseEndpoint string
	isArcPresent := isArcAgentPresent(el)
	if isArcPresent {
		arcEndpoint := getArcEndpoint(el)
		el.Info("Arc agent detected, using Arc endpoint: %s", arcEndpoint)
		baseEndpoint = arcEndpoint
	} else {
		el.Warn("environment variable %s not set, using WireProtocol fallback address", WireProtocolAddress)
		baseEndpoint = wireServerFallbackAddress
	}

	uri, _ := url.Parse(baseEndpoint)
	// For both Arc and Azure, use the same path structure
	uri.Path = fmt.Sprintf("applications/%s/%s", appName, operation)

	return uri.String(), nil
}

func buildUriUsingWireProtocolAddress(baseAddress string, appName string, operation string) (string, error) {
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
