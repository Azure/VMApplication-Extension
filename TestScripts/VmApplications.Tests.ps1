# Copyright (c) Microsoft Corporation.
# Licensed under the MIT license.
#
# PowerShell translations of the enabled tests in microsoft/lisa#4891, revision
# d36ca9e0802637c526fcbf1bea4ac2954d8141a6. Custom-action tests remain disabled
# there pending PIR metadata propagation and are not included in this suite.

function ConvertTo-CanonicalValue {
    param($Value)
    if ($Value -is [Collections.IDictionary]) {
        $result = [ordered]@{}
        foreach ($key in ($Value.Keys | Sort-Object)) { $result[$key] = ConvertTo-CanonicalValue $Value[$key] }
        return $result
    }
    if ($Value -is [Collections.IEnumerable] -and $Value -isnot [string]) {
        return ,@($Value | ForEach-Object { ConvertTo-CanonicalValue $_ })
    }
    $Value
}

function Assert-Equal {
    param($Actual, $Expected, [string] $Message = 'Values')
    $a = ConvertTo-CanonicalValue $Actual | ConvertTo-Json -Depth 80 -Compress
    $b = ConvertTo-CanonicalValue $Expected | ConvertTo-Json -Depth 80 -Compress
    if ($a -cne $b) { throw "$Message mismatch. Expected: $b; actual: $a" }
}

function Assert-True {
    param([bool] $Condition, [string] $Message)
    if (-not $Condition) { throw $Message }
}

function Get-Events {
    param([hashtable] $Snapshot)
    @($Snapshot.events | ForEach-Object { "$($_.app)|$($_.op)|$($_.version)|$($_.result)" })
}

function Assert-Events {
    param([string[]] $Expected, [switch] $Unordered)
    $snapshot = Get-Snapshot
    $actual = @(Get-Events $snapshot)
    if ($Unordered) { $actual = @($actual | Sort-Object); $Expected = @($Expected | Sort-Object) }
    Assert-Equal $actual @($Expected) 'Application executions'
    $snapshot
}

function Assert-States {
    param([hashtable] $Snapshot, [string[]] $Names)
    Assert-Equal @($Snapshot.states.Keys | Sort-Object) @($Names | Sort-Object) 'Installed fixture state'
}

function Wait-ApplicationView {
    param(
        [array] $Applications, [hashtable] $Outcomes = @{},
        [bool] $DeploymentFailure = $false, [hashtable] $CurrentVersions = @{}
    )
    $Applications = @((New-AnchorEntry)) + $Applications
    $desired = @{}
    $manual = @()
    foreach ($app in $Applications) {
        $name = ($app.packageReferenceId -split '/applications/')[1].Split('/')[0]
        if ($app['manuallyManaged']) { $manual += $name }
        else { $desired[$name] = $app.packageReferenceId.Split('/')[-1] }
    }
    $end = [DateTime]::UtcNow.AddSeconds(90)
    do {
        $properties = (Get-ArmBody $script:VmId -Expand instanceView).properties
        $view = $properties['instanceView']
        $results = @{}
        if ($view) { foreach ($app in @($view['vmApplications'])) { if ($app) { $results[$app.name] = $app } } }
        $matches = @($desired.Keys | Where-Object { -not $results.ContainsKey($_) }).Count -eq 0 -and
            @($results.Keys | Where-Object { -not $desired.ContainsKey($_) -and $_ -notin $manual }).Count -eq 0
        foreach ($name in $manual) {
            if ($results.ContainsKey($name)) {
                $result = $results[$name]
                $matches = $matches -and -not ($result['version'] -or $result['result'] -or $result['actionsPerformed'])
            }
        }
        foreach ($alias in $script:Names.Keys) {
            $name = $script:Names[$alias]
            if ($desired.ContainsKey($name)) {
                $result = if ($results.ContainsKey($name)) { $results[$name] } else { @{} }
                $version = if ($CurrentVersions.ContainsKey($alias)) { $CurrentVersions[$alias] } else { $desired[$name] }
                $outcome = if ($Outcomes.ContainsKey($alias)) { $Outcomes[$alias] } else { 'success' }
                $matches = $matches -and $result['version'] -eq $version -and ([string]$result['result']).ToLowerInvariant().Contains($outcome.ToLowerInvariant())
            }
        }
        if ($matches) {
            if (-not $DeploymentFailure) { Assert-True ($properties.provisioningState -notin @('Failed', 'Canceled')) 'Unexpected VM deployment failure.' }
            $script:LastView = $results
            return
        }
        if ([DateTime]::UtcNow -ge $end) {
            throw "Application instance view did not converge: desired=$($desired | ConvertTo-Json -Compress); results=$($results | ConvertTo-Json -Depth 20 -Compress); state=$($properties.provisioningState)"
        }
        Start-Sleep -Seconds 3
    } while ($true)
}

