@echo off
setlocal enabledelayedexpansion
echo ========================================
echo    BackendProxy Release Script
echo ========================================
echo.

REM Check if executable exists
if not exist bin\backendproxy.exe (
    echo [ERROR] Executable not found, please run build.bat first
    pause
    exit /b 1
)

REM Generate timestamp using PowerShell
for /f %%a in ('powershell -command "Get-Date -Format 'yyyyMMdd-HHmmss'"') do set timestamp=%%a

REM Create release directory
set release_dir=prod\backendproxy_%timestamp%
if not exist prod mkdir prod
if exist %release_dir% (
    echo [WARN] Release directory already exists: %release_dir%
)

echo [1/4] Creating release directory: %release_dir%
mkdir %release_dir%

echo [2/4] Copying executable...
copy bin\backendproxy.exe %release_dir%\ > nul

echo [3/4] Copying config file...
copy config.toml %release_dir%\ > nul

echo [4/4] Copying monitor static files...
if not exist %release_dir%\static mkdir %release_dir%\static
xcopy /y /q static\index.html %release_dir%\static\ > nul

echo.
echo ========================================
echo    Release Success!
echo ========================================
echo.
echo Release path: %release_dir%
echo.
echo Files:
echo   - backendproxy.exe
echo   - config.toml
echo   - static\index.html
echo.
echo ========================================
echo.
pause
