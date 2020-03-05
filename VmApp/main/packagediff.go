package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	vmextensionhelper "github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/go-kit/kit/log"
)

const (
	currentPackageStateFileName = "packagestate"
	proposedPackageStateSuffix  = "proposed"
	MaxUint                     = ^uint(0)
	MaxInt                      = int(MaxUint >> 1)
)

var (
	osDependency osDependencies = &osDependenciesImpl{}
)

type collectionResolveCallback func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage)

type osDependencies interface {
	stat(string) (os.FileInfo, error)
	readfile(string) ([]byte, error)
	writefile(string, []byte, os.FileMode) error
	removefile(string) error
}

type osDependenciesImpl struct{}

type proposedFile struct {
	Name       string
	FileNumber int
}

func (*osDependenciesImpl) stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (*osDependenciesImpl) readfile(name string) ([]byte, error) {
	return ioutil.ReadFile(name)
}

func (*osDependenciesImpl) writefile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

func (*osDependenciesImpl) removefile(name string) error {
	return os.Remove(name)
}

// getPackageStatePlan is the central method for determining what we need to do
// It factors in three sources of data
// 1) The requested package data received by the extension, which is passed in
// 2) The current package state, which we read from disk
// 3) The proposed package state, which are packages that a previous instantiation has not yet processed
//
// This method will combine these three and come up with a plan of action. However, it does not return that plan
// Instead, it writes them to disk as proposed package states and returns two booleans
// requiresChanges - indicates whether the calling code needs to do anything at all
// hasProposedState - whether we found any proposed states
// The calling code will factor these in to deciding whether to proceed (it needs to verify if the other process is running)
// The calling code will then retrieve packages one by one and process them
//
// Note that a potential race condition exists for which we currently cannot do much
// If one instance of the extension is installing version 1 for some VmApp, and another instance is started with
// a different version or removal, then both actions will be running simultaneously. Unfortunately, even if we
// signaled instance 1 to stop, it doesn't have the ability to do so. It only runs a script, and killing it isn't
// guaranteed to help because the install may actually be occurring on another process and there are likely
// artifacts that will need to be rolled back. If customers absolutely need this handled, then their installation
// and removal code needs to factor that in.
func getPackageStatePlan(ctx log.Logger, ext *vmextensionhelper.VMExtension, requested *vmPackageData) (requiresChanges bool, hasProposedState bool, _ error) {
	currentPackageState, err := getCurrentPackageState(ctx, ext)
	if err != nil {
		return false, false, err
	}

	proposedPackageState := getProposedPackageState(ctx, ext)

	// First we need to resolve the requested package state with our current package state
	resolveFromPackageStates(ctx, ext, requested, currentPackageState, collectionResolveCallback(sourcePackageNotNeeded), nil)

	// Now we need to examine our proposed package state with what we resolved above
	// If our resolved package state supersedes the proposed package state, then we must delete the proposed file
	resolveFromPackageStates(ctx, ext, requested, proposedPackageState, collectionResolveCallback(sourcePackageNotNeeded), collectionResolveCallback(proposedPackageNotNeeded))

	hasProposedPackages := false
	if proposedPackageState != nil && len(proposedPackageState.Packages) > 0 {
		hasProposedPackages = true
	}

	if len(requested.Packages) == 0 {
		ctx.Log("message", "No remaining package states after resolving.")
		return false, hasProposedPackages, nil
	}

	// Now write all package states for our plan
	maxProposedNumber := 0
	if proposedPackageState != nil {
		for _, proposedFile := range proposedPackageState.Packages {
			if proposedFile.ProposedFileNumber > maxProposedNumber {
				maxProposedNumber = proposedFile.ProposedFileNumber
			}
		}
	}

	err = writeProposedPackages(ctx, ext, requested, maxProposedNumber+1)
	if err != nil {
		return false, false, err
	}

	return true, hasProposedPackages, nil
}

