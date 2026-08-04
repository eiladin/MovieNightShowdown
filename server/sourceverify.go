package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// This file holds the settings screen's "check this source" routes.
//
// They exist because the alternative diagnostic is a movie night. A URL typo or a
// stale API key is otherwise discovered when four people are already holding
// their phones, and the failure surfaces as an empty deck with nothing naming the
// cause.
//
// Every route requires the setup token. Each one takes a URL from the request and
// makes the server fetch it, which is a request-forgery primitive pointed at the
// interior of whatever network this is deployed on: without the token they would
// let an unauthenticated caller port-scan a home LAN through the application and
// read the responses. The token is the whole boundary.
//
// The requests are built here rather than through JellyfinClient and PlexClient
// on purpose. Those clients exist to return movies, and their error values say so
// — they collapse every non-200 into one string. A check has to tell "nothing is
// listening there" from "something answered and rejected the credential", because
// those have different fixes and sending an operator to the wrong one is worse
// than saying nothing. That distinction needs the status code, so these calls keep
// it.

// statusError carries an upstream HTTP status so a check can classify a failure
// rather than merely report one.
type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string { return e.msg }

// rejectedCredential reports whether an error is the upstream refusing the
// credential, as opposed to the host being unreachable or answering with
// something unexpected.
func rejectedCredential(err error) bool {
	var se *statusError
	return errors.As(err, &se) &&
		(se.status == http.StatusUnauthorized || se.status == http.StatusForbidden)
}

// verifySourceResponse is the answer to any of the check routes.
//
// Message is written for an operator and is populated on success as well as
// failure: "connected, and here is the movie count" is the confirmation the
// button exists to give. It never contains the submitted credential.
type verifySourceResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// checkRequest carries candidate connection details.
//
// Every field is optional and falls back to what is stored, the same way the
// provider list does. The settings screen never receives a stored credential, so
// it has nothing to submit for one already saved — a check that demanded the
// secret could only ever be run against a credential being changed, which is the
// least interesting case.
type checkRequest struct {
	URL            string `json:"url"`
	Secret         string `json:"secret"`
	LibrarySection string `json:"librarySection"`
}

// decodeCheckRequest reads a check request, treating an absent body as "use
// everything that is stored".
func decodeCheckRequest(r *http.Request) checkRequest {
	var req checkRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	req.URL = strings.TrimSpace(req.URL)
	req.Secret = strings.TrimSpace(req.Secret)
	req.LibrarySection = strings.TrimSpace(req.LibrarySection)
	return req
}

// orStored picks the candidate value when one was supplied, otherwise the stored
// one.
func orStored(candidate, stored string) string {
	if candidate != "" {
		return candidate
	}
	return stored
}

