# Copyright (c) Microsoft Corporation.
# Licensed under the MIT license.
param([Parameter(Mandatory)][string] $Data)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$config = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($Data)) | ConvertFrom-Json
$roots = @(Get-ChildItem 'C:\Packages\Plugins' -Directory |
    Where-Object Name -Match '^Microsoft\.CPlat\.Core(\.Edp)?\.VMApplicationManagerWindows$')
$candidates = @($roots | Get-ChildItem -Directory |
    Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'bin\vm-application-manager.exe') })
if ($candidates.Count -ne 1) {
    $inventory = @($candidates | ForEach-Object { $_.FullName }) -join ', '
    throw "Expected one VM Applications handler directory; found $($candidates.Count): $inventory"
}
$directory = $candidates[0].FullName
if ($config.action -ne 'collect') {
    $anchor = 'C:\ProgramData\VmApplicationsLocalValidation\anchor.txt'
    if (-not (Test-Path -LiteralPath $anchor) -or [IO.File]::ReadAllText($anchor) -ne 'local-validation-anchor') {
        throw 'The bootstrap anchor application is not installed.'
    }
}
switch ($config.action) {
    'discover' { }
    'collect' {
        $archive = Join-Path $env:TEMP 'vmapplications-validation.zip'
        try {
            $paths = @(
                (Join-Path $directory 'Status'),
                (Join-Path $directory 'RuntimeSettings'),
                (Join-Path 'C:\WindowsAzure\Logs\Plugins' $candidates[0].Parent.Name),
                'C:\ProgramData\lisa-vmapplications',
                'C:\WindowsAzure\Logs\WaAppAgent.log'
            ) | Where-Object { Test-Path -LiteralPath $_ }
            Add-Type -AssemblyName System.IO.Compression
            $stream = [IO.File]::Open($archive, [IO.FileMode]::Create)
            $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create)
            try {
                foreach ($path in $paths) {
                    $files = if (Test-Path -LiteralPath $path -PathType Container) {
                        Get-ChildItem -LiteralPath $path -Recurse -File
                    } else { Get-Item -LiteralPath $path }
                    foreach ($file in $files) {
                        $entry = $zip.CreateEntry(($file.FullName -replace '^C:\\', '').Replace('\', '/'))
                        $inputStream = [IO.File]::Open($file.FullName, [IO.FileMode]::Open,
                            [IO.FileAccess]::Read, ([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete))
                        try {
                            $outputStream = $entry.Open()
                            try { $inputStream.CopyTo($outputStream) }
                            finally { $outputStream.Dispose() }
                        }
                        finally { $inputStream.Dispose() }
                    }
                }
            }
            finally { $zip.Dispose() }
            Invoke-WebRequest -UseBasicParsing -Uri $config.upload_uri -Method Put -InFile $archive `
                -TimeoutSec 120 -Headers @{ 'x-ms-blob-type' = 'BlockBlob' } | Out-Null
        }
        finally { if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force } }
    }
    { $_ -in @('install', 'verify') } {
        if ($directory -ne $config.directory) { throw 'The VM agent changed the active handler directory.' }
        foreach ($property in $config.hashes.PSObject.Properties) {
            $name = $property.Name
            $destination = Join-Path $directory "bin\$name"
            if ($config.action -eq 'install') {
                $source = $config.sources.$name
                if ((Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash -ne $property.Value) {
                    throw "Downloaded binary hash mismatch: $name"
                }
                Copy-Item -LiteralPath $source -Destination "$destination.validation-new" -Force
                Move-Item -LiteralPath "$destination.validation-new" -Destination $destination -Force
            }
            if ((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash -ne $property.Value) {
                throw "Installed binary hash mismatch: $destination"
            }
        }
    }
    default { throw "Unknown action: $($config.action)" }
}
Write-Output ('VMAPPS-VALIDATION:' + (@{
    directory = $directory; version = $candidates[0].Name; extension_name = $candidates[0].Parent.Name
} | ConvertTo-Json -Compress))
