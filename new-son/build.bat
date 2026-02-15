@echo off
rem Build Script for Linera (Run from Source directory)

echo Building Linera...
powershell -ExecutionPolicy Bypass -File build_safe.ps1

if exist Linera.exe (
    echo.
    echo ---------------------------------------------------
    echo SUCCESS: Build Complete! Linera.exe is ready.
    echo ---------------------------------------------------
) else (
    echo.
    echo ERROR: Build failed or Linera.exe not found.
)

pause
