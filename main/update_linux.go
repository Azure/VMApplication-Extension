package main

import (
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

func vmAppUpdateCallback(ext *vmextensionhelper.VMExtension) error {
	//no-op function
	return nil 
}