function Set-Applications {
    param(
        [AllowEmptyCollection()][array] $Applications, [hashtable] $Outcomes = @{},
        [bool] $DeploymentFailure = $false, [hashtable] $CurrentVersions = @{}
    )
    $script:ProfileStarted = $true
    Write-Step "Applying $($Applications.Count) applications."
    $response = Invoke-Arm PATCH $script:VmId @{
        location = $script:State.region
        properties = @{ applicationProfile = @{ galleryApplications = @((New-AnchorEntry)) + @($Applications) } }
    } -AllowFailure
    if ($response.Status -ge 400) {
        $errorText = $response.Body | ConvertTo-Json -Depth 30 -Compress
        Assert-True ($DeploymentFailure -and $errorText -match '(?i)VMExtension|VMApplication' -and
            $errorText -match "(?i)VMAppExtension|VMApplicationManager$Worker") "Unexpected VM update failure: $errorText"
    } else { Assert-True (-not $DeploymentFailure) 'Expected the VM update operation to fail, but it succeeded.' }
    Wait-ApplicationView $Applications $Outcomes $DeploymentFailure $CurrentVersions
}

function Assert-RejectedProfile {
    param([array] $Applications, [string] $Diagnostic)
    $before = (Get-ArmBody $script:VmId).properties['applicationProfile']
    $response = Invoke-Arm PATCH $script:VmId @{
        properties = @{ applicationProfile = @{ galleryApplications = @((New-AnchorEntry)) + @($Applications) } }
    } -AllowFailure
    $text = $response.Body | ConvertTo-Json -Depth 40 -Compress
    Assert-True ($response.Status -in @(400, 404, 409)) "CRP accepted invalid input: $Diagnostic"
    Assert-True ($text.ToLowerInvariant().Contains($Diagnostic.ToLowerInvariant())) "Unexpected CRP rejection: $text"
    Assert-Equal (Get-ArmBody $script:VmId).properties['applicationProfile'] $before 'Rejected profile left applications unchanged'
}

function Assert-ActionStatus {
    param([hashtable] $Snapshot, [string] $Alias, [string] $Operation, [string] $Result)
    Assert-True ($null -ne $Snapshot.status) 'No VM Applications handler status was found.'
    $matching = @($Snapshot.status.actions | Where-Object {
        $_.name -eq $script:Names[$Alias] -and $_.op -eq $Operation -and $_.result -ceq $Result
    })
    Assert-True ($matching.Count -gt 0) "Missing handler result: $Alias/$Operation/$Result."
}

function Set-ChildApplication {
    param([string] $Alias, [hashtable] $Properties, [switch] $Patch)
    $script:ProfileStarted = $true
    $method = if ($Patch) { 'PATCH' } else { 'PUT' }
    $null = Invoke-Arm $method "$($script:VmId)/vmApplications/$($script:Names[$Alias])" @{
        location = $script:State.region; properties = $Properties
    }
}

