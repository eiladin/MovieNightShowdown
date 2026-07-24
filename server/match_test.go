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
	// Roster of 5, host requires only 3 to agree.
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

// TestBuildLeaderboard_RanksByYesThenRating exercises buildLeaderboardLocked
// directly: it must return a leaderboard covering the whole deck even when
// nobody has finished swiping, ordered by YesCount desc with CommunityRating
// as the tiebreak.
func TestBuildLeaderboard_RanksByYesThenRating(t *testing.T) {
	s := newTestSession(3,
		Movie{ID: "m1", Title: "Arrival", CommunityRating: 7.5},
		Movie{ID: "m2", Title: "Dune", CommunityRating: 8.5},
		Movie{ID: "m3", Title: "Contact", CommunityRating: 6.0},
	)

	// m2 and m3 tie on yesCount (1 each); m2 has the higher rating and must
	// rank first between them. m1 has no yes votes and one no vote; the deck
	// is far from complete (only p1 has voted).
	s.recordSwipe("p1", "m1", false)
	s.recordSwipe("p1", "m2", true)
	s.recordSwipe("p1", "m3", true)

	lb := s.buildLeaderboardLocked()
	if lb == nil {
		t.Fatal("expected a non-nil leaderboard even though the deck isn't complete")
	}
	if len(lb) != 3 {
		t.Fatalf("expected all 3 deck movies in the leaderboard, got %d", len(lb))
	}
	if lb[0].Movie.ID != "m2" || lb[0].YesCount != 1 {
		t.Fatalf("expected m2 first (yesCount=1, rating=8.5), got %+v", lb[0])
	}
	if lb[1].Movie.ID != "m3" || lb[1].YesCount != 1 {
		t.Fatalf("expected m3 second (yesCount=1, rating=6.0, tiebreak loser), got %+v", lb[1])
	}
	if lb[2].Movie.ID != "m1" || lb[2].YesCount != 0 {
		t.Fatalf("expected m1 last (yesCount=0), got %+v", lb[2])
	}
}

func TestRecordSwipe_UnknownMovieIsIgnored(t *testing.T) {
	s := newTestSession(1, Movie{ID: "real", Title: "Arrival"})

	winner, matched := s.recordSwipe("p1", "ghost", true)
	if matched {
		t.Fatalf("expected no match for a movie not in the deck, got matched=true")
	}
	if winner != nil {
		t.Fatalf("expected nil winner and no match, got winner=%+v", winner)
	}
	if _, ok := s.Votes["ghost"]; ok {
		t.Fatal("expected a vote for an unknown movie to be ignored, not recorded")
	}
	if _, ok := s.LastSwipe["p1"]; ok {
		t.Fatal("expected no LastSwipe recorded for an ignored vote")
	}
}