func getNextProposedPackage(ctx log.Logger, ext *vmextensionhelper.VMExtension) *vmPackage {
	// Retrieve all of the proposed packages
	proposedPackageState := getProposedPackageState(ctx, ext)
	if proposedPackageState == nil {
		return nil
	}

	// Find the lowest number
	minProposedNumber := MaxInt
	var returnPackage vmPackage
	for _, proposedPackage := range proposedPackageState.Packages {
		if proposedPackage.ProposedFileNumber < minProposedNumber {
			minProposedNumber = proposedPackage.ProposedFileNumber
			returnPackage = proposedPackage
		}
	}

	return &returnPackage
}

func markProposedPackageFinished(ctx log.Logger, ext *vmextensionhelper.VMExtension, pkg *vmPackage) error {
	fileName := strconv.Itoa(pkg.ProposedFileNumber) + "." + proposedPackageStateSuffix
	filePath := path.Join(ext.HandlerEnv.DataFolder, fileName)
	ctx.Log("info", fmt.Sprintf("Removing file %s because processing has finished", filePath))

	err := os.Remove(filePath)
	if err != nil {
		ctx.Log("error", fmt.Sprintf("Unable to delete proposal file %s: %v", filePath, err))
	}

	return err
}

func writeProposedPackages(ctx log.Logger, ext *vmextensionhelper.VMExtension, requested *vmPackageData, startProposedNumber int) error {
	currentNumber := startProposedNumber
	for _, proposal := range requested.Packages {
		fileName := strconv.Itoa(currentNumber) + "." + proposedPackageStateSuffix
		filePath := path.Join(ext.HandlerEnv.DataFolder, fileName)

		// We wrap the package in a new package data to make deserialization simpler
		toWriteData := vmPackageData{Packages: []vmPackage{proposal}}

		// Serialize and write the file. If we fail, stop immediately because we now have lost an operation.
		b, err := json.Marshal(toWriteData)
		if err != nil {
			ctx.Log("message", "writeProposedPackages failed", "error", fmt.Errorf("Error marshaling data for %s: %v", filePath, err))
			return fmt.Errorf("Error marshaling data for %s: %v", filePath, err)
		}

		err = osDependency.writefile(filePath, b, 0)
		if err != nil {
			ctx.Log("message", "writeProposedPackages failed", "error", fmt.Errorf("Error writing data for %s: %v", filePath, err))
			return fmt.Errorf("Error writing data for %s: %v", filePath, err)
		}

		currentNumber++
	}

	return nil
}

func proposedPackageNotNeeded(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
	ctx.Log("message", fmt.Sprintf("Proposed package %s superceded by the current plan. Removing.", losingPackage.Name))

	fileName := strconv.Itoa(losingPackage.ProposedFileNumber) + "." + proposedPackageStateSuffix
	fullFilePath := path.Join(ext.HandlerEnv.DataFolder, fileName)
	err := os.Remove(fullFilePath)
	if err != nil {
		ctx.Log("error", fmt.Sprintf("Unable to delete proposal file %s: %v", losingPackage.Name, err))
	}
}

func sourcePackageNotNeeded(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
	ctx.Log("message", fmt.Sprintf("Requested package %s superceded by the existing packages. Not processing.", losingPackage.Name))
}

// resolveFromPackageStates iterates through all packages in source and determines if any in current are identical
// If source is superceded by current, then it will delete the item from source
// If source is superceded, it will call the sourceFunc callback
// If current is superceded, it will call the currentFunc callback
func resolveFromPackageStates(ctx log.Logger, ext *vmextensionhelper.VMExtension, source *vmPackageData, current *vmPackageData, sourceFunc collectionResolveCallback, currentFunc collectionResolveCallback) {
	resolvedPackages := make([]vmPackage, len(source.Packages))

	count := 0
	for _, sourcePackage := range source.Packages {
		currentPackage := findPackageState(sourcePackage.Name, current)
		if currentPackage != nil {
			ctx.Log("message", fmt.Sprintf("Requested package %s exists in current state. Resolving", sourcePackage.Name))
			resolvedPackage := resolvePackageState(ctx, &sourcePackage, currentPackage)
			if *resolvedPackage == sourcePackage {
				// The requested package won, so just call the callback
				resolvedPackages[count] = sourcePackage
				count++
				if currentFunc != nil {
					currentFunc(ctx, ext, currentPackage)
				}
			} else {
				// The current state won, so forget this item and call the callback
				if sourceFunc != nil {
					sourceFunc(ctx, ext, &sourcePackage)
				}
			}
		} else {
			resolvedPackages[count] = sourcePackage
			count++
		}
	}

	source.Packages = resolvedPackages[:count]
}

