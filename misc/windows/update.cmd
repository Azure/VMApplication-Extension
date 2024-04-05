IF "%PROCESSOR_ARCHITECTURE%"=="ARM64" (
    copy /Y %~dp0bin\extension-launcher-arm64.exe %~dp0bin\extension-launcher.exe
    copy /Y %~dp0bin\vm-application-manager-arm64.exe %~dp0bin\vm-application-manager.exe
)

%~dp0bin\vm-application-manager.exe "update"