package server

import "sort"

// recordSwipe records a vote and returns the winning movie if this swipe caused
// a match. Caller must hold the session lock.
func (s *Session) recordSwipe(participantID, movieID string, yes bool) (winner *Movie, matched bool) {
	if s.findMovie(movieID) == nil {
		return nil, false // ignore votes for movies not in the deck (defensive: guarantees a matched result never has a nil winner)
	}
	if s.Votes[movieID] == nil {
		s.Votes[movieID] = map[string]bool{}
	}
	s.Votes[movieID][participantID] = yes
	s.LastSwipe[participantID] = Swipe{MovieID: movieID, Yes: yes}

	if !yes {
		return nil, false // a "no" can never create a match (secret-kill)
	}
	// Win = at least RequiredCount participants voted "yes" on this movie.
	// Count "yes" votes rather than requiring an exact total-vote count so the
	// threshold works when RequiredCount is below the roster size: extra votes
	// and unrelated "no" votes neither trigger nor block a match, and a "no"
	// only holds the card back while it keeps enough YES votes out of reach
	// (undoing that "no" can revive it).
	yesCount := 0
	for _, v := range s.Votes[movieID] {
		if v {
			yesCount++
		}
	}
	if yesCount < s.RequiredCount {
		return nil, false
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

// checkSessionEndedLocked checks if all connected participants have finished the deck.
// Returns a leaderboard if they have.
func (s *Session) checkSessionEndedLocked() []LeaderboardEntry {
	var connected []string
	for _, p := range s.Participants {
		if p.Connected {
			connected = append(connected, p.ID)
		}
	}
	if len(connected) == 0 {
		return nil
	}

	deckSize := len(s.Deck)
	for _, pid := range connected {
		votedCount := 0
		for _, votes := range s.Votes {
			if _, ok := votes[pid]; ok {
				votedCount++
			}
		}
		if votedCount < deckSize {
			return nil
		}
	}

	return s.buildLeaderboardLocked()
}

// buildLeaderboardLocked builds the leaderboard from the votes cast so far,
// regardless of whether every participant has finished. Caller must hold s.mu.
func (s *Session) buildLeaderboardLocked() []LeaderboardEntry {
	var lb []LeaderboardEntry
	for _, movie := range s.Deck {
		yesCount := 0
		for _, vote := range s.Votes[movie.ID] {
			if vote {
				yesCount++
			}
		}
		lb = append(lb, LeaderboardEntry{Movie: movie, YesCount: yesCount})
	}
	sort.Slice(lb, func(i, j int) bool {
		if lb[i].YesCount != lb[j].YesCount {
			return lb[i].YesCount > lb[j].YesCount
		}
		return lb[i].Movie.CommunityRating > lb[j].Movie.CommunityRating
	})
	return lb
}
