package packageregistry

import (
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/Azure/VMApplication-Extension/internal/lockedfile"
	"path"
	"time"
)

const (
	localApplicationRegistryFileName                         = "applicationRegistry"
	localApplicationRegistryBackupFileName                   = "applicationRegistry.backup"
	activeChangesFileName                                    = "applicationRegistry.active"
	localApplicationRegistryFileDefaultTimeout time.Duration = 30 * time.Minute
)

// defines a map between the application name and the other properties of the application
type PackageRegistry map[string]*VMAppsPackage

type VMAppsPackages []*VMAppsPackage

type VMAppsPackage struct {
	ApplicationName       string `json:"ApplicationName"`
	PackageLocation       string `json:"location"`
	ConfigurationLocation string `json:"config"`
	Version               string `json:"version"`
	InstallCommand        string `json:"install"`
	RemoveCommand         string `json:"remove"`
	UpdateCommand         string `json:"update"`
	DirectDownloadOnly    bool   `json:"directOnly"`
}

type IPackageHandler interface {
	GetExistingPackages() (PackageRegistry, error)
	WriteToDisk(packageRegistry PackageRegistry) (error)
	Close()(error)
}

type PackageRegistryHandler struct {
	handlerEnv *vmextensionhelper.HandlerEnvironment
	lockedFile *lockedfile.LockedFile
}

// Keep the PackageRegistryHandler object alive as long as the package registry is being accessed to lock it
// Call PackageRegistryHandler.Close() to release locks on the registry file
func New(handlerEnv *vmextensionhelper.HandlerEnvironment, fileLockTimeout time.Duration) (*PackageRegistryHandler, error) {
	appRegistryFilePath := path.Join(handlerEnv.ConfigFolder, localApplicationRegistryFileName)
	fileLock, err := lockedfile.New(appRegistryFilePath, fileLockTimeout)
	if err != nil {
		return nil, err
	}

	return &PackageRegistryHandler{handlerEnv: handlerEnv, lockedFile: fileLock}, nil
}

// Closes the file handle, renders the object of the class PackageRegistryHandler unusable
func (self *PackageRegistryHandler) Close()(error) {
	return self.lockedFile.Close()
}


