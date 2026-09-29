$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$source = Join-Path $repoRoot "tyche"
$release = Join-Path $repoRoot "dist\tyche-dydx-release"
$stage = Join-Path $release "stage"
$root = Join-Path $stage "tyche"
$zip = Join-Path $release "tyche-dydx-deploy-20260815-final.zip"
$standalone = Join-Path $release "update-tyche-dydx-from-tmp.sh"

if (-not (Test-Path $source -PathType Container)) {
  throw "missing source: $source"
}

New-Item -ItemType Directory -Path $release -Force | Out-Null

$resolvedRelease = [System.IO.Path]::GetFullPath($release)
if (Test-Path $stage) {
  $resolvedStage = [System.IO.Path]::GetFullPath($stage)
  if (-not $resolvedStage.StartsWith($resolvedRelease, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "refusing to remove stage outside release directory: $resolvedStage"
  }
  Remove-Item -LiteralPath $stage -Recurse -Force
}
if (Test-Path $zip) {
  Remove-Item -LiteralPath $zip -Force
}
if (Test-Path $standalone) {
  Remove-Item -LiteralPath $standalone -Force
}

New-Item -ItemType Directory -Path $root -Force | Out-Null

$top = @("README.md", "go.mod", "go.sum", "package.json", "package-lock.json", "cmd", "deploy", "docs", "eth", "minter", "model", "proto", "sniffer", "tyche", "util")
$excludedDirs = @("logs", "build", "node_modules", "__pycache__", ".git")
$excludedExts = @(".exe", ".zip", ".gz", ".log", ".env")
$excludedNames = @("tyche", "tyche9000", "NOCPsniffer", "tyche.toml", ".env")
$excludedNamePatterns = @("__debug_bin*", "*_test.go")
$excludedRelativePatterns = @("cmd\tyche\*.toml*", "cmd\tyche\logs\*", "cmd\sniffer\logs\*", "deploy\update-tyche-dydx-from-tmp.sh")

foreach ($item in $top) {
  $path = Join-Path $source $item
  if (-not (Test-Path $path)) {
    continue
  }
  if (Test-Path $path -PathType Leaf) {
    if ($excludedNames -contains (Split-Path $path -Leaf)) {
      continue
    }
    $dest = Join-Path $root $item
    New-Item -ItemType Directory -Path (Split-Path $dest) -Force | Out-Null
    Copy-Item -LiteralPath $path -Destination $dest
    continue
  }

  Get-ChildItem -LiteralPath $path -Recurse -File | Where-Object {
    $relativeFromSource = $_.FullName.Substring($source.Length).TrimStart("\")
    $parts = $relativeFromSource -split "\\"
    $fileName = $_.Name
    $hasExcludedDir = @($parts | Where-Object { $excludedDirs -contains $_ }).Count -gt 0
    $hasExcludedNamePattern = @($excludedNamePatterns | Where-Object { $fileName -like $_ }).Count -gt 0
    $hasExcludedRelativePattern = @($excludedRelativePatterns | Where-Object { $relativeFromSource -like $_ }).Count -gt 0
    -not $hasExcludedDir -and
      -not $hasExcludedNamePattern -and
      -not $hasExcludedRelativePattern -and
      $excludedExts -notcontains $_.Extension -and
      $excludedNames -notcontains $_.Name
  } | ForEach-Object {
    $rel = $_.FullName.Substring($path.Length).TrimStart("\")
    $dest = Join-Path (Join-Path $root $item) $rel
    New-Item -ItemType Directory -Path (Split-Path $dest) -Force | Out-Null
    Copy-Item -LiteralPath $_.FullName -Destination $dest
  }
}

$secretPatterns = @(
  'PrivateKey\s*=\s*"[0-9a-fA-F]{64}"',
  'DYDX_SYSTEM_PRIVATE_KEY_HEX=.+',
  'DYDX_SYSTEM_MNEMONIC=.+',
  'NECO_HOT_WALLET_PRIVATE_KEY=.+',
  'internalAPIKey\s*=\s*"[0-9a-fA-F]{32,}"'
)
$leak = Get-ChildItem -LiteralPath $root -Recurse -File |
  Select-String -Pattern $secretPatterns |
  Select-Object -First 1
if ($leak) {
  throw "refusing to package possible secret: $($leak.Path):$($leak.LineNumber)"
}

Copy-Item -LiteralPath (Join-Path $source "deploy\update-tyche-dydx-from-tmp.sh") -Destination $standalone

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [System.IO.Compression.ZipFile]::Open($zip, [System.IO.Compression.ZipArchiveMode]::Create)
try {
  Get-ChildItem -LiteralPath $root -Recurse -File | ForEach-Object {
    $relative = $_.FullName.Substring($root.Length).TrimStart("\")
    $entryName = "tyche/" + ($relative -replace "\\", "/")
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile(
      $archive,
      $_.FullName,
      $entryName,
      [System.IO.Compression.CompressionLevel]::Optimal
    ) | Out-Null
  }
} finally {
  $archive.Dispose()
}

$archive = [System.IO.Compression.ZipFile]::OpenRead($zip)
try {
  $badEntry = $archive.Entries | Where-Object { $_.FullName.Contains("\") } | Select-Object -First 1
  if ($badEntry) {
    throw "zip entry uses Windows path separator: $($badEntry.FullName)"
  }
} finally {
  $archive.Dispose()
}

Get-ChildItem -LiteralPath $release | Select-Object Name,Length,Mode
