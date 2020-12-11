package actionplan

import (
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/commandhandler"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"math"
	"sort"
)

type action struct {
	vmAppPackage    *packageregistry.VMAppPackageCurrent
	actionToPerform packageregistry.ActionEnum
}

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
	ctx                         log.Logger
}

func New(currentPackageRegistry packageregistry.CurrentPackageRegistry, desiredVMAppCollection packageregistry.VMAppPackageIncomingCollection, environment *handlerenv.HandlerEnvironment, hostGaCommunicator hostgacommunicator.IHostGaCommunicator, ctx log.Logger) (*ActionPlan, error) {

	actionPlan := &ActionPlan{
		environment:                 environment,
		unorderedOperations:         make([]dependentActions, 0),
		orderedOperations:           make(map[int][]dependentActions),
		sortedOrder:                 make([]int, 0),
		unorderedImplicitUninstalls: make([]*action, 0),
		hostGaCommunicator:          hostGaCommunicator,
		ctx:                         ctx,
	}

	// get list of previously existing applications that don't exist in the new configuration
	packageRegistryIncoming := make(packageregistry.DesiredPackageRegistry)
	packageRegistryIncoming.Populate(desiredVMAppCollection)
	vmAppCurrentCollection := currentPackageRegistry.GetPackageCollection()
	for _, vmAppCurrent := range vmAppCurrentCollection {
		_, exists := packageRegistryIncoming[vmAppCurrent.ApplicationName]
		if !exists {
			deleteAction := &action{vmAppCurrent, packageregistry.Delete}
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
					deleteAction := &action{vmAppCurrent, packageregistry.Delete} // delete current and install incoming
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

func (actionPlan *ActionPlan) Execute(registryHandler packageregistry.IPackageRegistry, commandHandler commandhandler.ICommandHandler) error {
	// registry will be mutated and written to disk so that we can keep track of all the actions that have happened
	registry, err := registryHandler.GetExistingPackages()
	if err != nil {
		return err
	}

	var combinedErrors error = nil

	// handle unordered implicit uninstalls
	for _, act := range actionPlan.unorderedImplicitUninstalls {
		newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act)
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

					err = registryHandler.WriteToDisk(registry)
					if err != nil {
						combinedErrors = extensionerrors.CombineErrors(combinedErrors, err)
						return combinedErrors
					}
					break
				}

				newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act)
				combinedErrors = extensionerrors.CombineErrors(combinedErrors, newError)

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
			newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act)
			combinedErrors = extensionerrors.CombineErrors(combinedErrors, newError)

			if newError != nil {
				break // will skip the remaining dependant actions
			}
		}
	}
	return combinedErrors
}

func (actionPlan *ActionPlan) executeHelper(registryHandler packageregistry.IPackageRegistry,
	commandHandler commandhandler.ICommandHandler, registry packageregistry.CurrentPackageRegistry,
	act *action) (errorMessageToReturn error) {
	errorMessageToReturn = nil
	appName := act.vmAppPackage.ApplicationName

	// record new operation in the packageRegistry
	registry[appName] = act.vmAppPackage
	err := registryHandler.WriteToDisk(registry)
	if err != nil {
		return err
	}

	var commandToExecute string
	var isDeleteOperation = false
	switch act.actionToPerform {
	case packageregistry.Install:
		commandToExecute = act.vmAppPackage.InstallCommand
	case packageregistry.Delete:
		isDeleteOperation = true
		commandToExecute = act.vmAppPackage.RemoveCommand
	case packageregistry.Update:
		commandToExecute = act.vmAppPackage.UpdateCommand
	default:
		errorMessageToReturn = errors.Errorf("Unexpected Action to perform encountered %v", act.actionToPerform)
	}

	// try to execute only if you have a valid command to execute
	if errorMessageToReturn == nil {
		downloadPath := act.vmAppPackage.GetWorkingDirectory(actionPlan.environment)

		// download packages now
		if err := actionPlan.hostGaCommunicator.DownloadPackage(actionPlan.ctx, act.vmAppPackage.ApplicationName, downloadPath); err != nil {
			return err
		}

		// download configuration
		if err := actionPlan.hostGaCommunicator.DownloadConfig(actionPlan.ctx, act.vmAppPackage.ApplicationName, downloadPath); err != nil {
			return err
		}

		retCode, err := commandHandler.Execute(commandToExecute, downloadPath)
		if err != nil {
			errorMessageToReturn = errors.Wrapf(err, "Error executing command %v", commandToExecute)
		}
		if retCode != 0 {
			errorMessageToReturn = errors.Errorf("Command %v exited with non-zero error code", commandToExecute)
		}
	}

	if errorMessageToReturn != nil {
		registry[appName].OngoingOperation = packageregistry.Failed
	} else {
		if isDeleteOperation {
			delete(registry, appName)
		} else {
			registry[appName].OngoingOperation = packageregistry.NoAction
		}
	}
	err = registryHandler.WriteToDisk(registry)
	if err != nil {
		return err
	}
	return errorMessageToReturn
}
