package hostgacommunicator

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Azure/azure-extension-platform/pkg/logging"

	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/pkg/errors"
)

const (
	metadataOperation = "metadata"
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

func newMetadataRequestFactory(el *logging.ExtensionLogger, appName string) (*metadataRequestFactory, error) {
	url, err := getOperationURI(el, appName, metadataOperation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain operationURI")
	}

	return &metadataRequestFactory{url}, nil
}

// GetRequest returns a new request to download the URL
func (u metadataRequestFactory) GetRequest() (*http.Request, error) {
	return http.NewRequest("GET", u.url, nil)
}

func getMetadataRequestManager(el *logging.ExtensionLogger, appName string) (*requesthelper.RequestManager, error) {
	factory, err := newMetadataRequestFactory(el, appName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create request factory")
	}

	return requesthelper.GetRequestManager(factory, metadataRequestTimeout), nil
}
