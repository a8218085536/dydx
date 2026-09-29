$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$source = Join-Path $repoRoot "dydxtrading"
$release = Join-Path $repoRoot "dist\dydxtrading-release"
$stage = Join-Path $release "stage"
$root = Join-Path $stage "dydxtrading"
$zip = Join-Path $release "dydxtrading-deploy-20260815-final.zip"
$standalone = Join-Path $release "update-dydxtrading-from-tmp.sh"

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

$top = @("README.md", "go.mod", "go.sum", "Makefile", "Dockerfile", "docker-compose.yml", "cmd", "deploy", "sidecar", "testing")
$excludedDirs = @("data", "build", "logs", "node_modules", "__pycache__", ".git")
$excludedExts = @(".exe", ".zip", ".gz", ".log", ".env")
$excludedNames = @(".env")
$excludedNamePatterns = @("*_test.go", "__debug_bin*")

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
    -not $hasExcludedDir -and
      -not $hasExcludedNamePattern -and
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
  'DYDX_SYSTEM_PRIVATE_KEY_HEX=[0-9a-fA-F]{64}',
  'DYDX_TRADING_BEARER_TOKEN=[0-9a-fA-F]{32,}',
  'DYDX_EVENT_API_KEY=[0-9a-fA-F]{32,}',
  'TYCHE_INTERNAL_API_KEY=[0-9a-fA-F]{32,}'
)
$leak = Get-ChildItem -LiteralPath $root -Recurse -File |
  Where-Object { $_.Extension -ne ".md" } |
  Select-String -Pattern $secretPatterns |
  Select-Object -First 1
if ($leak) {
  throw "refusing to package possible secret: $($leak.Path):$($leak.LineNumber)"
}

Copy-Item -LiteralPath (Join-Path $root "deploy\update-dydxtrading-from-tmp.sh") -Destination $standalone

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [System.IO.Compression.ZipFile]::Open($zip, [System.IO.Compression.ZipArchiveMode]::Create)
try {
  Get-ChildItem -LiteralPath $root -Recurse -File | ForEach-Object {
    $relative = $_.FullName.Substring($root.Length).TrimStart("\")
    $entryName = "dydxtrading/" + ($relative -replace "\\", "/")
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
