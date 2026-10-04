Unicode true

####
## Seaglass's installer. It installs for the current user only
## (%LOCALAPPDATA%\Programs\Seaglass, no administrator rights), so
## Seaglass can update itself later without a UAC prompt.
##
## Build (after `wails3 build`), from this folder:
##   makensis -DARG_WAILS_AMD64_BINARY=..\..\..\bin\Seaglass.exe -DINFO_PRODUCTVERSION=1.0.0 project.nsi
## or from the repository root: `wails3 task installer VERSION=v1.0.0`.
##
## Command line (besides NSIS's own /S and /D=<folder>):
##   /relaunch   start Seaglass when done (used by its updater)
##   /tray       with /relaunch: start it in the tray
##
## Signing the uninstaller (see docs/SIGNING.md):
##   -DSIGN_CMD="<command>"   signs it while it's built (a local certificate);
##                            the command gets the file path as its last argument.
##   -DINNER                  builds bin\uninstaller-maker.exe, which only writes
##                            bin\uninstall.exe when run, so a remote signer
##                            (SignPath) can sign that file;
##   -DSIGNED_UNINSTALLER=<file>  then packs that signed uninstaller as it is.
####

!define INFO_PROJECTNAME    "Seaglass"
!define INFO_COMPANYNAME    "ApolloF"
!define INFO_PRODUCTNAME    "Seaglass"
!ifndef INFO_PRODUCTVERSION
    !define INFO_PRODUCTVERSION "0.0.0"
!endif
!define INFO_COPYRIGHT      "(c) 2026 ApolloF, AGPL-3.0 License"
!define PRODUCT_EXECUTABLE  "Seaglass.exe"
!define UNINST_KEY_NAME     "Seaglass"
# Seaglass was called WaterLauncher before 1.5.
!define OLD_PRODUCTNAME     "WaterLauncher"
!define OLD_EXECUTABLE      "WaterLauncher.exe"
!define OLD_UNINST_KEY      "Software\Microsoft\Windows\CurrentVersion\Uninstall\WaterLauncher"
!define WAILS_INSTALL_SCOPE "user"
!define REQUEST_EXECUTION_LEVEL "user"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define STARTUP_APPROVED_KEY "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run"

!include "wails_tools.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

# The version information needs four parts.
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Setup"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true
SetCompressor /SOLID lzma

!ifdef SIGN_CMD
    !uninstfinalize '${SIGN_CMD} "%1"' = 0
!endif

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Start Seaglass"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
!ifdef INNER
    OutFile "..\..\..\bin\uninstaller-maker.exe"
!else
    OutFile "..\..\..\bin\Seaglass-setup.exe"
!endif
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
# An update or reinstall goes where Seaglass already is.
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
ShowInstDetails nevershow
ShowUninstDetails nevershow

# Closes a running Seaglass (it may sit in the tray) and waits until
# its program file can be replaced. $0 is the folder to look in, $3 the
# program file's name (WaterLauncher's, for an install from before 1.5).
!macro CloseApp UN
Function ${UN}CloseApp
    IfFileExists "$0\$3" 0 done
    # "--quit" asks the running copy to close; it exits at once when none runs.
    ExecWait '"$0\$3" --quit'
    StrCpy $1 0
    retry:
        ClearErrors
        # Opening the exe for writing fails while it runs.
        FileOpen $2 "$0\$3" a
        IfErrors 0 closed
        IntOp $1 $1 + 1
        IntCmp $1 60 0 wait 0
            IfSilent fail
            MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Seaglass is still running. Close it (also from the tray), then choose Retry." IDRETRY again
            fail:
            SetErrorLevel 2
            Quit
            again:
            StrCpy $1 0
        wait:
        Sleep 250
        Goto retry
    closed:
        FileClose $2
    done:
FunctionEnd
!macroend
!insertmacro CloseApp ""
!insertmacro CloseApp "un."

Function .onInit
!ifdef INNER
    # Only write the uninstaller next to this exe, for signing.
    WriteUninstaller "$EXEDIR\uninstall.exe"
    Quit
