
# Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## Prerequisites
1. Install go (v1.19.4)
2. If using VSCode, install the Go extension (use gopls@v0.11.4)

## Developing and Testing
-  Run `go get ./...` to update packages before building
-  Run `go build ./...` to build the entire project
-  To run all tests with messages in command line, run `go test ./...`
    -  OS-specific tests may be run on an Azure VM or local VM (ex. WSL on Windows)
-  Please do not commit vendor files