// getJSON performs one authenticated GET against a source and decodes it.
//
// authHeader is empty for an unauthenticated probe, which is how the Jellyfin
// check separates "that is not a Jellyfin server" from "that key is wrong".
func getJSON(ctx context.Context, base, path string, q url.Values, authHeader, authValue string, out any) error {
	target := strings.TrimRight(base, "/") + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		// A URL the operator typed can fail to parse, and that is a report, not
		// a server fault.
		return &statusError{status: 0, msg: "that URL could not be parsed"}
	}
	req.Header.Set("Accept", "application/json")
	if authHeader != "" {
		req.Header.Set(authHeader, authValue)
	}

	// verifyTimeout is shared with the TMDB check: a settings screen waiting on a
	// hung request is worse than one told the check timed out.
	client := &http.Client{Timeout: verifyTimeout}
	resp, err := client.Do(req)
	if err != nil {
		// The error is not returned verbatim: it names the request URL, and the
		// credential travels in a header beside it. An operator does not need the
		// dial error to know nothing answered.
		return &statusError{status: 0, msg: "nothing answered at that address"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &statusError{
			status: resp.StatusCode,
			msg:    fmt.Sprintf("the server answered %s", resp.Status),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &statusError{status: resp.StatusCode, msg: "the response was not the JSON expected"}
	}
	return nil
}

// --- Jellyfin ---

type jellyfinPublicInfo struct {
	ServerName string `json:"ServerName"`
	Version    string `json:"Version"`
}

// handleVerifyJellyfin reports whether a Jellyfin URL and API key can read
// movies.
//
// It makes two calls, and the split is the point. /System/Info/Public needs no
// credential, so reaching it proves the URL points at a Jellyfin server; /Items
// then proves the API key works and says how many movies it can see. A single
// authenticated call would report a typo'd URL and a revoked key identically.
func (s *Server) handleVerifyJellyfin(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSetup(r) {
		unauthorized(w)
		return
	}
	req := decodeCheckRequest(r)
	cfg := s.config()
	base := orStored(req.URL, cfg.JellyfinURL)
	key := orStored(req.Secret, cfg.JellyfinAPIKey)

	if base == "" {
		writeJSON(w, http.StatusOK, verifySourceResponse{Message: "enter a server URL first"})
		return
	}
	if key == "" {
		writeJSON(w, http.StatusOK, verifySourceResponse{Message: "enter an API key first"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), verifyTimeout)
	defer cancel()

	var info jellyfinPublicInfo
	if err := getJSON(ctx, base, "/System/Info/Public", nil, "", "", &info); err != nil {
		writeJSON(w, http.StatusOK, verifySourceResponse{
			Message: "No Jellyfin server answered at that URL — " + err.Error() + ".",
		})
		return
	}

	q := url.Values{}
	q.Set("IncludeItemTypes", "Movie")
	q.Set("Recursive", "true")
	// One item is enough. Jellyfin reports TotalRecordCount uncapped regardless
	// of Limit, so the count costs nothing beyond this single row.
	q.Set("Limit", "1")
	var items jellyfinItemsResponse
	if err := getJSON(ctx, base, "/Items", q, "X-Emby-Token", key, &items); err != nil {
		if rejectedCredential(err) {
			writeJSON(w, http.StatusOK, verifySourceResponse{
				Message: fmt.Sprintf("Reached %s, but it rejected the API key.", serverLabel(info)),
			})
			return
		}
		writeJSON(w, http.StatusOK, verifySourceResponse{
			Message: fmt.Sprintf("Reached %s, but the movie query failed — %s.", serverLabel(info), err.Error()),
		})
		return
	}

	// Zero movies is not a wiring failure, so it is not reported as one — but it
	// is worth saying, because a correctly wired server with an empty library
	// deals an empty deck and the cause is not obvious from the swipe screen.
	if items.TotalRecordCount == 0 {
		writeJSON(w, http.StatusOK, verifySourceResponse{
			Valid:   true,
			Message: fmt.Sprintf("Connected to %s, but it reports no movies.", serverLabel(info)),
		})
		return
	}
	writeJSON(w, http.StatusOK, verifySourceResponse{
		Valid:   true,
		Message: fmt.Sprintf("Connected to %s — %d movies.", serverLabel(info), items.TotalRecordCount),
	})
}

// serverLabel names a Jellyfin server for a message, falling back to a generic
// noun when it did not report a name.
func serverLabel(info jellyfinPublicInfo) string {
	name := strings.TrimSpace(info.ServerName)
	if name == "" {
		return "the Jellyfin server"
	}
	if v := strings.TrimSpace(info.Version); v != "" {
		return fmt.Sprintf("%s (Jellyfin %s)", name, v)
	}
	return name
}

// jellyfinUser is one selectable Jellyfin account.
type jellyfinUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type jellyfinUsersResponse struct {
	Users []jellyfinUser `json:"users"`
}

// handleJellyfinUsers lists the Jellyfin accounts, so the user id behind the
// "unwatched only" filter can be chosen instead of transcribed.
//
// A Jellyfin user id is a 32-character hex string that lives in the admin
// dashboard's URL bar. Asking an operator to find and retype it is how the
// unwatched filter ends up quietly configured against a nonexistent account: the
// id is never validated at query time, it just returns nothing.
func (s *Server) handleJellyfinUsers(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSetup(r) {
		unauthorized(w)
		return
	}
	req := decodeCheckRequest(r)
	cfg := s.config()
	base := orStored(req.URL, cfg.JellyfinURL)
	key := orStored(req.Secret, cfg.JellyfinAPIKey)
	if base == "" || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"message": "a Jellyfin URL and API key are required to list users",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), verifyTimeout)
	defer cancel()

	var raw []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if err := getJSON(ctx, base, "/Users", nil, "X-Emby-Token", key, &raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"message": "could not read the user list from Jellyfin",
		})
		return
	}

	users := make([]jellyfinUser, 0, len(raw))
	for _, u := range raw {
		if u.ID == "" {
			continue
		}
		users = append(users, jellyfinUser{ID: u.ID, Name: u.Name})
	}
	// A stable order, because Jellyfin's is not: a select that reshuffles between
	// openings is one an operator cannot trust they read correctly.
	sort.Slice(users, func(i, j int) bool {
		if users[i].Name != users[j].Name {
			return users[i].Name < users[j].Name
		}
		return users[i].ID < users[j].ID
	})
	writeJSON(w, http.StatusOK, jellyfinUsersResponse{Users: users})
}

