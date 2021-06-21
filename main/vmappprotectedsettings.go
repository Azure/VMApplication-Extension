package main

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
	ActionScript string            `json:"actionScript"`
	Timestamp    string            `json:"timestamp"`
	Parameters   []ActionParameter `json:"parameters"`
	TickCount    uint64            `json:"tickCount"`
}

type ActionSettingCollection []*ActionSetting

type VmAppSetting struct {
	ApplicationName string          `json:"applicationName"`
	Order           *int            `json:"order"`
	Actions         ActionSettingCollection `json:"actions"`
}

type VmAppProtectedSettings []*VmAppSetting

func getVMAppProtectedSettings(settings *settings.HandlerSettings) (VmAppProtectedSettings, error) {
	vmAppProtectedSettings := VmAppProtectedSettings{}
	err := json.Unmarshal([]byte(settings.ProtectedSettings), &vmAppProtectedSettings)
	if err != nil {
		return nil, err
	}
	return vmAppProtectedSettings, err
}


func (a ActionSettingCollection) Len() int {
	return len(a)
}

func (a ActionSettingCollection) Less(i, j int) bool {
	return a[i].TickCount > a[j].TickCount
}

func (a ActionSettingCollection) Swap (i,j int) {
	a[i], a[j] = a[j], a[i]
}