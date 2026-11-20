#requires -Version 7.3
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT license.

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'Test-VmApplications.ps1')
$script:DeadlineUtc = [DateTime]::UtcNow.AddMinutes(10)
$script:Checks = 0

function Test-Check {
    param([string] $Name, [scriptblock] $Body)
    & $Body
    $script:Checks++
    Write-Host "PASS $Name"
}

function Assert-Throws {
    param([scriptblock] $Body, [string] $Pattern)
    $caught = $null
    try { & $Body } catch { $caught = $_ }
    if (-not $caught -or [string]$caught -notmatch $Pattern) { throw "Expected an exception matching $Pattern; got $caught" }
}

Test-Check 'All twelve enabled LISA cases are translated' {
    Assert-Equal $script:TestCases.Count 12
    Assert-Equal @($script:TestCases.Keys) @(
        'verify_installation_and_configuration', 'verify_application_lifecycle',
        'verify_ordered_operations', 'verify_ordered_failure_and_recovery',
        'verify_failure_policy', 'verify_replace_failure', 'verify_removal_and_reuse',
        'verify_remove_failure', 'verify_child_resource_operations',
        'verify_restart_persistence', 'verify_reboot_without_rerun', 'verify_reboot_with_rerun'
    )
}

Test-Check 'Structural equality preserves array ordering and ignores dictionary ordering' {
    Assert-Equal @{ b = @(1, 2); a = $true } @{ a = $true; b = @(1, 2) }
    Assert-Equal @() @()
    Assert-Throws { Assert-Equal @(1, 2) @(2, 1) } 'mismatch'
}

Test-Check 'Gallery entries preserve flags and exact versions' {
    $script:GalleryId = '/gallery'
    $script:Names = @{ a = 'app-a' }
    $entry = New-AppEntry a '2.0.0' @{ treatFailureAsDeploymentFailure = $false; order = 10 }
    Assert-Equal $entry.packageReferenceId '/gallery/applications/app-a/versions/2.0.0'
    Assert-Equal $entry.treatFailureAsDeploymentFailure $false
    Assert-Equal $entry.order 10
}

Test-Check 'Page blob data always has nonzero 512-byte padding' {
    Assert-Equal (ConvertTo-PageBytes ([byte[]]@(1, 2))).Length 512
    Assert-Equal (ConvertTo-PageBytes ([byte[]]::new(512))).Length 1024
}

Test-Check 'CSE paths match Windows and Linux download layouts' {
    $uri = 'https://storage.blob.core.windows.net/container/linux/hash/harness/build/file.exe?sig=private'
    $Worker = 'Windows'
    Assert-Equal (Get-CseDownloadPath $uri) 'linux\hash\harness\build\file.exe'
    $Worker = 'Linux'
    Assert-Equal (Get-CseDownloadPath $uri) 'file.exe'
}

Test-Check 'CSE output parsing accepts absent Linux substatuses and duplicate Windows messages' {
    $message = 'VMAPPS-VALIDATION:{"directory":"installed","version":"1.0.99"}'
    $status = @{ message = $message }
    foreach ($view in @(
        @{ statuses = @($status) },
        @{ statuses = @($status); substatuses = @($status) }
    )) {
        Assert-Equal (ConvertFrom-CustomScriptView $view).version '1.0.99'
    }
    Assert-Throws { ConvertFrom-CustomScriptView @{ statuses = @() } } 'marker'
}

Test-Check 'Windows application bootstrap fails when its package is missing' {
    $script:Worker = 'Windows'
    $script:Names = @{ a = "offline-missing-$([guid]::NewGuid().ToString('N'))" }
    $script:DefaultConfig = [byte[]]@(1)
    $script:PayloadHash = 'offline'
    $command = Get-FixtureCommand install a '1.0.0'
    $encoded = ($command -split ' ')[-1]
    $result = Invoke-Native powershell.exe @('-NoLogo', '-NoProfile', '-NonInteractive',
        '-EncodedCommand', $encoded) -AllowFailure
    Assert-True ($result.ExitCode -ne 0) 'Missing application media incorrectly reported success.'
}

