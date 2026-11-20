#requires -Version 7.3
<#
.SYNOPSIS
Builds and validates the local VM Applications extension on Linux and Windows.
.DESCRIPTION
Requires az login, Go and GitHub CLI. Creates two billable Azure VMs and runs the
PowerShell translations of microsoft/lisa#4891 simultaneously in separate threads.
Installs a real gallery application before replacing the binaries with CustomScript.
.EXAMPLE
.\TestScripts\Test-VmApplications.ps1 -KeepResources
.EXAMPLE
.\TestScripts\Test-VmApplications.ps1 -ResumeFrom <results-directory> -SkipPassed
#>
[CmdletBinding()]
param(
    [ValidateNotNullOrEmpty()][string] $Region = 'EastUS2',
    [ValidateNotNullOrEmpty()][string] $VmSize = 'Standard_D2s_v5',
    [string] $ResumeFrom,
    [switch] $SkipPassed,
    [switch] $KeepResources,
    [string[]] $TestName = @(),
    [ValidateRange(1, 24)][int] $TimeoutHours = 24,
    [Parameter(DontShow)][ValidateSet('', 'Linux', 'Windows')][string] $Worker = '',
    [Parameter(DontShow)][string] $OutputDirectory,
    [Parameter(DontShow)][string] $SubscriptionId,
    [Parameter(DontShow)][string] $Deadline
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:LisaRevision = 'd36ca9e0802637c526fcbf1bea4ac2954d8141a6'
$script:VmApi = '2025-04-01'
$script:GalleryApi = '2024-03-03'
$script:Secrets = [Collections.Generic.List[string]]::new()
$script:Repository = Split-Path $PSScriptRoot -Parent
$script:HarnessPath = $PSCommandPath

function Protect-Text {
    param([string] $Text)
    foreach ($secret in $script:Secrets) { if ($secret) { $Text = $Text.Replace($secret, '<redacted>') } }
    $Text -replace '(?i)([?&](?:sig|skoid|sktid|skt|ske|sks|skv|se|sp|sv|sr)=)[^&\s"''<>]+', '$1<redacted>'
}

function Write-Step {
    param([string] $Message)
    Write-Host "[$(Get-Date -Format HH:mm:ss)][$Worker] $(Protect-Text $Message)"
}

function Save-Json {
    param([string] $Path, $Value)
    [IO.File]::WriteAllText("$Path.tmp", ($Value | ConvertTo-Json -Depth 100), [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath "$Path.tmp" -Destination $Path -Force
}

function Get-RemainingSeconds {
    $remaining = ($script:DeadlineUtc - [DateTime]::UtcNow).TotalSeconds
    if ($remaining -le 0) { throw [TimeoutException]::new('The validation deadline expired.') }
    [int][math]::Ceiling($remaining)
}

function Invoke-Native {
    param(
        [string] $Executable, [string[]] $Arguments, [hashtable] $Environment = @{},
        [int] $TimeoutSeconds = 1800, [switch] $AllowFailure
    )
    $start = [Diagnostics.ProcessStartInfo]::new((Get-Command $Executable -ErrorAction Stop).Source)
    $start.WorkingDirectory = $script:Repository
    $start.UseShellExecute = $false
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    foreach ($key in $Environment.Keys) { $start.Environment[$key] = $Environment[$key] }
    $process = [Diagnostics.Process]::Start($start)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        $timer = [Diagnostics.Stopwatch]::StartNew()
        $limit = [math]::Min($TimeoutSeconds, (Get-RemainingSeconds))
        while (-not $process.WaitForExit(1000)) {
            if ($timer.Elapsed.TotalSeconds -ge $limit) {
                throw [TimeoutException]::new("$Executable exceeded its ${limit}s deadline.")
            }
        }
        $result = @{
            ExitCode = $process.ExitCode
            StdOut = $stdout.GetAwaiter().GetResult()
            StdErr = $stderr.GetAwaiter().GetResult()
        }
        if ($result.ExitCode -ne 0 -and -not $AllowFailure) {
            throw "$Executable failed ($($result.ExitCode)): $(Protect-Text ($result.StdErr + $result.StdOut))"
        }
        $result
    }
    finally {
        if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force; $process.WaitForExit() }
        $process.Dispose()
    }
}

function Invoke-Az {
    param([string[]] $Arguments, [switch] $AllowFailure, [switch] $StorageRetry, [int] $TimeoutSeconds = 1800)
    # Invoke Azure CLI's Python entry point directly: az.cmd loses '&' in URLs.
    $cli = (Get-Command az -ErrorAction Stop).Source
    $python = Join-Path (Split-Path (Split-Path $cli -Parent) -Parent) 'python.exe'
    $argumentsWithDefaults = @('-IBm', 'azure.cli') + $Arguments + @('--only-show-errors', '--output', 'json')
    if ($script:SubscriptionId) { $argumentsWithDefaults += @('--subscription', $script:SubscriptionId) }
    $end = [DateTime]::UtcNow.AddSeconds([math]::Min(600, (Get-RemainingSeconds)))
    do {
        $result = Invoke-Native $python $argumentsWithDefaults -AllowFailure -TimeoutSeconds $TimeoutSeconds
        if (-not $StorageRetry -or $result.ExitCode -eq 0 -or
            $result.StdErr -notmatch 'AuthorizationPermissionMismatch|AuthorizationFailure|\b403\b|do not have the required permissions') { break }
        Write-Step 'Waiting for storage RBAC propagation.'
        Start-Sleep -Seconds 10
    } while ([DateTime]::UtcNow -lt $end)
    if ($AllowFailure) { return $result }
    if ($result.ExitCode -ne 0) { throw "Azure CLI failed: $(Protect-Text ($result.StdErr + $result.StdOut))" }
    if ($result.StdOut.Trim()) { $result.StdOut | ConvertFrom-Json -AsHashtable }
}

function Invoke-ArmRequest {
    param([string] $Method, [string] $Uri, $Body = $null)
    if ($Uri.StartsWith('/')) { $Uri = 'https://management.azure.com' + $Uri }
    $parsed = [uri]$Uri
    if ($parsed.Scheme -ne 'https' -or $parsed.Host -ne 'management.azure.com') {
        throw "Refusing to send ARM credentials to $($parsed.Host)."
    }
    $parameters = @{
        Method = $Method; Uri = $Uri; Headers = @{}
        SkipHttpErrorCheck = $true
    }
    if ($null -ne $Body) {
        $parameters.ContentType = 'application/json'
        $parameters.Body = [Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 100 -Compress))
    }
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        if (-not $script:ArmToken -or [DateTime]::UtcNow -ge $script:TokenExpiry) {
            $token = Invoke-Az @('account', 'get-access-token', '--resource', 'https://management.azure.com/')
            $script:ArmToken = $token.accessToken
            $script:Secrets.Add($script:ArmToken)
            $script:TokenExpiry = if ($token.ContainsKey('expires_on')) {
                [DateTimeOffset]::FromUnixTimeSeconds([long]$token.expires_on).UtcDateTime.AddMinutes(-2)
            } else {
                [DateTime]::Parse($token.expiresOn, [Globalization.CultureInfo]::InvariantCulture).ToUniversalTime().AddMinutes(-2)
            }
        }
        $parameters.Headers.Authorization = "Bearer $($script:ArmToken)"
        $parameters.TimeoutSec = [math]::Min(120, (Get-RemainingSeconds))
        try {
            $response = Invoke-WebRequest @parameters
            break
        }
        catch [System.Threading.Tasks.TaskCanceledException], [System.Net.Http.HttpRequestException], [System.Net.WebException] {
            if ($Method -ne 'GET' -or $attempt -eq 3) { throw }
            Write-Warning "ARM GET transport failure (attempt $attempt/3), retrying: $(Protect-Text $_.Exception.Message)"
            Start-Sleep -Seconds (5 * $attempt)
        }
    }
    @{
        Status = [int]$response.StatusCode
        Body = if ($response.Content) { $response.Content | ConvertFrom-Json -AsHashtable } else { @{} }
        Headers = $response.Headers
    }
}

