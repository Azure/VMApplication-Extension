package actionplan

import (
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/cmd"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/pkg/errors"
	"math"
	"sort"
)

var minInt32 = int(math.MinInt32)

type action struct {
	vmAppPackage    *packageregistry.VMAppPackageCurrent
	actionToPerform packageregistry.ActionEnum
}

type dependentActions []*action

type ActionPlan struct {
	unorderedActions []dependentActions
	orderedActions   map[int][]dependentActions
	sortedOrder      []int
}

func New(currentPackageRegistry packageregistry.CurrentPackageRegistry, desiredVMAppCollection packageregistry.VMAppPackageIncomingCollection) (*ActionPlan, error) {

	actionPlan := &ActionPlan{
		unorderedActions: make([]dependentActions, 0),
		orderedActions:   make(map[int][]dependentActions),
		sortedOrder:      make([]int, 0),
	}

	// get list of previously existing applications that don't exist in the new configuration
	packageRegistryIncoming := make(packageregistry.DesiredPackageRegistry)
	packageRegistryIncoming.Populate(desiredVMAppCollection)
	vmAppCurrentCollection := currentPackageRegistry.GetPackageCollection()
	for _, vmAppCurrent := range vmAppCurrentCollection {
		_, exists := packageRegistryIncoming[vmAppCurrent.ApplicationName]
		if !exists {
			actionToPerform := &action{vmAppCurrent, packageregistry.Delete}
			actionPlan.insert(&minInt32, actionToPerform) // the order suggests that delete operations are performed first
		}
	}

	// the sort happens on the order of the VMAppIncomingPackage
	// this is necessary to maintain the order of operations of packages to install
	sort.Sort(desiredVMAppCollection)

	// second pass for updates and installs
	for _, vmAppIncoming := range desiredVMAppCollection {
		vmAppCurrent, exists := currentPackageRegistry[vmAppIncoming.ApplicationName]
		if exists {
			// updates without explicit update methods still require deletes
			versionComparison, err := utils.CompareVersion(&vmAppCurrent.Version, &vmAppIncoming.Version)
			if err != nil {
				return nil, err
			}
			if versionComparison != 0 {
				if len(vmAppIncoming.UpdateCommand) == 0 {
					// not the same version and there is no update command
					deleteAction := &action{vmAppCurrent, packageregistry.Delete} // delete current and install incoming
					installAction := &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Install}
					actionPlan.insert(vmAppIncoming.Order, deleteAction, installAction)
				} else {
					updateAction :=  &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Update}
					actionPlan.insert(vmAppIncoming.Order, updateAction)
				}
			}
		} else {
			installAction := &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Install}
			actionPlan.insert(vmAppIncoming.Order, installAction)
		}
	}

	sort.Ints(actionPlan.sortedOrder)

	return actionPlan, nil
}

func (actionPlan *ActionPlan) insert(order *int, implicitOrderActions ... *action, ) {
	if order == nil {
		actionPlan.unorderedActions = append(actionPlan.unorderedActions, implicitOrderActions)
	} else {
		orderedActions, present := actionPlan.orderedActions[*order]
		if present {
			orderedActions = append(orderedActions, implicitOrderActions)
		} else
		{
			actionPlan.orderedActions[*order] = []dependentActions{implicitOrderActions}
			actionPlan.sortedOrder = append(actionPlan.sortedOrder, *order)
		}
	}
}

func (actionPlan *ActionPlan) Execute(registryHandler packageregistry.IRegistryHandler, commandHandler cmd.ICommandHandler) (error) {
	// registry will be mutated and written to disk so that we can keep track of all the actions that have happened
	registry, err := registryHandler.GetExistingPackages()
	if err != nil {
		return err
	}

	for ; len(actionPlan.actionList) > 0; actionPlan.actionList = actionPlan.actionList[1:] {
		act := actionPlan.actionList[0]
		regKey := act.vmAppPackage.ApplicationName
		_, exists := registry[regKey]
		if exists {
			registry[regKey] = act.vmAppPackage
			registry[regKey].OngoingOperation = act.actionToPerform
		}

		registryHandler.WriteToDisk(registry)

		var commandToExecute string

		var isDeleteOperation = false

		switch (act.actionToPerform) {
		case packageregistry.Install:
			commandToExecute = act.vmAppPackage.InstallCommand
		case packageregistry.Delete:
			isDeleteOperation = true
			commandToExecute = act.vmAppPackage.RemoveCommand
		case packageregistry.Update:
			commandToExecute = act.vmAppPackage.UpdateCommand
		default:
			return errors.Errorf("Unexpected Action to perform encountered %v", act.actionToPerform)
		}
		retCode, err := commandHandler.Execute(commandToExecute)
		if err != nil {
			return errors.Wrapf(err, "Error executing command %v", commandToExecute)
		}
		if retCode != 0 {
			return errors.Errorf("Command %v exited with non-zero error code", commandToExecute)
		}

		if isDeleteOperation {
			delete(registry, regKey)
		} else {
			registry[regKey].OngoingOperation = packageregistry.NoAction
		}
		registryHandler.WriteToDisk(registry)
	}

	return nil
}
