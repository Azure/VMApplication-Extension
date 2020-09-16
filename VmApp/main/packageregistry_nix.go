package main

import (
	"os"
	"syscall"
	"time"
)

type lockedFile struct {
	FileDescriptor int
}

func FileLockInit(filePath string, timeout time.Duration) (*lockedFile, error) {
	file, err := syscall.Open(filePath, os.O_RDWR|os.O_CREATE, 0700)
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
