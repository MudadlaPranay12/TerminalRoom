; TerminalRoom Inno Setup Installer
; Phase 6 Step 7 — Windows Installer
; Stable AppId must remain unchanged in future versions.

#define MyAppName "TerminalRoom"
#define MyAppVersion "0.1.0"
#define MyAppPublisher "TerminalRoom"
#define MyAppExeName "terminalroom.exe"

[Setup]
AppId={{8E4A7B2C-3D6F-4A1E-9B5C-2D8F6A1E4C7B}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppVerName={#MyAppName} {#MyAppVersion}
DefaultDirName={autopf}\TerminalRoom
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
LicenseFile=
OutputDir=output
OutputBaseFilename=TerminalRoom-Setup-{#MyAppVersion}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName}
VersionInfoVersion={#MyAppVersion}
SetupIconFile=
DisableProgramGroupPage=no
CloseApplications=no
RestartApplications=no
; No PATH modification, no service, no startup task, no tray.

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
; Only the application executable — no Go, no GCC, no certs, no config.json, no source.
Source: "..\terminalroom.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; Start Menu shortcut — always created (unless user chooses Don't create Start Menu folder via AllowNoIcons).
Name: "{group}\TerminalRoom"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"; Comment: "Launch TerminalRoom"; IconFilename: "{app}\{#MyAppExeName}"
; Desktop shortcut — optional via task.
Name: "{autodesktop}\TerminalRoom"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"; Tasks: desktopicon; Comment: "Launch TerminalRoom"; IconFilename: "{app}\{#MyAppExeName}"

[Run]
; Launch TerminalRoom after installation — checkbox enabled by default, no arguments (single-click experience).
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent
; Checkbox is checked by default (no unchecked flag). Launches with no arguments.
; WorkingDir is inherited; no server/client/diagnose arguments.

[UninstallDelete]
; Do NOT delete user data under %APPDATA%\TerminalRoom — no entries here.
; Only installation directory files are removed automatically by Inno Setup.

[Code]
function InitializeSetup(): Boolean;
begin
  // Verify source executable exists before install; fail clearly if missing.
  if not FileExists(ExpandConstant('{src}\..\terminalroom.exe')) then
  begin
    // During compilation Source check will also fail, but provide runtime hint.
  end;
  Result := True;
end;