!endif
    !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext
    SetRegView 64

    # An install from before the rename (WaterLauncher). Its updater starts
    # this installer with /D=<its folder>: Seaglass goes next to it, into a
    # Seaglass folder, and the old program is removed. The library and
    # settings move over when Seaglass first starts.
    StrCpy $R6 0
    ReadRegStr $R5 HKCU "${OLD_UNINST_KEY}" "InstallLocation"
    StrCmp $R5 "" noOld
        StrCpy $0 $R5
        StrCpy $3 "${OLD_EXECUTABLE}"
        Call CloseApp
        StrCmp $INSTDIR $R5 0 keepDir
            ${GetParent} $R5 $R7
            StrCpy $INSTDIR "$R7\${INFO_PRODUCTNAME}"
        keepDir:
        Delete "$R5\${OLD_EXECUTABLE}"
        Delete "$R5\${OLD_EXECUTABLE}.old"
        Delete "$R5\${OLD_EXECUTABLE}.new"
        Delete "$R5\uninstall.exe"
        RMDir $R5
        Delete "$SMPROGRAMS\${OLD_PRODUCTNAME}.lnk"
        IfFileExists "$DESKTOP\${OLD_PRODUCTNAME}.lnk" 0 noOldDesktop
            Delete "$DESKTOP\${OLD_PRODUCTNAME}.lnk"
            StrCpy $R6 1 # it had a desktop shortcut: Seaglass gets one
        noOldDesktop:
        DeleteRegKey HKCU "${OLD_UNINST_KEY}"
    noOld:

    StrCpy $0 $INSTDIR
    StrCpy $3 "${PRODUCT_EXECUTABLE}"
    Call CloseApp

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files
    # The license and the third-party notices go with the program.
    File "/oname=LICENSE.txt" "..\..\..\LICENSE"
    File "..\..\..\THIRD_PARTY_NOTICES.txt"
    # Left behind by an update of a copy that wasn't installed.
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.old"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    # The desktop shortcut comes with a first install; updates don't bring
    # back one you deleted.
    IfSilent 0 desktop
    StrCmp $R6 1 desktop noDesktop
    desktop:
        CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    noDesktop:

!ifdef SIGNED_UNINSTALLER
    File "/oname=uninstall.exe" "${SIGNED_UNINSTALLER}"
!else
    WriteUninstaller "$INSTDIR\uninstall.exe"
!endif
    SetRegView 64
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
    WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "https://github.com/ApolloF/Seaglass"
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1

    # Started by Seaglass's updater: start it again.
    ${GetParameters} $R0
    ClearErrors
    ${GetOptions} $R0 "/relaunch" $R1
    IfErrors noRelaunch
        ClearErrors
        ${GetOptions} $R0 "/tray" $R1
        IfErrors 0 +3
            Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}" --updated'
            Goto noRelaunch
        Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}" --updated --tray'
    noRelaunch:
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    StrCpy $0 $INSTDIR
    StrCpy $3 "${PRODUCT_EXECUTABLE}"
    Call un.CloseApp

    # Only Seaglass's own files: the folder may have been chosen by hand.
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.old"
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.new"
    Delete "$INSTDIR\LICENSE.txt"
    Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    # Starting with Windows.
    DeleteRegValue HKCU "${RUN_KEY}" "${INFO_PRODUCTNAME}"
    DeleteRegValue HKCU "${STARTUP_APPROVED_KEY}" "${INFO_PRODUCTNAME}"
    DeleteRegValue HKCU "${RUN_KEY}" "${OLD_PRODUCTNAME}"
    DeleteRegValue HKCU "${STARTUP_APPROVED_KEY}" "${OLD_PRODUCTNAME}"

    IfSilent keepData
    MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Also delete your Seaglass library, settings, saved keys and downloaded art?$\r$\n$\r$\nYour games and their saves are not touched either way." IDNO keepData
        RMDir /r "$APPDATA\Seaglass"
        RMDir /r "$LOCALAPPDATA\Seaglass"
    keepData:

    !insertmacro wails.deleteUninstaller
    RMDir $INSTDIR
SectionEnd