func getCurrentPackageState(ctx log.Logger, ext *vmextensionhelper.VMExtension) (*vmPackageData, error) {
	// Operations for our current package state are:
	// install - we've installed this version, but have never updated it
	// update - we've updated the app to this version
	// delete - we used to have the app, and the last version we had was this one

	// Check if the file exists. If it doesn't, then we have no current data.
	packageStateFullPath := path.Join(ext.HandlerEnv.DataFolder, currentPackageStateFileName)
	exists, err := doesFileExist(packageStateFullPath)
	if err != nil {
		ctx.Log("event", "Failed to read current package state", "error", err)
		return nil, err
	}

	if !exists {
		ctx.Log("message", "No current package state")
		return nil, nil
	}

	return getPackageStateFromFile(ctx, packageStateFullPath)
}

func getPackageStateFromFile(ctx log.Logger, filePath string) (*vmPackageData, error) {
	b, err := osDependency.readfile(filePath)
	if err != nil {
		ctx.Log("message", "getPackageStateFromFile failed", "error", fmt.Errorf("Error reading %s: %v", filePath, err))
		return nil, fmt.Errorf("Error reading %s: %v", filePath, err)
	}

	if len(b) == 0 {
		ctx.Log("message", "Package state is empty")
		return &vmPackageData{Packages: []vmPackage{}}, nil
	}

	var packageData vmPackageData
	if err := json.Unmarshal(b, &packageData); err != nil {
		ctx.Log("message", "getPackageStateFromFile failed", "error", fmt.Errorf("error parsing current package state json: %v", err))
		return nil, fmt.Errorf("error parsing current package state json: %v", err)
	}

	return &packageData, nil
}

func getProposedPackageState(ctx log.Logger, ext *vmextensionhelper.VMExtension) *vmPackageData {
	// Operations are the same as our current package state, but the files will have one operation each
	// This method will read all existing files and combine their states
	// Files are of the form {num}.proposed. Note that num is NOT the sequence number, but is just an increasing number.
	var proposedFiles []proposedFile
	filepath.Walk(ext.HandlerEnv.DataFolder, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() {
			fileNumber, isProposedFile := getProposedFileNumber(f.Name())
			if isProposedFile {
				proposedFiles = append(proposedFiles, proposedFile{Name: f.Name(), FileNumber: fileNumber})
			}
		}
		return nil
	})

	if len(proposedFiles) == 0 {
		// No proposed files to process
		return nil
	}

	var packageData vmPackageData
	for _, proposedFile := range proposedFiles {
		filePath := path.Join(ext.HandlerEnv.DataFolder, proposedFile.Name)
		proposedPackages, err := getPackageStateFromFile(ctx, filePath)
		if err != nil {
			// If we can't read a proposed package state, move on
			// TODO: is this the correct behavior?
			ctx.Log("message", "Couldn't read proposed package state", "error", fmt.Errorf("Couldn't read proposed package state from %s: %v", filePath, err))
			continue
		}

		// Proposed package files should only have one package
		if len(proposedPackages.Packages) != 1 {
			ctx.Log("message", "Invalid package state file", "error", fmt.Errorf("Invalid package count in %s", filePath))
			continue
		}

		// If we already have this package state, resolve it
		proposedPackage := proposedPackages.Packages[0]
		proposedPackage.ProposedFileNumber = proposedFile.FileNumber
		pos, existingPackage := findPackageInSlice(proposedPackage.Name, packageData.Packages)
		if existingPackage == nil {
			packageData.Packages = append(packageData.Packages, proposedPackage)
		} else {
			// Figure out which package takes precedence
			resolvedPackage := resolvePackageState(ctx, existingPackage, &proposedPackage)

			// Remove the conflicting package. Here we need to keep the order
			pkgs := packageData.Packages
			copy(pkgs[pos:], pkgs[pos+1:])
			packageData.Packages = pkgs[:len(pkgs)-1]

			// Add the resolved package
			packageData.Packages = append(packageData.Packages, *resolvedPackage)
		}
	}

	return &packageData
}

