package main

import (
	"time"
)

const (
	localApplicationRegistryFileName                         = "applicationRegistry"
	localApplicationRegistryBackupFileName                   = "applicationRegistry.backup"
	activeChangesFileName                                    = "applicationRegistry.active"
	localApplicationRegistryFileDefaultTimeout time.Duration = 30 * time.Minute
)

// defines a map between the application name and the other properties of the application
type PackageRegistry map[string]VMAppsPackage

type VMAppsPackages []VMAppsPackage

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
	WriteToDisk(packageRegistry *PackageRegistry) (error)
}



