package server

import (
	"testing"
	"time"
)

func TestStore_CreateAndGet(t *testing.T) {
	store := NewStore(time.Hour)

	session := store.Create("Nate")
	if session.Code == "" {
		t.Fatal("expected a non-empty join code")
	}
	if len(session.Code) != 4 {
		t.Fatalf("expected a 4-char code, got %q", session.Code)
	}

	got, ok := store.Get(session.Code)
	if !ok {
		t.Fatalf("expected to find session by code %q", session.Code)
	}
	if got != session {
		t.Fatal("expected Get to return the same Session pointer created by Create")
	}

	admin, ok := got.Participants[got.AdminID]
	if !ok {
		t.Fatal("expected the admin to be registered as a participant")
	}
	if admin.Name != "Nate" || !admin.IsAdmin {
		t.Fatalf("expected admin participant named Nate with IsAdmin=true, got %+v", admin)
	}
	if admin.Token == "" {
		t.Fatal("expected the admin participant to have a resume token")
	}
	if got.Status != StatusLobby {
		t.Fatalf("expected a new session to start in StatusLobby, got %q", got.Status)
	}

	if _, ok := store.Get("ZZZZ"); ok {
		t.Fatal("expected an unknown code to not be found")
	}
}