function Invoke-Arm {
    param(
        [string] $Method, [string] $ResourceId, $Body = $null, [string] $Api = $script:VmApi,
        [switch] $AllowFailure, [switch] $NoWait, [string] $Expand = ''
    )
    $uri = "${ResourceId}?api-version=$Api"
    if ($Expand) { $uri += '&$expand=' + [uri]::EscapeDataString($Expand) }
    $response = Invoke-ArmRequest $Method $uri $Body
    if (-not $NoWait -and $response.Status -in @(200, 201, 202)) {
        $pollUri = $null
        foreach ($header in @('Azure-AsyncOperation', 'Location')) {
            if ($response.Headers.ContainsKey($header)) { $pollUri = [string]$response.Headers[$header][0]; break }
        }
        if ($pollUri) {
            do {
                Start-Sleep -Seconds 5
                $poll = Invoke-ArmRequest GET $pollUri
                if ($poll.Status -ge 400) { $response = $poll; break }
                $state = [string]$poll.Body['status']
                if ($state -in @('Failed', 'Canceled', 'Cancelled')) { $response = $poll; $response.Status = 409; break }
                if ($state -eq 'Succeeded' -or ($poll.Status -eq 200 -and -not $state)) { break }
            } while ($true)
        }
    }
    if (-not $AllowFailure -and $response.Status -ge 400) {
        throw "ARM $Method $ResourceId failed ($($response.Status)): $(Protect-Text ($response.Body | ConvertTo-Json -Depth 30 -Compress))"
    }
    $response
}

function Get-ArmBody {
    param([string] $Id, [string] $Api = $script:VmApi, [string] $Expand = '')
    (Invoke-Arm GET $Id -Api $Api -Expand $Expand).Body
}

