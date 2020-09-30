package cmd

type ICommandHandler interface{
	Execute(command string)(returnCode int, err error)
}
