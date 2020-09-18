package main

import (
	"encoding/json"
	"github.com/Azure/VMApplication-Extension/VmApp/constants"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"os"
	"path"
	"syscall"
	"time"
)

type lockedFile struct {
	FileDescriptor int
}
type PackageHandler struct {
	handlerEnv *vmextensionhelper.HandlerEnvironment
	lockedFile *lockedFile
}

type FileLockTimeoutError struct{
	message string
}

func FileLockTimeoutErrorInit(message string)(*FileLockTimeoutError){
	return &FileLockTimeoutError{message: message}
}

func (self *FileLockTimeoutError) Error() (string){
	return self.message
}

func PackageHandlerInit(handlerEnv *vmextensionhelper.HandlerEnvironment, fileLockTimeout time.Duration) (*PackageHandler, error) {
	appRegistryFilePath := path.Join(handlerEnv.ConfigFolder, localApplicationRegistryFileName)
	fileLock, err := FileLockInit(appRegistryFilePath, fileLockTimeout)
	if err != nil {
		return nil, err
	}

	return &PackageHandler{handlerEnv: handlerEnv, lockedFile: fileLock}, nil
}

// Closes the file handle, renders the object of the class PackageHandler unusable
func (self *PackageHandler) Close() {
	self.lockedFile.Close()
}

// returns a map of VMApps Name to VMAppsPackage for all packages that are currently installed on the VM
// do not call directly except for test, meant to be called from the wrapper function in packageregistry_linux or
// packageregistry_windows
func (self *PackageHandler) GetExistingPackages() (PackageRegistry, error) {
	// make an empty byte slice with 4KB default size
	fileBytes := make([]byte, 0, 4096)
	buffer := make([]byte, 4096, 4096)

	// reset the packageRegistryFileHandle
	syscall.Seek(self.lockedFile.FileDescriptor, 0, 0)
	for {
		nbytes, err := syscall.Read(self.lockedFile.FileDescriptor, buffer)
		if err != nil && err.Error() != "EOF" {
			return nil, err
		}
		if nbytes == 0 {
			break
		}
		fileBytes = append(fileBytes, buffer[:nbytes]...)
	}

	vmAppsPackages := VMAppsPackages{}
	err := json.Unmarshal(fileBytes, &vmAppsPackages)
	if err != nil {
		return nil, err
	}

	retval := make(map[string]VMAppsPackage)

	for _, v := range vmAppsPackages {
		retval[v.ApplicationName] = v
	}

	return retval, nil
}

// do not call directly except for test, meant to be called from the wrapper function in packageregistry_linux or
// packageregistry_windows
func (self *PackageHandler) WriteToDisk(packageRegistry *PackageRegistry) (error) {
	values := make(VMAppsPackages, 0)
	for _, v := range (*packageRegistry) {
		values = append(values, v)
	}
	bytes, err := json.Marshal(values)
	if err != nil {
		return err
	}

	// reset the packageRegistryFileHandle
	syscall.Seek(self.lockedFile.FileDescriptor, 0, 0)
	_, err = syscall.Write(self.lockedFile.FileDescriptor, bytes)
	if err != nil {
		return err
	}
	return nil
}

func FileLockInit(filePath string, timeout time.Duration) (*lockedFile, error) {
	file, err := syscall.Open(filePath, os.O_RDWR|os.O_CREATE, constants.FilePermissions_UserOnly_ReadWrite)
	if err != nil {
		// file cannot be open
		return nil, err
	}

	errChan := make(chan error)
	timeoutChan := make(chan struct{})

	go func() {
		// acquire exclusive lock on file
		err := syscall.Flock(file, syscall.LOCK_EX)
		select {
		case <-timeoutChan:
			syscall.Flock(file, syscall.LOCK_UN)
			syscall.Close(file)
		case errChan <- err:
		}
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return nil, err
		}
		return &lockedFile{FileDescriptor: file}, nil
	case <-time.After(timeout):
		close(timeoutChan)
		return nil, FileLockTimeoutErrorInit("File lock could not be acquired in the specified time")
	}
}

func (self *lockedFile) Close() (error) {
	return syscall.Close(self.FileDescriptor)
}
