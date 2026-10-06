# TapDeck control protocol v2

Control is JSON over pinned WSS `/ws`; native clients only. HTTP `/pair` and `/api/pair-info` are bootstrap endpoints and never expose the QR secret, device token, or UDP keys.

## Pairing and session

HTTP metadata and health return `version=2`; pairing links carry `v=2`. A version mismatch returns `error` with `code=version_mismatch` and an upgrade reason, then closes the socket. Android also checks metadata when restoring saved peers, and stops automatic retry on an incompatible version. TLS identity and existing credentials remain valid across this upgrade.

`hello`: `version=2`, `device_id`, `name`, `token`, `client_nonce` (32 random bytes, Base64URL), optional scanned `secret`.
`pair_challenge`: server nonce, request ID, code and `qr_verified`. Code is the first 16 bytes of SHA-256(`tapdeck-pair-v1` + raw SPKI SHA-256 + client nonce + server nonce), displayed as eight groups of four uppercase hex digits.
`pair_confirm`: matching `code`. URL clients also require approval in the PC window. Requests expire after 120 seconds. HTTP-derived pins remain provisional until both sides approve. QR secrets are one-use and valid for 120 seconds.
`ready`: session ID (16 hex characters), optional new device token, PC configuration, UDP port, two independent AES-256-GCM keys and four-byte nonce prefixes (Base64URL).

Every connection receives new keys. Android uses Keystore-protected persistence; Windows hashes tokens and protects its persistent TLS identity with DPAPI. Unpairing closes the session and removes credentials.

## Reliable controls

`heartbeat` echoes client monotonic `tick`; send every 250 ms, disconnect after 1 second without inbound controls. Clock values are used for RTT, not cross-host one-way subtraction.
`mouse_button`: button (`left`, `right`, `middle`), `down`, old `epoch`, `next_epoch`, cumulative `x/y/scroll_x/scroll_y`. PC flushes the old totals, applies the button, advances epoch, then applies any buffered new-epoch movement. Totals continue across epochs.
`shortcut`: zero-based `slot` and `revision`; a stale revision resynchronizes configuration without executing the key.
`mic_start`: unique hexadecimal `recording` and `mode` (`hold` or `toggle`); server responds `mic_ready` or `mic_error`. The server snapshots that mode and its keys when accepting the start. Client begins capture only after ready and only while the recording is still requested and the app is foreground. A late ready after cancellation is aborted. `mic_stop` drains at most 60 ms; the configured stop delay applies to voice hotkeys. `mic_abort` drops buffered audio immediately. `mic_stopped` ends the recording lifecycle.
`config`: PC-owned configuration with `schema_version=2`, `revision`, exactly eight shortcut slots (`label`, `chord`, `enabled`), `voice`, sensitivity and scroll direction. One to eight slots must be enabled. `shortcut.slot` remains the original zero-based slot 0–7, not the filtered UI position; disabled slots and stale revisions do not inject keys.

`voice`: `hold_key`, `toggle_start_key`, `toggle_stop_key`, `stop_delay_ms`. Empty keys mean audio only. Hold mode presses/releases the hold key; toggle mode sends a complete chord for each start/stop. Duplicate stops do not restart drain, and late drain completion or old recording IDs cannot end another recording. Aborts, device failure and disconnect immediately release held keys or send the snapshotted toggle-stop chord once. Reconnect never replays shortcuts or resumes recording.

Windows configuration migration backs up the exact legacy file as `config.v1.bak`, enables its original four slots, adds four disabled slots, and maps the old voice profile to the corresponding v2 fields. The UDP encoding below intentionally remains `TDK1` with unchanged crypto vectors.

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
