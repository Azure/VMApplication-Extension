package extdeserialization

import (
	"encoding/json"

	"github.com/Azure/azure-extension-platform/pkg/settings"
)

type ActionParameter struct {
	ParameterName  string `json:"name"`
	ParameterValue string `json:"value"`
}

type ActionSetting struct {
	ActionName   string            `json:"name"`
	ActionScript string            `json:"script"`
	Timestamp    string            `json:"timestamp"`
	Parameters   []ActionParameter `json:"parameters"`
	TickCount    uint64            `json:"tickCount"`
}

type VmAppProtectedSettings []*VmAppSetting
type VmAppSetting struct {
	ApplicationName                 string           `json:"applicationName"`
	Order                           *int             `json:"order"`
	TreatFailureAsDeploymentFailure bool             `json:"treatFailureAsDeploymentFailure"`
	Actions                         []*ActionSetting `json:"actions"`
	Version                         string           `json:"version"`
}

func GetParameterNames(settings ActionSetting) []string {
	var names []string
	for _, param := range settings.Parameters {
		names = append(names, param.ParameterName)
	}
	return names
}

func GetVMAppProtectedSettings(settings *settings.HandlerSettings) (VmAppProtectedSettings, error) {
	vmAppProtectedSettings := VmAppProtectedSettings{}
	err := json.Unmarshal([]byte(settings.ProtectedSettings), &vmAppProtectedSettings)
	if err != nil {
		return nil, err
	}
	return vmAppProtectedSettings, err
}
