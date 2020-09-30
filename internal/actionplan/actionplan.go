package actionplan

import (
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/cmd"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/pkg/errors"
	"sort"
)

type action struct {
	vmAppPackage    *packageregistry.VMAppPackageCurrent
	actionToPerform packageregistry.ActionEnum
}

type ActionPlan struct {
	actionList []*action
}

func New(currentPackageRegistry packageregistry.CurrentPackageRegistry, desiredVMAppCollection packageregistry.VMAppPackageIncomingCollection) (*ActionPlan, error) {
	actionList := make([]*action, 0)

	// get list of previously existing applications that don't exist in the new configuration
	packageRegistryIncoming := make(packageregistry.DesiredPackageRegistry)
	packageRegistryIncoming.Populate(desiredVMAppCollection)
	vmAppCurrentCollection := currentPackageRegistry.GetPackageCollection()
	for _, vmAppCurrent := range vmAppCurrentCollection {
		_, exists := packageRegistryIncoming[vmAppCurrent.ApplicationName]
		if exists {
			actionList = append(actionList, &action{vmAppCurrent, packageregistry.Delete})
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
					actionList = append(actionList, &action{vmAppCurrent, packageregistry.Delete}) // delete current and install incoming
					actionList = append(actionList, &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Install})
				} else {
					actionList = append(actionList, &action{packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(vmAppIncoming), packageregistry.Update})
				}
			}
		} else {

		}
	}

	return &ActionPlan{actionList: actionList}, nil
}

func (actionPlan *ActionPlan) Execute(registryHandler packageregistry.IRegistryHandler, commandHandler cmd.ICommandHandler) (error) {
	// registry will be mutated and written to disk so that we can keep track of all the actions that have happened
	registry, err := registryHandler.GetExistingPackages()
	if err != nil {
		return err
	}
	for ; len(actionPlan.actionList) > 0; actionPlan.actionList = actionPlan.actionList[1:] {
		// TODO: save actionList

		act := actionPlan.actionList[0]
		regKey := act.vmAppPackage.ApplicationName
		_, exists := registry[regKey]
		if exists {
			registry[regKey] = act.vmAppPackage
			registry[regKey].OngoingOperation = act.actionToPerform
		}

		// write the registry file so that we can keep track of what is going on
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
