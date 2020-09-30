package packageregistry

import (
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/Azure/VMApplication-Extension/internal/lockedfile"
	"math"
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

// this is needed so that VMAppPackageIncoming can be sorted by the order
func (self VMAppPackageIncomingCollection) Len() int {
	return len(self)
}

// this is needed so that VMAppPackageIncoming can be sorted by the order
// nulls last
func (self VMAppPackageIncomingCollection) Less(i, j int) bool {
	var orderAtI, orderAtJ int

	if self[i].Order == nil {
		orderAtI = math.MaxInt32
	} else {
		orderAtI = *self[i].Order
	}

	if self[j].Order == nil {
		orderAtJ = math.MaxInt32
	} else {
		orderAtJ = *self[j].Order
	}

	return orderAtI < orderAtJ
}

// this is needed so that VMAppPackageIncoming can be sorted by the order
func (self VMAppPackageIncomingCollection) Swap(i, j int) {
	temp := self[i]
	self[i] = self[j]
	self[j] = temp
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
