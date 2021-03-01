package actionplan

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/commandhandler"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)

type action struct {
	vmAppPackage    *packageregistry.VMAppPackageCurrent
	actionToPerform packageregistry.ActionEnum
}

const fiveKilo = 5 * 1024
const Success = "SUCCESS"

// when an update requires an implicit remove and uninstall, the install is dependent on the remove
// this data structure helps us skip the install if the remove fails
type dependentActions []*action

type ActionPlan struct {
	environment         *handlerenv.HandlerEnvironment
	unorderedOperations []dependentActions
	// we need to skip the actions that have a higher order number if applications in the lower order numbers fail
	// this data structure helps us achieve it
	orderedOperations map[int][]dependentActions
	// we keep the user provided orders sorted order to look up the orderedOperations map
	// remember to sort while initializing
	sortedOrder                 []int
	unorderedImplicitUninstalls []*action
	hostGaCommunicator          hostgacommunicator.IHostGaCommunicator
	logger                      *logging.ExtensionLogger
}

type IStatusMessage interface {
	ToJsonString() string
}

type StatusErrorMessage string

func (err StatusErrorMessage) ToJsonString() string {
	return string(err)
}

type PackageOperationResults []PackageOperationResult

func (packageOperationResults *PackageOperationResults) ToJsonString() (message string) {
	jsonBytes, err := json.Marshal(packageOperationResults)
	if err != nil {
		message = fmt.Sprintf("%v", packageOperationResults)
	} else {
		message = string(jsonBytes)
	}
	if len(message) > fiveKilo {
		// keep the message smaller than 5KB
		message = string(message[0 : fiveKilo-1])
	}
	return
}

type PackageOperationResult struct {
	PackageName string `json:"package"`
	AppVersion  string `json:"version"`
	Operation   string `json:"operation"`
	Result      string `json:"result"`
}

func appendExecutionResult(executionResult *PackageOperationResults, act *action, err error) {
	if err == nil {
		*executionResult = append(*executionResult, PackageOperationResult{PackageName: act.vmAppPackage.ApplicationName, AppVersion: act.vmAppPackage.Version, Operation: act.actionToPerform.ToString(), Result: Success})
	} else {
		*executionResult = append(*executionResult, PackageOperationResult{PackageName: act.vmAppPackage.ApplicationName, AppVersion: act.vmAppPackage.Version, Operation: act.actionToPerform.ToString(), Result: err.Error()})
	}
}

func appendExecutionResultExplicit(executionResult *PackageOperationResults, act *action, result string) {
	*executionResult = append(*executionResult, PackageOperationResult{PackageName: act.vmAppPackage.ApplicationName, AppVersion: act.vmAppPackage.Version, Operation: act.actionToPerform.ToString(), Result: result})
}

func New(currentPackageRegistry packageregistry.CurrentPackageRegistry, desiredVMAppCollection packageregistry.VMAppPackageIncomingCollection, environment *handlerenv.HandlerEnvironment, hostGaCommunicator hostgacommunicator.IHostGaCommunicator, logger *logging.ExtensionLogger) (*ActionPlan, error) {

	actionPlan := &ActionPlan{
		environment:                 environment,
		unorderedOperations:         make([]dependentActions, 0),
		orderedOperations:           make(map[int][]dependentActions),
		sortedOrder:                 make([]int, 0),
		unorderedImplicitUninstalls: make([]*action, 0),
		hostGaCommunicator:          hostGaCommunicator,
		logger:                      logger,
	}

	// get list of previously existing applications that don't exist in the new configuration
	packageRegistryIncoming := make(packageregistry.DesiredPackageRegistry)
	packageRegistryIncoming.Populate(desiredVMAppCollection)
	vmAppCurrentCollection := currentPackageRegistry.GetPackageCollection()
	for _, vmAppCurrent := range vmAppCurrentCollection {
		_, exists := packageRegistryIncoming[vmAppCurrent.ApplicationName]
		if !exists {
			deleteAction := &action{vmAppCurrent, packageregistry.Remove}
			actionPlan.unorderedImplicitUninstalls = append(actionPlan.unorderedImplicitUninstalls, deleteAction)
		}
	}

	// second pass for updates and installs
	for _, vmAppIncoming := range desiredVMAppCollection {
		vmAppCurrent, exists := currentPackageRegistry[vmAppIncoming.ApplicationName]
		if exists {
			// updates
			versionsEqual := utils.AreVersionsEqual(&vmAppCurrent.Version, &vmAppIncoming.Version)
			if !versionsEqual {
				if len(vmAppIncoming.UpdateCommand) == 0 {
					// not the same version and there is no update command
					deleteAction := &action{vmAppCurrent, packageregistry.Remove} // delete current and install incoming
					installAction := &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Install}
					actionPlan.insertOperation(vmAppIncoming.Order, deleteAction, installAction)
				} else {
					updateAction := &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Update}
					actionPlan.insertOperation(vmAppIncoming.Order, updateAction)
				}
			}
		} else {
			// installs
			installAction := &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Install}
			actionPlan.insertOperation(vmAppIncoming.Order, installAction)
		}
	}

	sort.Ints(actionPlan.sortedOrder)

	return actionPlan, nil
}

