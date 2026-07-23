package server

import "testing"

// newTestSession builds a minimal, directly-constructed Session for exercising
// the vote engine without going through the Store/WS layers.
func newTestSession(requiredCount int, deck ...Movie) *Session {
	return &Session{
		RequiredCount: requiredCount,
		Status:        StatusActive,
		Deck:          deck,
		Votes:         map[string]map[string]bool{},
		LastSwipe:     map[string]Swipe{},
	}
}

func TestRecordSwipe_MatchesWhenAllRequiredVoteYes(t *testing.T) {
	s := newTestSession(2, Movie{ID: "m1", Title: "Arrival"})

	winner, matched := s.recordSwipe("p1", "m1", true)
	if matched {
		t.Fatalf("expected no match after only one of two required yes votes, got matched=true")
	}
	if winner != nil {
		t.Fatalf("expected nil winner before the match, got %+v", winner)
	}

	winner, matched = s.recordSwipe("p2", "m1", true)
	if !matched {
		t.Fatal("expected a match once both required participants voted yes")
	}
	if winner == nil || winner.ID != "m1" {
		t.Fatalf("expected winner m1, got %+v", winner)
	}
}

func TestRecordSwipe_NoNeverMatchesEvenWithOtherYeses(t *testing.T) {
	s := newTestSession(3, Movie{ID: "m1", Title: "Arrival"})

	if _, matched := s.recordSwipe("p1", "m1", true); matched {
		t.Fatal("expected no match after first yes")
	}
	if _, matched := s.recordSwipe("p2", "m1", false); matched {
		t.Fatal("expected no match after a no vote")
	}
	// A third participant voting yes still can't create a match: p2's "no"
	// is recorded and len(votes) never reaches 3-all-yes.
	winner, matched := s.recordSwipe("p3", "m1", true)
	if matched {
		t.Fatalf("expected a 'no' vote to make the movie permanently unwinnable, got matched=true winner=%+v", winner)
	}

	votes := s.Votes["m1"]
	if len(votes) != 3 {
		t.Fatalf("expected all 3 votes recorded (secret-kill keeps the card, doesn't hide the vote), got %d", len(votes))
	}
	if votes["p2"] {
		t.Fatal("expected p2's vote to remain recorded as no")
	}
}

func TestUndo_RemovesTheVote(t *testing.T) {
	s := newTestSession(2, Movie{ID: "m1", Title: "Arrival"})

	s.recordSwipe("p1", "m1", true)
	if _, ok := s.Votes["m1"]["p1"]; !ok {
		t.Fatal("expected p1's vote to be recorded")
	}
	if _, ok := s.LastSwipe["p1"]; !ok {
		t.Fatal("expected p1's LastSwipe to be recorded")
	}

	// Undo, mirroring hub.go's handleUndo: delete the Votes entry, clear
	// LastSwipe.
	last := s.LastSwipe["p1"]
	delete(s.Votes[last.MovieID], "p1")
	delete(s.LastSwipe, "p1")

	if _, ok := s.Votes["m1"]["p1"]; ok {
		t.Fatal("expected p1's vote to be gone after undo")
	}
	if _, ok := s.LastSwipe["p1"]; ok {
		t.Fatal("expected p1's LastSwipe to be cleared after undo")
	}
}

func TestUndo_OfANoReenablesAMatch(t *testing.T) {
	s := newTestSession(2, Movie{ID: "m1", Title: "Arrival"})

	// p1 votes no (secret-kill), p2 votes yes: no match, and the movie is
	// currently unwinnable.
	if _, matched := s.recordSwipe("p1", "m1", false); matched {
		t.Fatal("expected no match after a no vote")
	}
	if _, matched := s.recordSwipe("p2", "m1", true); matched {
		t.Fatal("expected the movie to still be unwinnable after p1's no")
	}

	// Undo p1's no.
	last := s.LastSwipe["p1"]
	delete(s.Votes[last.MovieID], "p1")
	delete(s.LastSwipe, "p1")

	// p1 now votes yes: the movie should be winnable again.
	winner, matched := s.recordSwipe("p1", "m1", true)
	if !matched {
		t.Fatal("expected undoing a no and re-voting yes to re-enable the match")
	}
	if winner == nil || winner.ID != "m1" {
		t.Fatalf("expected winner m1, got %+v", winner)
	}
}

// TestRecordSwipe_SubRosterThresholdFires exercises the case the win condition
// used to get wrong: RequiredCount is set below the roster size, everyone
// swipes every card, and there is an unrelated "no" among the votes. A match
// must fire as soon as RequiredCount participants have voted YES, and neither
// an extra YES nor an unrelated NO may block it.
func TestRecordSwipe_SubRosterThresholdFires(t *testing.T) {
	// Roster of 5, admin requires only 3 to agree.
	s := newTestSession(3, Movie{ID: "m1", Title: "Arrival"})

	// One "no" arrives first: under the old exact-count logic this alone
	// permanently poisoned the card. It must not here.
	if _, matched := s.recordSwipe("p1", "m1", false); matched {
		t.Fatal("expected no match after a lone no vote")
	}
	// First two yes votes: threshold of 3 not yet met.
	if _, matched := s.recordSwipe("p2", "m1", true); matched {
		t.Fatal("expected no match after 1 yes vote")
	}
	if _, matched := s.recordSwipe("p3", "m1", true); matched {
		t.Fatal("expected no match after 2 yes votes (threshold is 3)")
	}
	// Third yes vote reaches the threshold even though total votes (4, incl.
	// the "no") already exceed RequiredCount.
	winner, matched := s.recordSwipe("p4", "m1", true)
	if !matched {
		t.Fatal("expected a match once 3 participants voted yes, despite an unrelated no and total votes > RequiredCount")
	}
	if winner == nil || winner.ID != "m1" {
		t.Fatalf("expected winner m1, got %+v", winner)
	}
}

// TestRecordSwipe_ExtraVotesPastThresholdStillMatch guards the other half of
// the old bug: once the YES threshold is reached, further votes (from a fifth
// participant swiping the whole deck) must not make the card stop matching.
func TestRecordSwipe_ExtraVotesPastThresholdStillMatch(t *testing.T) {
	s := newTestSession(3, Movie{ID: "m1", Title: "Dune"})

	s.recordSwipe("p1", "m1", true)
	s.recordSwipe("p2", "m1", true)
	if _, matched := s.recordSwipe("p3", "m1", true); !matched {
		t.Fatal("expected a match at exactly 3 yes votes")
	}
	// A late-arriving yes from a 4th person keeps the threshold satisfied.
	winner, matched := s.recordSwipe("p4", "m1", true)
	if !matched {
		t.Fatalf("expected the card to stay matched with a 4th yes vote, got matched=false")
	}
	if winner == nil || winner.ID != "m1" {
		t.Fatalf("expected winner m1, got %+v", winner)
	}
}
