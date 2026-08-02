# Security policy

Movie Night Showdown is a self-hosted app that sits between a browser and one
or more movie sources — a Jellyfin library, TMDB, or both. It holds credentials
for those sources and proxies every image request through itself, so the
interesting security surface is the boundary between what the server knows and
what a browser can reach. This page tells you how to report a problem privately,
and which problems we consider problems.

## Reporting a vulnerability

Report security issues through GitHub's
[private vulnerability reporting](https://github.com/eiladin/movie-night-showdown/security/advisories/new)
rather than opening a public issue. That keeps the details private until a fix
is available.

Include what you found, how to reproduce it, and what an attacker could do with
it. You'll get an acknowledgement as soon as I see it. This is a hobby project
maintained by one person, so please be patient with the timeline.

Only the latest release is supported. Fixes ship in a new release rather than
being backported.

## Understanding the threat model

Before you report something, it's worth knowing what this app is trying to be.
It's designed to run on a home network for the length of one movie night, and
several things that would be defects in an internet-facing service are
intentional here:

- **There are no accounts and no authentication.** Participants join a session
  by entering a short session code or scanning a QR code. Anyone who can reach
  the server and guess or obtain a code can join that session. This is the
  product, not an oversight.
- **Sessions are short-lived and held in memory.** They expire after
  `SESSION_TTL` — a few hours by default — and don't survive a restart.
- **The app isn't intended to be exposed to the public internet.** If you
  publish it, put it behind your own authentication and TLS.

Reports about missing authentication, missing rate limiting, or missing account
management will be closed as working as intended.

## What we want to hear about

Reports in these areas are very much wanted:

- **Credential leakage.** `JELLYFIN_API_KEY` and `TMDB_READ_TOKEN` must never
  reach a browser. Every upstream request is made server-side, and posters are
  proxied so that upstream URLs and credentials are never exposed to a client.
  Any path by which a client learns either value is a real vulnerability.
- **The image proxy.** `/api/images/{source}/{id}` is unauthenticated by design,
  but it must only ever fetch from a registered source's upstream. Path
  traversal, SSRF, a request routed to the wrong source's fetcher, or anything
  that makes it fetch an attacker-chosen URL is in scope.
- **The poster cache.** It writes files to disk under names derived from
  upstream identifiers. Any way to escape the cache directory, or to have one
  source overwrite another source's entries, is in scope.
- **Cross-session leakage.** One session learning another session's deck,
  roster, or code.
- **Dependency vulnerabilities**, where there's a plausible path to exploiting
  them through this app.

## What's out of scope

- Missing authentication, rate limiting, or account management, per the threat
  model above.
- Anything that requires an attacker who already holds the server's environment
  variables or has access to its filesystem.
- Denial of service by a participant who has already legitimately joined a
  session.

If you're not sure whether something falls inside the model, report it anyway
and say what you're unsure about. A report that turns out to be in scope is
worth far more than one that never gets sent.
