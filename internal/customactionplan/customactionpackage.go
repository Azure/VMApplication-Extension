package customactionplan

import "github.com/Azure/VMApplication-Extension/internal/extdeserialization"

type CustomActionPackage struct {
	ApplicationName		string                              `json:"application"`
	Version             string     			`json:"version"`
	ActionName   		string      			`json:"name"`
	Timestamp    		string                                `json:"timestamp"`
	Parameters			[]extdeserialization.ActionParameter `json:"parameterNames"`
	Status 				string                               `json:"status"`
	Stderr 				string 				`json:"stderr"`
	Stdout 				string 				`json:"stdout"`
}

type ActionPackageRegistry map[string][]*CustomActionPackage

func GetCurrentCustomActions(actions *ActionPlan) (*ActionPackageRegistry) {
	act := make(ActionPackageRegistry, 0)
	var actionPackage CustomActionPackage
	for _, a := range actions.sortedOrder {
		actionPackage = CustomActionPackage{
			ApplicationName: a.vmAppPackage.ApplicationName,
			Version:         a.vmAppPackage.Version,
			ActionName:      a.Action.ActionName,
			Timestamp:       a.Action.Timestamp,
			Parameters:      a.Action.Parameters,
			Status:          "",
			Stderr:          "",
			Stdout:          "",
		}
		(act)[actionPackage.ApplicationName] = append((act)[actionPackage.ApplicationName], &actionPackage)
	}
	return &act
}
