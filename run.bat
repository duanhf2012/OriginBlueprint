@echo off
setlocal

cd /d "%~dp0"

set "APP=%CD%\build\bin\OriginBlueprint.exe"
set "WAILS=%USERPROFILE%\go\bin\wails.exe"

if exist "%APP%" goto start_app

echo OriginBlueprint.exe was not found. Building the application...

if not exist "%WAILS%" (
    where wails >nul 2>nul
    if errorlevel 1 (
        echo.
        echo ERROR: Wails CLI was not found.
        echo Install it with:
        echo   go install github.com/wailsapp/wails/v2/cmd/wails@latest
        pause
        exit /b 1
    )
    set "WAILS=wails"
)

"%WAILS%" build
if errorlevel 1 (
    echo.
    echo ERROR: Build failed.
    pause
    exit /b 1
)

:start_app
echo Starting OriginBlueprint...
start "" "%APP%"

endlocal
