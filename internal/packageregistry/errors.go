package packageregistry

type FileIoTimeout struct{
	message string
}

func (self *FileIoTimeout) Error() (string){
	return self.message
}
