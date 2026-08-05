import type { LibraryOption, Settings as SettingsData, SettingsUpdate } from './api'

// SECRET_PLACEHOLDER is what a stored credential looks like on screen. The
// server never sends one back, so this is not a masked value — it is a marker
// meaning "something is stored". Leaving it untouched omits the field from the
// save, which is what preserves the stored credential.
export const SECRET_PLACEHOLDER = '••••••••'

// Draft is the editable form state. Secret fields hold either the placeholder
// (unchanged), an empty string (cleared), or a new value.
export interface Draft {
    publicUrl: string
    sessionTtl: string
    jellyfin: {
        enabled: boolean
        url: string
        apiKey: string
        userId: string
        libraries: LibraryOption[]
    }
    plex: {
        enabled: boolean
        url: string
        token: string
        librarySection: string
        libraries: LibraryOption[]
    }
    streaming: {
        enabled: boolean
        tmdbReadToken: string
        watchRegion: string
        providers: string[]
    }
}

export function draftFrom(s: SettingsData): Draft {
    return {
        publicUrl: s.publicUrl,
        sessionTtl: s.sessionTtl,
        jellyfin: {
            enabled: s.jellyfin.enabled,
            url: s.jellyfin.url,
            apiKey: s.jellyfin.apiKeySet ? SECRET_PLACEHOLDER : '',
            userId: s.jellyfin.userId,
            libraries: s.jellyfin.libraries ?? [],
        },
        plex: {
            enabled: s.plex.enabled,
            url: s.plex.url,
            token: s.plex.tokenSet ? SECRET_PLACEHOLDER : '',
            librarySection: s.plex.librarySection,
            libraries: s.plex.libraries ?? [],
        },
        streaming: {
            enabled: s.streaming.enabled,
            tmdbReadToken: s.streaming.tmdbReadTokenSet ? SECRET_PLACEHOLDER : '',
            watchRegion: s.streaming.watchRegion,
            providers: s.streaming.providers,
        },
    }
}

// secretUpdate turns a secret field's draft value into the request's shape.
// The three cases are distinct and all matter: untouched omits the field,
// emptied clears it explicitly, and anything else is a new value.
function secretUpdate(
    draft: string,
    wasSet: boolean,
): { value?: string; clear?: boolean } {
    if (draft === SECRET_PLACEHOLDER) return {}
    if (draft === '') return wasSet ? { clear: true } : {}
    return { value: draft }
}

// buildUpdate produces the save request from the draft and the settings it was
// seeded from.
export function buildUpdate(draft: Draft, from: SettingsData): SettingsUpdate {
    const jfKey = secretUpdate(draft.jellyfin.apiKey, from.jellyfin.apiKeySet)
    const plexToken = secretUpdate(draft.plex.token, from.plex.tokenSet)
    const tmdb = secretUpdate(draft.streaming.tmdbReadToken, from.streaming.tmdbReadTokenSet)

    return {
        publicUrl: draft.publicUrl,
        sessionTtl: draft.sessionTtl,
        jellyfin: {
            enabled: draft.jellyfin.enabled,
            url: draft.jellyfin.url,
            apiKey: jfKey.value,
            clearApiKey: jfKey.clear,
            userId: draft.jellyfin.userId,
            libraries: draft.jellyfin.libraries,
        },
        plex: {
            enabled: draft.plex.enabled,
            url: draft.plex.url,
            token: plexToken.value,
            clearToken: plexToken.clear,
            librarySection: draft.plex.librarySection,
            libraries: draft.plex.libraries,
        },
        streaming: {
            enabled: draft.streaming.enabled,
            tmdbReadToken: tmdb.value,
            clearTmdbReadToken: tmdb.clear,
            watchRegion: draft.streaming.watchRegion,
            providers: draft.streaming.providers,
        },
    }
}

// libraryKey renders a library list for comparison. Order is included because it
// decides the order sources are offered in, and the name because the picker shows
// it — the server treats both as source-affecting.
function libraryKey(libraries: LibraryOption[]): string {
    return libraries.map((l) => `${l.id}:${l.name}`).join(',')
}

// sourceAffecting reports whether a draft changes something that would rebuild
// the source set — which ends every active session. It mirrors the server's
// tiering; the server decides, this only decides whether to warn first.
export function sourceAffecting(draft: Draft, from: SettingsData): boolean {
    return (
        draft.jellyfin.enabled !== from.jellyfin.enabled ||
        draft.jellyfin.url !== from.jellyfin.url ||
        draft.jellyfin.userId !== from.jellyfin.userId ||
        draft.jellyfin.apiKey !== (from.jellyfin.apiKeySet ? SECRET_PLACEHOLDER : '') ||
        draft.plex.enabled !== from.plex.enabled ||
        draft.plex.url !== from.plex.url ||
        draft.plex.librarySection !== from.plex.librarySection ||
        draft.plex.token !== (from.plex.tokenSet ? SECRET_PLACEHOLDER : '') ||
        draft.streaming.enabled !== from.streaming.enabled ||
        draft.streaming.watchRegion !== from.streaming.watchRegion ||
        draft.streaming.tmdbReadToken !==
            (from.streaming.tmdbReadTokenSet ? SECRET_PLACEHOLDER : '') ||
        draft.streaming.providers.join(',') !== from.streaming.providers.join(',') ||
        // A library change rebuilds the source set and ends every session, so the
        // confirmation has to fire for it. This mirrors sourcesDiffer on the server,
        // which remains authoritative; only the server has a test that would notice
        // the two drifting apart.
        libraryKey(draft.jellyfin.libraries) !== libraryKey(from.jellyfin.libraries) ||
        libraryKey(draft.plex.libraries) !== libraryKey(from.plex.libraries)
    )
}
