package lockedfile

type FileLockTimeoutError struct{
	message string
}

func FileLockTimeoutErrorInit(message string)(*FileLockTimeoutError){
	return &FileLockTimeoutError{message: message}
}

func (self *FileLockTimeoutError) Error() (string){
	return self.message
}