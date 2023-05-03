BUNDLEDIR=bundle/linux
BINDIR=$(BUNDLEDIR)/bin
EXTENSIONVERSION=1.0.13

all: clean bundle

clean:
	-rm extension-launcher
	-rm extension-launcher-arm64
	-rm  vm-application-manager
	-rm  vm-application-manager-arm64
	-rm -R $(BUNDLEDIR)

extension-launcher:
	GOOS=linux GOARCH=amd64 go build -o extension-launcher -ldflags="-X 'main.ExtensionVersion=$(EXTENSIONVERSION)' -X 'main.ExecutableName=vm-application-manager'" ./launcher

extension-launcher-arm64:
	GOOS=linux GOARCH=arm64 go build -o extension-launcher-arm64 -ldflags="-X 'main.ExtensionVersion=$(EXTENSIONVERSION)' -X 'main.ExecutableName=vm-application-manager-arm64'" ./launcher

vm-application-manager:
	GOOS=linux GOARCH=amd64 go build -o  vm-application-manager -ldflags="-X 'main.ExtensionVersion=$(EXTENSIONVERSION)'" ./main

vm-application-manager-arm64:
	GOOS=linux GOARCH=arm64 go build -o vm-application-manager-arm64 -ldflags="-X 'main.ExtensionVersion=$(EXTENSIONVERSION)'"  ./main

bundle: extension-launcher extension-launcher-arm64 vm-application-manager vm-application-manager-arm64
	mkdir -p $(BINDIR)
	mv extension-launcher "$(BINDIR)/"
	mv extension-launcher-arm64 "$(BINDIR)/"
	mv vm-application-manager "$(BINDIR)/"
	mv vm-application-manager-arm64 "$(BINDIR)/"
	cp misc/linux/install.sh "$(BINDIR)/"
	cp misc/linux/update.sh "$(BINDIR)/"
	cp misc/linux/HandlerManifest.json "$(BUNDLEDIR)/"
	cd $(BUNDLEDIR) && zip -r vm-application-manager.zip ./*

.PHONY: clean bundle
