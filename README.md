# terminal-share

Live-share your terminal session over WebSockets. One binary, three commands.

## Install

```bash
go install github.com/nicholas/terminal-share@latest
```

Or build from source:

```bash
git clone https://github.com/nicholas/terminal-share.git
cd terminal-share
go build -o terminal-share .
```

## Usage

### 1. Start the relay server

```bash
terminal-share server -p 8080
```

### 2. Host a session

```bash
terminal-share host -s localhost:8080
```

This spawns your shell in a shared PTY and prints a session ID:

```
Session ID: a3f1c2
Share with: terminal-share join -s localhost:8080 a3f1c2
```

### 3. Join a session

```bash
terminal-share join -s localhost:8080 a3f1c2
```

The viewer sees the host's terminal in real time. Both sides can type.

## Architecture

```
Host's shell <--PTY--> host <--WS--> relay server <--WS--> join <--raw term--> Viewer
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

| Command  | Flag             | Default        | Description          |
|----------|------------------|----------------|----------------------|
| `server` | `-p`, `--port`   | `8080`         | Port to listen on    |
| `host`   | `-s`, `--server` | `localhost:8080` | Relay server address |
| `join`   | `-s`, `--server` | `localhost:8080` | Relay server address |
