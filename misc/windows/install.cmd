IF "%PROCESSOR_ARCHITECTURE%"=="ARM64" (
    copy /Y bin\extension-launcher-arm64.exe bin\extension-launcher.exe
    copy /Y bin\vm-application-manager-arm64.exe bin\vm-application-manager.exe
)

bin\vm-application-manager.exe "install"