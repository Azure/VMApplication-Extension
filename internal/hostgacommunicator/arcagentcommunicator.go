// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package hostgacommunicator

import (
	"os"
	"runtime"
	"sync"

	"github.com/Azure/azure-extension-platform/pkg/logging"
)

const (
	arcAgentWindowsPath = `C:\Program Files\AzureConnectedMachineAgent\himds.exe`
	arcAgentLinuxPath   = "/opt/azcmagent/bin/himds"
	arcFallbackEndpoint = "http://localhost:40342"
	arcImdsEnv          = "IMDS_ENDPOINT"
)

var (
	arcDetectOnce    sync.Once
	cachedArcPresent bool
)

// isArcAgentPresent performs a one-time detection of the Arc agent presence.
// Subsequent calls return the cached value to avoid repeated filesystem stats.
// This assumes Arc agent presence won't change during the lifetime of the
// extension process (which is a safe assumption for performance optimization).
func isArcAgentPresent(el *logging.ExtensionLogger) bool {
	arcDetectOnce.Do(func() {
		cachedArcPresent = isArcAgentPresentWithPaths(el, arcAgentWindowsPath, arcAgentLinuxPath)
		if cachedArcPresent {
			el.Info("Arc environment detection cached: present=true")
		} else {
			el.Info("Arc environment detection cached: present=false")
		}
	})
	return cachedArcPresent
}

// isArcAgentPresentWithPaths checks for the presence of Arc agent with configurable paths (for testing)
func isArcAgentPresentWithPaths(el *logging.ExtensionLogger, windowsPath, linuxPath string) bool {
	var arcAgentPath string

	switch runtime.GOOS {
	case "windows":
		arcAgentPath = windowsPath
	case "linux":
		arcAgentPath = linuxPath
	default:
		el.Warn("Unsupported OS: %s, assuming Arc agent not present", runtime.GOOS)
		return false
	}

	el.Info("Checking for Arc agent at: %s", arcAgentPath)

	if _, err := os.Stat(arcAgentPath); err == nil {
		el.Info("Arc agent found at: %s", arcAgentPath)
		return true
	} else if os.IsNotExist(err) {
		el.Info("Arc agent not found at: %s", arcAgentPath)
		return false
	} else {
		el.Warn("Error checking for Arc agent at %s: %v", arcAgentPath, err)
		return false
	}
}

// getArcEndpoint returns the Arc IMDS endpoint, falling back to default if not set - assumes that arc agent already exists
func getArcEndpoint(el *logging.ExtensionLogger) string {
	arcEndpoint := os.Getenv(arcImdsEnv)
	if arcEndpoint == "" {
		el.Info("%s not set, using fallback Arc endpoint %s", arcImdsEnv, arcFallbackEndpoint)
		return arcFallbackEndpoint
	}
	return arcEndpoint
}
