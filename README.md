## VMApplication Extension for Azure VMs
VMApplication extension is responsible for managing the lifecycle of [VMApplication](https://learn.microsoft.com/en-us/azure/virtual-machines/vm-applications) packages on Azure VMs and scalesets. This extension in automatically deployed to Azure VMs and scalesets when a VMApp is added to the VM or scaleset configuration. It requires the VM or scaleset to have the VM Agent enabled.

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

# Data Collection
The software may collect information about you and your use of the software and send it to Microsoft. Microsoft may use this information to provide services and improve our products and services. You may turn off the telemetry as described in the repository. There are also some features in the software that may enable you and Microsoft to collect data from users of your applications. If you use these features, you must comply with applicable law, including providing appropriate notices to users of your applications together with a copy of Microsoft’s privacy statement. Our privacy statement is located at https://go.microsoft.com/fwlink/?LinkID=824704. You can learn more about data collection and use in the help documentation and our privacy statement. Your use of the software operates as your consent to these practices.

# Trademarks
Trademarks This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft trademarks or logos is subject to and must follow [Microsoft’s Trademark & Brand Guidelines](https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks/usage/general). Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship. Any use of third-party trademarks or logos are subject to those third-party’s policies.

## Prerequisites
1. Install go (v1.24.0)
2. To build Windows packages [nmake](https://learn.microsoft.com/en-us/cpp/build/reference/nmake-reference?view=msvc-170) is required

## Developing and Testing
-  Run `go get ./...` to update packages before building
-  Run `go build ./...` to build the entire project
-  To run all tests with messages in command line, run `go test ./...`
    -  OS-specific tests may be run on an Azure VM or local VM (eg. WSL on Windows)
-  To build the extension zip packages
    - Windows:
        - execute `nmake -f makefile.win EXTENSIONVERSION=<n.n.n>`
    - Linux:
        - execute `make EXTENSIONVERSION=<n.n.n>`

-  Please do not check-in vendor files

### Azure functional validation (Linux and Windows)

From PowerShell 7.3 or later, run:

```powershell
.\TestScripts\Test-VmApplications.ps1
```

The harness creates **two billable amd64 VMs**, one Ubuntu 22.04 and one Windows
Server 2022, and runs them simultaneously in separate thread jobs and resource
groups. It requires Go, Azure CLI (`az login`), and GitHub CLI (`gh`).
No local Python installation or LISA checkout is required; the Ubuntu guest uses
its built-in Python 3 to execute the application fixture.
The active Azure CLI subscription is used. Permissions must include resource
creation/deletion and assigning Storage Blob Data Contributor.

The publishing flow and 12 enabled tests per OS are translated into PowerShell
from
[microsoft/lisa#4891](https://github.com/microsoft/lisa/pull/4891), pinned to
`d36ca9e0802637c526fcbf1bea4ac2954d8141a6`. Only the two guest application
fixtures are downloaded unchanged from that revision. This exercises real PIR/CAPS
publishing, lifecycle/configuration, ordering, recovery, failure policy, removal,
child-resource APIs, and reboot behavior. The four custom-action cases disabled
in that PR remain disabled; the harness does not silently turn off other tests.

Each VM first installs a real gallery application using the published extension.
Only then does CustomScript discover the installed production or EDP handler version, replace both
the manager and launcher with locally cross-compiled binaries, and verify SHA256
hashes. The build uses the installed extension name/version, and CustomScript
rechecks the hashes around every test, including reboot tests.
A separate anchor application remains installed throughout the suite because
CRP uninstalls the handler when the application profile becomes empty. The anchor
has its own installation sentinel, does not emit fixture audit events, and is
checked separately in instance view. Removing all **test** applications therefore
does not discard the local binaries.

No-operation status recovery preserves the existing missing/corrupted,
transitioning, disable-to-enable, and last-stable HostGA recovery behavior.
Historical registry failures are not promoted into new deployment failures.
The cross-sequence HostGA race is deferred until failures can be attributed to
the relevant request; failure-policy validation may still expose that race.
Failed implicit removal during replacement retains the outgoing
package/configuration for the next attempt.
Linux reboot handling also checks systemd's shutdown state when a command is
terminated before the handler receives its shutdown signal. Ordinary command
failures and signals outside shutdown remain failures.
Signal notifications stop before normal command-result cleanup and registry
writes; reboot handling retains them while persisting reboot state.

Resources use private blob containers, user-delegation SAS, disabled storage
shared keys, and no open inbound NSG ports. A Standard public IP supplies explicit
outbound connectivity; SSH, RDP, and WinRM are not used. Resources are deleted on
completion, including failures, unless `-KeepResources` is specified.

```powershell
# Keep the VMs, gallery, and storage while investigating failures.
.\TestScripts\Test-VmApplications.ps1 -Region EastUS2 -KeepResources

# Use the results directory printed by the first invocation.
# Temporarily skip passes only when their source fingerprint still matches.
.\TestScripts\Test-VmApplications.ps1 -ResumeFrom <results-directory> -SkipPassed -KeepResources

# Focus on one failing scenario on both operating systems.
.\TestScripts\Test-VmApplications.ps1 -ResumeFrom <results-directory> -TestName verify_failure_policy -KeepResources

# Re-enable every test for final validation, then delete the resources.
.\TestScripts\Test-VmApplications.ps1 -ResumeFrom <results-directory>
```

Results, logs, build hashes, and per-OS resource ownership state are saved under
`bundle\functional-validation`. Failed cases trigger CustomScript diagnostic
archive collection. Resume validates resource-group ownership and never deletes
unrelated groups. Changing source, including cached guest fixtures, invalidates
previously passed results.
`-Region` defaults to `EastUS2`, `-VmSize` to `Standard_D2s_v5`, and
`-TimeoutHours` to 24. Other regions, including EUAP regions, can be selected
explicitly; their CRP/HostGA rollout must support the tested API behavior.
An interrupted or deadline-terminated run can leave resources behind: resume the
run to clean up, or explicitly delete the resource groups recorded in each
`state.json`. **Retained resources continue to incur charges.**

Offline harness checks (no Azure resources):

```powershell
.\TestScripts\Test-VmApplications.Offline.Tests.ps1
```
