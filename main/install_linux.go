// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"os"
	"path/filepath"

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
		// DataFolder already exists — move each entry from backupDir into DataFolder, then remove backupDir
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			msg := fmt.Sprintf("failed to read backup dir '%s': %v", backupDir, err)
			ext.ExtensionLogger.Error(msg)
			ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
			return nil
		}
		for _, entry := range entries {
			src := filepath.Join(backupDir, entry.Name())
			dst := filepath.Join(dataFolder, entry.Name())

			if _, dstStatErr := os.Stat(dst); dstStatErr == nil {
				msg := fmt.Sprintf("destination already exists at '%s'; keeping existing entry and discarding backup entry '%s'", dst, src)
				ext.ExtensionLogger.Info(msg)
				continue
			} else if !os.IsNotExist(dstStatErr) {
				msg := fmt.Sprintf("failed to stat destination path '%s': %v", dst, dstStatErr)
				ext.ExtensionLogger.Error(msg)
				ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
				continue
			}

			if err := os.Rename(src, dst); err != nil {
				msg := fmt.Sprintf("failed to move '%s' to '%s': %v", src, dst, err)
				ext.ExtensionLogger.Error(msg)
				ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
				continue
			}
		}
		if err := os.RemoveAll(backupDir); err != nil {
			msg := fmt.Sprintf("failed to remove backup dir '%s': %v", backupDir, err)
			ext.ExtensionLogger.Error(msg)
			ext.ExtensionEvents.LogWarningEvent("ExtensionInstall", msg)
		}
	}

	msg := fmt.Sprintf("restored dataFolder backup from '%s' to '%s'", backupDir, dataFolder)
	ext.ExtensionLogger.Info(msg)
	ext.ExtensionEvents.LogInformationalEvent("ExtensionInstall", msg)
	return nil
}
