package packageregistry

import (
	"errors"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"time"
)


type PackageHandler struct {
	handlerEnv *vmextensionhelper.HandlerEnvironment
	lockedFile *lockedFile
}

func PackageHandlerInit(handlerEnv *vmextensionhelper.HandlerEnvironment, fileLockTimeout time.Duration) (*PackageHandler, error) {

}

