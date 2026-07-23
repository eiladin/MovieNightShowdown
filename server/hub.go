package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 8192
	sendBufferSize = 16
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// LAN-only app with no auth (see docs/TASKS.md > Locked product rules);
	// there is nothing origin-checking would protect here.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client is one WebSocket connection. Exactly one goroutine (writePump)
// writes to conn and exactly one goroutine (readPump) reads from it — never
// write to conn from anywhere else.
type Client struct {
	conn          *websocket.Conn
	send          chan []byte
	session       *Session
	participantID string // set once join() attaches this client to a participant
	token         string // from ?token=; used to match/resume a participant
}

// handleWS upgrades GET /ws?code=&token= to a WebSocket connection and
// starts the client's read/write pumps.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	token := r.URL.Query().Get("token")

	session, ok := s.store.Get(code)
	if !ok {
		http.Error(w, "unknown session code", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade failed: %v", err)
		return
	}

	client := &Client{
		conn:    conn,
		send:    make(chan []byte, sendBufferSize),
		session: session,
		token:   token,
	}

	go client.writePump()
	go client.readPump()
}

// readPump is the only goroutine that reads from conn. It dispatches
// messages and, on exit (error/close), detaches the client from its session
// and signals writePump to stop by closing send.
func (c *Client) readPump() {
	defer func() {
		c.session.removeClient(c)
		close(c.send)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			log.Printf("ws: invalid envelope: %v", err)
			continue
		}
		c.handleMessage(env)
	}
}

// writePump is the only goroutine that writes to conn.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// trySend enqueues a message for writePump without ever blocking the
// caller. A full buffer means an unusually slow client; the message is
// dropped rather than stalling the broadcaster.
func (c *Client) trySend(data []byte) {
	select {
	case c.send <- data:
	default:
		log.Printf("ws: send buffer full for participant %s, dropping message", c.participantID)
	}
}

func (c *Client) sendJSON(msgType string, payload interface{}) {
	data, err := newEnvelope(msgType, payload)
	if err != nil {
		log.Printf("ws: marshal %s: %v", msgType, err)
		return
	}
	c.trySend(data)
}

func (c *Client) sendError(message string) {
	c.sendJSON("error", ErrorPayload{Message: message})
}

func (c *Client) handleMessage(env Envelope) {
	switch env.Type {
	case "join":
		c.handleJoin(env.Payload)
	default:
		// swipe/undo/admin:start/admin:end land in later phases.
		log.Printf("ws: unhandled message type %q", env.Type)
	}
}

// handleJoin attaches this connection to a participant: an existing one if
// c.token matches, otherwise a new one (only while the session is still in
// the Lobby). It then sends session_state to this client and broadcasts
// participant_update to everyone.
func (c *Client) handleJoin(raw json.RawMessage) {
	var p JoinPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		c.sendError("invalid join payload")
		return
	}

	session := c.session
	session.mu.Lock()

	participant := findParticipantByTokenLocked(session, c.token)
	if participant == nil {
		if session.Status != StatusLobby {
			session.mu.Unlock()
			c.sendError("session already started")
			return
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = "Guest"
		}
		participant = &Participant{
			ID:    uuid.NewString(),
			Name:  name,
			Token: uuid.NewString(),
		}
		session.Participants[participant.ID] = participant
	}

	participant.Connected = true
	c.participantID = participant.ID
	c.token = participant.Token
	session.clients[participant.ID] = c

	state := SessionStatePayload{
		Status:            session.Status,
		Code:              session.Code,
		RequiredCount:     session.RequiredCount,
		Participants:      participantViewsLocked(session),
		YourParticipantID: participant.ID,
		YourToken:         participant.Token,
	}
	session.mu.Unlock()

	c.sendJSON("session_state", state)
	session.broadcastParticipants()
}

// findParticipantByTokenLocked returns the participant whose Token matches,
// or nil if token is empty or unmatched. Caller must hold session.mu.
func findParticipantByTokenLocked(session *Session, token string) *Participant {
	if token == "" {
		return nil
	}
	for _, p := range session.Participants {
		if p.Token == token {
			return p
		}
	}
	return nil
}

// participantViewsLocked returns a stable-ordered, wire-safe copy of the
// roster (Token is excluded via its json:"-" tag). Caller must hold session.mu.
func participantViewsLocked(session *Session) []Participant {
	views := make([]Participant, 0, len(session.Participants))
	for _, p := range session.Participants {
		views = append(views, *p)
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views
}

// removeClient detaches c from its session if it is still the active
// connection for its participant, marks the participant disconnected, and
// broadcasts the updated roster. The participant record itself is kept so
// it can resume later with its token.
func (s *Session) removeClient(c *Client) {
	s.mu.Lock()
	if c.participantID == "" {
		s.mu.Unlock()
		return
	}
	current, ok := s.clients[c.participantID]
	if !ok || current != c {
		// A newer connection already replaced this one (fast reconnect);
		// leave that connection's state alone.
		s.mu.Unlock()
		return
	}
	delete(s.clients, c.participantID)
	if p, ok := s.Participants[c.participantID]; ok {
		p.Connected = false
	}
	s.mu.Unlock()

	s.broadcastParticipants()
}

// broadcastParticipants sends the current roster to every attached client.
func (s *Session) broadcastParticipants() {
	s.mu.Lock()
	views := participantViewsLocked(s)
	s.mu.Unlock()

	s.broadcast("participant_update", ParticipantUpdatePayload{Participants: views})
}

// broadcast sends msgType/payload to every client currently attached to the
// session. It reads the client list under the lock, then sends outside it
// so a slow/blocked client can never stall session mutation.
func (s *Session) broadcast(msgType string, payload interface{}) {
	data, err := newEnvelope(msgType, payload)
	if err != nil {
		log.Printf("ws: marshal broadcast %s: %v", msgType, err)
		return
	}

	s.mu.Lock()
	clients := make([]*Client, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()

	for _, c := range clients {
		c.trySend(data)
	}
}
