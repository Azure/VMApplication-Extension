package main

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/status"
	"github.com/stretchr/testify/assert"
)

var vmAppCurrentCollection = packageregistry.VMAppPackageCurrentCollection{
	&packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "0.1.1",
		OngoingOperation: packageregistry.NoAction,
		Result:           "install succeeded",
	},
	&packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app2",
		Version:          "1.1.0",
		OngoingOperation: packageregistry.Failed,
		Result:           "update failed",
	},
}

type criticalErrorString string

func (c criticalErrorString) ToJsonString() string {
	return string(c)
}

func TestGetStatusMessageWithPackageOperationResults(t *testing.T) {
	actionsPerformed := actionplan.PackageOperationResults{
		actionplan.PackageOperationResult{
			PackageName: "app1",
			AppVersion:  "0.1.1",
			Operation:   "Install",
			Result:      actionplan.Success,
		},
	}
	statusMessage, _ := getStatusMessageAndError(vmAppCurrentCollection, &actionsPerformed)
	statusMessage1 := new(StatusMessageWithPackageOperationResults)
	err := json.Unmarshal([]byte(statusMessage), statusMessage1)
	assert.NoError(t, err)
	assertCollectionsMatch(t, vmAppCurrentCollection, statusMessage1.CurrentState)
	assert.EqualValues(t, actionsPerformed, statusMessage1.ActionsPerformed)
}

func TestGetStatusMessageWithCriticalError(t *testing.T) {
	var ce criticalErrorString = "critical error"
	statusMessage, statusError := getStatusMessageAndError(vmAppCurrentCollection, &ce)
	statusMessage2 := new(StatusMessageWithCriticalError)
	err := json.Unmarshal([]byte(statusMessage), statusMessage2)
	assert.NoError(t, err)
	assertCollectionsMatch(t, vmAppCurrentCollection, statusMessage2.CurrentState)
	assert.EqualValues(t, ce, statusMessage2.CriticalError)
	assert.EqualValues(t, ce, statusError.Error())
}

func TestStatusFormatter(t *testing.T) {
	actionsPerformed := actionplan.PackageOperationResults{
		actionplan.PackageOperationResult{
			PackageName: "app1",
			AppVersion:  "0.1.1",
			Operation:   "Install",
			Result:      actionplan.Success,
		},
	}
	statusMessage, _ := getStatusMessageAndError(vmAppCurrentCollection, &actionsPerformed)
	// assert that status message is a well fromed json
	statusMessage = statusMessageFormatter("enable", status.StatusSuccess, statusMessage)
	statusMessageInstance := new(StatusMessageWithPackageOperationResults)
	assert.NoError(t, json.Unmarshal([]byte(statusMessage), statusMessageInstance), "sould be able to parse status message")
}

func TestGetStatusMessageTruncatesStringsOver5KB(t *testing.T) {
	messageLength := fiveKilo + 100
	ce := criticalErrorString(generateRandomStringOfLength(messageLength))
	statusMessage, _ := getStatusMessageAndError(vmAppCurrentCollection, &ce)
	assert.Greater(t, len(ce), fiveKilo, "critical error string length should be greater than 5 kb")
	assert.Equal(t, fiveKilo, len(statusMessage), "statusMessage string should be less than 5 kb")
}

func assertCollectionsMatch(t *testing.T, vmAppCurrentCollection packageregistry.VMAppPackageCurrentCollection, vmAppCurrentForStatusCollection VmAppPackageCurrentForStatusCollection) {
	assert.Equal(t, len(vmAppCurrentCollection), len(vmAppCurrentForStatusCollection), "lengths should match")
	for i, vmAppCurrent := range vmAppCurrentCollection {
		vmAppStatus := vmAppCurrentForStatusCollection[i]
		assert.Equal(t, vmAppCurrent.ApplicationName, vmAppStatus.ApplicationName)
		assert.Equal(t, vmAppCurrent.Version, vmAppStatus.Version)
		assert.Equal(t, vmAppCurrent.Result, vmAppStatus.Result)
	}
}

func generateRandomStringOfLength(length int) string {
	charset := "abcdABCD1234"
	charsetLength := len(charset)
	sb := strings.Builder{}
	for i := 0; i < length; i++ {
		randChar := charset[rand.Intn(charsetLength)]
		sb.WriteString(string(randChar))
	}
	return sb.String()
}
