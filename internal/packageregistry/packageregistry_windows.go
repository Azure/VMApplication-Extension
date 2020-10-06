package packageregistry

import (
	"encoding/json"
	"github.com/Azure/VMApplication-Extension/internal/lockedfile"
	"golang.org/x/sys/windows"
	"syscall"
)

const (
	fileIOTimeoutInMilliseconds = 10000
)

// returns a map of VMApps Name to VMAppPackageCurrent for all packages that are currently installed on the VM
// do not call directly except for test, meant to be called from the wrapper function in packageregistry_linux or
// packageregistry_windows
func (self *RegistryHandler) GetExistingPackages() (CurrentPackageRegistry, error) {
	// make an empty byte slice with 4KB default size
	fileBytes := make([]byte, 0, 4096)
	buffer := make([]byte, 4096, 4096)

	ol, err := lockedfile.GetOverlapped()
	if err != nil {
		return nil, err
	}
	defer windows.Close(ol.HEvent)
	for {
		err := windows.ReadFile(self.lockedFile.FileHandle, buffer, nil, ol)
		if err != nil && err != syscall.ERROR_IO_PENDING {
			return nil, err
		}
		var readBytes uint32
		err = windows.GetOverlappedResult(self.lockedFile.FileHandle, ol, &readBytes, true)
		if err != nil {
			if err == windows.ERROR_HANDLE_EOF {
				break;
			}
			return nil, err
		}

		fileBytes = append(fileBytes, buffer[:readBytes]...)

		// modify ol to read next bytes
		longOffset := CombineTwoUint32ToUlong(ol.OffsetHigh, ol.Offset)
		longOffset += uint64(readBytes)
		ol.OffsetHigh, ol.Offset = SplitUlongToTwoUint32(longOffset)
		windows.ResetEvent(ol.HEvent)
	}

	vmAppsPackages := VMAppPackageCurrentCollection{}
	err = json.Unmarshal(fileBytes, &vmAppsPackages)
	if err != nil {
		return nil, err
	}

	packageRegistry := make(CurrentPackageRegistry)

	err = packageRegistry.Populate(vmAppsPackages)

	if err != nil {
		return nil, err
	}

	return packageRegistry, nil
}

func SplitUlongToTwoUint32(ulong uint64) (high uint32, low uint32) {
	low = uint32(ulong)
	high = uint32(ulong >> 32)
	return
}

func CombineTwoUint32ToUlong(high uint32, low uint32) (long uint64) {
	long = uint64(high)
	long = long << 32
	long += uint64(low)
	return long
}

// do not call directly except for test, meant to be called from the wrapper function in packageregistry_linux or
// packageregistry_windows
func (self *RegistryHandler) WriteToDisk(packageRegistry CurrentPackageRegistry) (error) {
	vmAppsPackages := packageRegistry.GetPackageCollection()
	bytes, err := json.Marshal(vmAppsPackages)
	if err != nil {
		return err
	}

	ol, err := lockedfile.GetOverlapped()
	if err != nil {
		return err
	}
	defer windows.Close(ol.HEvent)

	err = windows.WriteFile(self.lockedFile.FileHandle, bytes, nil, ol)

	if err != syscall.ERROR_IO_PENDING {
		return err
	}

	s, err := windows.WaitForSingleObject(ol.HEvent, fileIOTimeoutInMilliseconds)

	switch s {
	case syscall.WAIT_OBJECT_0:
		// success writing file
		return err
	case syscall.WAIT_TIMEOUT:
		windows.CancelIo(self.lockedFile.FileHandle)
		return &FileIoTimeout{"fileIO timed out"}
	default:
		return err
	}

	return nil
}
