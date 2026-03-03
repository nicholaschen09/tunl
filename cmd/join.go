package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/gorilla/websocket"
	"github.com/nicholas/terminal-share/protocol"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var joinServer string

var joinCmd = &cobra.Command{
	Use:     "join <session-id>",
	Aliases: []string{"j"},
	Short:   "Join a shared terminal session",
	Args:    cobra.ExactArgs(1),
	RunE:    runJoin,
}

func init() {
	joinCmd.Flags().StringVarP(&joinServer, "server", "s", "localhost:8080", "relay server address")
}

func runJoin(cmd *cobra.Command, args []string) error {
	sessionID := args[0]

	u := url.URL{
		Scheme:   "ws",
		Host:     joinServer,
		Path:     "/ws",
		RawQuery: fmt.Sprintf("session=%s&role=viewer", sessionID),
	}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("connect to relay: %w", err)
	}
	defer ws.Close()

	isTTY := term.IsTerminal(int(os.Stdin.Fd()))
	if isTTY {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("make raw: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	}

	done := make(chan struct{})

	// WS output -> local stdout
	go func() {
		defer close(done)
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
			case protocol.MsgOutput:
				os.Stdout.Write(payload)
			case protocol.MsgResize:
				// Future: could resize local terminal
			case protocol.MsgClose:
				fmt.Fprintf(os.Stderr, "\r\n[%s]\r\n", string(payload))
				return
			}
		}
	}()

	// Local stdin -> WS input
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				wsErr := ws.WriteMessage(websocket.BinaryMessage, protocol.Encode(protocol.MsgInput, buf[:n]))
				if wsErr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					return
				}
				return
			}
		}
	}()

	<-done
	return nil
}
