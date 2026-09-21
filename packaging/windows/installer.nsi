Unicode True
!include "MUI2.nsh"
!include "LogicLib.nsh"
!ifndef ARCH
 !define ARCH "amd64"
!endif
!ifndef VERSION
 !define VERSION "1.0.0"
!endif
!if "${ARCH}" == "amd64"
 !define LABEL "x64"
!else
 !define LABEL "arm64"
!endif
Name "SEU SC Bridge"
Icon "..\..\frontend\assets\app-icon.ico"
UninstallIcon "..\..\frontend\assets\app-icon.ico"
OutFile "..\..\dist\SEU-SC-Bridge-Setup-${LABEL}.exe"
InstallDir "$LOCALAPPDATA\Programs\SEUSC"
RequestExecutionLevel user
SetCompressor /SOLID lzma
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\seusc.exe"
!define MUI_FINISHPAGE_RUN_PARAMETERS "gui"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"
Section "SEU SC Bridge" SEC_MAIN
 SetShellVarContext current
 IfFileExists "$INSTDIR\seusc.exe" 0 +3
  ExecWait '"$INSTDIR\seusc.exe" stop'
  ExecWait '"$INSTDIR\seusc.exe" close-gui'
 Sleep 750
 SetOutPath "$INSTDIR"
 File "..\..\dist\windows-${ARCH}\seusc.exe"
 File "..\..\README.md"
 CreateDirectory "$SMPROGRAMS\SEU SC Bridge"
 CreateShortcut "$SMPROGRAMS\SEU SC Bridge\SEU SC Bridge.lnk" "$INSTDIR\seusc.exe" "gui"
 WriteUninstaller "$INSTDIR\Uninstall.exe"
 CreateShortcut "$SMPROGRAMS\SEU SC Bridge\Uninstall.lnk" "$INSTDIR\Uninstall.exe"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SEUSC" "DisplayName" "SEU SC Bridge"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SEUSC" "DisplayVersion" "${VERSION}"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SEUSC" "UninstallString" '"$INSTDIR\Uninstall.exe"'
 ExecWait '"$INSTDIR\seusc.exe" init' $0
 ${If} $0 != 0
  MessageBox MB_ICONEXCLAMATION "SSH configuration needs attention. Run seusc doctor after installation."
 ${EndIf}
 ExecWait '"$INSTDIR\seusc.exe" autostart on'
SectionEnd
Section "Uninstall"
 SetShellVarContext current
 ExecWait '"$INSTDIR\seusc.exe" stop'
 ExecWait '"$INSTDIR\seusc.exe" close-gui'
 DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "SEUSC"
 DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SEUSC"
 Delete "$SMPROGRAMS\SEU SC Bridge\SEU SC Bridge.lnk"
 Delete "$SMPROGRAMS\SEU SC Bridge\Uninstall.lnk"
 RMDir "$SMPROGRAMS\SEU SC Bridge"
 Delete "$INSTDIR\seusc.exe"
 Delete "$INSTDIR\README.md"
 Delete "$INSTDIR\Uninstall.exe"
 RMDir "$INSTDIR"
 ; User settings remain; log out before uninstall to clear login data.
SectionEnd
