param(
    [Parameter(Mandatory = $true)]
    [string]$BaseJar,

    [Parameter(Mandatory = $true)]
    [string]$OutputJar
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$basePath = [IO.Path]::GetFullPath((Resolve-Path -LiteralPath $BaseJar).Path)
$outputPath = [IO.Path]::GetFullPath($OutputJar)
if ($basePath -eq $outputPath) {
    throw 'Refusing to modify the base JAR in place'
}

$outputDirectory = Split-Path -Parent $outputPath
if (-not (Test-Path -LiteralPath $outputDirectory)) {
    New-Item -ItemType Directory -Path $outputDirectory | Out-Null
}

$javaHomeBin = if ($env:JAVA_HOME) { Join-Path $env:JAVA_HOME 'bin' } else { $null }
$javac = if ($javaHomeBin -and (Test-Path (Join-Path $javaHomeBin 'javac.exe'))) {
    Join-Path $javaHomeBin 'javac.exe'
} else {
    (Get-Command javac -ErrorAction Stop).Source
}
$java = Join-Path (Split-Path -Parent $javac) 'java.exe'
$jar = Join-Path (Split-Path -Parent $javac) 'jar.exe'
$javacVersion = (& $javac -version 2>&1 | Out-String).Trim()
if ($javacVersion -notmatch '^javac\s+(\d+)\.') {
    throw "Could not determine javac version: $javacVersion"
}
if ([int]$Matches[1] -lt 21) {
    throw "Java 21 or newer is required; found $javacVersion"
}

$sources = @(
    'openexch/production-patches/oms-market-maker-accounts/src/main/java/com/openexchange/oms/api/rest/RestApiHandler.java',
    'openexch/production-patches/oms-market-maker-accounts/src/main/java/com/openexchange/oms/app/OmsApplication.java',
    'openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/TycheApiKeyAuthenticationProvider.java',
    'openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/Authorizer.java',
    'openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/Principal.java',
    'openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/RoleBasedAuthorizer.java',
    'openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/grpc/GrpcAuth.java',
    'openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/grpc/GrpcAccountService.java',
    'openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/grpc/GrpcOrderService.java'
) | ForEach-Object { Join-Path $repoRoot $_ }
$testSource = Join-Path $repoRoot 'openexch/production-patches/oms-market-maker-accounts/src/test/java/com/openexchange/oms/api/rest/MarketMakerProductionRouteTest.java'

foreach ($source in @($sources) + $testSource) {
    if (-not (Test-Path -LiteralPath $source)) {
        throw "Required source is missing: $source"
    }
}

$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$work = [IO.Path]::GetFullPath((Join-Path $tempRoot ('oms-market-maker-' + [guid]::NewGuid().ToString('N'))))
if (-not $work.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Unsafe temporary path: $work"
}
$classes = Join-Path $work 'classes'
$testClasses = Join-Path $work 'test-classes'
New-Item -ItemType Directory -Path $classes, $testClasses | Out-Null

function Get-ZipEntryHashes([string]$Path) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $result = @{}
    $archive = [IO.Compression.ZipFile]::OpenRead($Path)
    try {
        foreach ($entry in $archive.Entries) {
            if ($entry.FullName.EndsWith('/')) { continue }
            if ($result.ContainsKey($entry.FullName)) {
                throw "Duplicate JAR entry: $($entry.FullName)"
            }
            $stream = $entry.Open()
            $sha = [Security.Cryptography.SHA256]::Create()
            try {
                $result[$entry.FullName] = [BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-', '')
            } finally {
                $sha.Dispose()
                $stream.Dispose()
            }
        }
    } finally {
        $archive.Dispose()
    }
    return $result
}

function Get-Sha256Hex([string]$Path) {
    $stream = [IO.File]::OpenRead($Path)
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        return [BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-', '').ToLowerInvariant()
    } finally {
        $sha.Dispose()
        $stream.Dispose()
    }
}

try {
    & $javac --release 21 -proc:none -cp $basePath -d $classes @sources
    if ($LASTEXITCODE -ne 0) { throw 'Overlay compilation failed' }

    Copy-Item -LiteralPath $basePath -Destination $outputPath -Force
    & $jar uf $outputPath -C $classes .
    if ($LASTEXITCODE -ne 0) { throw 'Candidate JAR update failed' }

    & $javac --release 21 -proc:none -cp $outputPath -d $testClasses $testSource
    if ($LASTEXITCODE -ne 0) { throw 'Production-route test compilation failed' }
    & $java -ea -cp "$testClasses;$outputPath" com.openexchange.oms.api.rest.MarketMakerProductionRouteTest
    if ($LASTEXITCODE -ne 0) { throw 'Production-route authorization tests failed' }

    $overlayEntries = @{}
    Get-ChildItem -LiteralPath $classes -Recurse -File | ForEach-Object {
        $name = $_.FullName.Substring($classes.Length + 1).Replace('\', '/')
        $overlayEntries[$name] = $true
    }
    $baseEntries = Get-ZipEntryHashes $basePath
    $candidateEntries = Get-ZipEntryHashes $outputPath

    foreach ($name in $baseEntries.Keys) {
        if ($overlayEntries.ContainsKey($name)) { continue }
        if (-not $candidateEntries.ContainsKey($name)) {
            throw "Candidate is missing non-overlay entry: $name"
        }
        if ($candidateEntries[$name] -ne $baseEntries[$name]) {
            throw "Non-overlay entry changed: $name"
        }
    }
    foreach ($name in $candidateEntries.Keys) {
        if (-not $baseEntries.ContainsKey($name) -and -not $overlayEntries.ContainsKey($name)) {
            throw "Unexpected candidate entry: $name"
        }
    }
    foreach ($name in $overlayEntries.Keys) {
        if (-not $candidateEntries.ContainsKey($name)) {
            throw "Compiled overlay entry is missing: $name"
        }
    }

    $baseHash = Get-Sha256Hex $basePath
    $candidateHash = Get-Sha256Hex $outputPath
    Write-Output "Base SHA-256: $baseHash"
    Write-Output "Candidate SHA-256: $candidateHash"
    Write-Output "Verified overlay entries: $($overlayEntries.Count)"
    Write-Output 'PASS: every non-overlay JAR entry is byte-identical to the production base'
} finally {
    if (($work.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) -and
            (Test-Path -LiteralPath $work)) {
        Remove-Item -LiteralPath $work -Recurse -Force
    }
}
