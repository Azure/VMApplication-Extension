package service

// Provides configurable parameters to a service
type ServiceConfig struct {
	Name        string // Required
	DisplayName string
	Description string
	Arguments   []string
	Executable  string // Required for Windows, not supported on Linux
	UnitContent string // Required for Linux, not supported on Windows

	EnvVars map[string]string
}

// Represents the service to be run or controlled
type Service interface {
	Install() error
	Uninstall() error
	Start() error
	Stop() error
	IsInstalled() bool
	IsRunning() bool
}

// Represents the service manager that is available on the given system
type ServiceManager interface {
	String() string
	DetectIsAvailable() bool
	New(c *ServiceConfig) (Service, error) // Creates a new service on the system
}
