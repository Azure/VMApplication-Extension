BUNDLEDIR=bundle/linux/prod
BUNDLEDIR_TEST=bundle/linux/test
BINDIR=$(BUNDLEDIR)/bin
BINDIR_TEST=$(BUNDLEDIR_TEST)/bin
EXTENSIONVERSION=1.0.18
ALLOWED_EXT1=Microsoft.CPlat.Core.VMApplicationManagerLinux
ALLOWED_EXT2=Microsoft.CPlat.Core.EDP.VMApplicationManagerLinux

# Allow overriding from the command line; default to the prod extension name
EXTENSIONNAME ?= $(ALLOWED_EXT1)

all: clean collect-licenses bundle-prod bundle-test

clean:
	-rm extension-launcher
	-rm extension-launcher-arm64
	-rm  vm-application-manager
	-rm  vm-application-manager-arm64
	-rm -R $(BUNDLEDIR)
	-rm -R licenses

extension-launcher: validate-extension-name
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o extension-launcher -ldflags="-X 'main.ExtensionName=$(EXTENSIONNAME)' -X 'main.ExtensionVersion=$(EXTENSIONVERSION)' -X 'main.ExecutableName=vm-application-manager'" ./launcher

extension-launcher-arm64: validate-extension-name
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o extension-launcher-arm64 -ldflags="-X 'main.ExtensionName=$(EXTENSIONNAME)' -X 'main.ExtensionVersion=$(EXTENSIONVERSION)' -X 'main.ExecutableName=vm-application-manager-arm64'" ./launcher

vm-application-manager: validate-extension-name
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o  vm-application-manager -ldflags="-X 'main.ExtensionName=$(EXTENSIONNAME)' -X 'main.ExtensionVersion=$(EXTENSIONVERSION)'" ./main

vm-application-manager-arm64: validate-extension-name
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o vm-application-manager-arm64 -ldflags="-X 'main.ExtensionName=$(EXTENSIONNAME)' -X 'main.ExtensionVersion=$(EXTENSIONVERSION)'"  ./main


.PHONY: validate-extension-name
validate-extension-name:
	@case "$(EXTENSIONNAME)" in \
	  "$(ALLOWED_EXT1)"|"$(ALLOWED_EXT2)" ) ;; \
	  * ) echo "Invalid EXTENSIONNAME: $(EXTENSIONNAME)"; \
	       echo "Valid values: $(ALLOWED_EXT1) or $(ALLOWED_EXT2)"; \
	       echo "Examples: make EXTENSIONNAME=\"$(ALLOWED_EXT1)\""; \
	       echo "          make EXTENSIONNAME=\"$(ALLOWED_EXT2)\""; \
	       exit 1 ;; \
	esac

collect-licenses:
	@echo "Collecting open source licenses..."
	@if ! command -v go-licenses >/dev/null 2>&1; then \
		echo "Installing go-licenses..."; \
		go install github.com/google/go-licenses@latest; \
	fi
	mkdir -p licenses/reports
	go-licenses save ./main --save_path=licenses/texts
	go-licenses csv ./main > licenses/reports/THIRD_PARTY_LICENSES.csv
	@echo "License collection complete!"

bundle-prod: extension-launcher extension-launcher-arm64 vm-application-manager vm-application-manager-arm64
	@echo "Packaging PROD bundle into $(BUNDLEDIR) with EXTENSIONNAME=$(ALLOWED_EXT1)"
	mkdir -p $(BINDIR)
	mv extension-launcher "$(BINDIR)/"
	mv extension-launcher-arm64 "$(BINDIR)/"
	mv vm-application-manager "$(BINDIR)/"
	mv vm-application-manager-arm64 "$(BINDIR)/"
	cp misc/linux/install.sh "$(BINDIR)/"
	cp misc/linux/update.sh "$(BINDIR)/"
	cp misc/linux/HandlerManifest.json "$(BUNDLEDIR)/"
	cp -r licenses "$(BUNDLEDIR)/"
	cd $(BUNDLEDIR) && zip -r vm-application-manager.zip ./*

bundle-test:
	@echo "Building and packaging TEST bundle into $(BUNDLEDIR_TEST) with EXTENSIONNAME=$(ALLOWED_EXT2)"
	$(MAKE) EXTENSIONNAME=$(ALLOWED_EXT2) extension-launcher extension-launcher-arm64 vm-application-manager vm-application-manager-arm64
	mkdir -p $(BINDIR_TEST)
	mv extension-launcher "$(BINDIR_TEST)/"
	mv extension-launcher-arm64 "$(BINDIR_TEST)/"
	mv vm-application-manager "$(BINDIR_TEST)/"
	mv vm-application-manager-arm64 "$(BINDIR_TEST)/"
	cp misc/linux/install.sh "$(BINDIR_TEST)/"
	cp misc/linux/update.sh "$(BINDIR_TEST)/"
	cp misc/linux/HandlerManifest.json "$(BUNDLEDIR_TEST)/"
	cp -r licenses "$(BUNDLEDIR_TEST)/"
	cd $(BUNDLEDIR_TEST) && zip -r vm-application-manager.zip ./*

.PHONY: clean bundle collect-licenses
