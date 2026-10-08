# TapDeck control protocol v2

Control is JSON over pinned WSS `/ws`; native clients only. HTTP `/pair` and `/api/pair-info` are bootstrap endpoints and never expose device tokens or UDP keys. The PC QR contains the HTTP `/pair` URL, used to install/open the Android app.

Application versions are independent of wire version 2. See [versioning](../docs/versioning.md) for package delivery and [verification](../docs/verification.md) for tested combinations.

## Pairing and session

HTTP metadata and health return `version=2`; pairing links carry `v=2`. A version mismatch returns `error` with `code=version_mismatch` and an upgrade reason, then closes the socket. Android also checks metadata when restoring saved peers, and stops automatic retry on an incompatible version. TLS identity and existing credentials remain valid across this upgrade.

| Message | Fields and behaviour |
|---|---|
| `hello` | `version=2`, `device_id`, `name`, `token`, `client_nonce` (32 random bytes, Base64URL). Legacy `secret` is ignored and never authorizes pairing. |
| `pair_challenge` | Server nonce, request ID, code and legacy `qr_verified=false`. Code is the first 16 bytes of SHA-256(`tapdeck-pair-v1` + raw SPKI SHA-256 + client nonce + server nonce), shown as eight groups of four uppercase hex digits. |
| `pair_confirm` | Legacy matching `code`, accepted and ignored. First pairing always requires PC approval after comparing codes; expires after 120 seconds. Valid saved tokens reconnect without another approval. |

Since 0.3.15, explicit refusal (including closing the PC prompt) returns `error` with `code=pairing_rejected`; an approval deadline returns `code=pairing_expired`. Both include a human-readable `reason` and then close the connection. Android 0.3.15 stops retrying that attempt, preserves unrelated saved credentials, and waits for another explicit connect action. Older clients may ignore these additive reasons; the wire version remains 2. The receiver keeps one reader active through pending approval and the established session, removes disconnected requests promptly, ignores controls until approval, and rechecks expiry before accepting an approval.

`ready`: session ID (16 hex characters), optional new device token, PC configuration, UDP port, two independent AES-256-GCM keys and four-byte nonce prefixes (Base64URL). Windows 0.3.3 adds optional `features` (`touchpad_zoom`, `three_finger`) and `double_click_ms` (actual Windows setting). Missing capabilities disable only the new gestures; Android then shows an upgrade notice when attempted. Missing/invalid double-click time defaults to 500 ms. Old clients can ignore these fields; the version stays 2.

### Identity, local settings and ownership

PC configuration retains `sensitivity=2.0` for older clients. Android applies a local multiplier (0.5–3.0, default 1.0) before encoding mouse movement; PC updates do not overwrite it. Scroll, zoom and gesture thresholds are unscaled. Haptic feedback, keyboard mode and control position are local preferences with no wire fields.

Every connection receives new keys. Android uses Keystore-protected persistence; Windows hashes tokens and protects its persistent TLS identity with DPAPI. Per-device unpairing persists the removal before closing only that device's sessions, invalidates pending approvals and rechecks authorization before admitting racing sessions. A persistence failure leaves the original authorization active.

Android 0.3.16 stores multiple PCs in an encrypted local catalog keyed by normalized SPKI SHA-256, and retains a single active connection and the same phone device ID. Switching releases the old session's inputs, invalidates queued callbacks/datagrams and connects only the selected PC; recording preparation/transmission/stopping blocks user target changes. Aliases and voice-profile selection are per-PC local preferences. The old single-peer record and voice preference migrate atomically. Revocation clears only the matching token and preserves the named catalog entry for manual re-pairing; failed/cancelled new pairings preserve all existing records and are not restored. No wire fields, protocol version or PC configuration schema change.

`error` with `code=pairing_revoked` precedes revocation closure and is also returned for invalid saved tokens. Android 0.3 clears the matching computer credential, stops automatic retry, and requires a new user connection followed by PC approval. Legacy clients may ignore this error but cannot reuse the token. The local device-list API exposes only IDs, names, times and online state; the v2 pairing store backs up legacy records to `paired.v1.bak`.

