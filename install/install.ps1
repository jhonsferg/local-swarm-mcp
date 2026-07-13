#!/usr/bin/env pwsh
# local-swarm-mcp - Windows installer
#
# Usage (one-liner):
#   irm https://raw.githubusercontent.com/jhonsferg/local-swarm-mcp/main/install/install.ps1 | iex
#
# Customise with environment variables before piping:
#   $env:LSM_INSTALL_DIR = "$env:USERPROFILE\.local\bin"
#   $env:LSM_VERSION     = "v0.3.0"
#
# Override base URLs for local/offline testing:
#   $env:LSM_TEST_API_BASE = "http://localhost:8765"   # replaces https://api.github.com
#   $env:LSM_TEST_DL_BASE  = "http://localhost:8765"   # replaces https://github.com

$ErrorActionPreference = "Stop"

$REPO       = "jhonsferg/local-swarm-mcp"
$InstallDir = if ($env:LSM_INSTALL_DIR)   { $env:LSM_INSTALL_DIR }   else { "$env:USERPROFILE\.local\bin" }
$Version    = if ($env:LSM_VERSION)       { $env:LSM_VERSION }       else { "latest" }
$ApiBase    = if ($env:LSM_TEST_API_BASE) { $env:LSM_TEST_API_BASE } else { "https://api.github.com" }
$DlBase     = if ($env:LSM_TEST_DL_BASE)  { $env:LSM_TEST_DL_BASE }  else { "https://github.com" }

# -- Terminal helpers ----------------------------------------------------------
function Write-Step([string]$msg) { Write-Host "  -> $msg" -ForegroundColor Cyan }
function Write-Ok([string]$msg)   { Write-Host "  v  $msg" -ForegroundColor Green }
function Abort([string]$msg) {
    Write-Host "`n  x  $msg" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "  local-swarm-mcp" -ForegroundColor Cyan -NoNewline
Write-Host " installer" -ForegroundColor White
Write-Host ""

# -- 1. Detect architecture -----------------------------------------------------
$isArm = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture `
         -eq [System.Runtime.InteropServices.Architecture]::Arm64

$Arch = if ($isArm) { "arm64" } else { "amd64" }

if (-not [System.Environment]::Is64BitOperatingSystem) {
    Abort "32-bit Windows is not supported."
}

Write-Step "Detected platform: windows-$Arch"

# -- 2. Resolve version ----------------------------------------------------------
if ($Version -eq "latest") {
    Write-Step "Fetching latest release from $ApiBase..."
    try {
        $rel     = Invoke-RestMethod "$ApiBase/repos/$REPO/releases/latest"
        $Version = $rel.tag_name
    }
    catch {
        Abort "Could not fetch latest version: $_"
    }
}

Write-Step "Installing local-swarm-mcp $Version"

# -- 3. Download archive and checksums --------------------------------------------
$ArchiveName = "local-swarm-mcp_windows_$Arch.zip"
$BaseUrl     = "$DlBase/$REPO/releases/download/$Version"
$TmpDir      = Join-Path ([System.IO.Path]::GetTempPath()) "local-swarm-mcp-install-$([System.Guid]::NewGuid())"
New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null
$TmpZip        = Join-Path $TmpDir $ArchiveName
$TmpChecksums  = Join-Path $TmpDir "checksums.txt"

try {
    Write-Step "Downloading $ArchiveName..."
    $ProgressPreference = "SilentlyContinue"
    Invoke-WebRequest -Uri "$BaseUrl/$ArchiveName" -OutFile $TmpZip -UseBasicParsing

    Write-Step "Downloading checksums.txt..."
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $TmpChecksums -UseBasicParsing
}
catch {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
    Abort "Download failed.`n  URL: $BaseUrl/$ArchiveName`n  Error: $_"
}

# -- 4. Verify checksum ------------------------------------------------------------
Write-Step "Verifying checksum..."
$checksumLine = Select-String -Path $TmpChecksums -Pattern "  $ArchiveName$" | Select-Object -First 1
if (-not $checksumLine) {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
    Abort "No checksum entry found for $ArchiveName in checksums.txt."
}
$expected = ($checksumLine.Line -split "\s+")[0]
$actual   = (Get-FileHash -Path $TmpZip -Algorithm SHA256).Hash.ToLower()

if ($expected -ne $actual) {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
    Abort "Checksum mismatch for $ArchiveName.`n  Expected: $expected`n  Actual:   $actual"
}
Write-Ok "Checksum verified"

# -- 5. Extract and install binary ---------------------------------------------------
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

$Dest = Join-Path $InstallDir "local-swarm-mcp.exe"

Write-Step "Extracting..."
try {
    Expand-Archive -Path $TmpZip -DestinationPath $TmpDir -Force
    $ExtractedExe = Join-Path $TmpDir "local-swarm-mcp.exe"
    if (-not (Test-Path $ExtractedExe)) {
        Abort "Archive did not contain local-swarm-mcp.exe"
    }
    if (Test-Path $Dest) { Remove-Item -Force $Dest -ErrorAction SilentlyContinue }
    Move-Item -Force $ExtractedExe $Dest
}
catch {
    Abort "Extraction failed: $_"
}
finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}

Write-Ok "Installed to $Dest"

try {
    $installedVersion = & $Dest -version 2>&1
    Write-Ok "Binary check: $installedVersion"
}
catch {
    Abort "Installed binary failed to run: $_"
}

# -- 6. Summary -----------------------------------------------------------------------
Write-Host ""
Write-Host "  local-swarm-mcp $Version installed!" -ForegroundColor Green
Write-Host ""
Write-Host "  Next steps:" -ForegroundColor White
Write-Host ""
Write-Host "  1. Make sure $InstallDir is on your PATH." -ForegroundColor White
Write-Host "  2. Start the daemon and add backends via the dashboard or -register-host -" -ForegroundColor White
Write-Host "     no config file needed. See the README's `"Configuring backends`" section:" -ForegroundColor White
Write-Host "       https://github.com/$REPO" -ForegroundColor Cyan
Write-Host "  3. Register local-swarm-mcp with your MCP client - see:" -ForegroundColor White
Write-Host "       https://github.com/$REPO#registering-with-an-mcp-client" -ForegroundColor Cyan
Write-Host ""
