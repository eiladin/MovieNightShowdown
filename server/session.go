package server

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status is the lifecycle state of a Session.
type Status string

const (
	StatusLobby   Status = "lobby"
	StatusActive  Status = "active"
	StatusMatched Status = "matched"
	StatusEnded   Status = "ended"
)

// Swipe records a participant's most recent vote, kept for undo.
type Swipe struct {
	MovieID string `json:"movieId"`
	Yes     bool   `json:"yes"`
}

// Participant is one device/person in a session. Token is never sent to
// other participants (json:"-"); it is only handed to its own owner, once,
// via the REST create-session response or a session_state message addressed
// to that participant.
type Participant struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsAdmin   bool   `json:"isAdmin"`
	Connected bool   `json:"connected"`
	Token     string `json:"-"`
}

// Session is one in-memory "movie night". See docs/PLAN.md > Data model.
type Session struct {
	Code          string
	AdminID       string
	RequiredCount int
	Locked        bool
	Status        Status
	Deck          []Movie
	Participants  map[string]*Participant
	Votes         map[string]map[string]bool // movieID -> (participantID -> yes?)
	LastSwipe     map[string]Swipe           // participantID -> last swipe (for undo)
	WinnerID      string
	CreatedAt     time.Time

	// mu guards every mutable field above plus clients. Hold it for the
	// shortest time possible; never block on a channel send while holding it.
	mu      sync.Mutex
	clients map[string]*Client // participantID -> currently attached connection
}

// codeAlphabet excludes visually-confusable characters (0/O, 1/I).
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomCode() string {
	b := make([]byte, 4)
	for i := range b {
		b[i] = codeAlphabet[rand.Intn(len(codeAlphabet))]
	}
	return string(b)
}

// Store holds every live Session, keyed by join code.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewStore creates a Store and starts its TTL sweeper goroutine.
func NewStore(ttl time.Duration) *Store {
	st := &Store{sessions: map[string]*Session{}, ttl: ttl}
	go st.sweepExpired()
	return st
}

// Create makes a new Session with a unique code and the admin as its first
// (not-yet-connected) participant.
func (st *Store) Create(adminName string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()

	code := st.uniqueCodeLocked()
	adminID := uuid.NewString()

	session := &Session{
		Code:    code,
		AdminID: adminID,
		Status:  StatusLobby,
		Participants: map[string]*Participant{
			adminID: {ID: adminID, Name: adminName, IsAdmin: true, Token: uuid.NewString()},
		},
		Votes:     map[string]map[string]bool{},
		LastSwipe: map[string]Swipe{},
		CreatedAt: time.Now(),
		clients:   map[string]*Client{},
	}
	st.sessions[code] = session
	return session
}

// Get looks up a Session by its join code.
func (st *Store) Get(code string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[code]
	return s, ok
}

// uniqueCodeLocked generates a code not already in use. Caller must hold st.mu.
func (st *Store) uniqueCodeLocked() string {
	for {
		code := randomCode()
		if _, exists := st.sessions[code]; !exists {
			return code
		}
	}
}

// sweepExpired periodically removes sessions older than the configured TTL.
// Runs for the lifetime of the process; intended to be started via `go`.
func (st *Store) sweepExpired() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		st.mu.Lock()
		now := time.Now()
		for code, s := range st.sessions {
			if now.Sub(s.CreatedAt) > st.ttl {
				s.Close()
				delete(st.sessions, code)
			}
		}
		st.mu.Unlock()
	}
}

// Close gracefully terminates all connected clients when the session expires.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.clients {
		if c.conn != nil {
			c.conn.Close()
		}
	}
}

// --- POST /api/sessions ---

type createSessionRequest struct {
	AdminName string `json:"adminName"`
}

type createSessionResponse struct {
	Code          string `json:"code"`
	JoinURL       string `json:"joinURL"`
	ParticipantID string `json:"participantId"`
	Token         string `json:"token"`
}

// handleCreateSession lets an admin start a new session. The admin becomes
// participant #1; the response carries the resume token their browser must
// keep (localStorage) and send back on the /ws connection.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.AdminName = strings.TrimSpace(req.AdminName)
	if req.AdminName == "" {
		http.Error(w, "adminName is required", http.StatusBadRequest)
		return
	}

	session := s.store.Create(req.AdminName)
	admin := session.Participants[session.AdminID]

	resp := createSessionResponse{
		Code:          session.Code,
		JoinURL:       strings.TrimRight(s.cfg.PublicURL, "/") + "/join/" + session.Code,
		ParticipantID: admin.ID,
		Token:         admin.Token,
	}

	log.Printf("session created: code=%s admin=%s", session.Code, req.AdminName)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
