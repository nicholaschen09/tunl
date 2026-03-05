package cmd

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nicholaschen09/tunl/protocol"
	"github.com/spf13/cobra"
)

var serverPort int

var serverCmd = &cobra.Command{
	Use:     "server",
	Aliases: []string{"srv", "s"},
	Short:   "Start the WebSocket relay server",
	RunE:    runServer,
}

func init() {
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 8080, "port to listen on")
}

type session struct {
	mu      sync.Mutex
	host    *websocket.Conn
	hostMu  sync.Mutex // serializes writes to the host conn
	viewers []*websocket.Conn
}

var (
	sessions   = make(map[string]*session)
	sessionsMu sync.Mutex
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func runServer(cmd *cobra.Command, args []string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS)

	addr := fmt.Sprintf(":%d", serverPort)
	log.Printf("relay server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	role := r.URL.Query().Get("role")

	if sessionID == "" || (role != "host" && role != "viewer") {
		http.Error(w, "missing session or invalid role", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}

	switch role {
	case "host":
		handleHost(conn, sessionID)
	case "viewer":
		handleViewer(conn, sessionID)
	}
}

func handleHost(conn *websocket.Conn, id string) {
	sessionsMu.Lock()
	if _, exists := sessions[id]; exists {
		sessionsMu.Unlock()
		conn.WriteMessage(websocket.BinaryMessage, protocol.Encode(protocol.MsgClose, []byte("session already exists")))
		conn.Close()
		return
	}
	s := &session{host: conn}
	sessions[id] = s
	sessionsMu.Unlock()

	log.Printf("session %s: host connected", id)
	defer func() {
		sessionsMu.Lock()
		delete(sessions, id)
		sessionsMu.Unlock()

		s.mu.Lock()
		for _, v := range s.viewers {
			v.WriteMessage(websocket.BinaryMessage, protocol.Encode(protocol.MsgClose, []byte("host disconnected")))
			v.Close()
		}
		s.viewers = nil
		s.mu.Unlock()

		conn.Close()
		log.Printf("session %s: host disconnected", id)
	}()

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.BinaryMessage {
			continue
		}

		s.mu.Lock()
		alive := s.viewers[:0]
		for _, v := range s.viewers {
			if err := v.WriteMessage(websocket.BinaryMessage, data); err != nil {
				v.Close()
				continue
			}
			alive = append(alive, v)
		}
		s.viewers = alive
		s.mu.Unlock()
	}
}

func handleViewer(conn *websocket.Conn, id string) {
	sessionsMu.Lock()
	s, exists := sessions[id]
	sessionsMu.Unlock()

	if !exists {
		conn.WriteMessage(websocket.BinaryMessage, protocol.Encode(protocol.MsgClose, []byte("session not found")))
		conn.Close()
		return
	}

	s.mu.Lock()
	s.viewers = append(s.viewers, conn)
	s.mu.Unlock()

	log.Printf("session %s: viewer connected (%d total)", id, len(s.viewers))
	defer func() {
		s.mu.Lock()
		for i, v := range s.viewers {
			if v == conn {
				s.viewers = append(s.viewers[:i], s.viewers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		conn.Close()
		log.Printf("session %s: viewer disconnected", id)
	}()

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.BinaryMessage {
			continue
		}

		s.hostMu.Lock()
		err = s.host.WriteMessage(websocket.BinaryMessage, data)
		s.hostMu.Unlock()
		if err != nil {
			return
		}
	}
}
