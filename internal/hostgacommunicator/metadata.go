package hostgacommunicator

import (
	"net/http"
	"time"

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
	Operation          string `json:"operation"`
	InstallCommand     string `json:"install"`
	UpdateCommand      string `json:"update"`
	RemoveCommand      string `json:"remove"`
	DirectDownloadOnly bool   `json:"directOnly"`
}

type metadataRequestFactory struct {
	url string
}

func newMetadataRequestFactory(appName string) (*metadataRequestFactory, error) {
	url, err := getOperationURI(appName, metadataOperation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain operationURI")
	}

	return &metadataRequestFactory{url}, nil
}

// GetRequest returns a new request to download the URL
func (u metadataRequestFactory) GetRequest() (*http.Request, error) {
	return http.NewRequest("GET", u.url, nil)
}

func getMetadataRequestManager(appName string) (*requesthelper.RequestManager, error) {
	factory, err := newMetadataRequestFactory(appName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create request factory")
	}

	return requesthelper.GetRequestManager(factory, metadataRequestTimeout), nil
}
