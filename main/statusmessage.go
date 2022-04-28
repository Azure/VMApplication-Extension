package main

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
)

const fiveKilo = 5 * 1024

type VmAppPackageCurrentForStatusCollection []*VmAppPackageCurrentForStatus

type VmAppPackageCurrentForStatus struct {
	ApplicationName string `json:"applicationName"`
	Version         string `json:"version"`
	Result          string `json:"result"`
}

type StatusMessage1 struct {
	CurrentState     VmAppPackageCurrentForStatusCollection `json:"CurrentState"`
	ActionsPerformed actionplan.PackageOperationResults     `json:"ActionsPerformed"`
}

type StatusMessage2 struct {
	CurrentState  VmAppPackageCurrentForStatusCollection `json:"CurrentState"`
	CriticalError string                                 `json:"CriticalError"`
}

func getVmAppCurrentForStatus(vmAppCurrentCollection packageregistry.VMAppPackageCurrentCollection) VmAppPackageCurrentForStatusCollection {
	vmAppCurrentForStatusCollection := make(VmAppPackageCurrentForStatusCollection, 0)
	for _, vmApp := range vmAppCurrentCollection {
		vmAppCurrentForStatusCollection = append(vmAppCurrentForStatusCollection, &VmAppPackageCurrentForStatus{
			ApplicationName: vmApp.ApplicationName,
			Version:         vmApp.Version,
			Result:          vmApp.Result,
		})
	}
	return vmAppCurrentForStatusCollection
}

func getStatusMessage(vmAppCurrentCollection packageregistry.VMAppPackageCurrentCollection, result actionplan.IResult) string {
	vmAppCurrentForStatusCollection := getVmAppCurrentForStatus(vmAppCurrentCollection)
	packageOperationResults, ok := result.(*actionplan.PackageOperationResults)
	var statusMessageString string

	if ok {
		statusMessageA := StatusMessage1{
			CurrentState:     vmAppCurrentForStatusCollection,
			ActionsPerformed: *packageOperationResults,
		}

		statusM, err := json.MarshalIndent(statusMessageA, "", " ")
		if err != nil {
			statusMessageString = fmt.Sprintf("%v", statusMessageA)
		} else {
			statusMessageString = string(statusM)
		}

	} else {
		statusMessage := StatusMessage2{
			CurrentState:  vmAppCurrentForStatusCollection,
			CriticalError: result.ToJsonString(),
		}
		statusM, err := json.MarshalIndent(statusMessage, "", " ")
		if err != nil {
			statusMessageString = fmt.Sprintf("%v", statusMessage)
		} else {
			statusMessageString = string(statusM)
		}
	}

	if len(statusMessageString) > fiveKilo {
		statusMessageString = statusMessageString[:fiveKilo]
	}

	return statusMessageString
}
