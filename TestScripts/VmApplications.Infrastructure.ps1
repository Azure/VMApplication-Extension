# Copyright (c) Microsoft Corporation.
# Licensed under the MIT license.

function New-InfrastructureTemplate {
    param([string] $PrincipalId, [string] $PrincipalType)
    $location = $script:State.region
    $storage = $script:State.storage
    $profile = @{
        computerName = 'vmapps-test'; adminUsername = 'vmappsadmin'
        adminPassword = "[parameters('adminPassword')]"
    }
    if ($Worker -eq 'Linux') {
        $profile.linuxConfiguration = @{ disablePasswordAuthentication = $false; provisionVMAgent = $true }
        $image = @{ publisher = 'Canonical'; offer = '0001-com-ubuntu-server-jammy'; sku = '22_04-lts-gen2'; version = 'latest' }
    } else {
        $profile.windowsConfiguration = @{ provisionVMAgent = $true; enableAutomaticUpdates = $true }
        $image = @{ publisher = 'MicrosoftWindowsServer'; offer = 'WindowsServer'; sku = '2022-datacenter-azure-edition'; version = 'latest' }
    }
    @{
        '$schema' = 'https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#'
        contentVersion = '1.0.0.0'; parameters = @{ adminPassword = @{ type = 'secureString' } }
        resources = @(
            @{
                type = 'Microsoft.Network/virtualNetworks'; apiVersion = '2024-05-01'; name = 'validation'; location = $location
                properties = @{
                    addressSpace = @{ addressPrefixes = @('10.77.0.0/16') }
                    subnets = @(@{ name = 'validation'; properties = @{ addressPrefix = '10.77.0.0/24'; defaultOutboundAccess = $false } })
                }
            },
            @{
                type = 'Microsoft.Network/publicIPAddresses'; apiVersion = '2024-05-01'; name = 'validation'; location = $location
                sku = @{ name = 'Standard' }; properties = @{ publicIPAllocationMethod = 'Static' }
            },
            @{
                type = 'Microsoft.Network/networkSecurityGroups'; apiVersion = '2024-05-01'; name = 'validation'; location = $location
                properties = @{ securityRules = @() }
            },
            @{
                type = 'Microsoft.Network/networkInterfaces'; apiVersion = '2024-05-01'; name = 'validation'; location = $location
                dependsOn = @(
                    "[resourceId('Microsoft.Network/virtualNetworks', 'validation')]",
                    "[resourceId('Microsoft.Network/publicIPAddresses', 'validation')]",
                    "[resourceId('Microsoft.Network/networkSecurityGroups', 'validation')]"
                )
                properties = @{
                    networkSecurityGroup = @{ id = "[resourceId('Microsoft.Network/networkSecurityGroups', 'validation')]" }
                    ipConfigurations = @(@{ name = 'primary'; properties = @{
                        privateIPAllocationMethod = 'Dynamic'
                        subnet = @{ id = "[resourceId('Microsoft.Network/virtualNetworks/subnets', 'validation', 'validation')]" }
                        publicIPAddress = @{ id = "[resourceId('Microsoft.Network/publicIPAddresses', 'validation')]" }
                    } })
                }
            },
            @{
                type = 'Microsoft.Compute/virtualMachines'; apiVersion = '2024-07-01'; name = 'vmapps-test'; location = $location
                dependsOn = @("[resourceId('Microsoft.Network/networkInterfaces', 'validation')]")
                properties = @{
                    hardwareProfile = @{ vmSize = $VmSize }; osProfile = $profile
                    storageProfile = @{ imageReference = $image; osDisk = @{ createOption = 'FromImage'; managedDisk = @{ storageAccountType = 'Premium_LRS' } } }
                    networkProfile = @{ networkInterfaces = @(@{ id = "[resourceId('Microsoft.Network/networkInterfaces', 'validation')]" }) }
                    securityProfile = @{ securityType = 'Standard' }
                }
            },
            @{
                type = 'Microsoft.Storage/storageAccounts'; apiVersion = '2023-01-01'; name = $storage; location = $location
                kind = 'StorageV2'; sku = @{ name = 'Standard_LRS' }
                properties = @{
                    allowSharedKeyAccess = $false; allowBlobPublicAccess = $false; defaultToOAuthAuthentication = $true
                    supportsHttpsTrafficOnly = $true; minimumTlsVersion = 'TLS1_2'
                }
            },
            @{
                type = 'Microsoft.Authorization/roleAssignments'; apiVersion = '2022-04-01'
                name = "[guid(resourceId('Microsoft.Storage/storageAccounts', '$storage'), '$PrincipalId')]"
                scope = "[resourceId('Microsoft.Storage/storageAccounts', '$storage')]"
                dependsOn = @("[resourceId('Microsoft.Storage/storageAccounts', '$storage')]")
                properties = @{
                    principalId = $PrincipalId; principalType = $PrincipalType
                    roleDefinitionId = "[subscriptionResourceId('Microsoft.Authorization/roleDefinitions', 'ba92f5b4-2d11-453d-a403-e96b0029c9fe')]"
                }
            }
        )
    }
}

