# TerminalRoom

A private, temporary, two-person communication application.

## What is TerminalRoom?

TerminalRoom is **not** another permanent chat application. It is a tool for creating temporary, private communication sessions between exactly two people.

The core flow:

```
CREATE → INVITE ONE PERSON → JOIN → PRIVATE COMMUNICATION → END → DESTROY
```

- **No permanent accounts**
- **No contact lists**
- **No groups or channels**
- **No profiles**
- **No permanent chat history**
- **No databases**
- **No cloud infrastructure**

Just a room for now.

## Current Architecture (Phase 5)

```
TerminalRoom/
├── cmd/
│   └── terminalroom/
│       └── main.go              # Entry point (server/client/diagnose modes)
├── internal/
│   ├── config/
│   │   ├── config.go            # Windows-aware config + %APPDATA% paths
│   │   └── config_test.go       # Config precedence tests
│   ├── room/
│   │   ├── room.go              # Room model with per-participant relay
│   │   ├── room_test.go         # Room tests
│   │   └── manager.go           # RoomManager for room lifecycle
│   ├── server/
│   │   ├── server.go            # TLS server with relay goroutines
│   │   └── server_test.go       # Integration tests
│   ├── client/
│   │   └── client.go            # Polished terminal client interface
│   ├── protocol/
│   │   └── protocol.go          # Wire protocol for communication
│   ├── invitation/
│   │   ├── invitation.go        # Invitation token system
│   │   └── invitation_test.go   # Invitation tests
│   ├── network/
│   │   ├── network.go           # Private network address discovery
│   │   └── network_test.go      # Network tests
│   ├── terminal/
│   │   ├── terminal.go          # Terminal formatting utilities
│   │   └── terminal_test.go     # Terminal formatting tests
│   └── tlsutil/
│       └── tlsutil.go           # TLS certificate management
├── go.mod
├── README.md
└── .gitignore
```

### Key Components

