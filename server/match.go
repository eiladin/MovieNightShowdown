package server

// recordSwipe records a vote and returns the winning movie if this swipe caused
// a match. Caller must hold the session lock.
func (s *Session) recordSwipe(participantID, movieID string, yes bool) (winner *Movie, matched bool) {
	if s.Votes[movieID] == nil {
		s.Votes[movieID] = map[string]bool{}
	}
	s.Votes[movieID][participantID] = yes
	s.LastSwipe[participantID] = Swipe{MovieID: movieID, Yes: yes}

	if !yes {
		return nil, false // a "no" can never create a match (secret-kill)
	}
	// Win = every participant voted, and all votes are "yes".
	votes := s.Votes[movieID]
	if len(votes) != s.RequiredCount {
		return nil, false
	}
	for _, v := range votes {
		if !v {
			return nil, false
		}
	}
	return s.findMovie(movieID), true
}

// findMovie returns a pointer to the Deck entry with the given ID, or nil if
// it is not in the deck. Caller must hold the session lock (only called from
// recordSwipe, above).
func (s *Session) findMovie(movieID string) *Movie {
	for i := range s.Deck {
		if s.Deck[i].ID == movieID {
			return &s.Deck[i]
		}
	}
	return nil
}

// progressLocked summarizes swipe progress after a vote affecting movieID,
// without revealing any individual participant's vote (see ProgressPayload).
// Caller must hold s.mu.
func (s *Session) progressLocked(movieID string) ProgressPayload {
	resolved := 0
	for _, votes := range s.Votes {
		if len(votes) >= s.RequiredCount {
			resolved++
		}
	}
	remaining := len(s.Deck) - resolved
	if remaining < 0 {
		remaining = 0
	}
	return ProgressPayload{
		ParticipantsSwiped: len(s.Votes[movieID]),
		ParticipantsTotal:  s.RequiredCount,
		CardsRemaining:     remaining,
	}
}
