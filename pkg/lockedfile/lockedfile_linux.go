package lockedfile

import (
	"github.com/Azure/VMApplication-Extension/VmApp/constants"
	"os"
	"syscall"
	"time"
)

type LockedFile struct {
	fileDescriptor int
	metadata *Metadata
}

func newInner(filePath string, timeout time.Duration, metadata *Metadata) (*LockedFile, error) {
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
		return &LockedFile{file, metadata}, nil
	case <-time.After(timeout):
		close(timeoutChan)
		return nil, FileLockTimeoutError{"file lock could not be acquired in the specified time"}
	}
}

func (self *LockedFile) ReadLockedFile()([]byte, error){
	fileBytes := make([]byte, 0, 4096)
	buffer := make([]byte, 4096, 4096)

	// reset the packageRegistryFileHandle
	syscall.Seek(self.FileDescriptor, 0, 0)
	for {
		nbytes, err := syscall.Read(self.FileDescriptor, buffer)
		if err != nil && err.Error() != "EOF" {
			return nil, err
		}
		if nbytes == 0 {
			break
		}
		fileBytes = append(fileBytes, buffer[:nbytes]...)
	}
	return fileBytes, nil
}

func (self *LockedFile) WriteLockedFile(bytes []byte)(error){
	syscall.Seek(self.FileDescriptor, 0, syscall.FILE_BEGIN)
	_, err = syscall.Write(self.FileDescriptor, bytes)
	if err != nil {
		return err
	}
	syscall.SetEndOfFile(self.FileDescriptor)
	return nil
}

func (self *LockedFile) closeInner() (error) {
	return syscall.Close(self.FileDescriptor)
}
