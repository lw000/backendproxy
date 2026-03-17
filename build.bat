@echo off
echo ========================================
echo    BackendProxy Build Script
echo ========================================
echo.

REM Check if Go is installed
go version > nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go is not installed
    pause
    exit /b 1
)

echo [1/4] Cleaning old build files...
if exist bin (
    rmdir /s /q bin
)
mkdir bin

echo [2/4] Downloading dependencies...
go mod tidy
if errorlevel 1 (
    echo [ERROR] Failed to download dependencies
    pause
    exit /b 1
)

echo [3/4] Compiling program...
go build -o bin/backendproxy.exe main.go
if errorlevel 1 (
    echo [ERROR] Compilation failed
    pause
    exit /b 1
)

echo [4/4] Creating log directory...
if not exist logs mkdir logs

echo.
echo ========================================
echo    Build Success!
echo ========================================
echo.
echo Executable: bin\backendproxy.exe
echo Config:     config.toml
echo Monitor:    http://localhost:9090
echo.
echo Usage:
echo   .\bin\backendproxy.exe
echo   or
echo   .\bin\backendproxy.exe -config=custom_config.toml
echo ========================================
echo.
pause
