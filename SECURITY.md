# Security Policy

## Reporting a vulnerability

Please report security issues privately through GitHub's
[private vulnerability reporting](https://github.com/eiladin/movie-night-showdown/security/advisories/new)
rather than opening a public issue.

Include what you found, how to reproduce it, and what an attacker could do with
it. You will get an acknowledgement as soon as I see it. This is a hobby
project maintained by one person, so please be patient with timelines.

## Supported versions

Only the latest release is supported. Fixes ship in a new release rather than
being backported.

## Threat model — please read before reporting

This is a self-hosted app designed to run on a home network for the length of
one movie night. Several things that would be defects in an internet-facing
service are intentional here, and reports about them will be closed as working
as intended:

- **There are no accounts and no authentication.** Participants join a session
  by entering a short session code or scanning a QR code. Anyone who can reach
  the server and guess or obtain a code can join that session. This is the
  product, not an oversight.
- **Sessions are short-lived and in-memory.** They expire (`SESSION_TTL`,
  default a few hours) and do not survive a restart.
- **The app is not intended to be exposed to the public internet.** If you
  publish it, put it behind your own authentication and TLS.

## What is in scope

Reports in these areas are very much wanted:

- **Credential leakage.** `JELLYFIN_API_KEY` and `TMDB_READ_TOKEN` must never
  reach a browser. Every upstream request is made server-side, and posters are
  proxied so upstream URLs and credentials are never exposed to a client. Any
  path by which a client learns either value is a real vulnerability.
- **The image proxy.** `/api/images/{source}/{id}` is unauthenticated by
  design, but it must only ever fetch from a registered source's upstream. Path
  traversal, SSRF, requests routed to the wrong source's fetcher, or anything
  that makes it fetch an attacker-chosen URL is in scope.
- **The poster cache.** It writes files derived from upstream identifiers. Any
  way to escape the cache directory or to have one source overwrite another's
  entries is in scope.
- **Cross-session leakage.** One session learning another session's deck,
  roster, or code.
- **Dependency vulnerabilities** with a plausible path to exploitation here.

## What is out of scope

- Missing authentication, rate limiting, or account management, per the threat
  model above.
- Anything requiring an attacker who already has the server's environment
  variables or filesystem access.
- Denial of service by a participant who has already legitimately joined a
  session.
