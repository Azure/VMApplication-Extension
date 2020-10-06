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

type ActionEnum int

const (
	NoAction ActionEnum = iota
	Install
	Update
	Delete
	Failed
	Skipped
)

// defines a map between the application name and the other properties of the application
type CurrentPackageRegistry map[string]*VMAppPackageCurrent

type DesiredPackageRegistry map[string]*VMAppPackageIncoming

type VMAppPackageCurrentCollection []*VMAppPackageCurrent

type VMAppPackageCurrent struct {
	ApplicationName       string `json:"ApplicationName"`
	PackageLocation       string `json:"location"`
	ConfigurationLocation string `json:"config"`
	Version               string `json:"version"`
	InstallCommand        string `json:"install"`
	RemoveCommand         string `json:"remove"`
	UpdateCommand         string `json:"update"`
	DirectDownloadOnly    bool   `json:"directOnly"`
	OngoingOperation      ActionEnum `json:"OngoingOperation"`
}

func (vmAppPackageCurrent *VMAppPackageCurrent) GetWorkingDirectory(environment *vmextensionhelper.HandlerEnvironment)(string){
	return path.Join(environment.DataFolder, vmAppPackageCurrent.ApplicationName)
}


type VMAppPackageIncomingCollection []*VMAppPackageIncoming

type VMAppPackageIncoming struct {
	ApplicationName       string `json:"ApplicationName"`
	PackageLocation       string `json:"location"`
	ConfigurationLocation string `json:"config"`
	Version               string `json:"version"`
	InstallCommand        string `json:"install"`
	RemoveCommand         string `json:"remove"`
	UpdateCommand         string `json:"update"`
	DirectDownloadOnly    bool   `json:"directOnly"`
	Order                 *int
}

type IRegistryHandler interface {
	GetExistingPackages() (CurrentPackageRegistry, error)
	WriteToDisk(packageRegistry CurrentPackageRegistry) (error)
	Close() (error)
}

type RegistryHandler struct {
	handlerEnv *vmextensionhelper.HandlerEnvironment
	lockedFile *lockedfile.LockedFile
}


// Keep the RegistryHandler object alive as long as the package registry is being accessed to lock it
// Call RegistryHandler.Close() to release locks on the registry file
func New(handlerEnv *vmextensionhelper.HandlerEnvironment, fileLockTimeout time.Duration) (IRegistryHandler, error) {
	appRegistryFilePath := path.Join(handlerEnv.ConfigFolder, localApplicationRegistryFileName)
	fileLock, err := lockedfile.New(appRegistryFilePath, fileLockTimeout)
	if err != nil {
		return nil, err
	}

	return &RegistryHandler{handlerEnv: handlerEnv, lockedFile: fileLock}, nil
}

// Closes the file handle, renders the object of the class RegistryHandler unusable
func (self *RegistryHandler) Close() (error) {
	return self.lockedFile.Close()
}