Keyboard queues, held keys, mouse-button references and voice tokens belong to a session. Disconnect/unpair cancels that session's queued/executing actions and releases its inputs; global release is reserved for stopping the receiver or exiting. The audio engine admits one recording at a time and uses an internal recording ID distinct from the client wire ID; push, stop and abort all check ownership. There is no multi-device mixing.

## Reliable controls

### Heartbeat and mouse barriers

`heartbeat` echoes client monotonic `tick`; send every 250 ms, disconnect after 1 second without inbound controls. Clock values are used for RTT, not cross-host one-way subtraction.

`mouse_button`: button (`left`, `right`, `middle`), `down`, old `epoch`, `next_epoch`, cumulative `x/y/scroll_x/scroll_y`. PC flushes the old totals, applies the button, advances epoch, then applies any buffered new-epoch movement. Totals continue across epochs.

`zoom`: nonzero integer `steps` from -4 to +4 (positive enlarges), plus the same `epoch`, `next_epoch`, cumulative `x/y/scroll_x/scroll_y` barrier. `gesture`: `action` is strictly `up` or `down`, with that barrier. Both are reliable WSS controls. The PC settles preceding movement/scroll, accepts an owner-scoped worker action and advances the epoch; late old-epoch packets are ignored. Queue/admission failure closes the session rather than leaving client/server epochs divergent. Larger Android zoom bursts are split into consecutive messages.

Zoom sends Ctrl-down, vertical wheel (`steps × 120`) and Ctrl-up as one SendInput batch, adding/removing Ctrl only if it was not already held. All input backends share modifier ownership. Alt/Shift/Win conflict with zoom; any modifier conflicts with window shortcuts. Worker execution failures return `error` with `code=touchpad_error` and a user-facing reason. Queued/executing gestures are canceled by owner revocation or disconnect, without clearing other sessions' held keys or voice actions. The worker globally tracks normal windows / desktop / Task View, reconciled by native foreground and visibility events; three-finger operations are Win+D, Win+Tab and Esc.
### Keyboard controls

| Message | Fields and behaviour |
|---|---|
| `shortcut` | Zero-based `slot` 0–7 and `revision`; sends one complete chord. |
| `shortcut_hold_start` | `slot`, `revision`, unique nonempty `hold` ID (up to 64 characters); holds the configured chord. Repeating an active ID is a no-op. |
| `shortcut_hold_stop` | Matching `hold`; releases the stored chord regardless of later configuration changes. Unknown IDs are ignored. |
| `key_down` / `key_up` | `text` (nonempty, up to 128 characters), `revision`; per-session held-key ownership. An unmatched `key_up` is ignored. |

Stale revisions return the latest `config` without injecting input. Disabled slots are rejected. Disconnect and revocation cancel only the owning session's input.

### Voice lifecycle

`mic_start`: unique hexadecimal `recording`, `mode` (`hold` or `toggle`), and, when the PC advertises `voice_profiles`, `profile_id` and configuration `revision`. The PC selects the exact enabled ID and checks revision; unknown/disabled IDs or stale revisions return `mic_error` with the same `recording`, followed by the latest `config`, without starting audio or injecting keys. Legacy mode-only requests select the first enabled profile of that type; no match returns the same error/recovery pair. The server snapshots the profile ID, name, mode, keys and stop delay on acceptance. Client begins capture only after `mic_ready` and only while the recording is still requested and the app is foreground. A late ready after cancellation is aborted. `mic_stop` drains at most 60 ms; the configured stop delay applies to voice hotkeys. `mic_abort` drops buffered audio immediately. `mic_stopped` ends the recording lifecycle.

### PC configuration and compatibility

`config`: PC-owned configuration with `schema_version=3` since 0.3.9, `revision`, exactly eight shortcut slots (`label`, `chord`, `enabled`), `voice`, sensitivity and scroll direction. Control protocol remains v2. One to eight shortcut slots must be enabled; zero to three voice profiles may be enabled. `shortcut.slot` remains the original zero-based slot 0–7, not the filtered UI position; disabled slots and stale revisions do not inject keys.