Test-Check 'Infrastructure has secure storage and no inbound openings on both operating systems' {
    $script:State = @{ region = 'centraluseuap'; storage = 'teststorage' }
    foreach ($Worker in @('Linux', 'Windows')) {
        $template = New-InfrastructureTemplate 'test-principal' 'User'
        $resources = @{}
        foreach ($resource in $template.resources) { $resources[$resource.type] = $resource }
        Assert-Equal $resources['Microsoft.Network/networkSecurityGroups'].properties.securityRules @()
        $storage = $resources['Microsoft.Storage/storageAccounts'].properties
        Assert-Equal $storage.allowSharedKeyAccess $false
        Assert-Equal $storage.allowBlobPublicAccess $false
        $nic = $resources['Microsoft.Network/networkInterfaces']
        Assert-Equal $nic.properties.ipConfigurations[0].properties.subnet.id "[resourceId('Microsoft.Network/virtualNetworks/subnets', 'validation', 'validation')]"
        $vm = $resources['Microsoft.Compute/virtualMachines']
        Assert-True ($vm.properties.osProfile.ContainsKey("$($Worker.ToLowerInvariant())Configuration")) 'Missing OS-specific configuration.'
        Assert-Equal $template.parameters.adminPassword.type 'secureString'
    }
}

Test-Check 'ARM polling waits for terminal success and reports terminal errors' {
    $script:Polls = 0
    function Invoke-ArmRequest {
        param($Method, $Uri, $Body)
        if ($Method -eq 'PUT') { return @{ Status = 202; Body = @{}; Headers = @{ 'Azure-AsyncOperation' = @('https://management.azure.com/poll') } } }
        $script:Polls++
        @{ Status = 200; Body = @{ status = if ($script:Polls -eq 1) { 'Running' } else { 'Succeeded' } }; Headers = @{} }
    }
    function Start-Sleep { param($Seconds) }
    $response = Invoke-Arm PUT '/test' @{}
    Assert-Equal $script:Polls 2
    Assert-Equal $response.Status 202
    function Invoke-ArmRequest {
        param($Method, $Uri, $Body)
        if ($Method -eq 'PUT') { return @{ Status = 202; Body = @{}; Headers = @{ 'Azure-AsyncOperation' = @('https://management.azure.com/poll') } } }
        @{ Status = 200; Body = @{ status = 'Failed'; error = @{ code = 'ExpectedError' } }; Headers = @{} }
    }
    Assert-Throws { Invoke-Arm PUT '/test' @{} } 'ExpectedError'
}

Test-Check 'ARM rejects token forwarding to other hosts' {
    Assert-Throws { Invoke-ArmRequest GET 'https://example.invalid/steal-token' } 'Refusing'
}

Test-Check 'ARM token caching respects the token expiry rather than its acquisition time' {
    $script:ArmToken = ''
    $script:ExpectedExpiry = [DateTimeOffset]::UtcNow.AddMinutes(5).ToUnixTimeSeconds()
    function Invoke-Az {
        @{ accessToken = 'offline-token'; expires_on = $script:ExpectedExpiry }
    }
    function Invoke-WebRequest {
        @{ StatusCode = 200; Content = '{}'; Headers = @{} }
    }
    $null = Invoke-ArmRequest GET '/test'
    Assert-Equal $script:TokenExpiry ([DateTimeOffset]::FromUnixTimeSeconds($script:ExpectedExpiry).UtcDateTime.AddMinutes(-2))
}

Test-Check 'ARM retries transient reads within a limit but never replays mutations' {
    $script:ArmToken = 'offline-token'
    $script:TokenExpiry = [DateTime]::UtcNow.AddHours(1)
    $script:HttpAttempts = 0
    $script:AlwaysFail = $false
    function Start-Sleep { param($Seconds) }
    function Invoke-WebRequest {
        $script:HttpAttempts++
        if ($script:AlwaysFail -or $script:HttpAttempts -lt 3) {
            throw [System.Threading.Tasks.TaskCanceledException]::new('transient transport failure')
        }
        @{ StatusCode = 200; Content = '{"value":"complete"}'; Headers = @{} }
    }
    $response = Invoke-ArmRequest GET '/test'
    Assert-Equal $script:HttpAttempts 3
    Assert-Equal $response.Body.value 'complete'
    $script:AlwaysFail = $true
    $script:HttpAttempts = 0
    Assert-Throws { Invoke-ArmRequest GET '/test' } 'transient transport failure'
    Assert-Equal $script:HttpAttempts 3
    $script:HttpAttempts = 0
    Assert-Throws { Invoke-ArmRequest POST '/test' } 'transient transport failure'
    Assert-Equal $script:HttpAttempts 1
}

