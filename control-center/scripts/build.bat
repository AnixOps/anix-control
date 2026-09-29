@echo off
REM Build Script for Windows
REM Usage: scripts\build.bat [version] [platform]

setlocal enabledelayedexpansion

set VERSION=%1
if "%VERSION%"=="" set VERSION=dev

set PLATFORM=%2
if "%PLATFORM%"=="" set PLATFORM=all

for /f "tokens=*" %%a in ('git rev-parse --short HEAD 2^>nul') do set COMMIT=%%a
if "%COMMIT%"=="" set COMMIT=unknown

for /f "tokens=*" %%a in ('powershell -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'"') do set DATE=%%a

set LDFLAGS=-s -w -X "main.version=%VERSION%" -X "main.commit=%COMMIT%" -X "main.date=%DATE%"

echo Building AnixOps Control Center
echo Version: %VERSION%
echo Commit: %COMMIT%
echo Date: %DATE%
echo.

REM Clean
if exist dist rmdir /s /q dist
mkdir dist

if "%PLATFORM%"=="all" (
    call :build linux amd64
    call :build linux arm64
    call :build windows amd64
    call :build windows arm64
    call :build darwin amd64
    call :build darwin arm64
) else (
    for /f "tokens=1,2 delims=-" %%a in ("%PLATFORM%") do (
        call :build %%a %%b
    )
)

echo.
echo Build complete! Artifacts in dist\
dir dist
goto :eof

:build
set GOOS=%1
set GOARCH=%2
set PLATFORM_NAME=%GOOS%-%GOARCH%
set OUTPUT_DIR=dist\%PLATFORM_NAME%

echo Building for %PLATFORM_NAME%...

mkdir %OUTPUT_DIR%

set BINARY_NAME=anixops
if "%GOOS%"=="windows" set BINARY_NAME=anixops.exe

set CGO_ENABLED=0
set GOOS=%GOOS%
set GOARCH=%GOARCH%
go build -ldflags="%LDFLAGS%" -o "%OUTPUT_DIR%\%BINARY_NAME%" cmd\anixops\main.go

set TUI_NAME=anixops-tui
if "%GOOS%"=="windows" set TUI_NAME=anixops-tui.exe
go build -ldflags="%LDFLAGS%" -o "%OUTPUT_DIR%\%TUI_NAME%" cmd\anixops-tui\main.go

echo Built %PLATFORM_NAME%
goto :eof