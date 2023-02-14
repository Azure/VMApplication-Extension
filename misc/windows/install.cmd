IF "%PROCESSOR_ARCHITECTURE%"=="ARM64"(
    copy /Y bin\extension-launcher-arm64 bin\extension-launcher.exe
    copy /Y bin\vm-application-manager-arm64 bin\vm-application-manager.exe
) ELSE (
    copy /Y bin\extension-launcher bin\extension-launcher.exe
    copy /Y bin\vm-application-manager bin\vm-application-manager.exe
)
bin\vm-application-manager.exe "install"