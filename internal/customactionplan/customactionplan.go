package customactionplan

import (
	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"

	//"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	//"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)

type action struct {
	// vmAppPackage is not a pointer type on purpose, we don't want it to be mutated
	vmAppPackage    packageregistry.VMAppPackageCurrent
	Action 			ActionSetting
}

type ActionCollection []*action

func (a ActionCollection) Len() int {
	return len(a)
}

func (a ActionCollection) Less(i, j int) bool {
	return a[i].Action.TickCount < a[j].Action.TickCount
}

func (a ActionCollection) Swap (i,j int) {
	a[i], a[j] = a[j], a[i]
}

const Success = "SUCCESS"
const Failed = "FAILED"

type ActionPlan struct {
	environment         *handlerenv.HandlerEnvironment
	sortedOrder                 ActionCollection
	logger                      *logging.ExtensionLogger
}


type StatusErrorMessage string

func (err StatusErrorMessage) ToJsonString() string {
	return string(err)
}

func appendExecutionResult(executionResult *actionplan.PackageOperationResults , act *action, err error) {
	if err == nil {
		*executionResult = append(*executionResult,actionplan.PackageOperationResult {
			PackageName: act.vmAppPackage.ApplicationName,
			AppVersion: act.vmAppPackage.Version,
			Operation: act.Action.ActionName,
			Timestamp: act.Action.Timestamp,
			Result: Success,
		})
	} else {
		*executionResult = append(*executionResult,actionplan.PackageOperationResult {
			PackageName: act.vmAppPackage.ApplicationName,
			AppVersion: act.vmAppPackage.Version,
			Operation: act.Action.ActionName,
			Timestamp: act.Action.Timestamp,
			Result: err.Error(),
		})
	}
}

func New(settings []*VmAppSetting, appPackage packageregistry.CurrentPackageRegistry, environment *handlerenv.HandlerEnvironment, logger *logging.ExtensionLogger) (*ActionPlan, error) {

	actionPlan := &ActionPlan{
		environment:                 environment,
		sortedOrder:                 make([]*action, 0),
		logger:                      logger,
	}

	tickCountFile := path.Join(actionPlan.environment.ConfigFolder, "tickCount")
	tc := int64(0)
	if _, err := os.Stat(tickCountFile); !os.IsNotExist(err) {

		tickCount, err := ioutil.ReadFile(tickCountFile)
		tc, err = strconv.ParseInt(string(tickCount), 10, 64)
		//fmt.Println(tc)
		if err != nil {
			tc = 0
			//fmt.Println(tc)
		}

	}
	//fmt.Println(tc)

	//var newAction action
	for _, app := range settings {
		_, containsApp := appPackage[app.ApplicationName]
		if app.Actions != nil && len(app.Actions) != 0 && containsApp{
			for _, a := range app.Actions {
				if uint64(tc) < a.TickCount  {
					newAction := action {
						vmAppPackage: *appPackage[app.ApplicationName],
						Action: *a,
					}
					actionPlan.sortedOrder = append(actionPlan.sortedOrder, &newAction)
				}
			}
		}
	}
	sort.Sort(actionPlan.sortedOrder)
	return actionPlan, nil
}

func (actionPlan *ActionPlan) Execute(eem *extensionevents.ExtensionEventManager, commandHandler commandhandler.ICommandHandlerWithEnvVariables, result *actionplan.PackageOperationResults) (error, actionplan.IResult) {
	var combinedErrors error = nil
	actionRegistry := GetCurrentCustomActions(actionPlan)

	for _, act := range actionPlan.sortedOrder {
		newError := actionPlan.executeHelper(commandHandler, *actionRegistry, act, eem)
		combinedErrors = extensionerrors.CombineErrors(combinedErrors, newError)
		appendExecutionResult(result, act, newError)

	}

	return combinedErrors, result
}


