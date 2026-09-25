# TerminalRoom Protocol

**Version:** 0.1.0  
**Transport:** TLS 1.2+ (ECDSA P-256, pinned `server.crt`), TCP, newline-delimited text  
**Framing:** `TYPE|PAYLOAD\n` (UTF-8, `|` is first separator, payload may contain `|`)

This document describes the **existing** implementation (`internal/protocol`, `internal/server`, `internal/client`). No redesign.

## 1. Framing

```
Message := Type "|" Payload "\n"   // Payload may be empty, may contain "|", no "\n" inside
Type    := "CREATE" | "JOIN" | "ROOM_ID" | "INV_TOKEN" | "PARTICIPANT_ID" | "READY" | "CHAT" | "LEAVE" | "ERROR" | "SYSTEM" | "FULL" | "NOT_FOUND" | "EXPIRED" | "USED" | "EXPIRY" | "ENDPOINT"
Payload := 0..4096 bytes (MaxPayloadSize), total line ≤8192 (MaxLineSize, includes Type+"|"+Payload+"\n")
```

- Receiver: `protocol.Decode(*bufio.Reader)` via `readLineLimited(8192)` (bounded `ReadSlice` loop, no `ReadString` unbounded, no `io.ReadAll`).
- Sender: `protocol.Send(io.Writer, Message)` validates `len(Payload) ≤4096` and `len(Encode()) ≤8192` before `Fprint`+`Flush`, returns `payload too large` / `message too large` otherwise.
- Malformed (no `|`, no `\n`, empty line, `>8192`, `payload>4096`) → `Decode` returns error; server `handleConnection` sends `ERROR` and closes, `handleParticipant` logs and returns (per-connection only).

## 2. Message Types (actual `protocol.Msg*`)

| Type | Dir | Payload | Meaning |
|------|-----|---------|---------|
| `CREATE` | C→S | `""` or `"<secs>"` (`600`–`3600`, per-room TTL, empty=server default 10m) | Request new room |
| `JOIN` | C→S | `TRINV-48hex` | Join via invitation token |
| `ROOM_ID` | S→C | `TR-6` (`A-Z23456789`) | Created room ID |
| `INV_TOKEN` | S→C | `TRINV-48hex` | Invitation token (single-use, 10m expiry, room-bound) |
| `PARTICIPANT_ID` | S→C | `1` or `2` | Your slot |
| `EXPIRY` | S→C | RFC3339 `2006-01-02T15:04:05Z` | `ExpiresAt` |
| `ENDPOINT` | S→C | `host:port` (private/loopback/tailscale/ULA, validated `ValidateEndpoint`) | Advertised dial address |
| `READY` | S→C | `Secure two-person session established.` | Join succeeded, `ACTIVE` |
| `SYSTEM` | S→C | free text | `Waiting for friend…`, `Friend joined.`, `Room is expiring…`, `Room is closing.` |
| `CHAT` | C→S then S→C | `"<senderID>|<text>"` (`text` 0..4096, user input) S→C is `BroadcastExcept` | Chat, only when `State==ACTIVE`, non-blocking `select ch<-msg: default:` |
| `LEAVE` | C→S / S→C | `""` | Graceful leave; S→C `LEAVE` to peer |
| `ERROR` | S→C | free text | First-message not `CREATE`/`JOIN`, or `CREATE` with invalid TTL |
| `NOT_FOUND` | S→C | `Invalid invitation.` / `Room not found.` | Bad token / no room |
| `EXPIRED` | S→C | `Invitation expired.` | Token `IsExpired` |
| `USED` | S→C | `Invitation already used.` | `Consume` second time |
| `FULL` | S→C | `Room is full.` | `len==2` |

Aliases in prompt: `INVITATION` = `INV_TOKEN`, `MESSAGE` = `CHAT`, others map 1:1.

## 3. Authentication Flow

1. Client `tls.Dial` to `addr` with `ClientConfig(certDir)` (`RootCAs` = pinned `server.crt`, `MinVersion TLS1.2`, `InsecureSkipVerify false`).
2. First message must be `CREATE` or `JOIN`. Anything else → `ERROR` + close.
3. `JOIN` → `invitations.ValidateByToken(token)` (`RLock` lookup → `IsExpired` → `Consume` per-inv `Mu` atomic). Only 1 success for concurrent same token.
4. On success, `rooms.GetRoom(inv.RoomID)` → `room.Join("user2")` (`Lock` + `allowsJoin` + `len<2` atomic). Token remains credential; endpoint is routing only.

## 4. Authorization Flow

* `RoomID` alone insufficient (`TestRoomIDCannotJoin`).
* Checks: token exists, not expired, not consumed, `ConstantTimeCompare(RoomID)`, room `IsDestroyed==false`, `State` allows join (`WAITING`/`ACTIVE` but `len<2`), capacity 2.
* `Validate(token,roomID)` also used for `WrongRoom` test.

## 5. Valid Message Flow

**Create:**
```
C                         S
|--- TLS Handshake (pinned) --->|
|--- CREATE|600|1200..3600 or "" --->|
|                           | CreateRoomWithTTL(ttl) (default 10m), SetOnExpire, StartTimer, Create invitation (10m), Join user1
|<-- ROOM_ID|TR-... --------|
|<-- INV_TOKEN|TRINV-... ---|
|<-- EXPIRY|RFC3339 --------|
|<-- ENDPOINT|host:port ----|
|<-- PARTICIPANT_ID|1 ------|
|<-- SYSTEM|Waiting... -----|
| (chat loop)               |
```