Test-Check 'ARM refreshes credentials that expire during a read retry' {
    $script:ArmToken = 'first-token'
    $script:TokenExpiry = [DateTime]::UtcNow.AddMinutes(5)
    $script:HttpAttempts = 0
    $script:TokenRequests = 0
    function Start-Sleep { param($Seconds) }
    function Invoke-Az {
        $script:TokenRequests++
        @{ accessToken = 'refreshed-token'; expires_on = [DateTimeOffset]::UtcNow.AddHours(1).ToUnixTimeSeconds() }
    }
    function Invoke-WebRequest {
        param($Headers)
        $script:HttpAttempts++
        if ($script:HttpAttempts -eq 1) {
            $script:TokenExpiry = [DateTime]::MinValue
            throw [System.Threading.Tasks.TaskCanceledException]::new('transient transport failure')
        }
        Assert-Equal $Headers.Authorization 'Bearer refreshed-token'
        @{ StatusCode = 200; Content = '{}'; Headers = @{} }
    }
    $null = Invoke-ArmRequest GET '/test'
    Assert-Equal $script:HttpAttempts 2
    Assert-Equal $script:TokenRequests 1
}

Test-Check 'Action assertions preserve order unless explicitly unordered' {
    function Get-Snapshot {
        @{ events = @(
            @{ app = 'b'; op = 'install'; version = '1.0.0'; result = 'success' },
            @{ app = 'a'; op = 'install'; version = '1.0.0'; result = 'success' }
        ) }
    }
    $expected = @('a|install|1.0.0|success', 'b|install|1.0.0|success')
    $null = Assert-Events $expected -Unordered
    Assert-Throws { Assert-Events $expected } 'mismatch'
}

Test-Check 'Instance view validates versions, outcomes and manually managed apps' {
    $script:Names = @{ a = 'app-a'; d = 'app-d'; anchor = 'app-anchor' }
    $script:GalleryId = '/gallery'
    function Get-ArmBody {
        @{
            properties = @{
                provisioningState = 'Succeeded'
                instanceView = @{ vmApplications = @(
                    @{ name = 'app-a'; version = '2.0.0'; result = 'Update SUCCESS' },
                    @{ name = 'app-d' },
                    @{ name = 'app-anchor'; version = '1.0.0'; result = 'Install SUCCESS' }
                ) }
            }
        }
    }
    $script:VmId = '/vm'
    Wait-ApplicationView @((New-AppEntry a '2.0.0'), (New-AppEntry d -Properties @{ manuallyManaged = $true }))
    Assert-Equal $script:LastView['app-a'].version '2.0.0'
}

Test-Check 'Empty test profiles retain the anchor instead of uninstalling the handler' {
    $script:Names = @{ anchor = 'app-anchor' }
    $script:GalleryId = '/gallery'
    $script:State = @{ region = 'eastus2' }
    $script:VmId = '/vm'
    function Invoke-Arm {
        param($Method, $ResourceId, $Body, [switch] $AllowFailure)
        $script:CapturedProfile = $Body.properties.applicationProfile.galleryApplications
        @{ Status = 200; Body = @{} }
    }
    function Wait-ApplicationView { param($Applications, $Outcomes, $DeploymentFailure, $CurrentVersions) }
    Set-Applications @()
    Assert-Equal @($script:CapturedProfile).Count 1
    Assert-Equal $script:CapturedProfile[0].packageReferenceId '/gallery/applications/app-anchor/versions/1.0.0'
}

Test-Check 'Native build environments are isolated from the parent thread' {
    $before = $env:GOOS
    $result = Invoke-Native (Join-Path $PSHOME 'pwsh.exe') @(
        '-NoProfile', '-NonInteractive', '-Command', '[Console]::Write($env:GOOS)'
    ) -Environment @{ GOOS = 'linux' }
    Assert-Equal $result.StdOut 'linux'
    Assert-Equal $env:GOOS $before
}

Test-Check 'Native failures and deadlines are explicit' {
    $tool = Join-Path $PSHOME 'pwsh.exe'
    $result = Invoke-Native $tool @('-NoProfile', '-NonInteractive', '-Command', 'exit 7') -AllowFailure
    Assert-Equal $result.ExitCode 7
    Assert-Throws { Invoke-Native $tool @('-NoProfile', '-NonInteractive', '-Command', 'exit 7') } 'failed'
    Assert-Throws { Invoke-Native $tool @('-NoProfile', '-NonInteractive', '-Command', 'Start-Sleep -Seconds 30') -TimeoutSeconds 1 } 'deadline'
}

Test-Check 'Logs redact both registered secrets and SAS credentials' {
    $script:Secrets.Add('private-token')
    $text = Protect-Text 'private-token https://example/x?sv=secret&sig=private&sp=rw'
    Assert-True ($text -notmatch 'private-token|=secret|=private|=rw') 'Secrets leaked into diagnostic text.'
}

Write-Host "All $($script:Checks) offline checks passed."