function Test-InstallationAndConfiguration {
    Set-Control @{ configuration = @{ a = Get-BytesHash $script:OverrideConfig } }
    $applications = @(
        (New-AppEntry a -Properties @{ configurationReference = $script:OverrideUri }),
        (New-AppEntry b), (New-AppEntry c), (New-AppEntry d -Properties @{ manuallyManaged = $true })
    )
    Set-Applications $applications
    $snapshot = Assert-Events @('a|install|1.0.0|success', 'b|install|1.0.0|success', 'c|install|1.0.0|success') -Unordered
    Assert-States $snapshot @('a', 'b', 'c')
    Assert-Equal $snapshot.status.handler "VMApplicationManager$Worker" 'CRP-selected handler'
    Assert-RejectedProfile ($applications + @((New-AppEntry other))) $script:Names.other
    Assert-RejectedProfile ($applications + @($applications[0])) 'same packageReferenceId'
    Assert-RejectedProfile ($applications + @(@{})) 'packageReferenceId'
    $invalid = New-AppEntry b
    $invalid.packageReferenceId = 'not-a-resource-id'
    Assert-RejectedProfile ($applications + @($invalid)) 'not-a-resource-id'
    $withoutB = @($applications | Where-Object packageReferenceId -NE (Get-AppReference b))
    Assert-RejectedProfile ($withoutB + @((New-AppEntry b -Properties @{ configurationReference = 'http://example.invalid/x' }))) 'configurationReference'
    $null = Assert-Events @(Get-Events $snapshot)
}

function Test-ApplicationLifecycle {
    $a = New-AppEntry a
    Set-Applications @($a)
    $expected = @('a|install|1.0.0|success')
    $null = Assert-Events $expected
    Set-Applications @($a)
    $b = New-AppEntry b -Properties @{ order = 2 }
    Set-Applications @((New-AppEntry a -Properties @{ order = 1 }), $b)
    $expected += 'b|install|1.0.0|success'
    $null = Assert-Events $expected
    Set-Applications @((New-AppEntry a '2.0.0' @{ order = 1 }), $b)
    $expected += 'a|update|2.0.0|success'
    $null = Assert-Events $expected
    Set-Applications @((New-AppEntry a -Properties @{ order = 1 }), $b)
    $expected += 'a|update|1.0.0|success'
    $null = Assert-Events $expected
    Set-Applications @((New-AppEntry a '3.0.0' @{ order = 1 }), $b)
    $null = Assert-Events ($expected + @('a|remove|1.0.0|success', 'a|install|3.0.0|success'))
}

function Test-OrderedOperations {
    Set-Applications @((New-AppEntry d), (New-AppEntry a))
    Set-Control @{ requires = @{ b = @('a'); c = @('b') } }
    Set-Applications @((New-AppEntry c -Properties @{ order = 30 }), (New-AppEntry a '2.0.0' @{ order = 10 }), (New-AppEntry b -Properties @{ order = 20 }))
    $events = @(Get-Events (Get-Snapshot))
    Assert-Equal @($events[0..1] | Sort-Object) @('a|install|1.0.0|success', 'd|install|1.0.0|success') 'Initial unordered installations'
    Assert-Equal @($events | Select-Object -Skip 2) @(
        'd|remove|1.0.0|success', 'a|update|2.0.0|success', 'b|install|1.0.0|success', 'c|install|1.0.0|success'
    ) 'Ordered removal, update and installation'
}

function Test-OrderedFailureAndRecovery {
    Set-Control @{ failures = @{ 'a.install' = $true } }
    $a = New-AppEntry a -Properties @{ order = 1; treatFailureAsDeploymentFailure = $false }
    $b = New-AppEntry b -Properties @{ order = 1 }
    $d = New-AppEntry d
    Set-Applications @($a, $b, (New-AppEntry c -Properties @{ order = 2 }), $d) @{ a = 'failed'; c = 'skipped' }
    $snapshot = Assert-Events @('a|install|1.0.0|failed', 'b|install|1.0.0|success', 'd|install|1.0.0|success') -Unordered
    $expected = @(Get-Events $snapshot)
    Set-Applications @($a, $b, $d) @{ a = 'failed' }
    $null = Assert-Events $expected
    Set-Control @{ failures = @{} }
    Set-Applications @((New-AppEntry a '2.0.0' @{ order = 1 }), $b, (New-AppEntry c -Properties @{ order = 2 }), $d)
    $snapshot = Assert-Events ($expected + @('a|update|2.0.0|success', 'c|install|1.0.0|success'))
    Assert-States $snapshot @('a', 'b', 'c', 'd')
}

