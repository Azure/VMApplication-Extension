package protectedsettings

import "github.com/Azure/VMApplication-Extension/internal/packageregistry"

type ProtectedSettings []*AppSettings

type AppSettings struct {
	ApplicationName string `json:"applicationName"`
	Order           *int   `json:"order"`
}

func Parse(jsonBytes []byte)

func (prot ProtectedSettings)GetIncomingApplicationCollection()(incomingCollection packageregistry.VMAppPackageIncomingCollection,err error){
	// TODO: implement this function
	return
}