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
	} else if statErr != nil {
		msg := fmt.Sprintf("failed to stat DataFolder backup path '%s': %v", backupDir, statErr)
		ext.ExtensionLogger.Error(msg)
		ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
		return nil
	}

	dataFolderInfo, statErr := os.Stat(dataFolder)
	if statErr != nil && !os.IsNotExist(statErr) {
		msg := fmt.Sprintf("failed to stat DataFolder path '%s': %v", dataFolder, statErr)
		ext.ExtensionLogger.Error(msg)
		ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
		return nil
	}

	if statErr == nil && !dataFolderInfo.IsDir() {
		if err := os.RemoveAll(dataFolder); err != nil {
			msg := fmt.Sprintf("failed to remove non-directory DataFolder path '%s': %v", dataFolder, err)
			ext.ExtensionLogger.Error(msg)
			ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
			return nil
		}
	}

	if err := moveDirsAll(backupDir, dataFolder, mvDirsPreferenceDestDir); err != nil {
		msg := fmt.Sprintf("failed to restore backup dir '%s' into DataFolder '%s' with destdir preference: %v", backupDir, dataFolder, err)
		ext.ExtensionLogger.Error(msg)
		ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
		return nil
	}

	msg := fmt.Sprintf("restored dataFolder backup from '%s' to '%s'", backupDir, dataFolder)
	ext.ExtensionLogger.Info(msg)
	ext.ExtensionEvents.LogInformationalEvent("ExtensionInstall", msg)
	return nil
}
