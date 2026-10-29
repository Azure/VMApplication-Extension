// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package hostgacommunicator

import (
	stderrors "errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Azure/azure-extension-platform/pkg/logging"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/pkg/errors"
)

const (
	metadataOperation         = "metadata"
	versionQueryParameterName = "version"
)

var (
	metadataRequestTimeout = 30 * time.Second
)

// VMAppMetadata contains the format of the metadata returned by HostGAPlugin
type VMAppMetadata struct {
	ApplicationName    string `json:"name"`
	Version            string `json:"version"`
	InstallCommand     string `json:"install"`
	UpdateCommand      string `json:"update"`
	RemoveCommand      string `json:"remove"`
	DirectDownloadOnly bool   `json:"directOnly"`
	ConfigExists       bool
	PackageFileName    string `json:"packageFileName"`
	ConfigFileName     string `json:"configFileName"`
	RebootBehavior     string `json:"scriptBehaviorAfterReboot"`
}

type VMAppMetadataReceiver struct {
	ApplicationName    string `json:"name"`
	Version            string `json:"version"`
	InstallCommand     string `json:"install"`
	UpdateCommand      string `json:"update"`
	RemoveCommand      string `json:"remove"`
	DirectDownloadOnly string `json:"directOnly"`
	Package            string `json:"package"`
	Config             string `json:"config"`
	PackageFileName    string `json:"packageFileName"`
	ConfigFileName     string `json:"configFileName"`
	RebootBehavior     string `json:"scriptBehaviorAfterReboot"`
}

func (receiver *VMAppMetadataReceiver) MapToVMAppMetadata() *VMAppMetadata {
	directDownloadOnly, err := strconv.ParseBool(receiver.DirectDownloadOnly)
	if err != nil {
		// assume directDownloadOnly is false when parsing fails
		directDownloadOnly = false
	}

	configExists := receiver.Config != ""
	vmAppMetadata := VMAppMetadata{
		ApplicationName:    receiver.ApplicationName,
		Version:            receiver.Version,
		InstallCommand:     receiver.InstallCommand,
		UpdateCommand:      receiver.UpdateCommand,
		RemoveCommand:      receiver.RemoveCommand,
		DirectDownloadOnly: directDownloadOnly,
		ConfigExists:       configExists,
		PackageFileName:    receiver.PackageFileName,
		ConfigFileName:     receiver.ConfigFileName,
		RebootBehavior:     receiver.RebootBehavior,
	}
	return &vmAppMetadata
}

type metadataRequestFactory struct {
	url string
}

var _ requesthelper.RequestFactory = (*metadataRequestFactory)(nil)

func newMetadataRequestFactory(el *logging.ExtensionLogger, appName string) (*metadataRequestFactory, error) {
	url, err := getOperationURI(el, appName, metadataOperation, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain operationURI")
	}

	return &metadataRequestFactory{url}, nil
}

// GetRequest returns a new request to download the URL
func (u metadataRequestFactory) GetRequest() (*http.Request, error) {
	return http.NewRequest("GET", u.url, nil)
}

// with metadataRequestFactory, there is no fallback request implemented, so this always returns an error.
func (u metadataRequestFactory) GetFallbackRequest() (*http.Request, error) {
	return nil, errors.Wrap(stderrors.ErrUnsupported, "metadataRequestFactory does not implement a fallback request")
}

func getMetadataRequestManager(el *logging.ExtensionLogger, appName string, version string) (*requesthelper.RequestManager, bool, error) {
	isArc := isArcAgentPresent(el)
	var factory requesthelper.RequestFactory
	var err error
	if isArc {
		factory, err = newMetadataRequestFactory(el, appName)
	} else {
		factory, err = newVersionedMetadataRequestFactory(el, appName, version)
	}
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to create request factory")
	}

	return requesthelper.GetRequestManager(factory, metadataRequestTimeout), isArc, nil
}

type versionedMetadataRequestFactory struct {
	url                  string
	vmAppMetadataVersion string
}

var _ requesthelper.RequestFactory = (*versionedMetadataRequestFactory)(nil)

func newVersionedMetadataRequestFactory(el *logging.ExtensionLogger, appName string, version string) (*versionedMetadataRequestFactory, error) {
	url, err := getOperationURI(el, appName, metadataOperation, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain operationURI")
	}

	return &versionedMetadataRequestFactory{url: url, vmAppMetadataVersion: version}, nil
}

func (u versionedMetadataRequestFactory) GetRequest() (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, u.url, nil)
	if err != nil {
		return nil, err
	}

	appendQueryParams(req.URL, map[string]string{versionQueryParameterName: u.vmAppMetadataVersion})
	return req, nil
}

// GetFallbackRequest returns a new request using the base URL without adding the version query parameter.
func (u versionedMetadataRequestFactory) GetFallbackRequest() (*http.Request, error) {
	return http.NewRequest(http.MethodGet, u.url, nil)
}
