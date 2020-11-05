package hostgacommunicator

import (
	"fmt"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
)

type IHostGaCommunicator interface {
	GetVMAppInfo(appName string) (*packageregistry.VMAppPackageIncoming, error)
}

type HostGaCommunicator struct{}

func (*HostGaCommunicator) GetVMAppInfo(appName string) (*packageregistry.VMAppPackageIncoming, error) {
	// TODO: implement this
	return nil, fmt.Errorf("not implemented")
}