function ConvertTo-EncodedJson {
    param($Value)
    [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes(($Value | ConvertTo-Json -Depth 40 -Compress)))
}

function Get-BytesHash {
    param([byte[]] $Bytes)
    [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($Bytes)).ToLowerInvariant()
}

function ConvertTo-PageBytes {
    param([byte[]] $Bytes)
    $padded = [byte[]]::new($Bytes.Length + 512 - $Bytes.Length % 512)
    [Array]::Copy($Bytes, $padded, $Bytes.Length)
    return ,$padded
}

function New-BlobSas {
    param([string] $Name, [string] $Permissions = 'r')
    $sas = Invoke-Az @('storage', 'blob', 'generate-sas', '--account-name', $script:State.storage,
        '--container-name', 'lisa-vmapplications', '--name', $Name, '--auth-mode', 'login',
        '--as-user', '--permissions', $Permissions, '--https-only',
        '--expiry', [DateTime]::UtcNow.AddHours(24).ToString('yyyy-MM-ddTHH:mmZ')) -StorageRetry
    "https://$($script:State.storage).blob.core.windows.net/lisa-vmapplications/${Name}?$sas"
}

function Send-Blob {
    param([string] $Path, [string] $Name, [switch] $Page)
    if (-not $script:Uploaded.Contains($Name)) {
        $type = if ($Page) { 'page' } else { 'block' }
        $null = Invoke-Az @('storage', 'blob', 'upload', '--account-name', $script:State.storage,
            '--container-name', 'lisa-vmapplications', '--name', $Name, '--file', $Path,
            '--type', $type, '--overwrite', 'true', '--auth-mode', 'login') -StorageRetry
        $null = $script:Uploaded.Add($Name)
    }
    New-BlobSas $Name
}

function Get-AppReference {
    param([string] $Alias, [string] $Version = '1.0.0')
    "$($script:GalleryId)/applications/$($script:Names[$Alias])/versions/$Version"
}

function New-AppEntry {
    param([string] $Alias, [string] $Version = '1.0.0', [hashtable] $Properties = @{})
    $entry = @{ packageReferenceId = Get-AppReference $Alias $Version; treatFailureAsDeploymentFailure = $true }
    foreach ($key in $Properties.Keys) { $entry[$key] = $Properties[$key] }
    $entry
}

function New-AnchorEntry {
    New-AppEntry anchor
}

function Get-FixtureFiles {
    param([string] $Alias)
    $name = $script:Names[$Alias]
    $package = if ($Alias -eq 'c') { if ($Worker -eq 'Windows') { 'payload-data.zip' } else { 'payload data.zip' } } else { $name }
    $config = if ($Alias -eq 'c') { if ($Worker -eq 'Windows') { 'settings-data.json' } else { 'settings data.json' } } else { "${name}_config" }
    @{ package = $package; config = $config; config_sha256 = if ($Alias -in @('a', 'c')) { Get-BytesHash $script:DefaultConfig } else { $null } }
}

function Get-FixtureCommand {
    param([string] $Operation, [string] $Alias, [string] $Version, [string] $Action = '')
    if ($Alias -eq 'anchor') {
        if ($Worker -eq 'Linux') {
            if ($Operation -eq 'remove') { return 'rm -f -- /var/lib/vmapplications-local-validation-anchor' }
            return "printf '%s' local-validation-anchor > /var/lib/vmapplications-local-validation-anchor"
        }
        $anchorScript = if ($Operation -eq 'remove') {
            "Remove-Item -LiteralPath 'C:\ProgramData\VmApplicationsLocalValidation\anchor.txt' -Force"
        } else {
            "New-Item -ItemType Directory -Path 'C:\ProgramData\VmApplicationsLocalValidation' -Force | Out-Null; [IO.File]::WriteAllText('C:\ProgramData\VmApplicationsLocalValidation\anchor.txt','local-validation-anchor')"
        }
        return 'powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand ' +
            [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("`$ErrorActionPreference='Stop'; $anchorScript"))
    }
    $files = Get-FixtureFiles $Alias
    $data = ConvertTo-EncodedJson @{
        app = $Alias; name = $script:Names[$Alias]; version = $Version
        package = $files.package; config = $files.config; config_sha256 = $files.config_sha256
        sha256 = $script:PayloadHash; action = $Action
    }
    if ($Worker -eq 'Linux') {
        $bootstrap = "import sys,zipfile;exec(compile(zipfile.ZipFile(sys.argv[1]).read('fixture.py'),'fixture.py','exec'))"
        return "python3 -c `"$bootstrap`" '$($files.package)' $Operation $data"
    }
    $bootstrap = @"
`$ErrorActionPreference='Stop';
Add-Type -AssemblyName System.IO.Compression.FileSystem;
`$z=[IO.Compression.ZipFile]::OpenRead((Join-Path (Get-Location) '$($files.package)'));
try{`$r=New-Object IO.StreamReader(`$z.GetEntry('fixture.ps1').Open());try{`$s=`$r.ReadToEnd()}finally{`$r.Dispose()}}finally{`$z.Dispose()};
& ([scriptblock]::Create(`$s)) -Operation '$Operation' -Data '$data'
"@
    'powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand ' +
        [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($bootstrap))
}