func (actionPlan *ActionPlan) insertOperation(order *int, dependentActions1 ...*action) {
	if order == nil {
		actionPlan.unorderedOperations = append(actionPlan.unorderedOperations, dependentActions1)
	} else {
		orderedActions, present := actionPlan.orderedOperations[*order]
		if present {
			actionPlan.orderedOperations[*order] = append(orderedActions, dependentActions1)
		} else {
			actionPlan.orderedOperations[*order] = []dependentActions{dependentActions1}
			actionPlan.sortedOrder = append(actionPlan.sortedOrder, *order)
		}
	}
}

func (actionPlan *ActionPlan) Execute(registryHandler packageregistry.IPackageRegistry, eem *extensionevents.ExtensionEventManager, commandHandler commandhandler.ICommandHandler) (error, IStatusMessage) {
	// registry will be mutated and written to disk so that we can keep track of all the actions that have happened
	registry, err := registryHandler.GetExistingPackages()
	if err != nil {
		return err, StatusErrorMessage(err.Error())
	}

	var combinedErrors error = nil
	executionResult := make(PackageOperationResults, 0)

	// handle unordered implicit uninstalls
	for _, act := range actionPlan.unorderedImplicitUninstalls {
		newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act, eem)
		appendExecutionResult(&executionResult, act, newError)
		combinedErrors = extensionerrors.CombineErrors(combinedErrors, newError)
	}

	// handle ordered operations
	var atLeastOneActionFailed = false
	var actionFailedAtOrder = math.MaxInt32
	for _, order := range actionPlan.sortedOrder {
		actionsInTheSameOrder := actionPlan.orderedOperations[order]
		for _, depActions := range actionsInTheSameOrder {
			for _, act := range depActions {
				// if we encountered and error in the past, skip all the operations for a higher order
				if atLeastOneActionFailed && order > actionFailedAtOrder {
					appName := act.vmAppPackage.ApplicationName

					registry[appName] = act.vmAppPackage
					registry[appName].OngoingOperation = packageregistry.Skipped

					appendExecutionResultExplicit(&executionResult, act, "Skipped, lower order operation failed")

					err = registryHandler.WriteToDisk(registry)
					if err != nil {
						combinedErrors = extensionerrors.CombineErrors(combinedErrors, err)
						return combinedErrors, &executionResult
					}
					break
				}

				newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act, eem)
				combinedErrors = extensionerrors.CombineErrors(combinedErrors, newError)
				appendExecutionResult(&executionResult, act, newError)

				if newError != nil {
					atLeastOneActionFailed = true
					actionFailedAtOrder = order
					// no need to execute the remaining dependent operations
					break
				}
			}
		}
	}

	// handle remaining unordered operations
	for _, depActions := range actionPlan.unorderedOperations {
		for _, act := range depActions {
			newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act, eem)
			combinedErrors = extensionerrors.CombineErrors(combinedErrors, newError)
			appendExecutionResult(&executionResult, act, newError)

			if newError != nil {
				break // will skip the remaining dependant actions
			}
		}
	}
	return combinedErrors, &executionResult
}
