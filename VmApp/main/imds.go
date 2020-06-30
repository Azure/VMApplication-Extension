package main

import (
	"fmt"

	"github.com/go-kit/kit/log"
)

const (
	imdsEndpoint           = "169.254.169.254"
	imdsAPIVersion         = "2018-02-01"
	applicationURI         = "http://%s/metadata/applications/%s/%s?cid=%s&api-version=%s"
	configurationOperation = "configuration"
	metadataOperation      = "metadata"
	packageOperation       = "package"
)

type vmApplication struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Operation  string `json:"operation"`
	Install    string `json:"install"`
	Update     string `json:"update"`
	Remove     string `json:"remove"`
	DirectOnly bool   `json:"directOnly"`
}

type imdsDownloader struct {
}

func (*imdsDownloader) getMetadata(ctx log.Logger, applicationName string, containerID string) (*vmApplication, error) {
	uri := getImdsURI(applicationName, containerID, metadataOperation)
}

func (*imdsDownloader) getPackage(ctx log.Logger, applicationName string, containerID string, downloadDir string) (string, error) {
	uri := getImdsURI(applicationName, containerID, packageOperation)
}

func (*imdsDownloader) getConfiguration(ctx log.Logger, applicationName string, containerID string, downloadDir string) (string, error) {
	uri := getImdsURI(applicationName, containerID, configurationOperation)
}

func getImdsURI(applicationName string, containerID string, operation string) string {
	return fmt.Sprintf(applicationURI, imdsEndpoint, applicationName, operation, containerID, imdsAPIVersion)
}
