// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
)

// splitPathAroundVersionedDirWindows splits dirpath into (head, versionedDirName, tail) by walking up to find an ancestor
// directory whose name is a bare version string (e.g. "1.0.10").
func splitPathAroundVersionedDirWindows(dirpath string) (head, versionedDirName, tail string, errorToReturn error) {
	// contains an array of comparison functions that will be run to determine the version dir
	// to have robustness, if the first way of comparison fails, use the next one
	var dirnameCheckers []func(currentFolderName string) bool

	currentExtensionVersion, err := vmextensionhelper.GetGuestAgentEnvironmetVariable(vmextensionhelper.GuestAgentEnvVarExtensionVersion)
	if err == nil {
		// checks against 'current extension version' populated by Guest Agent
		dirnameCheckers = append(dirnameCheckers, getCaseInsensitiveStringEqualityChecker(currentExtensionVersion))
	}

	updateExtensionVersion, err := vmextensionhelper.GetGuestAgentEnvironmetVariable(vmextensionhelper.GuestAgentEnvVarUpdateToVersion)
	if err == nil {
		// checks against 'extension version to update' populated by Guest Agent
		dirnameCheckers = append(dirnameCheckers, getCaseInsensitiveStringEqualityChecker(updateExtensionVersion))
	}

	// check against extension version variable
	dirnameCheckers = append(dirnameCheckers, getCaseInsensitiveStringEqualityChecker(ExtensionVersion))

	// check against extension version pattern
	dirnameCheckers = append(dirnameCheckers, utils.IsValidVersionString)

	return splitPathAroundVersionedDir(dirpath, dirnameCheckers)
}

func getCaseInsensitiveStringEqualityChecker(knownString string) func(currentString string) bool {
	return func(currentString string) bool {
		return strings.EqualFold(knownString, currentString)
	}
}

func vmAppUpdateCallback(ext *vmextensionhelper.VMExtension) error {
	// for extension update on windows, we retrieve the applicationRegistry.active file from a previous version of the extension

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

	folderPathThatContainsAllTheVersions, versionedDirName, relativePathToConfigFolder, err := splitPathAroundVersionedDirWindows(ext.HandlerEnv.ConfigFolder)
	if err != nil {
		return err
	}
	dirnameChecker := getCaseInsensitiveStringEqualityChecker(ExtensionVersion)
	if !dirnameChecker(versionedDirName) {
		msg := fmt.Sprintf("ExtensionVersion '%s' is not part of the ext.HandlerEnv.ConfigFolder path '%s'", ExtensionVersion, ext.HandlerEnv.ConfigFolder)
		ext.ExtensionLogger.Warn(msg)
		ext.ExtensionEvents.LogWarningEvent("ExtensionUpdate", msg)
	}

	previousPackageRegistryFilePath, err := getMostRecentlyUpdatedPackageRegistryFile(folderPathThatContainsAllTheVersions, relativePathToConfigFolder, utils.IsValidVersionString)
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

	// do the following operations in a best effort manner
	err = moveDownloadDirToCurrentVersion(ext)
	if err != nil {
		ext.ExtensionLogger.Warn("Failed to move download directory to current version with error: %v", err)
		ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to move download directory to current version with error: %v", err))
	} else {
		msg = "successfully moved download directory to current version"
		ext.ExtensionLogger.Info(msg)
		ext.ExtensionEvents.LogInformationalEvent("ExtensionUpdate", msg)

		if err = updateDownloadDirInPackageRegistryFile(ext); err != nil {
			ext.ExtensionLogger.Warn("Failed to update download directory in package registry file with error: %v", err)
			ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to update download directory in package registry file with error: %v", err))
		} else {
			msg = "successfully updated download directory paths in package registry file"
			ext.ExtensionLogger.Info(msg)
			ext.ExtensionEvents.LogInformationalEvent("ExtensionUpdate", msg)
		}
	}
	return nil
}

