package main

import "errors"

type lockedFile struct {
	FileDescriptor int
}

func FileLockInit(filePath string, timeout time.Duration) (*lockedFile, error) {
	return nil, errors.New("Not implemented")
}

func (self *lockedFile) Close() (error) {
	return errors.New("Not implemented")
}
