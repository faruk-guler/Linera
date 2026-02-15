# Script to Build Linera Application with Metadata
$ErrorActionPreference = "Stop"
$exePath = "Linera.exe"
$sysoPath = "resource.syso"

Write-Host "Linera Build Initialization (With Metadata Attempt 2)..." -ForegroundColor Cyan

# 1. Clean previous build and resources
if (Test-Path $exePath) { Remove-Item -Path $exePath -Force -ErrorAction SilentlyContinue }
if (Test-Path $sysoPath) { Remove-Item -Path $sysoPath -Force -ErrorAction SilentlyContinue }
if (Test-Path "rsrc.syso") { Remove-Item -Path "rsrc.syso" -Force -ErrorAction SilentlyContinue }

# 2. Generate Resources (Icon + Manifest + Version Info)
Write-Host "Generating resources with goversioninfo..." -ForegroundColor Cyan

if (Get-Command "goversioninfo" -ErrorAction SilentlyContinue) {
    # Try forcing 64-bit
    goversioninfo -64 -o $sysoPath
    if (Test-Path $sysoPath) {
        Write-Host "Resources generated ($sysoPath)." -ForegroundColor Green
    }
    else {
        Write-Host "WARNING: resource.syso not found after running goversioninfo." -ForegroundColor Yellow
    }
}
else {
    Write-Host "WARNING: goversioninfo not found! Building without extended metadata." -ForegroundColor Yellow
    if (Test-Path "c:/Users/SISTEM/go/bin/rsrc.exe") {
        Write-Host "Using rsrc fallback..." -ForegroundColor Yellow
        & "c:/Users/SISTEM/go/bin/rsrc.exe" -manifest Linera.manifest -ico img/linera.ico -o rsrc.syso
    }
}

# 3. Build the application
Write-Host "Building project..." -ForegroundColor Cyan
# Try without stripping debug info first if it helps debug, but user wants release.
# We'll stick to release flags.
go build -ldflags="-s -w -H=windowsgui" -o $exePath

if (Test-Path $exePath) {
    Write-Host "SUCCESS: Linera.exe created successfully." -ForegroundColor Green
    Write-Host "Metadata (Version, Company) embedded." -ForegroundColor Cyan
}
else {
    Write-Host "ERROR: Build failed." -ForegroundColor Red
    exit 1
}
