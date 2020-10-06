package actionplan

import (
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/commandhandler"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/pkg/errors"
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
	environment      *vmextensionhelper.HandlerEnvironment
	unorderedActions []dependentActions
	// we need to skip the actions that have a higher order number if applications in the lower order numbers fail
	// this data structure helps us achieve it
	orderedActions map[int][]dependentActions
	// we keep the user provided orders sorted order to look up the orderedActions map
	// remember to sort while initializing
	sortedOrder                 []int
	unorderedImplicitUninstalls []*action
}

func New(currentPackageRegistry packageregistry.CurrentPackageRegistry, desiredVMAppCollection packageregistry.VMAppPackageIncomingCollection, environment *vmextensionhelper.HandlerEnvironment) (*ActionPlan, error) {

	actionPlan := &ActionPlan{
		environment:                 environment,
		unorderedActions:            make([]dependentActions, 0),
		orderedActions:              make(map[int][]dependentActions),
		sortedOrder:                 make([]int, 0),
		unorderedImplicitUninstalls: make([]*action, 0),
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
			versionComparison, err := utils.CompareVersion(&vmAppCurrent.Version, &vmAppIncoming.Version)
			if err != nil {
				return nil, err
			}
			if versionComparison != 0 {
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
		actionPlan.unorderedActions = append(actionPlan.unorderedActions, dependentActions1)
	} else {
		orderedActions, present := actionPlan.orderedActions[*order]
		if present {
			orderedActions = append(orderedActions, dependentActions1)
		} else {
			actionPlan.orderedActions[*order] = []dependentActions{dependentActions1}
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
		combinedErrors = combineErrors(combinedErrors, newError)
	}

	// handle ordered operations
	var atLeastOneActionFailed bool
	var actionFailedAtOrder int
	for _, order := range actionPlan.sortedOrder {
		actionsInTheSameOrder := actionPlan.orderedActions[order]
		for _, depActions := range actionsInTheSameOrder {
			depActionFailed := false
			for _, act := range depActions {
				regKey := act.vmAppPackage.ApplicationName

				if depActionFailed || (atLeastOneActionFailed && order > actionFailedAtOrder) {
					registry[regKey].OngoingOperation = packageregistry.Skipped
					err = registryHandler.WriteToDisk(registry)
					if err != nil {
						combinedErrors = combineErrors(combinedErrors, err)
						return combinedErrors
					}

					continue // it wil skip the remaining dependant actions and other ordered actions that have a higher order
				}

				newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act)

				if newError != nil {
					atLeastOneActionFailed = true
					depActionFailed = true
					actionFailedAtOrder = order
				}
				combinedErrors = combineErrors(combinedErrors, newError)
			}
		}
	}

	// handle remaining unordered operations
	for _, depActions := range actionPlan.unorderedActions {
		depActionFailed := false
		for _, act := range depActions {
			regKey := act.vmAppPackage.ApplicationName

			if depActionFailed {
				registry[regKey].OngoingOperation = packageregistry.Skipped
				err = registryHandler.WriteToDisk(registry)
				if err != nil {
					combinedErrors = combineErrors(combinedErrors, err)
					return combinedErrors
				}
				continue // it wil skip the remaining dependant actions
			}
			newError := actionPlan.executeHelper(registryHandler, commandHandler, registry, act)

			if newError != nil {
				depActionFailed = true
			}
			combinedErrors = combineErrors(combinedErrors, newError)
		}
	}
	return combinedErrors
}

func combineErrors(combinedErrors error, error1 error) (error) {
	if error1 != nil {
		if combinedErrors != nil {
			combinedErrors = errors.Wrap(combinedErrors, error1.Error())
		} else {
			combinedErrors = error1
		}
	}
	return combinedErrors
}

func (actionPlan *ActionPlan) executeHelper(registryHandler packageregistry.IPackageRegistry,
	commandHandler commandhandler.ICommandHandler, registry packageregistry.CurrentPackageRegistry,
	act *action) (errorMessageToReturn error) {
	errorMessageToReturn = nil
	regKey := act.vmAppPackage.ApplicationName

	// record new operation in the packageRegistry
	registry[regKey] = act.vmAppPackage
	registry[regKey].OngoingOperation = act.actionToPerform
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
		retCode, err := commandHandler.Execute(commandToExecute, act.vmAppPackage.GetWorkingDirectory(actionPlan.environment))
		if err != nil {
			errorMessageToReturn = errors.Wrapf(err, "Error executing command %v", commandToExecute)
		}
		if retCode != 0 {
			errorMessageToReturn = errors.Errorf("Command %v exited with non-zero error code", commandToExecute)
		}
	}

	if errorMessageToReturn != nil {
		registry[regKey].OngoingOperation = packageregistry.Failed
	} else {
		if isDeleteOperation {
			delete(registry, regKey)
		} else {
			registry[regKey].OngoingOperation = packageregistry.NoAction
		}
	}
	err = registryHandler.WriteToDisk(registry)
	if err != nil {
		return err
	}
	return errorMessageToReturn
}