function Initialize-Infrastructure {
    if (Test-Path -LiteralPath $script:StatePath) {
        $script:State = Get-Content -LiteralPath $script:StatePath -Raw | ConvertFrom-Json -AsHashtable
    } else {
        $suffix = [guid]::NewGuid().ToString('N').Substring(0, 12)
        $script:State = @{
            subscription = $SubscriptionId; region = $Region.ToLowerInvariant(); os = $Worker
            owner = [guid]::NewGuid().ToString('N'); group = "rg-vmapps-validation-$($Worker.ToLowerInvariant())-$suffix"
            storage = "vmappval$suffix"; vm = 'vmapps-test'; lisa_revision = $script:LisaRevision
        }
        Save-Json $script:StatePath $script:State
    }
    foreach ($pair in @(@('subscription', $SubscriptionId), @('region', $Region.ToLowerInvariant()), @('os', $Worker))) {
        Assert-Equal $script:State[$pair[0]] $pair[1] "Resume $($pair[0])"
    }
    if ($SkipPassed -and -not $script:State['provisioned']) { throw '-SkipPassed requires VMs retained with -KeepResources.' }
    $script:VmId = "/subscriptions/$SubscriptionId/resourceGroups/$($script:State.group)/providers/Microsoft.Compute/virtualMachines/$($script:State.vm)"
    $script:GalleryId = "/subscriptions/$SubscriptionId/resourceGroups/$($script:State.group)/providers/Microsoft.Compute/galleries/vmappsval$($script:State.owner.Substring(0, 12))"
    $groupId = "/subscriptions/$SubscriptionId/resourceGroups/$($script:State.group)"
    $group = Invoke-Arm GET $groupId -Api '2022-09-01' -AllowFailure
    if ($group.Status -eq 404) {
        $script:OwnsGroup = $true
        $null = Invoke-Arm PUT $groupId @{
            location = $script:State.region; tags = @{ VmApplicationsValidation = $script:State.owner }
        } -Api '2022-09-01'
    } elseif ($group.Status -eq 200 -and $group.Body.tags.VmApplicationsValidation -eq $script:State.owner) {
        $script:OwnsGroup = $true
    } else { throw "Refusing to use unowned or inaccessible group $($script:State.group)." }
    if (-not $script:State['provisioned']) {
        $token = Invoke-Az @('account', 'get-access-token', '--resource', 'https://management.azure.com/')
        $segment = $token.accessToken.Split('.')[1].Replace('-', '+').Replace('_', '/')
        $segment = $segment.PadRight($segment.Length + (4 - $segment.Length % 4) % 4, '=')
        $claims = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($segment)) | ConvertFrom-Json -AsHashtable
        $type = if ($claims['idtyp'] -eq 'app') { 'ServicePrincipal' } else { 'User' }
        $template = New-InfrastructureTemplate $claims.oid $type
        $password = 'Vma!' + [Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(30))
        $script:Secrets.Add($password)
        Write-Step "Provisioning $Worker VM and private storage in $($script:State.region)."
        $deploymentId = "$groupId/providers/Microsoft.Resources/deployments/validation-infrastructure"
        $null = Invoke-Arm PUT $deploymentId @{
            properties = @{ mode = 'Incremental'; template = $template; parameters = @{ adminPassword = @{ value = $password } } }
        } -Api '2022-09-01'
        $deployment = Get-ArmBody $deploymentId '2022-09-01'
        Write-Step "Deployment correlation ID: $($deployment.properties.correlationId)"
        $script:State.provisioned = $true
        Save-Json $script:StatePath $script:State
    } else {
        $null = Get-ArmBody $script:VmId
        Write-Step "Reusing owned VM $($script:State.group)/$($script:State.vm)."
    }
    $null = Invoke-Az @('storage', 'container', 'create', '--account-name', $script:State.storage,
        '--name', 'lisa-vmapplications', '--public-access', 'off', '--auth-mode', 'login') -StorageRetry
    $hasher = [Security.Cryptography.IncrementalHash]::CreateHash([Security.Cryptography.HashAlgorithmName]::SHA256)
    try {
        $hasher.AppendData([Text.Encoding]::UTF8.GetBytes($script:LisaRevision))
        $paths = @((Join-Path $script:Repository 'go.mod'), (Join-Path $script:Repository 'go.sum'))
        foreach ($folder in @('main', 'launcher', 'internal', 'pkg', 'misc', 'TestScripts')) {
            $paths += Get-ChildItem (Join-Path $script:Repository $folder) -Recurse -File |
                Where-Object Extension -In @('.go', '.ps1', '.py', '.json', '.sh', '.cmd') | Select-Object -ExpandProperty FullName
        }
        foreach ($path in ($paths | Sort-Object)) {
            $hasher.AppendData([Text.Encoding]::UTF8.GetBytes([IO.Path]::GetRelativePath($script:Repository, $path)))
            $hasher.AppendData([IO.File]::ReadAllBytes($path))
        }
        $script:SourceDigest = [Convert]::ToHexString($hasher.GetHashAndReset()).ToLowerInvariant()
    }
    finally { $hasher.Dispose() }
}
