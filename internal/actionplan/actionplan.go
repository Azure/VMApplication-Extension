package actionplan

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)

type action struct {
	// vmAppPackage is not a pointer type on purpose, we don't want it to be mutated
	vmAppPackage                    packageregistry.VMAppPackageCurrent
	treatFailureAsDeploymentFailure bool
	actionToPerform                 packageregistry.ActionEnum
}

const Success = "SUCCESS"
const Failed = "FAILED"

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

type IResult interface {
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
	return
}

type PackageOperationResult struct {
	PackageName                     string `json:"package"`
	AppVersion                      string `json:"version"`
	Operation                       string `json:"operation"`
	Result                          string `json:"result"`
	TreatFailureAsDeploymentFailure bool   `json:"treatFailureAsDeploymentFailure"`
	Timestamp                       string `json:"timestamp"`
}

func appendExecutionResult(executionResult *PackageOperationResults, act *action, err error) {
	if err == nil {
		*executionResult = append(*executionResult, PackageOperationResult{PackageName: act.vmAppPackage.ApplicationName, AppVersion: act.vmAppPackage.Version, Operation: act.actionToPerform.ToString(), Result: Success, TreatFailureAsDeploymentFailure: act.treatFailureAsDeploymentFailure})
	} else {
		*executionResult = append(*executionResult, PackageOperationResult{PackageName: act.vmAppPackage.ApplicationName, AppVersion: act.vmAppPackage.Version, Operation: act.actionToPerform.ToString(), Result: err.Error(), TreatFailureAsDeploymentFailure: act.treatFailureAsDeploymentFailure})
	}
}

func appendExecutionResultExplicit(executionResult *PackageOperationResults, act *action, result string) {
	*executionResult = append(*executionResult, PackageOperationResult{PackageName: act.vmAppPackage.ApplicationName, AppVersion: act.vmAppPackage.Version, Operation: act.actionToPerform.ToString(), Result: result, TreatFailureAsDeploymentFailure: act.treatFailureAsDeploymentFailure})
}

type failedDeploymentError struct {
	appsWithTreatFailureAsDeploymentFailure []string
	additionalErrorForContext               error
}

func (err *failedDeploymentError) Error() string {
	stringBuilder := strings.Builder{}
	stringBuilder.WriteString("Install/Update failed for VMApps with 'TreatFailureAsDeploymentFailure' set to true:" + constants.NewLineCharacter)
	stringBuilder.WriteString(strings.Join(err.appsWithTreatFailureAsDeploymentFailure, constants.NewLineCharacter))
	stringBuilder.WriteString(constants.NewLineCharacter)
	if err.additionalErrorForContext != nil {
		// TODO: limit the length of all the errors
		stringBuilder.WriteString(fmt.Sprintf("Additional errors: %s%s", err.additionalErrorForContext.Error(), constants.NewLineCharacter))
	}
	return stringBuilder.String()
}

func updateFailDeploymentError(failDeploymentError *failedDeploymentError, act *action, singleExecutionError error) *failedDeploymentError {
	if singleExecutionError != nil && act.treatFailureAsDeploymentFailure && (act.actionToPerform == packageregistry.Install || act.actionToPerform == packageregistry.Update) {
		if failDeploymentError == nil {
			failDeploymentError = &failedDeploymentError{appsWithTreatFailureAsDeploymentFailure: []string{}}
		}
		failDeploymentError.appsWithTreatFailureAsDeploymentFailure = append(failDeploymentError.appsWithTreatFailureAsDeploymentFailure, act.vmAppPackage.ApplicationName)
	}
	return failDeploymentError
}

type ExecuteError struct {
	failedDeploymentErr   *failedDeploymentError
	combinedExecuteErrors error
}

func (executeError *ExecuteError) GetCombinedExecuteError() error {
	return executeError.combinedExecuteErrors
}

func (executeError *ExecuteError) SetFailedDeploymentErr(err *failedDeploymentError) {
	executeError.failedDeploymentErr = err
}

func (executeError *ExecuteError) SetCombinedExecuteErrors(errs error) {
	executeError.combinedExecuteErrors = errs
}

func (exeucuteError *ExecuteError) update(act *action, singleExecutionError error) {
	exeucuteError.failedDeploymentErr = updateFailDeploymentError(exeucuteError.failedDeploymentErr, act, singleExecutionError)
	exeucuteError.combinedExecuteErrors = extensionerrors.CombineErrors(exeucuteError.combinedExecuteErrors, singleExecutionError)
}

func (exeucuteError *ExecuteError) GetErrorIfDeploymentFailed() error {
	if exeucuteError.failedDeploymentErr == nil {
		return nil
	}
	exeucuteError.failedDeploymentErr.additionalErrorForContext = exeucuteError.combinedExecuteErrors
	return exeucuteError.failedDeploymentErr
}