**Join:**
```
C                          S
|--- TLS Handshake -------->|
|--- JOIN|TRINV-... ------->| ValidateByToken→GetRoom→Join user2
|<-- PARTICIPANT_ID|2 ------|
|<-- EXPIRY|... ------------|
|<-- ENDPOINT|... ----------|
|<-- READY|... -------------|
| (both chat: CHAT|1|text ↔ CHAT|2|text via BroadcastExcept) |
```

## 6. Invalid / Errors

* No `|` → `malformed message: ...`
* Empty line → `empty message`
* `>8192` → `message too large: exceeds 8192 bytes` (bounded, no huge alloc)
* `payload>4096` → `payload too large`
* `CREATE` with invalid TTL (`<600`/`>3600`/`abc`/`10m`) → `ERROR|invalid duration: must be 600-3600 seconds (10-60 minutes)` + close, no room created.
* `JOIN` missing token → `ERROR|missing invitation token`
* `JOIN` bad → `NOT_FOUND`/`EXPIRED`/`USED`/`FULL` as above.
* Unauthenticated `CHAT` before `CREATE`/`JOIN` → `ERROR|Expected CREATE or JOIN...`
* `LEAVE` or `Decode` `EOF`/`closed` → `Leave` → `Unregister+close` (owner) → `Leave` → if `len==0` and not `Closing` → `Destroyed` → `RemoveRoom`/`RemoveByRoom`.

## 7. Room Lifecycle Interaction

```
NewRoom (WAITING, len=0, ExpiresAt=Now+TTL) 
  → Join user1 (still WAITING, len=1) 
  → Join user2 (ACTIVE, len=2) 
  → BeginExpiring (from WAITING/ACTIVE) → BroadcastAll(expiring) → Sleep 1s → BroadcastAll(closing) → BeginClosing → Sleep 500ms → FinishDestroying (delete writers, close(done)) → RemoveRoom
  → Leave empty → Destroyed (if not Closing)
```

`CREATED` and `AUTHENTICATING` are logical sub-phases of `WAITING` and the `ValidateByToken→Join` transaction; no extra runtime states (F-09 intentional).

`IsActive` = `State==ACTIVE`; `IsExpiring` = `Expiring||Closing`; `IsDestroyed` = `Destroyed`; `Done` closed once via `sync.Once`.

## 8. Participant Lifecycle

* `Join` assigns `ID = nextSlot++` (1,2), `State→ACTIVE` when 2.
* Each `handleParticipant` registers `writers[id]=chan(64)`, spawns writer `go select { <-sendCh; <-Done; <-quit }` (F-06 owner-close after `delete`).
* `BroadcastExcept`/`BroadcastAll` `RLock` + `State!=Destroyed/Closing` + `select ch<-msg: default:` (non-blocking, slow peer drops).
* On `Leave`/`Decode` error/`quit`, `BroadcastExcept(LEAVE)` to peer, `Unregister` (delete only) + `close(sendCh)` (owner), `Leave` → `Destroy` if empty → `RemoveRoom`.

## 9. Invitation Lifecycle

* `GenerateToken` `crypto/rand` 24B → `TRINV-48hex` (192-bit, `IsValidTokenFormat` hex check).
* `Create(roomID, 10m)` stores `Token, RoomID, CreatedAt, ExpiresAt=Now+10m`.
* `Consume` `Mu` → `if Consumed||IsExpired → false else Consumed=true → true` (single-use, replay `USED`).
* `ValidateByToken` + `GetRoom` + `Join` atomic per step; second concurrent `JOIN` gets `USED`.
* `RemoveByRoom` on `Destroy`/`FinishDestroying` invalidates remaining tokens.
* Bundled `TRINV-...@host:port` — token is credential, endpoint is routing only, validated via `ValidateEndpoint` (private/ULA/loopback/tailscale, rejects `0.0.0.0`/`[::]`/`8.8.8.8`/`example.com`/missing port).

## 10. Room Destruction

* Trigger: `Leave` empties, `Destroy()` explicit, or TTL expiry `FinishDestroying`.
* Effects: `State=Destroyed`, `close(done)` once, `delete(writers)` (no close by Room), `StopExpirationTimer`, `RemoveRoom`, `RemoveByRoom` (invitations), no persistence, `LOG` `Room %s destroyed and removed`, peer gets `LEAVE`/`SYSTEM` then `Decode` EOF → `Friend left. Session ended.`

## 11. Security Properties Preserved

* TLS pinning, `TLS1.2+`, `InsecureSkipVerify false`, ECDSA P-256, `CertValidity 30d`, random 128-bit serial, private/Tailscale/ULA SANs, atomic cert replace, `0600` key.
* Private-network-only (`ValidateBindHost`/`ValidateEndpoint` + `ClassifyAddress` including ULA `fc00::/7`), no public fallback, `0.0.0.0`/`[::]` rejected (F-01/F-02/F-08).
* Invitation single-use, room-bound, 10m expiry, 192-bit.
* Max 2, isolation via per-room `writers` map, `BroadcastExcept` no echo.
* F-05 `4096`/`8192` bounded, F-06 owner-close, F-07 strict `PORT`/`ROOM_TTL`/`config.json` errors, F-03 random serial, F-04 30d.