- **Room Model**: Concurrency-safe room with 2-person capacity, per-participant writer channels, room lifecycle states, TTL-based expiration, and atomic destruction
- **RoomManager**: Thread-safe room creation (with TTL), lookup, and destruction
- **Server**: TLS server with relay goroutines, panic recovery, atomic join flow, TTL-based room expiration, and private network binding
- **Client**: Polished terminal interface with room info display, invitation presentation, chat commands, live countdown, expiration warnings, and certificate pinning
- **Protocol**: Text-based wire protocol with explicit message types for authentication flow, room expiry, and endpoint advertisement
- **Terminal**: Formatting utilities for duration, timestamps, banners, and consistent terminal output
- **Invitation**: Cryptographically secure temporary invitation tokens with atomic consumption and room-scoped cleanup
- **Network**: Private network address discovery supporting Tailscale (100.64.0.0/10), RFC 1918 (10.x, 172.16-31.x, 192.168.x), with interface enumeration
- **TLS**: Certificate management with pinning for client trust
- **Config**: Windows-aware configuration (`%APPDATA%\TerminalRoom\config.json` + `certs\`) with env-var precedence, no CWD dependence

## How to Build

```bash
go build -o terminalroom.exe ./cmd/terminalroom
```

## Windows Installation (Installer)

TerminalRoom provides a Windows installer for end users. No Go, GCC, MSYS2, or development tools are required.

### Installing

1. Download `TerminalRoom-Setup-0.1.0.exe` (from `installer/output/` or a release artifact).
2. Double-click the installer and follow the wizard:
   Welcome → Installation location → Start Menu/Desktop shortcut options → Install → Finish
3. On the final page, leave `Launch TerminalRoom` checked (default) to start TerminalRoom immediately. It launches `terminalroom.exe` with no arguments — the single-click launcher experience.

### Where It Is Installed

- Default location: `%ProgramFiles%\TerminalRoom\` (e.g. `C:\Program Files\TerminalRoom\`)
  - The installer uses `{autopf}\TerminalRoom` and requires elevation only for installation.
  - The application itself does **not** require administrator privileges when run.
- No modification of `PATH`.
- No Windows service, no startup task, no tray application.

### Shortcuts

- **Start Menu:** `TerminalRoom` → launches `terminalroom.exe` (no arguments, `WorkingDir` = install dir)
- **Desktop:** `TerminalRoom` → launches `terminalroom.exe` (created via the `Create desktop icon` task, checked by default)

Both shortcuts launch the normal no-argument launcher. No separate shortcuts are created for `server` / `client` / `diagnose` — those modes remain available via command line:
```
"C:\Program Files\TerminalRoom\terminalroom.exe" server
"C:\Program Files\TerminalRoom\terminalroom.exe" client
"C:\Program Files\TerminalRoom\terminalroom.exe" diagnose
```

### No Bundled Certificates or Configuration

The installer ships **only** `terminalroom.exe`. It does **not** bundle:

- TLS certificates (`certs\`)
- `config.json`
- source code or development tools

On first launch the application creates its runtime directories automatically:

```
%APPDATA%\TerminalRoom\config.json   (optional, created only if you save settings)
%APPDATA%\TerminalRoom\certs\server.crt
%APPDATA%\TerminalRoom\certs\server.key
```

Existing configuration precedence is unchanged: `environment variable > config.json > default`; `CERT_DIR` overrides `%APPDATA%\TerminalRoom\certs` only if explicitly set.

### Uninstall

Use `Settings → Apps → TerminalRoom → Uninstall` (or `Control Panel → Programs and Features`).

- The uninstaller removes only files it installed into `C:\Program Files\TerminalRoom\`.
- It **does not** delete `%APPDATA%\TerminalRoom\` — your `config.json`, `certs\`, and generated certificates are preserved. Remove that folder manually only if you intend to reset all runtime data.

### Firewall

The installer does **not** create any Windows Firewall rule. No broad, public, or any-profile rule is added.

When TerminalRoom first listens on a private address, Windows may display its normal firewall permission dialog. That behavior is expected — allow access for **Private networks** only. TerminalRoom is intended to operate over private networking (Tailscale `100.64.0.0/10` or RFC 1918 `10/8`, `172.16/12`, `192.168/16`). Do **not** enable broad or public-network firewall exceptions; the application rejects public binding and `0.0.0.0` unless `DEVELOPMENT_MODE=1`.

### Requirements

- Windows 10/11 x64 (`windows/amd64`)
- No Go/GCC/runtime tools required
- Private network (Tailscale, LAN, or VPN) for normal operation; `SERVER_ADDR=localhost` for local development

### Building the Installer (Developers)

```bash
# Build the application first
go build -o terminalroom.exe ./cmd/terminalroom

# Compile the installer (requires Inno Setup 6)
"C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\terminalroom.iss
# Output: installer/output/TerminalRoom-Setup-0.1.0.exe
```

Installer source is `installer/terminalroom.iss` (stable `AppId={{8E4A7B2C-3D6F-4A1E-9B5C-2D8F6A1E4C7B}`). Generated `installer/output/*.exe` is gitignored.

## How to Run

### Terminal 1: Start the Server

```bash
./terminalroom.exe server
```

The server automatically detects private network interfaces (Tailscale, RFC 1918) and binds to them.

For development on localhost:

```bash
SERVER_ADDR=localhost ./terminalroom.exe server
```

With custom settings:

```bash
PORT=8080 ROOM_TTL=300 SERVER_ADDR=192.168.1.100 ./terminalroom.exe server
```

### Check Network Diagnostics

```bash
./terminalroom.exe diagnose
```

This shows whether a private network is available and the detected address.

### Terminal 2: Create a Room (User A)

```bash
./terminalroom.exe client
```

Select option `[1] CREATE PRIVATE ROOM`. The room info, invitation token, and server endpoint are displayed. Copy the invitation token and send it to your friend.

### Terminal 3: Join the Room (User B)

```bash
./terminalroom.exe client
```

Select option `[2] JOIN PRIVATE ROOM`. Paste the invitation token shared by User A. The server address defaults to localhost; leave empty or enter the server's address.

### Communication

Once both participants are connected:

```
[17:42:03] Friend: Hello!
[17:42:08] You: Hey, how are you?
[17:42:12] Friend: I'm good.
```

Type `/quit` or press `Ctrl+C` to leave.

## Chat Commands

During an active session, the following commands are available:

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/info` | Show room and session information |
| `/quit` | Leave the room |

Example `/info` output:

```
Room Information
────────────────────────────────────────────────
Room       : TR-XXXXXX
Status     : ACTIVE
Participant: You + Friend
Expires    : in 07:31
Network    : Private
Transport  : TLS
Persistence: None
────────────────────────────────────────────────
```

## Terminal Experience

### Create Flow

```
================================================
              TERMINALROOM
       PRIVATE • TEMPORARY • TWO-PERSON
================================================

> Establishing secure connection...
> Secure connection established.

================================================
PRIVATE ROOM // TERMINAL
================================================

> Nothing permanent.
> Just a room for now.

[1] CREATE PRIVATE ROOM
[2] JOIN PRIVATE ROOM
[3] EXIT

> 1

> Creating private room...
> Secure environment ready.

+----------------------------------------------+
| PRIVATE ROOM                                 |
|                                              |
| Room       : TR-XXXXXX                       |
| Expires    : in 10 minutes                   |
| Network    : Private                         |
| Security   : TLS + invitation                |
|                                              |
+----------------------------------------------+

INVITATION
────────────────────────────────────────────────
TRINV-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
────────────────────────────────────────────────

> Copy the invitation above and send it to your friend.
> The invitation is temporary and can only be used once.
> Waiting for friend...
```

### Join Flow

```
> Enter invitation: TRINV-xxx...
> Server address (leave empty for default):
>
> Verifying invitation...
> Room expires in 10 minutes.
> Secure two-person session established.
```

### Chat Interface

```
================================================
ROOM: TR-XXXXXX
STATUS: ACTIVE
EXPIRES IN: 08:42
================================================

> Type your message and press Enter.
> Type /help for available commands.

[17:42:03] Friend: Hello!
> Hey!
[17:42:08] You: Hey!
```

### Expiration Warnings

As the room approaches TTL expiry, progressive warnings appear:

```
> Room expires in 5 minutes.
> Room expires in 2 minutes.
> Room expires in 1 minute.
> Room expires in 30 seconds.

> Room is expiring. Connection will close shortly.
> Room is closing.

> Session ended.
> Room destroyed.
> No room data was persisted.
```

### Session End

When a participant leaves or disconnects:

```
> Friend left.
> Session ended.
```

Or when the user quits:

```
> Leaving room...
> Session ended.
> Disconnected.
```

## How Rooms Are Isolated

Each room operates independently. Messages are delivered only to participants within the same room. There is no global message channel. Room A's participants cannot see Room B's messages.

## How Disconnect Cleanup Works

When a participant disconnects:
1. The server detects the broken connection
2. The other participant is notified with a leave message
3. The disconnected participant is removed from the room
4. The room's writer channel is cleaned up
5. If the room is empty, it is destroyed and removed from memory
6. Associated invitations are cleaned up
7. The Room ID and Invitation token no longer resolve to an active room

Cleanup is idempotent and safe if triggered multiple times.

## Room Lifecycle

Rooms progress through a defined state machine:

```
StateWaiting ──(2nd participant joins)──> StateActive
StateWaiting ──(TTL expires)────────────> StateExpiring ──> StateClosing ──> StateDestroyed
StateWaiting ──(creator leaves)─────────> StateDestroyed
StateActive  ──(all participants leave)─> StateDestroyed
StateActive  ──(TTL expires)────────────> StateExpiring ──> StateClosing ──> StateDestroyed
```

- **StateWaiting**: Room created, waiting for second participant
- **StateActive**: Two participants connected, session active
- **StateExpiring**: TTL expired, participants notified, grace period active
- **StateClosing**: Grace period ended, connections being torn down
- **StateDestroyed**: Room fully destroyed, cleaned up from memory

### TTL and Expiration

- Default room TTL: 10 minutes
- Configurable per-server via `NewWithTTL()`
- On TTL expiry, participants receive SYSTEM messages:
  1. `"Room is expiring. Connection will close shortly."` (1 second grace period)
  2. `"Room is closing."` (500ms before destruction)
- After destruction, room and invitations are removed from memory
- New joins are rejected for rooms in expiring/closing/destroyed states
- Client displays progressive warnings at 5, 2, 1, and 30 seconds remaining

### Abandoned Room Cleanup

If a room creator disconnects before a second participant joins:
1. Room transitions directly to StateDestroyed
2. Writer channels are closed
3. Room and invitations are cleaned up immediately

## Connection Flow

### Create Flow

```
Client                          Server
  │                                │
  │──── TLS Handshake ────────────>│  Certificate pinned by client
  │──── CREATE ───────────────────>│
  │                                │  Discover private network
  │                                │  Bind to private interface
  │<─── ROOM_ID ──────────────────│  Room created
  │<─── INV_TOKEN ────────────────│  Invitation generated
  │<─── EXPIRY ───────────────────│  Room expiration time (RFC3339)
  │<─── ENDPOINT ─────────────────│  Server endpoint address
  │<─── PARTICIPANT_ID ───────────│  Creator assigned slot 1
  │<─── SYSTEM (waiting) ─────────│  "Waiting for friend..."
  │                                │
  │  (Client enters chat loop)     │
```

### Join Flow

```
Client                          Server
  │                                │
  │──── TLS Handshake ────────────>│  Certificate pinned by client
  │──── JOIN|<token> ─────────────>│
  │                                │  Validate invitation (atomic)
  │                                │  Check room exists
  │                                │  Check room is joinable
  │                                │  Join room (atomic capacity check)
  │<─── PARTICIPANT_ID ───────────│  Joiner assigned slot 2
  │<─── EXPIRY ───────────────────│  Room expiration time (RFC3339)
  │<─── ENDPOINT ─────────────────│  Server endpoint address
  │<─── READY ────────────────────│  "Secure two-person session established."
  │                                │
  │  (Both participants chat)      │
```

## Security Features

### TLS Encryption
- All communication encrypted using TLS 1.2+
- Self-signed ECDSA P-256 certificates generated automatically
- Certificate and key stored in `%APPDATA%\TerminalRoom\certs\` (Windows) — no manual `CERT_DIR` setup required for normal use
- Private key stored with 0600 permissions (0700 directory)
- Client pins the server certificate (no InsecureSkipVerify)
- Override via `CERT_DIR` environment variable if needed (see Configuration)

### Invitation Tokens
- Cryptographically secure random tokens (24 bytes of entropy)
- Format: `TRINV-<48 hex characters>`
- One-time use only (atomic consumption)
- Expire after 10 minutes (configurable)
- Bound to a specific room
- Concurrent use attempts: only one succeeds

### Authentication vs Authorization
- **Authentication**: Validates that a connection presents a valid temporary credential (invitation token)
- **Authorization**: Validates that the credential is allowed to enter THIS specific room

### Private Network Binding
- Server automatically detects private network interfaces:
  - **Tailscale**: 100.64.0.0/10 (CGNAT range)
  - **RFC 1918**: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
- Binds to private address when available
- Refuses to start if no private network detected (unless `SERVER_ADDR` override)
- Endpoint address advertised to clients via `MsgEndpoint` protocol message
- `ENDPOINT_ADDR` separates bind address from advertised address

### Room Access Control
- Room ID alone is NOT sufficient for joining
- Invitation token is required
- Token must not be expired
- Token must not be already consumed
- Token must match the target room
- Room must not be full
- Capacity check and join are atomic

### Message Relay
- Per-participant writer channels prevent deadlock
- No room locks held during network writes
- Messages delivered only to correct room participants
- Sender does not receive own messages (no echo)
- Failed connections cleaned up safely

### Error Handling
- Panics recovered at connection boundary
- Server survives client failures
- Malformed messages rejected safely
- Cleanup is idempotent
- No sensitive information in error messages

### Sensitive Logging
- Room IDs logged (safe)
- Invitation tokens NEVER logged
- Private keys NEVER logged
- Chat messages NEVER logged
- Only operational events logged

## Current Limitations

- **Self-signed certificates**: No certificate authority trust chain
- **Certificate distribution**: Client must have access to server certificate
- **No persistent storage**: All state is in-memory; server restart loses everything
- **No reconnection**: Disconnected participants must rejoin with a new invitation
- **No message history**: Messages are not stored or replayed
- **No rate limiting**: No protection against abuse
- **Single server**: No clustering or distributed state
- **Terminal only**: No GUI, no web interface
- **Fixed TTL**: Room expiration is per-server, not per-room configurable by users

## Security Limitations

The following are explicitly **not** fully addressed:

- **Certificate trust**: Self-signed certificates require manual distribution to clients
- **Tailscale integration**: Network discovery is passive (address detection only, not Tailscale API)
- **Host compromise**: No protection if the server or client machine is compromised
- **Side-channel attacks**: No protection against screenshots, copying, or keyloggers
- **Complete anonymity**: Server knows IP addresses of participants on private network
- **Production identity**: No CA-signed certificates or identity verification
- **Forward secrecy**: TLS provides some, but not full forward secrecy guarantee
- **Public network fallback**: Server refuses to start on public networks — requires private network

**PUBLIC NETWORK OPERATION IS NOT SUPPORTED.** TerminalRoom requires a private network (Tailscale, LAN, or VPN) to function.

## Windows Configuration (Phase 6 Step 1)

Normal Windows usage no longer requires manual `CERT_DIR` setup. The application now uses Windows-appropriate user storage:

- **Config file:** `%APPDATA%\TerminalRoom\config.json`
- **Certificates:** `%APPDATA%\TerminalRoom\certs\` (`server.crt` + `server.key`)

Both directories are created automatically with secure permissions (cert directory `0700`, private key `0600`). You can double-click `terminalroom.exe` without worrying about the current working directory.

### config.json

Optional, minimal. If present, it may contain:

```json
{
  "port": 9090,
  "server_addr": "192.168.1.100",
  "endpoint_addr": "192.168.1.100:9090",
  "room_ttl": 600
}
```

Only the fields above are supported. No secrets, tokens, or chat history are ever stored. Missing or malformed `config.json` is ignored and defaults are used.

### Precedence

```
PORT / SERVER_ADDR / ENDPOINT_ADDR / ROOM_TTL:
  Environment variable  >  config.json  >  default

CERT_DIR:
  Environment variable  >  %APPDATA%\TerminalRoom\certs

DEVELOPMENT_MODE:
  Environment variable only (no config file)
```

Examples:

```bash
# config.json port is used if no env var
# PORT env overrides config.json
PORT=8080 terminalroom server

# Use custom cert location (overrides %APPDATA%)
CERT_DIR=D:\my\certs terminalroom server
```

### Defaults

| Setting | Default |
|---------|---------|
| `PORT` / `port` | `9090` |
| `ROOM_TTL` / `room_ttl` | `600` (10 minutes) |
| `SERVER_ADDR` | auto-detect private address |
| `ENDPOINT_ADDR` | auto-detect |
| `CERT_DIR` | `%APPDATA%\TerminalRoom\certs` |
| `DEVELOPMENT_MODE` | unset/false |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server listen port | `9090` |
| `SERVER_ADDR` | Server address for client / bind address | Auto-detect (`localhost:9090` for client default) |
| `ENDPOINT_ADDR` | Advertised server address (overrides bind) | Auto-detected |
| `CERT_DIR` | Certificate directory | `%APPDATA%\TerminalRoom\certs` |
| `ROOM_TTL` | Room time-to-live in seconds | `600` (10 minutes) |
| `DEVELOPMENT_MODE` | Set to `1` to allow `SERVER_ADDR=0.0.0.0` | unset |

## Running Tests

```bash
go test ./...
```

## Development Phase

### Phase 1 — Core Communication COMPLETE
- [x] Basic room creation and joining
- [x] Two-person capacity limit
- [x] Message relay between participants

### Phase 2 — Secure Room Access COMPLETE
- [x] TLS encryption for all communication
- [x] Automatic self-signed certificate generation
- [x] Client certificate pinning (no InsecureSkipVerify)
- [x] Private key restricted permissions (0600)
- [x] Cryptographically secure invitation tokens
- [x] One-time use invitation enforcement (atomic)
- [x] Invitation expiration (10 minutes default)
- [x] Room-specific invitation binding
- [x] Invitation-based authentication flow
- [x] Room-based authorization
- [x] Strict two-person access limit
- [x] Atomic capacity check and join
- [x] Per-participant message relay
- [x] Room message isolation
- [x] Participant identity in protocol
- [x] Dynamic chat display (You/Friend based on participant ID)
- [x] Panic recovery at connection boundary
- [x] Server survives client failures
- [x] Idempotent cleanup
- [x] Sensitive logging (no tokens in logs)
- [x] Comprehensive test coverage

### Phase 3 — Ephemeral Room Engine COMPLETE
- [x] Room lifecycle state machine (Waiting → Active → Expiring → Closing → Destroyed)
- [x] Configurable room TTL (default 10 minutes)
- [x] TTL-based room expiration with timer
- [x] Expiration notifications to participants (SYSTEM messages)
- [x] Grace period before destruction (1s expiry warning + 500ms closing warning)
- [x] Early destruction paths (abandoned rooms, empty rooms)
- [x] Join rejection for expiring/closing/destroyed rooms
- [x] Invitation cleanup on room destruction
- [x] EXPIRY protocol message with RFC3339 timestamp
- [x] Client expiry time display
- [x] Server TTL configuration (`NewWithTTL`)
- [x] Atomic destruction with `sync.Once`
- [x] Broadcast safety during closing state
- [x] Comprehensive Phase 3 test coverage (39+ tests passing)

### Phase 4 — Private Networking COMPLETE
- [x] Private network address discovery (Tailscale, RFC 1918, loopback)
- [x] Private server binding (auto-detect private interface)
- [x] Public network refusal (no fallback)
- [x] Endpoint advertisement (MsgEndpoint protocol message)
- [x] Network diagnostics command (`diagnose`)
- [x] Server address resolution for clients (join flow)
- [x] SERVER_ADDR environment variable support
- [x] ENDPOINT_ADDR environment variable support (advertised address)
- [x] Interface enumeration (all OS interfaces)
- [x] Comprehensive network package tests
- [x] Network address classification tests (Tailscale, RFC 1918, loopback, public)
- [x] Endpoint parsing tests
- [x] Bind/endpoint resolution tests

### Phase 5 — Polished Terminal COMPLETE
- [x] Polished terminal banner and startup presentation
- [x] Room info box display with room details
- [x] Invitation presentation with copy instructions
- [x] Human-friendly relative expiry display ("in 10 minutes")
- [x] Live countdown timer (MM:SS format)
- [x] Progressive expiration warnings (5m, 2m, 1m, 30s)
- [x] Consistent participant identity ("You" / "Friend")
- [x] Session active notification for creator
- [x] Chat command system (/help, /info, /quit)
- [x] /info command with room status, network, and transport details
- [x] Chat timestamps ([HH:MM:SS] format)
- [x] Visible input prompt (>)
- [x] User-friendly error messages (no raw Go errors)
- [x] Clean server startup output (no duplicate banners)
- [x] Polished session end messages
- [x] Terminal formatting utilities package
- [x] Comprehensive terminal package tests
- [x] Removed accidental root `nul` file
- [x] All existing tests continue passing

### Phase 6 (Future)
- GUI
- Windows desktop application
- File transfer
- Voice communication
- Persistent accounts
- Database integration
- Cloud backend

## License

Private project. Not for distribution.
