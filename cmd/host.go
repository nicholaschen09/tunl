package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/nicholas/terminal-share/protocol"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var hostServer string

var hostCmd = &cobra.Command{
	Use:     "host",
	Aliases: []string{"h"},
	Short:   "Host a shared terminal session",
	RunE:    runHost,
}

func init() {
	hostCmd.Flags().StringVarP(&hostServer, "server", "s", "localhost:8080", "relay server address")
}

func genSessionID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func runHost(cmd *cobra.Command, args []string) error {
	sessionID := genSessionID()

	scheme := "ws"
	if strings.HasSuffix(joinServer, ":443") || strings.Contains(joinServer, ".ngrok") {
		scheme = "wss"
	}
	u := url.URL{
		Scheme:   scheme,
		Host:     joinServer,
		Path:     "/ws",
		RawQuery: fmt.Sprintf("session=%s&role=viewer", sessionID),
	}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("connect to relay: %w", err)
	}
	defer ws.Close()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	c := exec.Command(shell)
	c.Env = os.Environ()
	ptmx, err := pty.Start(c)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}
	defer ptmx.Close()

	isTTY := term.IsTerminal(int(os.Stdin.Fd()))
	if isTTY {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("make raw: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)

		if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
		}
	}

	fmt.Fprintf(os.Stderr, "\r\nSession ID: %s\r\nShare with: terminal-share join -s %s %s\r\n\r\n", sessionID, hostServer, sessionID)

	done := make(chan struct{})

	if isTTY {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		go func() {
			for range sigCh {
				if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
					pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
					ws.WriteMessage(websocket.BinaryMessage, protocol.EncodeResize(uint16(cols), uint16(rows)))
				}
			}
		}()
	}

	// PTY output -> local stdout + WS broadcast
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
				ws.WriteMessage(websocket.BinaryMessage, protocol.Encode(protocol.MsgOutput, buf[:n]))
			}
			if err != nil {
				break
			}
		}
		close(done)
	}()

	// WS input from viewers -> PTY
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			msgType, payload, err := protocol.Decode(data)
			if err != nil {
				continue
			}
			switch msgType {
			case protocol.MsgInput:
				ptmx.Write(payload)
			case protocol.MsgClose:
				return
			}
		}
	}()

	if isTTY {
		go func() {
			io.Copy(ptmx, os.Stdin)
		}()
	}

	// Wait for the shell to exit
	<-done
	c.Wait()
	ws.WriteMessage(websocket.BinaryMessage, protocol.Encode(protocol.MsgClose, []byte("session ended")))

	log.SetOutput(os.Stderr)
	fmt.Fprintf(os.Stderr, "\r\nSession ended.\r\n")
	return nil
}
