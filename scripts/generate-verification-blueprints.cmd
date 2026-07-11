@echo off
setlocal
set "SCRIPT=%~dp0generate-verification-blueprints.ps1"
set "OUTPUT=%~dp0..\examples\verification-blueprints"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
  "$source = [System.IO.File]::ReadAllText($env:SCRIPT, [System.Text.Encoding]::UTF8); $block = [ScriptBlock]::Create($source); & $block -OutputRoot $env:OUTPUT"
exit /b %ERRORLEVEL%
