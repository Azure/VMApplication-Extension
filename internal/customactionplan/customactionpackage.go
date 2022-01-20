package customactionplan

type ActionParameter struct {
	ParameterName  string `json:"name"`
	ParameterValue string `json:"value"`
}

type ActionSetting struct {
	ActionName   string            `json:"name"`
	ActionScript string            `json:"script"`
	Timestamp    string            `json:"timestamp"`
	Parameters   []ActionParameter `json:"parameters"`
	TickCount    uint64            `json:"tickCount"`
}

type VmAppSetting struct {
	ApplicationName string           `json:"name"`
	Order           *int             `json:"order"`
	Actions         []*ActionSetting `json:"actions"`
}

func getParameterNames (settings ActionSetting) ([]string) {
	var names []string
	for _, param := range settings.Parameters {
		names = append(names, param.ParameterName)
	}
	return names
}

type CustomActionPackage struct {
	ApplicationName		string				`json:"application"`
	Version             string     			`json:"version"`
	ActionName   		string      		`json:"name"`
	Timestamp    		string      		`json:"timestamp"`
	Parameters			[]ActionParameter	`json:"parameterNames"`
	Status 				string 				`json:"status"`
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
