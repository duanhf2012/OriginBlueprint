@echo off
setlocal

cd /d "%~dp0"

set "APP=%CD%\build\bin\OriginBlueprint.exe"
set "WAILS=%USERPROFILE%\go\bin\wails.exe"
set "APP_VERSION=%~1"

if "%APP_VERSION%"=="" (
    if exist "%CD%\VERSION" (
        set /p APP_VERSION=<"%CD%\VERSION"
    )
)

if "%APP_VERSION%"=="" (
    set "APP_VERSION=0.0.0"
)

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

echo Building OriginBlueprint %APP_VERSION% with the latest frontend...
set "VITE_APP_VERSION=%APP_VERSION%"
"%WAILS%" build
if errorlevel 1 (
    echo.
    echo ERROR: Build failed.
    pause
    exit /b 1
)

echo Default node JSON files are embedded in the executable.
if exist "%CD%\build\bin\nodes" (
    echo Removing stale external node JSON files from build output...
    powershell -NoProfile -ExecutionPolicy Bypass -Command "$dst='%CD%\build\bin\nodes'; if ((Resolve-Path -LiteralPath $dst).Path.StartsWith((Resolve-Path -LiteralPath '%CD%\build\bin').Path)) { Remove-Item -LiteralPath $dst -Recurse -Force }"
    if errorlevel 1 (
        echo.
        echo ERROR: Failed to remove stale build nodes directory.
        pause
        exit /b 1
    )
)

:start_app
echo Starting OriginBlueprint...
start "" "%APP%"

endlocal