func updateDownloadDirInPackageRegistryFile(ext *vmextensionhelper.VMExtension) error {
	packageRegistry, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		return err
	}
	defer packageRegistry.Close()
	existingPackages, err := packageRegistry.GetExistingPackages()
	if err != nil {
		return err
	}

	if len(existingPackages) == 0 {
		return nil
	}

	downloadDirBeforeVersion, _, downloadDirAfterVersion, err := splitPathAroundVersionedDirWindows(ext.HandlerEnv.DataFolder)
	if err != nil {
		return err
	}

	// Build a regex that matches: <prefix>/<any_version>/<suffix> in DownloadDir paths
	// Use forward slashes because we normalize file paths to forward slash before running the regex
	// after replacement, the file path will be converted back to OS specific separator (baclskash in this case for windows)
	// by using filepath.FromSlash
	escapedPrefix := regexp.QuoteMeta(filepath.ToSlash(downloadDirBeforeVersion))
	escapedSuffix := regexp.QuoteMeta(filepath.ToSlash(downloadDirAfterVersion))
	downloadDirVersionRegex := regexp.MustCompile(escapedPrefix + `/[^/]+/` + escapedSuffix)
	replacement := filepath.ToSlash(downloadDirBeforeVersion) + "/" + ExtensionVersion + "/" + filepath.ToSlash(downloadDirAfterVersion)

	for packageName, packageInfo := range existingPackages {
		normalized := filepath.ToSlash(packageInfo.DownloadDir)
		updated := downloadDirVersionRegex.ReplaceAllLiteralString(normalized, replacement)
		if updated == normalized {
			ext.ExtensionLogger.Warn("Could not update downloadDir for package '%s', no version segment matched", packageName)
			ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Could not update downloadDir for package '%s', no version segment matched", packageName))
		} else {
			packageInfo.DownloadDir = filepath.FromSlash(updated)
		}
	}

	return packageRegistry.WriteToDisk(existingPackages)
}

// move the download directory from old version to new version
func moveDownloadDirToCurrentVersion(ext *vmextensionhelper.VMExtension) error {
	packageRegistry, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		return err
	}
	defer packageRegistry.Close()

	rootOfAllVersions, versionedDirName, relativePathAfterVersion, err := splitPathAroundVersionedDirWindows(ext.HandlerEnv.DataFolder)
	if err != nil {
		return err
	}

	if !strings.EqualFold(versionedDirName, ExtensionVersion) {
		msg := fmt.Sprintf("ExtensionVersion mismatch: ext.HandlerEnv.DataFolder path '%s' does contain versionedDirName '%s'", ext.HandlerEnv.DataFolder, versionedDirName)
		ext.ExtensionLogger.Warn(msg)
		ext.ExtensionEvents.LogWarningEvent("ExtensionVersion", msg)
	}

	entries, err := os.ReadDir(rootOfAllVersions)
	if err != nil {
		return err
	}
	var downloadDirectoryForAllVersions []string
	for _, entry := range entries {
		if entry.IsDir() && utils.IsValidVersionString(entry.Name()) {
			dirName := filepath.Join(rootOfAllVersions, entry.Name(), relativePathAfterVersion)
			downloadDir, err := os.Stat(dirName)
			if err != nil {
				ext.ExtensionLogger.Warn("Skipping directory %s when looking for download directories to move, with error: %v", dirName, err)
				ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Skipping directory %s when looking for download directories to move, with error: %v", dirName, err))
				continue
			}
			if !downloadDir.IsDir() || strings.EqualFold(entry.Name(), versionedDirName) {
				//if the config folder is not a directory, or if the version folder is the same as the current version, then skip it
				continue
			}
			downloadDirectoryForAllVersions = append(downloadDirectoryForAllVersions, dirName)
		}
	}

	for _, downloadDir := range downloadDirectoryForAllVersions {
		directoryContents, err := os.ReadDir(downloadDir)
		if err != nil {
			ext.ExtensionLogger.Warn("Failed to read directory %s with error: %v", downloadDir, err)
			ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to read directory %s with error: %v", downloadDir, err))
			continue
		}
		for _, entry := range directoryContents {
			if entry.IsDir() {
				sourceDirFullPath := filepath.Join(downloadDir, entry.Name())
				destDirFullPath := filepath.Join(ext.HandlerEnv.DataFolder, entry.Name())
				err = copySubdirectoryUsingRobocopy(sourceDirFullPath, destDirFullPath)
				// copy the directory from current entry to ext.HandlerEnv.DataFolder
				if err != nil {
					ext.ExtensionLogger.Warn("Failed to copy directory from %s to %s with error: %s", sourceDirFullPath, destDirFullPath, err.Error())
					ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to copy directory from %s to %s with error: %s", sourceDirFullPath, destDirFullPath, err.Error()))
				}
			}
		}
	}

	return nil
}

func copySubdirectoryUsingRobocopy(src, dst string) error {
	cmd := exec.Command("robocopy", src, dst, "/E", "/sl", "/NFL", "/NDL", "/NJH", "/NJS")
	err := cmd.Run()
	// robocopy exit codes 0-7 are success/informational
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() < 8 {
		return nil
	}
	return err
}
