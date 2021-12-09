package customactionplan

import (
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)

type action struct {
	// vmAppPackage is not a pointer type on purpose, we don't want it to be mutated
	vmAppPackage packageregistry.VMAppPackageCurrent
	Action       ActionSetting
}

type ActionCollection []*action

func (a ActionCollection) Len() int {
	return len(a)
}

func (a ActionCollection) Less(i, j int) bool {
	return a[i].Action.TickCount < a[j].Action.TickCount
}

func (a ActionCollection) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

type ActionPlan struct {
	environment *handlerenv.HandlerEnvironment
	sortedOrder ActionCollection
	logger      *logging.ExtensionLogger
}

type StatusErrorMessage string

func (err StatusErrorMessage) ToJsonString() string {
	return string(err)
}

func appendExecutionResult(executionResult *actionplan.PackageOperationResults, act *action, err error) {
	if err == nil {
		*executionResult = append(*executionResult, actionplan.PackageOperationResult{
			PackageName: act.vmAppPackage.ApplicationName,
			AppVersion:  act.vmAppPackage.Version,
			Operation:   act.Action.ActionName,
			Timestamp:   act.Action.Timestamp,
			Result:      actionplan.Success,
		})
	} else {
		*executionResult = append(*executionResult, actionplan.PackageOperationResult{
			PackageName: act.vmAppPackage.ApplicationName,
			AppVersion:  act.vmAppPackage.Version,
			Operation:   act.Action.ActionName,
			Timestamp:   act.Action.Timestamp,
			Result:      err.Error(),
		})
	}
}

func New(settings []*VmAppSetting, appPackage packageregistry.CurrentPackageRegistry, environment *handlerenv.HandlerEnvironment, logger *logging.ExtensionLogger) (*ActionPlan, error) {

	actionPlan := ActionPlan{
		environment: environment,
		sortedOrder: make([]*action, 0),
		logger:      logger,
	}

	tc := int64(0)
	tickCountFile := path.Join(actionPlan.environment.ConfigFolder, "tickCount")
	_, err := os.Stat(tickCountFile)
	if err != nil {
		logger.Info("Tick count file not found, setting tick count to 0")
		tc = 0
	} else {
		tickCount, err := ioutil.ReadFile(tickCountFile)
		if err != nil {
			logger.Error("Cannot read tick count file")
		}
		tc, err = strconv.ParseInt(string(tickCount), 10, 64)
		if err != nil {
			logger.Error("Tick count from file cannot be converted to integer")
		}
	}

	for _, app := range settings {
		_, containsApp := appPackage[app.ApplicationName]
		if app.Actions != nil && len(app.Actions) != 0 && containsApp {
			for _, a := range app.Actions {
				if uint64(tc) < a.TickCount {
					logger.Info("adding custom action %v to custom action plan", a.ActionName)
					newAction := action{
						vmAppPackage: *appPackage[app.ApplicationName],
						Action:       *a,
					}
					actionPlan.sortedOrder = append(actionPlan.sortedOrder, &newAction)
				} else {
					logger.Info("custom action %v has a low tick count %v compared to %v and will not be executed", a.ActionName, a.TickCount, tc)
				}
			}
		} else {
			logger.Info("application %v is not currently on the VM and cannot have its custom actions executed", app.ApplicationName)
		}
	}
	sort.Sort(actionPlan.sortedOrder)
	return &actionPlan, nil
}

func (actionPlan *ActionPlan) Execute(extEventManager *extensionevents.ExtensionEventManager, commandHandler commandhandler.ICommandHandlerWithEnvVariables, result *actionplan.PackageOperationResults) (error, actionplan.IResult) {
	var combinedErrors error = nil
	actionRegistry := GetCurrentCustomActions(actionPlan)

	for _, act := range actionPlan.sortedOrder {
		newError := actionPlan.executeHelper(commandHandler, *actionRegistry, act, extEventManager)
		combinedErrors = extensionerrors.CombineErrors(combinedErrors, newError)
		appendExecutionResult(result, act, newError)
	}

	if combinedErrors != nil {
		extEventManager.LogWarningEvent("CustomActionExecution", combinedErrors.Error())
	}

	return combinedErrors, result
}
