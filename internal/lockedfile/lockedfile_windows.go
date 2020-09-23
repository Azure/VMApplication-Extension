package lockedfile

import (
	"golang.org/x/sys/windows"
	"syscall"
	"time"
)

const (
	reserved = 0
	allBytes = ^uint32(0)
)

type LockedFile struct {
	FileHandle windows.Handle
}

func New(filePath string, timeout time.Duration) (*LockedFile, error) {
	name, err := windows.UTF16PtrFromString(filePath)
	if err != nil {
		return nil, err
	}

	// Open for asynchronous I/O so that we can timeout waiting for the lock.
	// Also open shared so that other processes can open the file (but will
	// still need to lock it).
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		uint32(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE),
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_FLAG_OVERLAPPED|windows.FILE_ATTRIBUTE_NORMAL,
		0)

	if err != nil {
		return nil, err
	}

	ol, err := GetOverlapped()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(ol.HEvent)

	err = windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, reserved, allBytes, allBytes, ol)
	if err == nil {
		return &LockedFile{handle}, nil
	}

	// ERROR_IO_PENDING is expected when we're waiting on an asychronous event
	// to occur.
	if err != syscall.ERROR_IO_PENDING {
		return nil, err
	}

	timeoutInMilliseconds := uint32(timeout / time.Millisecond)
	s, err := windows.WaitForSingleObject(ol.HEvent, timeoutInMilliseconds)

	switch s {
	case syscall.WAIT_OBJECT_0:
		// success!
		return &LockedFile{handle}, nil
	case syscall.WAIT_TIMEOUT:
		windows.CancelIo(handle)
		return nil, &FileLockTimeoutError{"file lock could not be acquired in the specified time"}
	default:
		return nil, err
	}
}

func (self *LockedFile) Close() (error) {
	err := windows.UnlockFileEx(self.FileHandle, reserved, allBytes, allBytes, &windows.Overlapped{HEvent:0})
	if err != nil{
		return err
	}

	return windows.Close(self.FileHandle)
}

func GetOverlapped() (*windows.Overlapped, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}

	return &windows.Overlapped{HEvent: event}, nil
}