// --- Plex ---

// handleVerifyPlex reports whether a Plex URL and token can read a movie
// library.
//
// Plex has no unauthenticated endpoint worth probing, so this is one call:
// /library/sections both proves the token and says which movie libraries exist.
// Naming them is the useful part — a server with more than one movie section
// needs librarySection set, and discovery would otherwise deal from whichever
// Plex listed first.
func (s *Server) handleVerifyPlex(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSetup(r) {
		unauthorized(w)
		return
	}
	req := decodeCheckRequest(r)
	cfg := s.config()
	base := orStored(req.URL, cfg.PlexURL)
	token := orStored(req.Secret, cfg.PlexToken)
	section := orStored(req.LibrarySection, cfg.PlexLibrarySection)

	if base == "" {
		writeJSON(w, http.StatusOK, verifySourceResponse{Message: "enter a server URL first"})
		return
	}
	if token == "" {
		writeJSON(w, http.StatusOK, verifySourceResponse{Message: "enter a token first"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), verifyTimeout)
	defer cancel()

	var parsed plexResponse
	if err := getJSON(ctx, base, "/library/sections", nil, "X-Plex-Token", token, &parsed); err != nil {
		if rejectedCredential(err) {
			writeJSON(w, http.StatusOK, verifySourceResponse{
				Message: "Reached the server, but it rejected the token.",
			})
			return
		}
		writeJSON(w, http.StatusOK, verifySourceResponse{
			Message: "No Plex server answered at that URL — " + err.Error() + ".",
		})
		return
	}

	var movieSections []plexDirect
	for _, d := range parsed.MediaContainer.Directory {
		if d.Type == "movie" {
			movieSections = append(movieSections, d)
		}
	}
	if len(movieSections) == 0 {
		writeJSON(w, http.StatusOK, verifySourceResponse{
			Message: "Connected, but this server has no movie library.",
		})
		return
	}

	// A configured section that does not exist is a misconfiguration the app
	// would otherwise report as an empty library.
	if section != "" {
		for _, d := range movieSections {
			if d.Key == section {
				writeJSON(w, http.StatusOK, verifySourceResponse{
					Valid:   true,
					Message: fmt.Sprintf("Connected — using the %q library.", d.name()),
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, verifySourceResponse{
			Message: fmt.Sprintf("Connected, but there is no movie library with key %q. Available: %s.",
				section, describeSections(movieSections)),
		})
		return
	}

	if len(movieSections) > 1 {
		// Not a failure: discovery will pick the first and the app will work.
		// It is reported because "work" here means dealing from a library the
		// operator did not choose.
		writeJSON(w, http.StatusOK, verifySourceResponse{
			Valid: true,
			Message: fmt.Sprintf("Connected. This server has several movie libraries (%s) — "+
				"set a library section to choose, or %q will be used.",
				describeSections(movieSections), movieSections[0].name()),
		})
		return
	}
	writeJSON(w, http.StatusOK, verifySourceResponse{
		Valid:   true,
		Message: fmt.Sprintf("Connected — using the %q library.", movieSections[0].name()),
	})
}

// describeSections lists movie libraries as "Title (key)", so the value that has
// to go in the form is next to the name the operator recognizes.
func describeSections(sections []plexDirect) string {
	parts := make([]string, 0, len(sections))
	for _, d := range sections {
		parts = append(parts, fmt.Sprintf("%q (key %s)", d.name(), d.Key))
	}
	return strings.Join(parts, ", ")
}