function Test-FailurePolicy {
    $expected = @()
    foreach ($failDeployment in @($false, $true)) {
        Set-Control @{ failures = @{ 'a.install' = $true } }
        Set-Applications @((New-AppEntry a -Properties @{ treatFailureAsDeploymentFailure = $failDeployment })) @{ a = 'failed' } $failDeployment
        $expected += 'a|install|1.0.0|failed'
        $null = Assert-Events $expected
        Set-Control @{ failures = @{} }
        Set-Applications @()
        $expected += 'a|remove|1.0.0|success'
        Set-Applications @((New-AppEntry a -Properties @{ treatFailureAsDeploymentFailure = $failDeployment }))
        $expected += 'a|install|1.0.0|success'
        $null = Assert-Events $expected
        Set-Control @{ failures = @{ 'a.update' = $true } }
        Set-Applications @((New-AppEntry a '2.0.0' @{ treatFailureAsDeploymentFailure = $failDeployment })) @{ a = 'failed' } $failDeployment
        $expected += 'a|update|2.0.0|failed'
        $null = Assert-Events $expected
        Set-Control @{ failures = @{} }
        Set-Applications @()
        $expected += 'a|remove|2.0.0|success'
    }
    $null = Assert-Events $expected
}

function Test-ReplaceFailure {
    Set-Applications @((New-AppEntry a))
    Set-Control @{ failures = @{ 'a.remove' = $true } }
    Set-Applications @((New-AppEntry a '3.0.0' @{ treatFailureAsDeploymentFailure = $false })) @{ a = 'failed' } $false @{ a = '1.0.0' }
    $snapshot = Get-Snapshot
    $events = @(Get-Events $snapshot)
    Assert-Equal $events[0] 'a|install|1.0.0|success' 'Initial installation'
    Assert-True ($events.Count -gt 1) 'Missing failed removal attempts.'
    Assert-True (@($events | Select-Object -Skip 1 | Where-Object { $_ -ne 'a|remove|1.0.0|failed' }).Count -eq 0) 'Replacement ran despite failed old-version removal.'
    Assert-States $snapshot @('a')
    Assert-Equal $snapshot.states.a.version '1.0.0' 'Preserved old version'
}

function Test-RemovalAndReuse {
    Set-Control @{ mutate = @('a') }
    Set-Applications @((New-AppEntry a -Properties @{ order = 1 }), (New-AppEntry b -Properties @{ order = 2 }))
    Set-Applications @((New-AppEntry b))
    $expected = @('a|install|1.0.0|success', 'b|install|1.0.0|success', 'a|remove|1.0.0|success')
    $snapshot = Assert-Events $expected
    Assert-States $snapshot @('b')
    Assert-True (-not $snapshot.present.a -and $snapshot.present.b) 'Removal did not preserve the peer application.'
    Set-Applications @()
    $expected += 'b|remove|1.0.0|success'
    $snapshot = Assert-Events $expected
    Assert-States $snapshot @()
    Assert-True (@($snapshot.present.Values | Where-Object { $_ }).Count -eq 0) 'Application files remain after removal.'
    Set-Applications @((New-AppEntry a))
    $null = Assert-Events ($expected + @('a|install|1.0.0|success'))
}

