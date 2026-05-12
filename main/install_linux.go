// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"os"

	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
)

// vmAppInstallCallback is called by the extension platform during the install operation.
// If a DataFolder backup was created by the update callback (DataFolder + ".backup"), it is
// restored here: the backup is renamed back to DataFolder so that downloaded application
// packages are preserved across extension version updates.
// If DataFolder already exists (e.g. created by the platform before calling this callback),
// the contents of the backup are merged into it instead.
func vmAppInstallCallback(ext *vmextensionhelper.VMExtension) error {
	dataFolder := ext.HandlerEnv.DataFolder
	backupDir := getDataFolderBackupPath(ext)

	if _, statErr := os.Stat(backupDir); os.IsNotExist(statErr) {
		msg := fmt.Sprintf("no DataFolder backup found at '%s', nothing to restore", backupDir)
		ext.ExtensionLogger.Info(msg)
		ext.ExtensionEvents.LogInformationalEvent("ExtensionInstall", msg)
		return nil
	}

	if _, statErr := os.Stat(dataFolder); os.IsNotExist(statErr) {
		// DataFolder doesn't exist — simple rename
		if err := os.Rename(backupDir, dataFolder); err != nil {
			msg := fmt.Sprintf("failed to restore DataFolder backup from '%s' to '%s': %v", backupDir, dataFolder, err)
			ext.ExtensionLogger.Error(msg)
			ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
			return nil
		}
	} else {
		// DataFolder already exists — merge backup into DataFolder while keeping existing destination entries on clash.
		if err := moveDirsAll(backupDir, dataFolder, mvDirsPreferenceDestDir); err != nil {
			msg := fmt.Sprintf("failed to merge backup dir '%s' into existing DataFolder '%s': %v", backupDir, dataFolder, err)
			ext.ExtensionLogger.Error(msg)
			ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
			return nil
		}
	}

	msg := fmt.Sprintf("restored dataFolder backup from '%s' to '%s'", backupDir, dataFolder)
	ext.ExtensionLogger.Info(msg)
	ext.ExtensionEvents.LogInformationalEvent("ExtensionInstall", msg)
	return nil
}
