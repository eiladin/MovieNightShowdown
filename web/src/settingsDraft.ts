import type { Settings as SettingsData, SettingsUpdate } from './api'

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
    cacheDir: string
    jellyfin: { enabled: boolean; url: string; apiKey: string; userId: string }
    plex: { enabled: boolean; url: string; token: string; librarySection: string }
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
        cacheDir: s.cacheDir,
        jellyfin: {
            enabled: s.jellyfin.enabled,
            url: s.jellyfin.url,
            apiKey: s.jellyfin.apiKeySet ? SECRET_PLACEHOLDER : '',
            userId: s.jellyfin.userId,
        },
        plex: {
            enabled: s.plex.enabled,
            url: s.plex.url,
            token: s.plex.tokenSet ? SECRET_PLACEHOLDER : '',
            librarySection: s.plex.librarySection,
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
        cacheDir: draft.cacheDir,
        jellyfin: {
            enabled: draft.jellyfin.enabled,
            url: draft.jellyfin.url,
            apiKey: jfKey.value,
            clearApiKey: jfKey.clear,
            userId: draft.jellyfin.userId,
        },
        plex: {
            enabled: draft.plex.enabled,
            url: draft.plex.url,
            token: plexToken.value,
            clearToken: plexToken.clear,
            librarySection: draft.plex.librarySection,
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
        draft.streaming.providers.join(',') !== from.streaming.providers.join(',')
    )
}