func New(currentPackageRegistry packageregistry.CurrentPackageRegistry, desiredVMAppCollection packageregistry.VMAppPackageIncomingCollection, environment *handlerenv.HandlerEnvironment, hostGaCommunicator hostgacommunicator.IHostGaCommunicator, logger *logging.ExtensionLogger) *ActionPlan {

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
		_, existsInNewConfiguration := packageRegistryIncoming[vmAppCurrent.ApplicationName]
		if !existsInNewConfiguration {
			if vmAppCurrent.OngoingOperation == packageregistry.Skipped {
				// remove the package without from the registry without calling the remove command
				logger.Info("Application %v not in incoming package collection. Cleaning up data for previously skipped installation.", vmAppCurrent.ApplicationName)
				deleteAction := &action{*vmAppCurrent, false, packageregistry.Cleanup}
				actionPlan.unorderedImplicitUninstalls = append(actionPlan.unorderedImplicitUninstalls, deleteAction)
			} else {
				logger.Info("Application %v not in incoming package collection. Marking to delete.", vmAppCurrent.ApplicationName)
				deleteAction := &action{*vmAppCurrent, false, packageregistry.Remove}
				actionPlan.unorderedImplicitUninstalls = append(actionPlan.unorderedImplicitUninstalls, deleteAction)
			}
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
					logger.Info("Application %v has version %v, but %v is desired. No update command exists, so removing and adding",
						vmAppCurrent.ApplicationName, vmAppCurrent.Version, vmAppIncoming.Version)
					deleteAction := &action{*vmAppCurrent, false, packageregistry.RemoveForUpdate} // delete current and install incoming
					installAction := &action{*packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), vmAppIncoming.TreatFailureAsDeploymentFailure, packageregistry.Install}
					actionPlan.insertOperation(vmAppIncoming.Order, deleteAction, installAction)
				} else {
					logger.Info("Application %v has version %v, but %v is desired. Will call update.",
						vmAppCurrent.ApplicationName, vmAppCurrent.Version, vmAppIncoming.Version)
					updateAction := &action{*packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), vmAppIncoming.TreatFailureAsDeploymentFailure, packageregistry.Update}
					actionPlan.insertOperation(vmAppIncoming.Order, updateAction)
				}
			} else if vmAppCurrent.NumRebootsOccurred > 0 {
				logger.Info("Application %v with version %v already exists on system, but previous %v operation resulted in a reboot. Retrying operation because rerunAfterReboot is set.",
					vmAppCurrent.ApplicationName, vmAppCurrent.Version, vmAppCurrent.OngoingOperation.ToString())
				// Pass in vmAppCurrent instead of vmAppIncoming since exact version already exists in registry and contains the number of reboots occurred so far
				actionAfterReboot := &action{*vmAppCurrent, vmAppIncoming.TreatFailureAsDeploymentFailure, vmAppCurrent.OngoingOperation}
				actionPlan.insertOperation(vmAppIncoming.Order, actionAfterReboot)
			}
		} else {
			// installs
			logger.Info("Application %v does not exist on the system. Installing", vmAppIncoming.ApplicationName)
			installAction := &action{*packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), vmAppIncoming.TreatFailureAsDeploymentFailure, packageregistry.Install}
			actionPlan.insertOperation(vmAppIncoming.Order, installAction)
		}
	}

	sort.Ints(actionPlan.sortedOrder)

	return actionPlan
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

func (actionPlan *ActionPlan) Execute(registryHandler packageregistry.IPackageRegistry, eem *extensionevents.ExtensionEventManager, commandHandler commandhandler.ICommandHandler) (*ExecuteError, IResult) {
	var executeError *ExecuteError = &ExecuteError{failedDeploymentErr: nil, combinedExecuteErrors: nil}
	// registry will be mutated and written to disk so that we can keep track of all the actions that have happened
	registry, err := registryHandler.GetExistingPackages()
	if err != nil {
		return executeError, StatusErrorMessage(err.Error())
	}
	executionResult := make(PackageOperationResults, 0)

	// handle unordered implicit uninstalls
	for _, act := range actionPlan.unorderedImplicitUninstalls {
		newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act, eem)
		appendExecutionResult(&executionResult, act, newError)
		executeError.update(act, newError)
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
					actionPlan.logger.Warn("Skipping application %v because a previous application failed", act.vmAppPackage.ApplicationName)
					appName := act.vmAppPackage.ApplicationName
					currentVmApp := act.vmAppPackage
					registry[appName] = &currentVmApp
					currentVmApp.OngoingOperation = packageregistry.Skipped
					currentVmApp.Result = "skipped, lower order operation failed"

					appendExecutionResultExplicit(&executionResult, act, "Skipped, lower order operation failed")

					err = registryHandler.WriteToDisk(registry)
					if err != nil {
						executeError.combinedExecuteErrors = extensionerrors.CombineErrors(executeError.combinedExecuteErrors, err)
						return executeError, &executionResult
					}
					break
				}

				newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act, eem)
				executeError.update(act, newError)
				appendExecutionResult(&executionResult, act, newError)

				if newError != nil {
					actionPlan.logger.Warn("Application %v failed. Skipping the remaining applications", act.vmAppPackage.ApplicationName)
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
			executeError.update(act, newError)
			appendExecutionResult(&executionResult, act, newError)

			if newError != nil {
				actionPlan.logger.Warn("Application %v failed. Skipping the remaining applications", act.vmAppPackage.ApplicationName)
				break // will skip the remaining dependant actions
			}
		}
	}
	return executeError, &executionResult
}
