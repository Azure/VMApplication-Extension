package packageregistry

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/lockedfile"
)

const (
	lockFileName                                             = "VMApp.lockfile"
	localApplicationRegistryBackupFileName                   = "applicationRegistry.backup"
	LocalApplicationRegistryFileName                         = "applicationRegistry.active"
	localApplicationRegistryFileDefaultTimeout time.Duration = 30 * time.Minute
)

type ActionEnum int

const (
	NoAction ActionEnum = iota
	Install
	Update
	Remove
	Failed
	Skipped
	// cleanup happens when a VMApp was previously skipped due to failure of an operation with lower order
	// and the VMApp has been subsequently removed from the VM/VMSS application profile
	// we need not call the remove command
	Cleanup
)

const defaultConfigFileNameSuffix = "_config"

func (act ActionEnum) ToString() string {
	switch act {
	case NoAction:
		return "NoAction"
	case Install:
		return "Install"
	case Update:
		return "Update"
	case Remove:
		return "Remove"
	case Failed:
		return "Failed"
	case Skipped:
		return "Skipped"
	case Cleanup:
		return "Cleanup"
	default:
		return "UnknownAction"
	}
}

// defines a map between the application name and the other properties of the application
type CurrentPackageRegistry map[string]*VMAppPackageCurrent

type DesiredPackageRegistry map[string]*VMAppPackageIncoming

type VMAppPackageCurrentCollection []*VMAppPackageCurrent

type VMAppPackageCurrent struct {
	ApplicationName                 string     `json:"applicationName"`
	Version                         string     `json:"version"`
	InstallCommand                  string     `json:"install"`
	RemoveCommand                   string     `json:"remove"`
	UpdateCommand                   string     `json:"update"`
	DirectDownloadOnly              bool       `json:"directOnly"`
	ConfigExists                    bool       `json:"configExists"`
	OngoingOperation                ActionEnum `json:"ongoingOperation"`
	DownloadDir                     string     `json:"downloadDir"`
	PackageFileName                 string     `json:"packageFileName"`
	ConfigFileName                  string     `json:"configFileName"`
	PackageFileMD5Checksum          []byte     `json:"packageFileMD5Checksum"`
	ConfigFileMD5Checksum           []byte     `json:"configFileMD5Checksum"`
	Result                          string     `json:"result"`
	TreatFailureAsDeploymentFailure bool       `json:"treatFailureAsDeploymentFailure"`
}

func (vmAppPackageCurrent *VMAppPackageCurrent) GetWorkingDirectory(environment *handlerenv.HandlerEnvironment) string {
	return path.Join(environment.DataFolder, vmAppPackageCurrent.ApplicationName, vmAppPackageCurrent.Version)
}

type VMAppPackageIncomingCollection []*VMAppPackageIncoming

type VMAppPackageIncoming struct {
	ApplicationName                 string `json:"applicationName"`
	Version                         string `json:"version"`
	InstallCommand                  string `json:"install"`
	RemoveCommand                   string `json:"remove"`
	UpdateCommand                   string `json:"update"`
	DirectDownloadOnly              bool   `json:"directOnly"`
	Order                           *int   `json:"order"`
	ConfigExists                    bool   `json:"configExists"`
	PackageFileName                 string `json:"packageFileName"`
	ConfigFileName                  string `json:"configFileName"`
	TreatFailureAsDeploymentFailure bool   `json:"treatFailureAsDeploymentFailure"`
}

type IPackageRegistryHandler interface {
	GetExistingPackages() (CurrentPackageRegistry, error)
	WriteToDisk(packageRegistry CurrentPackageRegistry) error
	Close() error
}

type PackageRegistryHandler struct {
	handlerEnv *handlerenv.HandlerEnvironment
	lockedFile lockedfile.ILockedFile
}

// Keep the PackageRegistry object alive as long as the package registry is being accessed to lock it
// Call PackageRegistry.Close() to release locks on the registry file
func New(handlerEnv *handlerenv.HandlerEnvironment, fileLockTimeout time.Duration) (IPackageRegistryHandler, error) {
	appRegistryFilePath := path.Join(handlerEnv.ConfigFolder, lockFileName)
	fileLock, err := lockedfile.New(appRegistryFilePath, fileLockTimeout)
	if err != nil {
		return nil, err
	}

	return &PackageRegistryHandler{handlerEnv: handlerEnv, lockedFile: fileLock}, nil
}

// Closes the file handle, renders the object of the class PackageRegistry unusable
func (packageRegistryHandler *PackageRegistryHandler) Close() error {
	return packageRegistryHandler.lockedFile.Close()
}

func (packageRegistryHandler *PackageRegistryHandler) GetExistingPackages() (CurrentPackageRegistry, error) {
	var currentPackageRegistry CurrentPackageRegistry
	currentPackageRegistry = nil
	localApplicationRegistryFilePath := packageRegistryHandler.getLocalApplicationRegistryFilePath()

	vmAppPackageCurrentCollection := VMAppPackageCurrentCollection{}
	_, err := os.Stat(localApplicationRegistryFilePath)
	if err == nil {
		// The file exists
		fileBytes, err := ioutil.ReadFile(localApplicationRegistryFilePath)
		if err != nil {
			return currentPackageRegistry, err
		}

		if len(fileBytes) > 0 {
			err = json.Unmarshal(fileBytes, &vmAppPackageCurrentCollection)
			if err != nil {
				return currentPackageRegistry, err
			}
		}
	} else if !os.IsNotExist(err) {
		return currentPackageRegistry, err
	}

	currentPackageRegistry = make(CurrentPackageRegistry)
	err = currentPackageRegistry.Populate(vmAppPackageCurrentCollection)
	return currentPackageRegistry, err
}

func (packageRegistryHandler *PackageRegistryHandler) WriteToDisk(packageRegistry CurrentPackageRegistry) error {
	regFile := packageRegistryHandler.getLocalApplicationRegistryFilePath()
	regFileBackup := packageRegistryHandler.getLocalApplicationRegistryBackupFilePath()
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

func (packageRegistryHandler *PackageRegistryHandler) getLocalApplicationRegistryFilePath() string {
	return path.Join(packageRegistryHandler.handlerEnv.ConfigFolder, LocalApplicationRegistryFileName)
}

func (packageRegistryHandler *PackageRegistryHandler) getLocalApplicationRegistryBackupFilePath() string {
	return path.Join(packageRegistryHandler.handlerEnv.ConfigFolder, localApplicationRegistryBackupFileName)
}