function Initialize-Fixtures {
    $suffix = if ($Worker -eq 'Windows') { 'ps1' } else { 'py' }
    $fixturePath = Join-Path $script:Directory "vm_applications_fixture.$suffix"
    if (-not (Test-Path -LiteralPath $fixturePath)) {
        $text = (Invoke-Native gh @('api',
            "repos/microsoft/lisa/contents/lisa/microsoft/testsuites/vm_extensions/runtime_extensions/scripts/vm_applications_fixture.${suffix}?ref=$($script:LisaRevision)",
            '-H', 'Accept: application/vnd.github.raw+json')).StdOut
        [IO.File]::WriteAllText($fixturePath, ($text -replace "`r", ''), [Text.UTF8Encoding]::new($false))
    }
    $script:FixtureScript = [IO.File]::ReadAllText($fixturePath) -replace "`r", ''
    $stream = [IO.MemoryStream]::new()
    $archive = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $true)
    try {
        $entry = $archive.CreateEntry("fixture.$suffix", [IO.Compression.CompressionLevel]::NoCompression)
        $entry.LastWriteTime = [DateTimeOffset]::new(1980, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
        $entryStream = $entry.Open()
        try { $bytes = [Text.Encoding]::UTF8.GetBytes($script:FixtureScript); $entryStream.Write($bytes) }
        finally { $entryStream.Dispose() }
    }
    finally { $archive.Dispose() }
    $payload = ConvertTo-PageBytes $stream.ToArray()
    $stream.Dispose()
    $script:DefaultConfig = ConvertTo-PageBytes ([Text.Encoding]::ASCII.GetBytes("LISA default configuration`r`n") + [byte[]]@(0, 1, 255))
    $script:OverrideConfig = [Text.Encoding]::ASCII.GetBytes("LISA override configuration`n") + [byte[]]@(0, 2, 254)
    $script:PayloadHash = Get-BytesHash $payload
    # Publishing commands are immutable too; bump this revision when they change.
    $script:FixtureDigest = Get-BytesHash ($payload + [Text.Encoding]::ASCII.GetBytes('3') + $script:DefaultConfig)
    $script:Names = @{}
    foreach ($alias in @('a', 'b', 'c', 'd', 'other', 'anchor')) {
        $script:Names[$alias] = "lisa-vmapps-$($Worker.ToLowerInvariant())-$alias-$($script:FixtureDigest.Substring(0, 8))"
    }
    $prefix = "$($Worker.ToLowerInvariant())/$($script:FixtureDigest)"
    $payloadPath = Join-Path $script:Directory 'package.zip'
    $configPath = Join-Path $script:Directory 'default.bin'
    $overridePath = Join-Path $script:Directory 'override.bin'
    [IO.File]::WriteAllBytes($payloadPath, $payload)
    [IO.File]::WriteAllBytes($configPath, $script:DefaultConfig)
    [IO.File]::WriteAllBytes($overridePath, $script:OverrideConfig)
    $script:OverrideUri = Send-Blob $overridePath "$prefix/override.bin"
    $packageUri = Send-Blob $payloadPath "$prefix/package.zip" -Page
    $configUri = Send-Blob $configPath "$prefix/default.bin" -Page
    $null = Invoke-Arm PUT $script:GalleryId @{
        location = $script:State.region; properties = @{ description = 'Disposable local VM Applications validation' }
    } -Api $script:GalleryApi
    $pending = [Collections.Generic.List[string]]::new()
    foreach ($alias in @('a', 'b', 'c', 'd', 'other', 'anchor')) {
        $appId = "$($script:GalleryId)/applications/$($script:Names[$alias])"
        $os = if ($alias -eq 'other') { if ($Worker -eq 'Windows') { 'Linux' } else { 'Windows' } } else { $Worker }
        $null = Invoke-Arm PUT $appId @{
            location = $script:State.region; properties = @{ supportedOSType = $os }
        } -Api $script:GalleryApi
        $versions = if ($alias -eq 'a') { @('1.0.0', '2.0.0', '3.0.0', '4.0.0', '5.0.0') } else { @('1.0.0') }
        foreach ($version in $versions) {
            $id = Get-AppReference $alias $version
            $existing = Invoke-Arm GET $id -Api $script:GalleryApi -AllowFailure
            if ($existing.Status -eq 404) {
                Write-Step "Publishing $alias/$version to PIR/CAPS."
                $actions = @{}
                foreach ($operation in @('install', 'remove', 'update')) { $actions[$operation] = Get-FixtureCommand $operation $alias $version }
                if ($alias -eq 'a' -and $version -eq '3.0.0') { $actions.Remove('update') }
                $source = @{ mediaLink = $packageUri }
                if ($alias -in @('a', 'c')) { $source.defaultConfigurationLink = $configUri }
                $settings = @{ scriptBehaviorAfterReboot = if ($alias -eq 'a' -and $version -eq '5.0.0') { 'Rerun' } else { 'None' } }
                if ($alias -eq 'c') { $files = Get-FixtureFiles $alias; $settings.packageFileName = $files.package; $settings.configFileName = $files.config }
                $publishing = @{
                    source = $source; manageActions = $actions; replicaCount = 1; settings = $settings
                    targetRegions = @(@{ name = $script:State.region; regionalReplicaCount = 1 })
                }
                $null = Invoke-Arm PUT $id @{
                    location = $script:State.region; tags = @{ lisa_fixture = $script:FixtureDigest }
                    properties = @{ publishingProfile = $publishing }
                } -Api $script:GalleryApi -NoWait
            } elseif ($existing.Status -ne 200 -or $existing.Body.tags.lisa_fixture -ne $script:FixtureDigest) {
                throw "Existing fixture does not match: $id (HTTP $($existing.Status))."
            }
            $pending.Add($id)
        }
    }
    $end = [DateTime]::UtcNow.AddMinutes(30)
    $lastReport = [DateTime]::MinValue
    while ($pending.Count) {
        foreach ($id in @($pending.ToArray())) {
            $properties = (Get-ArmBody $id $script:GalleryApi 'ReplicationStatus').properties
            $replication = $properties['replicationStatus']
            if ($properties.provisioningState -eq 'Failed' -or ($replication -and $replication.aggregatedState -eq 'Failed')) {
                throw "PIR/CAPS replication failed: $id"
            }
            if ($properties.provisioningState -eq 'Succeeded' -and $replication -and $replication.aggregatedState -eq 'Completed') { $null = $pending.Remove($id) }
        }
        if ($pending.Count) {
            if ([DateTime]::UtcNow -ge $end) { throw "PIR/CAPS replication timed out: $($pending -join ', ')" }
            if (([DateTime]::UtcNow - $lastReport).TotalSeconds -ge 60) {
                Write-Step "Waiting for $($pending.Count) gallery versions to replicate."
                $lastReport = [DateTime]::UtcNow
            }
            Start-Sleep -Seconds 15
        }
    }
}

