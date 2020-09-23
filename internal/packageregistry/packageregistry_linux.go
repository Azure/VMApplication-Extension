package packageregistry

import (
	"encoding/json"
	"syscall"
)

// returns a map of VMApps Name to VMAppsPackage for all packages that are currently installed on the VM
// do not call directly except for test, meant to be called from the wrapper function in packageregistry_linux or
// packageregistry_windows
func (self *PackageRegistryHandler) GetExistingPackages() (PackageRegistryHandler, error) {
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
func (self *PackageRegistryHandler) WriteToDisk(packageRegistry PackageRegistryHandler) (error) {
	values := make(VMAppsPackages, 0)
	for _, v := range (packageRegistry) {
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


