// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
)

// vmAppInstallCallback is called by the extension platform during the install operation.
// No special install handling is required on Windows.
func vmAppInstallCallback(ext *vmextensionhelper.VMExtension) error {
	return nil
}
