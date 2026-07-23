package server

import "encoding/json"

// Envelope is the wire shape of every WebSocket message in both directions:
// {"type": "...", "payload": {...}}. See docs/PLAN.md > API + WebSocket
// protocol for the full table of types.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// newEnvelope marshals a typed payload into a ready-to-send Envelope.
func newEnvelope(msgType string, payload interface{}) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Type: msgType, Payload: raw})
}

// --- Client -> Server payloads ---

// JoinPayload is sent right after connecting. Name is only used when no
// existing participant is matched via the connection's ?token=.
type JoinPayload struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// AdminStartPayload is sent by the admin to lock the roster and deal the
// deck (Phase 4). RequiredCount is optional: 0 means "default to the locked
// headcount".
type AdminStartPayload struct {
	Filters       Filters `json:"filters"`
	MaxMovies     int     `json:"maxMovies"`
	RequiredCount int     `json:"requiredCount"`
}

// SwipePayload records one participant's vote on one movie (Phase 4).
type SwipePayload struct {
	MovieID string `json:"movieID"`
	Dir     string `json:"dir"` // "yes" | "no"
}

// UndoPayload takes no fields; it reverses the sender's last swipe (Phase 4).
type UndoPayload struct{}

// AdminEndPayload takes no fields; it force-ends the session (Phase 5).
type AdminEndPayload struct{}

// AdminPickPayload is sent by the admin to pick a winner from the leaderboard (Phase 5).
type AdminPickPayload struct {
	MovieID string `json:"movieID"`
}

// --- Server -> Client payloads ---

// SessionStatePayload is the full snapshot sent to a client right after it
// joins. YourParticipantID/YourToken are only meaningful to the recipient —
// they are never included in the broadcast participant_update payload.
type SessionStatePayload struct {
	Status            Status        `json:"status"`
	Code              string        `json:"code"`
	RequiredCount     int           `json:"requiredCount"`
	Participants      []Participant `json:"participants"`
	YourParticipantID string        `json:"yourParticipantId"`
	YourToken         string        `json:"yourToken"`
}

// DeckPayload is the ordered, capped deck dealt at admin:start (Phase 4).
type DeckPayload struct {
	Movies []Movie `json:"movies"`
}

// ParticipantUpdatePayload carries the current roster after a join, leave,
// or connection-state change.
type ParticipantUpdatePayload struct {
	Participants []Participant `json:"participants"`
}

// ProgressPayload is a lobby/HUD summary of swipe progress; it never reveals
// who voted which way on a specific movie (Phase 4).
type ProgressPayload struct {
	ParticipantsSwiped int `json:"participantsSwiped"`
	ParticipantsTotal  int `json:"participantsTotal"`
	CardsRemaining     int `json:"cardsRemaining"`
}

// MatchPayload announces the winning movie (Phase 4/5).
type MatchPayload struct {
	Movie Movie `json:"movie"`
}

// LeaderboardEntry is one row of the no-match leaderboard (Phase 5).
type LeaderboardEntry struct {
	Movie    Movie `json:"movie"`
	YesCount int   `json:"yesCount"`
}

// SessionEndedPayload is sent when the deck is exhausted with no match
// (Phase 5).
type SessionEndedPayload struct {
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
}

// ErrorPayload reports a rejected action (e.g. joining a started session)
// back to the single client that caused it.
type ErrorPayload struct {
	Message string `json:"message"`
}
