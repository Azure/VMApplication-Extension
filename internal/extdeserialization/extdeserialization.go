package extdeserialization

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

func GetParameterNames(settings ActionSetting) []string {
	var names []string
	for _, param := range settings.Parameters {
		names = append(names, param.ParameterName)
	}
	return names
}

