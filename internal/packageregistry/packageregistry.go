package packageregistry

import (
	"encoding/json"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper/constants"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper/handlerenv"
	"github.com/Azure/VMApplication-Extension/pkg/lockedfile"
	"io/ioutil"
	"os"
	"path"
	"time"
)

const (
	lockFileName                                             = "VMApp.lockfile"
	localApplicationRegistryBackupFileName                   = "applicationRegistry.backup"
	localApplicationRegistryFileName                         = "applicationRegistry.active"
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
	ApplicationName       string     `json:"applicationName"`
	PackageLocation       string     `json:"location"`
	ConfigurationLocation string     `json:"config"`
	Version               string     `json:"version"`
	InstallCommand        string     `json:"install"`
	RemoveCommand         string     `json:"remove"`
	UpdateCommand         string     `json:"update"`
	DirectDownloadOnly    bool       `json:"directOnly"`
	OngoingOperation      ActionEnum `json:"ongoingOperation"`
}

func (vmAppPackageCurrent *VMAppPackageCurrent) GetWorkingDirectory(environment *handlerenv.HandlerEnvironment) string {
	return path.Join(environment.DataFolder, vmAppPackageCurrent.ApplicationName, vmAppPackageCurrent.Version)
}

type VMAppPackageIncomingCollection []*VMAppPackageIncoming

type VMAppPackageIncoming struct {
	ApplicationName       string `json:"applicationName"`
	PackageLocation       string `json:"location"`
	ConfigurationLocation string `json:"config"`
	Version               string `json:"version"`
	InstallCommand        string `json:"install"`
	RemoveCommand         string `json:"remove"`
	UpdateCommand         string `json:"update"`
	DirectDownloadOnly    bool   `json:"directOnly"`
	Order                 *int   `json:"order"`
}

type IPackageRegistry interface {
	GetExistingPackages() (CurrentPackageRegistry, error)
	WriteToDisk(packageRegistry CurrentPackageRegistry) error
	Close() error
}

type PackageRegistry struct {
	handlerEnv *handlerenv.HandlerEnvironment
	lockedFile lockedfile.ILockedFile
}

// Keep the PackageRegistry object alive as long as the package registry is being accessed to lock it
// Call PackageRegistry.Close() to release locks on the registry file
func New(handlerEnv *handlerenv.HandlerEnvironment, fileLockTimeout time.Duration) (IPackageRegistry, error) {
	appRegistryFilePath := path.Join(handlerEnv.ConfigFolder, lockFileName)
	fileLock, err := lockedfile.New(appRegistryFilePath, fileLockTimeout)
	if err != nil {
		return nil, err
	}

	return &PackageRegistry{handlerEnv: handlerEnv, lockedFile: fileLock}, nil
}

// Closes the file handle, renders the object of the class PackageRegistry unusable
func (self *PackageRegistry) Close() error {
	return self.lockedFile.Close()
}

func (self *PackageRegistry) GetExistingPackages() (CurrentPackageRegistry, error) {
	var currentPackageRegistry CurrentPackageRegistry
	currentPackageRegistry = nil
	localApplicationRegistryFilePath := self.getLocalApplicationRegistryFilePath()

	fileBytes, err := ioutil.ReadFile(localApplicationRegistryFilePath)
	if err != nil {
		return currentPackageRegistry, err
	}
	vmAppPackageCurrentCollection := VMAppPackageCurrentCollection{}
	if len(fileBytes) > 0 {
		err = json.Unmarshal(fileBytes, &vmAppPackageCurrentCollection)
		if err != nil {
			return currentPackageRegistry, err
		}
	}
	currentPackageRegistry = make(CurrentPackageRegistry)
	err = currentPackageRegistry.Populate(vmAppPackageCurrentCollection)
	return currentPackageRegistry, err
}

func (self *PackageRegistry) WriteToDisk(packageRegistry CurrentPackageRegistry) error {
	regFile := self.getLocalApplicationRegistryFilePath()
	regFileBackup := self.getLocalApplicationRegistryBackupFilePath()
	var doesBackupFileExist = false
	err := os.Rename(regFile, regFileBackup)
	if err != nil {
		if !os.IsNotExist(err) {
			// return on errors other than source file does not exist for os.Rename operation
			return err
		}
	} else {
		doesBackupFileExist = true
	}

	vmAppPackageCurrentCollection := packageRegistry.GetPackageCollection()
	bytes, err := json.Marshal(vmAppPackageCurrentCollection)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(regFile, bytes, constants.FilePermissions_UserOnly_ReadWrite)

	if doesBackupFileExist {
		return os.Remove(regFileBackup)
	} else {
		return err
	}
}

func (self *PackageRegistry) getLocalApplicationRegistryFilePath() string {
	return path.Join(self.handlerEnv.ConfigFolder, localApplicationRegistryFileName)
}

func (self *PackageRegistry) getLocalApplicationRegistryBackupFilePath() string {
	return path.Join(self.handlerEnv.ConfigFolder, localApplicationRegistryBackupFileName)
}