`voice`: `profiles` and global `stop_delay_ms` (0–1000, default 200). `profiles` contains exactly three entries in stable ID order `voice-1`, `voice-2`, `voice-3`. Each has `id`, `name`, `enabled`, `mode`, `hold_key`, `toggle_start_key`, `toggle_stop_key`; hidden keys remain stored across mode changes. Names trim whitespace, require a nonempty result, and permit at most 16 units (ASCII code point = 1, other Unicode code point = 2). Empty keys mean audio only. Hold mode presses/releases the hold key; toggle mode sends a complete chord for each start/stop. Duplicate stops do not restart drain, and late drain completion or old recording IDs cannot end another recording. Aborts, device failure and disconnect immediately release held keys or send the snapshotted toggle-stop chord once. Reconnect never replays shortcuts or resumes recording.

Windows 0.3.9 advertises `voice_profiles` in `ready.features`. For legacy readers, flat `voice.hold_key`, `toggle_start_key`, `toggle_stop_key` project the first enabled profile of the respective type, or empty values when none exists. Android connecting to a PC without this capability displays the legacy two groups and sends mode-only requests. The phone's trigger gesture is distinct from the selected PC hotkey mode: holding Space sends the selected profile ID/mode but always stops on release. Only a sticky toggle recording consumes the first touchpad click to stop recording. All voice selection changes are disabled during preparing/transmitting/stopping; live PC changes apply to the next recording.

Windows schema 0/1 migration expands four shortcuts to eight and maps the old voice mode; schema 2 retains its eight slots. Both migrate existing toggle keys into `voice-1` and hold keys into `voice-2`, preserving empty keys, and add disabled `voice-3`. Since 0.3.12 its name is “自定义语音输入”, mode is `hold`, and all hotkeys are empty; existing schema 3 entries remain unchanged. New installs enable toggle `RightCtrl+L` / `RightCtrl+L` and hold `RightAlt`. Original bytes are backed up as `config.v1.bak` or `config.v2.bak`, with numeric suffixes if a different backup already exists; validation/backup/save failure leaves the original config intact. UDP remains `TDK1` with unchanged crypto vectors.

Windows 0.3 adds the optional PC-only `keyboard_backend` field (`auto`, `hid`, or `sendinput`; missing means `auto`). This does not change control version 2 or the Android audio protocol. Keyboard pulses run in a child process over inherited anonymous pipes and hold for 50 ms. `mic_ready` is sent after the snapshotted start action completes. A stop received while preparing cancels queued start actions immediately; late completion cannot revive capture. Child input state is cleaned on pipe EOF, including one snapshotted toggle-stop action if its start had reached key-down. Unsupported HID keys use the software backend in auto mode, and shared modifier ownership spans both routes.

## UDP encoding

All integers are little endian. Packets are at most 1200 bytes.

| Header offset | Size | Meaning |
|---|---:|---|
| 0 | 4 | ASCII `TDK1` |
| 4 | 1 | 1=mouse, 2=audio |
| 5 | 3 | Reserved zero |
| 8 | 8 | Session ID |
| 16 | 8 | Unsigned channel sequence |
| 24 | variable | AES-GCM ciphertext followed by 16-byte tag |

Nonce = channel prefix (4 bytes) + little-endian sequence (8 bytes). The entire 24-byte header is AAD. Sequences belong to the whole WSS connection; stopping/restarting the microphone never resets them. Mouse accepts only the latest sequence. Audio permits a 256-packet replay window, committed only after authentication.

Mouse plaintext is 36 bytes: uint32 epoch and four int64 cumulative totals in 1/1024 PC pixel or wheel unit. Audio plaintext is 976 bytes: uint64 recording ID, uint64 sample position, then 480 signed 16-bit mono samples at 48 kHz. Sample position is divisible by 480. Missing audio is concealed, expired recordings rejected, queues bounded to six frames. Small sample slips compensate independent device clocks.

Kotlin and Go tests share the deterministic AES-GCM vector for key bytes 00..1f, prefix 11223344, session 0102030405060708, sequence 9 and movement (3,1024,-2048,0,122880):

```
54444b310100000008070605040302010900000000000000733488da86b890b41259a3809e84f94ad41b8662462290bac51fca3471e832104297707dd4debe4b3dc28aaaf5a7aaf549e21408
```