function Test-RemoveFailure {
    Set-Applications @((New-AppEntry a -Properties @{ order = 1 }), (New-AppEntry b -Properties @{ order = 2 }))
    Set-Control @{ failures = @{ 'a.remove' = $true } }
    Set-Applications @((New-AppEntry b), (New-AppEntry c))
    $snapshot = Assert-Events @(
        'a|install|1.0.0|success', 'b|install|1.0.0|success', 'a|remove|1.0.0|failed', 'c|install|1.0.0|success'
    )
    Assert-ActionStatus $snapshot a Remove FAILED
}

function Test-ChildResourceOperations {
    $a = New-AppEntry a
    Set-ChildApplication a $a
    Set-ChildApplication b (New-AppEntry b)
    Wait-ApplicationView @($a, (New-AppEntry b))
    $id = "$($script:VmId)/vmApplications/$($script:Names.a)"
    Set-ChildApplication a (New-AppEntry a '2.0.0') -Patch
    Wait-ApplicationView @((New-AppEntry a '2.0.0'), (New-AppEntry b))
    Assert-Equal (Get-ArmBody $id -Expand instanceView).properties.instanceView.version '2.0.0' 'Child instance view version'
    $collection = Get-ArmBody "$($script:VmId)/vmApplications"
    Assert-True ($script:Names.anchor -in $collection.value.name) 'The anchor application disappeared from the child collection.'
    Assert-Equal @($collection.value.name | Where-Object { $_ -ne $script:Names.anchor } | Sort-Object) @(@($script:Names.a, $script:Names.b) | Sort-Object) 'Child collection'
    $null = Invoke-Arm PATCH $id @{ tags = @{ lisa = $script:Control.run } }
    Assert-Equal (Get-ArmBody $id).tags.lisa $script:Control.run 'Tag-only child update'
    $null = Invoke-Arm DELETE $id
    Wait-ApplicationView @((New-AppEntry b))
    $null = Assert-Events @(
        'a|install|1.0.0|success', 'b|install|1.0.0|success', 'a|update|2.0.0|success', 'a|remove|2.0.0|success'
    )
}

function Test-RestartPersistence {
    Set-Applications @((New-AppEntry a))
    $boot = (Get-Snapshot).boot
    $null = Invoke-Arm POST "$($script:VmId)/restart"
    Set-Applications @((New-AppEntry a), (New-AppEntry b))
    $snapshot = Assert-Events @('a|install|1.0.0|success', 'b|install|1.0.0|success')
    Assert-True ($snapshot.boot -ne $boot) 'The VM did not reboot.'
}

function Test-RebootBehavior {
    param([bool] $Rerun)
    $boot = (Get-Snapshot).boot
    Set-Control @{ reboot = @{ a = 'install' } }
    $version = if ($Rerun) { '5.0.0' } else { '4.0.0' }
    $outcome = if ($Rerun) { 'success' } else { 'reboot' }
    Set-Applications @((New-AppEntry a $version)) @{ a = $outcome }
    $expected = @("a|install|$version|reboot")
    if ($Rerun) { $expected += "a|install|$version|success" }
    $snapshot = Assert-Events $expected
    Assert-True ($snapshot.boot -ne $boot) 'The VM did not reboot.'
}

$script:TestCases = [ordered]@{
    verify_installation_and_configuration = { Test-InstallationAndConfiguration }
    verify_application_lifecycle = { Test-ApplicationLifecycle }
    verify_ordered_operations = { Test-OrderedOperations }
    verify_ordered_failure_and_recovery = { Test-OrderedFailureAndRecovery }
    verify_failure_policy = { Test-FailurePolicy }
    verify_replace_failure = { Test-ReplaceFailure }
    verify_removal_and_reuse = { Test-RemovalAndReuse }
    verify_remove_failure = { Test-RemoveFailure }
    verify_child_resource_operations = { Test-ChildResourceOperations }
    verify_restart_persistence = { Test-RestartPersistence }
    verify_reboot_without_rerun = { Test-RebootBehavior $false }
    verify_reboot_with_rerun = { Test-RebootBehavior $true }
}
