# tunl

https://github.com/user-attachments/assets/3e0c996c-7968-407f-bd25-4d8fa369b94d

Live-share your terminal session over WebSockets. One binary, three commands.

## Install

```bash
go install github.com/nicholaschen09/tunl@latest
```

Or build from source:

```bash
git clone https://github.com/nicholaschen09/tunl.git
cd tunl
go build -o tshare .
```

## Quick Start

**1. Start the relay server**

```bash
tshare server
```

**2. Host a session** (in another terminal)

```bash
tshare host
```

This spawns your shell in a shared PTY and prints a session ID:

```
Session ID: a3f1c2
Share with: tshare join -s localhost:8080 a3f1c2
```

**3. Join a session**

```bash
tshare join a3f1c2
```

The viewer sees the host's terminal in real time. 

## Sharing Over the Internet

Use [ngrok](https://ngrok.com) to expose your relay server publicly:

```bash
# terminal 1 — start the relay
tshare server

# terminal 2 — tunnel it through ngrok
ngrok http 8080
```

ngrok will print a URL like `https://abc123.ngrok-free.app`. Use it with port 443:

```bash
# terminal 3 — host a session
tshare host -s abc123.ngrok-free.app:443

# on any other machine — join
tshare join -s abc123.ngrok-free.app:443 a3f1c2
```

TLS is auto-detected when the server address uses port 443 or contains `.ngrok`.

## Architecture

```
Host's shell ←PTY→ host ←WS→ relay server ←WS→ join ←raw term→ Viewer
```

- **server** — WebSocket relay that routes messages between one host and any number of viewers per session.
- **host** — Spawns a PTY, streams output to the relay, and accepts input from viewers.
- **join** — Puts the terminal in raw mode, displays host output, and sends keystrokes back.

## Wire Protocol

Binary WebSocket frames with a 1-byte type prefix:

| Byte | Type   | Payload              |
|------|--------|----------------------|
| 0x01 | Output | Raw PTY bytes        |
| 0x02 | Input  | Raw keystrokes       |
| 0x03 | Resize | cols + rows (uint16) |
| 0x04 | Close  | Optional reason      |

## Flags

| Command | Flag             | Default          | Description          |
|---------|------------------|------------------|----------------------|
| server  | `-p`, `--port`   | `8080`           | Port to listen on    |
| host    | `-s`, `--server` | `localhost:8080` | Relay server address |
| join    | `-s`, `--server` | `localhost:8080` | Relay server address |

Each command also has a short alias: `server`→`srv`/`s`, `host`→`h`, `join`→`j`.
