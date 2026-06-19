@echo off
setlocal

cd /d "%~dp0"

set "APP=%CD%\build\bin\OriginBlueprint.exe"
set "WAILS=%USERPROFILE%\go\bin\wails.exe"

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

echo Building OriginBlueprint with the latest frontend...
"%WAILS%" build
if errorlevel 1 (
    echo.
    echo ERROR: Build failed.
    pause
    exit /b 1
)

if exist "%CD%\nodes" (
    echo Syncing node JSON files...
    powershell -NoProfile -ExecutionPolicy Bypass -Command "$src='%CD%\nodes'; $dst='%CD%\build\bin\nodes'; if (Test-Path $dst) { Remove-Item -LiteralPath $dst -Recurse -Force }; New-Item -ItemType Directory -Force -Path $dst | Out-Null; Copy-Item -Path (Join-Path $src '*') -Destination $dst -Recurse -Force"
    if errorlevel 1 (
        echo.
        echo ERROR: Failed to sync nodes directory.
        pause
        exit /b 1
    )
)

:start_app
echo Starting OriginBlueprint...
start "" "%APP%"

endlocal
