@echo off
rem start.cmd - start updown from its release package (Windows). Copied into every
rem release by the build. It only reads release.conf and runs BUILD - no OS detection:
rem the package already is for one OS (docs/contract.md section 1). Arguments pass through.
rem   start.cmd [--config FILE] [--check] [--version] [FILE ...]
setlocal
set "HERE=%~dp0"
for /f "usebackq eol=# tokens=1,* delims==" %%A in ("%HERE%release.conf") do set "%%A=%%B"
"%HERE%%BUILD:/=\%" %*
exit /b %ERRORLEVEL%