function Invoke-Guest {
    param([string] $Operation, [hashtable] $Properties = @{})
    $marker = "VMAPPS-RC-$([guid]::NewGuid().ToString('N')):"
    $data = @{} + $script:Control + $Properties
    $data.marker = $marker
    $encoded = ConvertTo-EncodedJson $data
    $source = if ($Worker -eq 'Windows') {
        "`$ErrorActionPreference='Stop'`n& {`n$($script:FixtureScript)`n} -Operation '$Operation' -Data '$encoded'"
    } else {
        "set -eu`npython3 - $Operation $encoded <<'VMAPPS_FIXTURE'`n$($script:FixtureScript)`nVMAPPS_FIXTURE`n"
    }
    $id = "$($script:VmId)/runCommands/vmapps-validation"
    $null = Invoke-Arm PUT $id @{
        location = $script:State.region
        properties = @{ source = @{ script = $source }; timeoutInSeconds = 120; treatFailureAsDeploymentFailure = $true }
    }
    $view = (Get-ArmBody $id -Expand instanceView).properties.instanceView
    Assert-Equal $view.executionState 'Succeeded' "Managed Run Command $Operation state"
    Assert-Equal $view.exitCode 0 "Managed Run Command $Operation exit code: $($view['error'])"
    $lines = @(([string]$view['output']) -split '\r?\n' | Where-Object { $_.StartsWith($marker) })
    Assert-Equal $lines.Count 1 "Fresh output marker for $Operation"
    $lines[0].Substring($marker.Length) | ConvertFrom-Json -AsHashtable
}

function Set-Control {
    param([hashtable] $Changes = @{})
    foreach ($key in $Changes.Keys) { $script:Control[$key] = $Changes[$key] }
    $null = Invoke-Guest configure
}

function Get-Snapshot {
    $snapshot = Invoke-Guest inspect
    Save-Json (Join-Path $script:Directory "fixture-snapshot-$($script:Control.run).json") $snapshot
    Assert-Equal $snapshot.run $script:Control.run 'Fresh fixture run'
    Assert-Equal $snapshot.unrelated (Get-BytesHash ([Text.Encoding]::ASCII.GetBytes("LISA VMApplications unrelated file`n"))) 'Unrelated sentinel'
    if ($null -eq $snapshot['events'] -or $null -eq $snapshot['states']) { throw 'Fixture snapshot is missing events or states.' }
    $snapshot
}

