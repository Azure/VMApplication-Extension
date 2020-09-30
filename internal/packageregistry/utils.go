package packageregistry

import "github.com/pkg/errors"

func (self CurrentPackageRegistry)Populate(collection VMAppPackageCurrentCollection)(error){
	for _, v :=  range collection{
		if _, exists := self[v.ApplicationName]; exists{
			return errors.Errorf("Duplicate application name %s detected in application registry", v.ApplicationName)
		}
		self[v.ApplicationName] = v
	}
	return nil
}

func (self CurrentPackageRegistry)GetPackageCollection()(collection VMAppPackageCurrentCollection){
	collection = make(VMAppPackageCurrentCollection, 0)
	for _, value := range self{
		collection = append(collection, value)
	}
	return collection
}

func (self DesiredPackageRegistry)Populate(collection VMAppPackageIncomingCollection)(error){
	for _, v :=  range collection{
		if _, exists := self[v.ApplicationName]; exists{
			return errors.Errorf("Duplicate application name %s detected in application registry", v.ApplicationName)
		}
		self[v.ApplicationName] = v
	}
	return nil
}

func (self DesiredPackageRegistry)GetPackageCollection()(collection VMAppPackageIncomingCollection){
	collection = make(VMAppPackageIncomingCollection, 0)
	for _, value := range self{
		collection = append(collection, value)
	}
	return collection
}

func VMAppPackageIncomingToVmAppPackageCurrent(incoming *VMAppPackageIncoming)(current *VMAppPackageCurrent){
	current = &VMAppPackageCurrent{
		ApplicationName:incoming.ApplicationName,
		PackageLocation:incoming.PackageLocation,
		ConfigurationLocation:incoming.ConfigurationLocation,
		Version:incoming.Version,
		InstallCommand:incoming.InstallCommand,
		RemoveCommand:incoming.RemoveCommand,
		UpdateCommand:incoming.UpdateCommand,
		DirectDownloadOnly:incoming.DirectDownloadOnly,
		OngoingOperation:NoAction,
	}
	return current
}
