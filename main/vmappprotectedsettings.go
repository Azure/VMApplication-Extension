package main

import (
	"encoding/json"
	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/azure-extension-platform/pkg/settings"
)

type VmAppProtectedSettings []*extdeserialization.VmAppSetting

func getVMAppProtectedSettings(settings *settings.HandlerSettings) (VmAppProtectedSettings, error) {
	vmAppProtectedSettings := VmAppProtectedSettings{}
	err := json.Unmarshal([]byte(settings.ProtectedSettings), &vmAppProtectedSettings)
	if err != nil {
		return nil, err
	}
	return vmAppProtectedSettings, err
}