function New-TestContext {
    $script:Control = @{
        vm_id = $script:VmId; run = [guid]::NewGuid().ToString('N')
        failures = @{}; configuration = @{}; requires = @{}; reboot = @{}; mutate = @()
        parameters = @{ LISA_REQUIRED = "literal ' `" `$ ; & $([char]0x2603)"; LISA_OPTIONAL = 'default-value' }
    }
    Set-Control
}

function Get-CseDownloadPath {
    param([string] $Uri)
    $path = [uri]::UnescapeDataString(([uri]$Uri).AbsolutePath.TrimStart('/'))
    if ($Worker -eq 'Windows') { return ($path.Split('/', 2)[1]).Replace('/', '\') }
    $path.Split('/')[-1]
}

function ConvertFrom-CustomScriptView {
    param([hashtable] $View)
    $messages = (@($View['statuses']) + @($View['substatuses']) |
        Where-Object { $null -ne $_ } | ForEach-Object { $_['message'] }) -join "`n"
    $matches = @([regex]::Matches($messages, 'VMAPPS-VALIDATION:(\{[^\r\n]*\})') |
        ForEach-Object { $_.Groups[1].Value } | Select-Object -Unique)
    Assert-Equal $matches.Count 1 'CustomScript fresh success marker'
    $matches[0] | ConvertFrom-Json -AsHashtable
}

function Invoke-CustomScript {
    param([string] $Action)
    $suffix = if ($Worker -eq 'Windows') { 'ps1' } else { 'py' }
    $guestPath = Join-Path $PSScriptRoot "Invoke-VmApplicationsGuest.$suffix"
    $hash = (Get-FileHash -LiteralPath $guestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $uri = Send-Blob $guestPath "harness/$hash/Invoke-VmApplicationsGuest.$suffix"
    $uris = @($uri)
    $config = @{ action = $Action }
    if ($Action -in @('install', 'verify')) { $config.directory = $script:Installed.directory; $config.hashes = $script:Hashes }
    if ($Action -eq 'install') {
        $uris += $script:BinaryUris.Values
        $config.sources = @{}
        foreach ($name in $script:BinaryUris.Keys) { $config.sources[$name] = Get-CseDownloadPath $script:BinaryUris[$name] }
    }
    if ($Action -eq 'collect') {
        $name = "diagnostics/$([guid]::NewGuid().ToString('N')).$(if ($Worker -eq 'Windows') { 'zip' } else { 'tar.gz' })"
        $config.upload_uri = New-BlobSas $name 'cw'
    }
    $encoded = ConvertTo-EncodedJson $config
    $command = if ($Worker -eq 'Windows') {
        "powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$(Get-CseDownloadPath $uri)`" -Data $encoded"
    } else { "python3 Invoke-VmApplicationsGuest.py $encoded" }
    Write-Step "CustomScript: $Action."
    $id = "$($script:VmId)/extensions/VmApplicationsValidationCSE"
    $null = Invoke-Arm PUT $id @{
        location = $script:State.region
        properties = @{
            publisher = if ($Worker -eq 'Windows') { 'Microsoft.Compute' } else { 'Microsoft.Azure.Extensions' }
            type = if ($Worker -eq 'Windows') { 'CustomScriptExtension' } else { 'CustomScript' }
            typeHandlerVersion = if ($Worker -eq 'Windows') { '1.10' } else { '2.1' }
            autoUpgradeMinorVersion = $true; forceUpdateTag = [guid]::NewGuid().ToString()
            protectedSettings = @{ fileUris = $uris; commandToExecute = $command }
        }
    }
    $view = (Get-ArmBody $id -Expand instanceView).properties.instanceView
    $result = ConvertFrom-CustomScriptView $view
    if ($Action -eq 'collect') {
        $path = Join-Path $script:Directory (Split-Path $name -Leaf)
        $null = Invoke-Az @('storage', 'blob', 'download', '--account-name', $script:State.storage,
            '--container-name', 'lisa-vmapplications', '--name', $name, '--file', $path, '--auth-mode', 'login')
        Write-Step "Downloaded diagnostics: $path"
    }
    $result
}

function Clear-FailedCustomScript {
    $id = "$($script:VmId)/extensions/VmApplicationsValidationCSE"
    $response = Invoke-Arm GET $id -AllowFailure
    if ($response.Status -eq 404) { return }
    if ($response.Status -ne 200) { throw "Cannot inspect CSE: HTTP $($response.Status)." }
    if ($response.Body.properties.provisioningState -eq 'Failed') {
        Write-Step 'Removing failed harness CSE before further VM updates.'
        $null = Invoke-Arm DELETE $id
    }
}

function Save-Diagnostics {
    try { $null = Invoke-CustomScript collect }
    catch { Write-Warning "Diagnostic collection failed: $(Protect-Text ($_ | Out-String))" }
    finally { Clear-FailedCustomScript }
}

function Build-AndInstall {
    $script:Installed = Invoke-CustomScript discover
    $version = $script:Installed.version
    $identity = $script:Installed.extension_name
    if ($version -notmatch '^\d+\.\d+\.\d+(\.\d+)?$' -or
        $identity -notmatch "^Microsoft\.CPlat\.Core(\.EDP)?\.VMApplicationManager$Worker$") { throw 'Unexpected installed handler identity/version.' }
    $script:Hashes = @{}
    $script:BinaryUris = @{}
    $extension = if ($Worker -eq 'Windows') { '.exe' } else { '' }
    foreach ($build in @(@('vm-application-manager', 'main'), @('extension-launcher', 'launcher'))) {
        $name = $build[0] + $extension
        $path = Join-Path $script:Directory $name
        $flags = "-X main.ExtensionName=$identity -X main.ExtensionVersion=$version"
        if ($build[1] -eq 'launcher') { $flags += " -X main.ExecutableName=vm-application-manager$extension" }
        Write-Step "Building $Worker/amd64 $name for $identity $version."
        $null = Invoke-Native go @('build', '-ldflags', $flags, '-o', $path, ".\$($build[1])") `
            -Environment @{ GOOS = $Worker.ToLowerInvariant(); GOARCH = 'amd64'; CGO_ENABLED = '0' }
        $script:Hashes[$name] = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        $script:BinaryUris[$name] = Send-Blob $path "harness/$($script:Hashes[$name])/$name"
    }
    $null = Invoke-CustomScript install
    $null = Invoke-CustomScript verify
    Save-Json (Join-Path $script:Directory 'installed-binaries.json') @{
        directory = $script:Installed.directory; version = $version; extension_name = $identity
        hashes = $script:Hashes; source_digest = $script:SourceDigest
    }
}

. (Join-Path $PSScriptRoot 'VmApplications.Tests.ps1')
. (Join-Path $PSScriptRoot 'VmApplications.Infrastructure.ps1')

function Start-Worker {
    $script:SubscriptionId = $SubscriptionId
    $script:DeadlineUtc = [DateTime]::Parse($Deadline).ToUniversalTime()
    $script:Directory = Join-Path $OutputDirectory $Worker.ToLowerInvariant()
    $null = New-Item -ItemType Directory -Path $script:Directory -Force
    $script:StatePath = Join-Path $script:Directory 'state.json'
    $script:ArmToken = ''
    $script:Uploaded = [Collections.Generic.HashSet[string]]::new()
    $script:OwnsGroup = $false
    $script:ProfileStarted = $false
    $transcript = Join-Path $script:Directory "powershell-$([DateTime]::UtcNow.ToString('yyyyMMddTHHmmss')).log"
    $null = Start-Transcript -LiteralPath $transcript
    try {
        Initialize-Infrastructure
        Clear-FailedCustomScript
        Initialize-Fixtures
        $script:SourceDigest = Get-BytesHash ([Text.Encoding]::UTF8.GetBytes("$($script:SourceDigest):$($script:FixtureDigest)"))
        New-TestContext
        $script:ProfileStarted = $true
        # CRP uninstalls the handler for an empty profile. Retain a separate,
        # audited anchor so every test continues using the replaced binaries.
        $null = Invoke-Arm PATCH $script:VmId @{ location = $script:State.region; properties = @{
            applicationProfile = @{ galleryApplications = @((New-AnchorEntry)) }
        }}
        $null = Invoke-Guest cleanup
        New-TestContext
        $null = Assert-Events @()
        Build-AndInstall
        $script:ResultsPath = Join-Path $script:Directory 'powershell-results.json'
        $script:Results = @{}
        if (Test-Path -LiteralPath $script:ResultsPath) {
            $previous = Get-Content -LiteralPath $script:ResultsPath -Raw | ConvertFrom-Json -AsHashtable
            if ($previous.source_digest -eq $script:SourceDigest) { $script:Results = $previous.cases }
        }
        foreach ($test in $script:TestCases.GetEnumerator()) {
            if ($SkipPassed -and $script:Results.ContainsKey($test.Key) -and $script:Results[$test.Key].status -eq 'passed') {
                Write-Step "Temporarily skipping previously passed $($test.Key)."
                continue
            }
            Write-Step "TEST $($test.Key)"
            $timer = [Diagnostics.Stopwatch]::StartNew()
            $failure = ''
            try {
                New-TestContext
                $null = Invoke-CustomScript verify
                & $test.Value
                $null = Invoke-CustomScript verify
            }
            catch {
                $failure = Protect-Text ($_ | Format-List * -Force | Out-String)
                Write-Warning "FAIL $($test.Key): $failure"
                try { $null = Invoke-CustomScript verify }
                catch { $failure += Protect-Text ($_ | Out-String); Write-Warning "Local binary verification also failed: $(Protect-Text ($_ | Out-String))" }
                Save-Diagnostics
            }
            finally {
                try { Set-Control @{ failures = @{} }; Set-Applications @(); $null = Invoke-Guest cleanup }
                catch { throw "Test cleanup failed; refusing to reuse this VM. $(Protect-Text ($_ | Out-String))" }
            }
            $script:Results[$test.Key] = @{ status = if ($failure) { 'failed' } else { 'passed' }; error = $failure; duration_seconds = $timer.Elapsed.TotalSeconds }
            Save-Json $script:ResultsPath @{ source_digest = $script:SourceDigest; lisa_revision = $script:LisaRevision; cases = $script:Results }
            Write-Step "$(if ($failure) { 'FAIL' } else { 'PASS' }) $($test.Key)"
        }
        $failed = @($script:TestCases.Keys | Where-Object { -not $script:Results.ContainsKey($_) -or $script:Results[$_].status -ne 'passed' })
        if ($failed.Count) { throw "Failed tests: $($failed -join ', ')" }
        Write-Step "All $($script:TestCases.Count) functional tests passed."
    }
    catch {
        Write-Warning (Protect-Text "$($_.Exception.Message)`n$($_.ScriptStackTrace)")
        throw
    }
    finally {
        if ($script:OwnsGroup) {
            if ($KeepResources) { Write-Warning "Keeping $($script:State.group); charges continue." }
            else {
                Write-Step "Deleting owned group $($script:State.group)."
                $null = Invoke-Arm DELETE "/subscriptions/$SubscriptionId/resourceGroups/$($script:State.group)" -Api '2022-09-01'
                $script:State.provisioned = $false
                Save-Json $script:StatePath $script:State
            }
        }
        $null = Stop-Transcript
    }
}

function Start-Validation {
    foreach ($tool in @('go', 'az', 'gh', 'Start-ThreadJob')) { $null = Get-Command $tool -ErrorAction Stop }
    if ($SkipPassed -and -not $ResumeFrom) { throw '-SkipPassed requires -ResumeFrom and retained VMs.' }
    $script:DeadlineUtc = [DateTime]::UtcNow.AddHours($TimeoutHours)
    $script:SubscriptionId = ''
    $account = Invoke-Az @('account', 'show')
    $output = if ($ResumeFrom) { (Resolve-Path -LiteralPath $ResumeFrom).Path } else {
        Join-Path $script:Repository "bundle\functional-validation\$([DateTime]::UtcNow.ToString('yyyyMMddTHHmmss'))-$([guid]::NewGuid().ToString('N').Substring(0, 8))"
    }
    $null = New-Item -ItemType Directory -Path $output -Force
    $lock = [IO.File]::Open((Join-Path $output 'validation.lock'), 'OpenOrCreate', 'ReadWrite', 'None')
    $jobs = @()
    try {
        Write-Host "Results: $output"
        Write-Warning "Subscription $($account.id): TWO billable VMs. No inbound ports are opened."
        foreach ($os in @('Linux', 'Windows')) {
            $arguments = @{
                Worker = $os; OutputDirectory = $output; SubscriptionId = $account.id
                Deadline = $script:DeadlineUtc.ToString('o'); Region = $Region; VmSize = $VmSize
                KeepResources = $KeepResources.IsPresent; SkipPassed = $SkipPassed.IsPresent
                TestName = $TestName
            }
            $jobs += Start-ThreadJob -Name "VmApplications-$os" -ArgumentList $script:HarnessPath, $arguments -ScriptBlock {
                param($Path, $Parameters)
                & $Path @Parameters
            }
        }
        $drained = [Collections.Generic.HashSet[int]]::new()
        while (@($jobs | Where-Object State -In @('NotStarted', 'Running')).Count) {
            foreach ($job in $jobs) {
                if (-not $drained.Contains($job.Id)) {
                    $job | Receive-Job -ErrorAction Continue
                    if ($job.State -notin @('NotStarted', 'Running')) { $null = $drained.Add($job.Id) }
                }
            }
            $null = Get-RemainingSeconds
            Start-Sleep -Seconds 5
        }
        $jobs | Where-Object { -not $drained.Contains($_.Id) } | Receive-Job -ErrorAction Continue
        if (@($jobs | Where-Object State -NE Completed).Count) { throw "Validation failed; see $output." }
        Write-Host "All selected Linux and Windows VM Applications functional tests passed ($($script:TestCases.Count) per OS)." -ForegroundColor Green
    }
    finally {
        $jobs | Where-Object State -In @('NotStarted', 'Running') | Stop-Job
        $jobs | Remove-Job -Force
        $lock.Dispose()
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    if ($TestName.Count) {
        $selected = [ordered]@{}
        foreach ($name in $TestName) {
            if (-not $script:TestCases.Contains($name)) { throw "Unknown test: $name" }
            $selected[$name] = $script:TestCases[$name]
        }
        $script:TestCases = $selected
    }
    if ($Worker) { Start-Worker } else { Start-Validation }
}
