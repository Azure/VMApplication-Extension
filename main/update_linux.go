// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
)

// package registry file is in the config dir, which has the pattern
// /var/lib/waagent/Microsoft.CPlat.Core.VMApplicationManagerLinux-<extensionVersion>/config
// need to move it from an older version to the current one, if it exists
func vmAppUpdateCallback(ext *vmextensionhelper.VMExtension) error {

	packageRegistryFilePathForCurrentVersion := filepath.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	_, err := os.Stat(packageRegistryFilePathForCurrentVersion)
	if !os.IsNotExist(err) {
		msg := fmt.Sprintf("package registry file '%s' already exists for current version, no need to copy from older version, update operation completed.", packageRegistryFilePathForCurrentVersion)
		ext.ExtensionLogger.Info(msg)
		ext.ExtensionEvents.LogInformationalEvent("ExtensionUpdate", msg)
		return nil
	}

	if _, statErr := os.Stat(ext.HandlerEnv.ConfigFolder); os.IsNotExist(statErr) {

		err = os.MkdirAll(ext.HandlerEnv.ConfigFolder, 0755)
		if err != nil {
			return fmt.Errorf("failed to create config folder '%s': %w", ext.HandlerEnv.ConfigFolder, err)
		}
		msg := fmt.Sprintf("created config folder '%s'", ext.HandlerEnv.ConfigFolder)
		ext.ExtensionLogger.Info(msg)
		ext.ExtensionEvents.LogInformationalEvent("ExtensionUpdate", msg)
	}

	head, versionedDirName, tail, err := splitPathAroundVersionedDirLinux(ext.HandlerEnv.ConfigFolder)
	if err != nil {
		return err
	}
	dirnameChecker := getDirNameCheckerWithKnownExtensionVersion(ExtensionVersion)
	if !dirnameChecker(versionedDirName) {
		msg := fmt.Sprintf("ExtensionVersion '%s' is not part of the ext.HandlerEnv.ConfigFolder path '%s'", ExtensionVersion, ext.HandlerEnv.ConfigFolder)
		ext.ExtensionLogger.Warn(msg)
		ext.ExtensionEvents.LogWarningEvent("ExtensionUpdate", msg)
	}

	previousPackageRegistryFilePath, err := getMostRecentlyUpdatedPackageRegistryFile(head, tail, getDirNameCheckerWithExtensionVersionPattern())
	if err != nil {
		return err
	}

	previousPackageRegistryContent, err := os.ReadFile(previousPackageRegistryFilePath)
	if err != nil {
		return err
	}

	// Creates and writes previous registry content to package registry file for new extension version
	err = os.WriteFile(packageRegistryFilePathForCurrentVersion, previousPackageRegistryContent, 0666)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("successfully copied package registry file from '%s' to '%s'", previousPackageRegistryFilePath, packageRegistryFilePathForCurrentVersion)
	ext.ExtensionLogger.Info(msg)
	ext.ExtensionEvents.LogInformationalEvent("ExtensionUpdate", msg)

	// Overwrite the package registry for older version to be an empty list of applications
	err = os.WriteFile(previousPackageRegistryFilePath, emptyPackageRegistryContent, 0666)
	if err == nil {
		msg = fmt.Sprintf("successfully cleared package registry file for older version at '%s'", previousPackageRegistryFilePath)
		ext.ExtensionLogger.Info(msg)
		ext.ExtensionEvents.LogInformationalEvent("ExtensionUpdate", msg)
	}

	return nil
}

// splitPathAroundVersionedDirLinux splits dirpath into (head, versionedDirName, tail) by walking up to find an ancestor
// directory whose name matches ExtensionName-<version> (e.g. "Microsoft.CPlat.Core.VMApplicationManagerLinux-1.0.10").
func splitPathAroundVersionedDirLinux(dirpath string) (head, versionedDirName, tail string, errorToReturn error) {
	// contains an array of comparison functions that will be run to determine the version dir
	// to have robustness, if the first way of comparison fails, use the next one
	var dirnameCheckers []func(currentFolderName string) bool

	currentExtensionVersion, err := vmextensionhelper.GetGuestAgentEnvironmetVariable(vmextensionhelper.GuestAgentEnvVarExtensionVersion)
	if err == nil {
		// checks against 'current extension version' populated by Guest Agent
		dirnameCheckers = append(dirnameCheckers, getDirNameCheckerWithKnownExtensionVersion(currentExtensionVersion))
	}

	updateExtensionVersion, err := vmextensionhelper.GetGuestAgentEnvironmetVariable(vmextensionhelper.GuestAgentEnvVarUpdateToVersion)
	if err == nil {
		// checks against 'extension version to update' populated by Guest Agent
		dirnameCheckers = append(dirnameCheckers, getDirNameCheckerWithKnownExtensionVersion(updateExtensionVersion))
	}

	// check against extension version variable
	dirnameCheckers = append(dirnameCheckers, getDirNameCheckerWithKnownExtensionVersion(ExtensionVersion))

	// check against extension version pattern
	dirnameCheckers = append(dirnameCheckers, getDirNameCheckerWithExtensionVersionPattern())

	return splitPathAroundVersionedDir(dirpath, dirnameCheckers)
}

func getDirNameCheckerWithKnownExtensionVersion(extensionVersion string) func(currentDirName string) bool {
	expectedDirName := ExtensionName + "-" + extensionVersion
	return func(currentDirName string) bool {
		return strings.EqualFold(expectedDirName, currentDirName)
	}
}

func getDirNameCheckerWithExtensionVersionPattern() func(currentDirName string) bool {
	return func(currentDirName string) bool {
		marker := ExtensionName + "-"
		idx := strings.Index(strings.ToLower(currentDirName), strings.ToLower(marker))
		if idx >= 0 {
			versionPart := currentDirName[idx+len(marker):]
			return utils.IsValidVersionString(versionPart)
		}
		return false
	}
}