func findPackageState(name string, packageData *vmPackageData) *vmPackage {
	if packageData != nil {
		_, p := findPackageInSlice(name, packageData.Packages)

		return p
	}

	return nil
}

func findPackageInSlice(name string, packages []vmPackage) (int, *vmPackage) {
	for pos, p := range packages {
		if strings.Compare(name, p.Name) == 0 {
			return pos, &p
		}
	}

	return -1, nil
}

func resolvePackageState(ctx log.Logger, first *vmPackage, second *vmPackage) *vmPackage {
	// Rules for package merging
	// If both are install/update, then choose the higher version. If either is install, then set that state to install.
	// If first is removed, but second is install and has a higher version, then choose second
	// If second is remove, then choose second
	ctx.Log("message", fmt.Sprintf("Resolving package state for %s. First(version=%s, operation=%s) Second(version=%s, operation=%s)", first.Name, first.Version, first.Operation, second.Version, second.Operation))
	if first.isInstallOrUpdate() && second.isInstallOrUpdate() {
		// If they are equal, stick with first
		compare := compareVersions(ctx, first.Version, second.Version)
		if compare == 0 {
			ctx.Log("message", fmt.Sprintf("Versions are identical. Choosing the first"))
			if second.isInstall() {
				first.Operation = operationInstall
			}
			return first
		}

		if compare == -1 {
			ctx.Log("message", fmt.Sprintf("Choosing the second due to a higher version."))
			if first.isInstall() {
				second.Operation = operationInstall
			}
			return second
		}

		ctx.Log("message", fmt.Sprintf("Choosing the first due to a higher version"))
		if second.isInstall() {
			first.Operation = operationInstall
		}
		return first
	}

	if first.isRemove() {
		if second.isInstall() && compareVersions(ctx, first.Version, second.Version) == -1 {
			ctx.Log("message", fmt.Sprintf("Choosing the second due to a higher version"))
			return second
		}

		if second.isRemove() {
			ctx.Log("message", "Both packages are remove. Choosing the second.")
			return second
		}

		ctx.Log("message", fmt.Sprintf("Choosing first due to a higher version"))
		return first
	}

	// By logic, we only reach here if the second is remove but the first is not
	ctx.Log("message", fmt.Sprintf("Second is remove, so choosing the second"))
	return second
}

func compareVersions(ctx log.Logger, first string, second string) int {
	firstParts := strings.Split(first, ".")
	secondParts := strings.Split(second, ".")

	for i := 0; i < len(firstParts); i++ {
		if len(secondParts) <= i {
			// Second has a subversion that first doesn't have
			return 1
		}

		firstPart, err := strconv.Atoi(firstParts[i])
		if err != nil {
			ctx.Log("message", fmt.Sprintf("Cannot parse version number for %s", first))
			return strings.Compare(first, second)
		}

		secondPart, err := strconv.Atoi(secondParts[i])
		if err != nil {
			ctx.Log("message", fmt.Sprintf("Cannot parse version number for %s", second))
			return strings.Compare(first, second)
		}

		if firstPart > secondPart {
			return 1
		}

		if firstPart < secondPart {
			return -1
		}
	}

	if len(secondParts) > len(firstParts) {
		// Second has a subpart that first doesn't have
		return -1
	}

	return 0
}

func getProposedFileNumber(fileName string) (fileNumber int, isProposedFile bool) {
	if !strings.HasSuffix(fileName, proposedPackageStateSuffix) {
		return 0, false
	}

	parts := strings.Split(fileName, ".")
	if len(parts) != 2 {
		return 0, false
	}

	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}

	return n, true
}

func doesFileExist(filePath string) (bool, error) {
	_, err := osDependency.stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return true, err
	}

	return true, nil
}
