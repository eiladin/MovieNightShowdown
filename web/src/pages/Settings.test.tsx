import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Settings from './Settings'
import { clearSetupToken } from '../setupToken'
import { buildUpdate, draftFrom, SECRET_PLACEHOLDER, sourceAffecting } from '../settingsDraft'
import { jsonResponse } from '../test/fetchMock'
import type { Settings as SettingsData } from '../api'

const TOKEN = 'setup-token'

function settingsFixture(overrides: Partial<SettingsData> = {}): SettingsData {
    return {
        publicUrl: 'http://nas:8080',
        sessionTtl: '4h',
        runtime: { port: '8080', cacheDir: '/cache', configPath: '/config/config.yaml' },
        jellyfin: { enabled: false, url: '', apiKeySet: false, userId: '' },
        plex: {
            enabled: true,
            url: 'http://plex.local:32400',
            tokenSet: true,
            librarySection: '2',
        },
        streaming: {
            enabled: false,
            tmdbReadTokenSet: false,
            watchRegion: 'US',
            providers: [],
        },
        provenance: {},
        restartRequired: false,
        ...overrides,
    }
}

interface MockOptions {
    settings?: SettingsData
    saveResponse?: SettingsData
    saveStatus?: number
    saveBody?: unknown
    verifyValid?: boolean
    providers?: { id: string; name: string }[]
    getStatus?: number
}

let posted: { url: string; body: unknown }[] = []

function installFetch(opts: MockOptions = {}) {
    posted = []
    const settings = opts.settings ?? settingsFixture()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === 'string' ? input : input.toString()
        const body = init?.body ? JSON.parse(init.body as string) : undefined
        if (init?.method === 'POST') posted.push({ url, body })

        if (url === '/api/settings' && init?.method !== 'POST') {
            if (opts.getStatus === 401) {
                return jsonResponse({}, { ok: false, status: 401 })
            }
            return jsonResponse(settings)
        }
        if (url === '/api/settings' && init?.method === 'POST') {
            if (opts.saveStatus) {
                return jsonResponse(opts.saveBody ?? {}, { ok: false, status: opts.saveStatus })
            }
            return jsonResponse(opts.saveResponse ?? { ...settings, outcome: 'applied' })
        }
        if (url === '/api/settings/verify/tmdb') {
            return jsonResponse({ valid: opts.verifyValid ?? false, message: 'nope' })
        }
        if (url === '/api/settings/providers') {
            return jsonResponse({ region: 'US', providers: opts.providers ?? [] })
        }
        throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
}

function renderSettings() {
    return render(
        <MemoryRouter>
            <Settings />
        </MemoryRouter>,
    )
}

// enterToken walks past the token gate, which every test has to do because the
// token is deliberately not persisted.
async function enterToken(user: ReturnType<typeof userEvent.setup>) {
    await user.type(screen.getByLabelText('Setup token'), TOKEN)
    await user.click(screen.getByRole('button', { name: 'Continue' }))
}

describe('Settings', () => {
    beforeEach(() => {
        clearSetupToken()
    })

    afterEach(() => {
        cleanup()
        vi.unstubAllGlobals()
        vi.restoreAllMocks()
    })

    it('asks for the setup token before showing anything', async () => {
        installFetch()
        renderSettings()

        expect(screen.getByLabelText('Setup token')).toBeTruthy()
        // The token comes from the server log and nowhere else, so the screen
        // has to say so.
        expect(screen.getByText(/prints the token to its log/i)).toBeTruthy()
        expect(screen.queryByRole('heading', { name: 'Plex' })).toBeNull()
    })

    it('sends the token as a bearer credential and renders the sections in order', async () => {
        const fetchMock = installFetch()
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        await waitFor(() => expect(screen.getByRole('heading', { name: 'Plex' })).toBeTruthy())

        const init = fetchMock.mock.calls[0][1] as RequestInit
        expect((init.headers as Record<string, string>).Authorization).toBe(`Bearer ${TOKEN}`)

        const headings = screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)
        expect(headings).toEqual(['Jellyfin', 'Plex', 'Streaming', 'Server', 'Container'])
    })

    it('shows container-level settings without offering to change them', async () => {
        installFetch()
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByText('Listen port')).toBeTruthy())

        expect(screen.getByText('8080')).toBeTruthy()
        expect(screen.getByText('/cache')).toBeTruthy()
        expect(screen.getByText('/config/config.yaml')).toBeTruthy()
        // Reported, never editable: there is no input for any of them.
        expect(screen.queryByLabelText('Listen port')).toBeNull()
        expect(screen.queryByLabelText('Poster cache directory')).toBeNull()
    })

    it('re-prompts for the token when the server rejects it', async () => {
        installFetch({ getStatus: 401 })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        await waitFor(() => expect(screen.getByText(/was not accepted/i)).toBeTruthy())
        expect(screen.getByLabelText('Setup token')).toBeTruthy()
    })

    it('hides a section’s fields when its toggle is off', async () => {
        installFetch()
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeTruthy())

        // Jellyfin starts disabled, so only Plex's URL field exists.
        expect(screen.getAllByLabelText('Server URL')).toHaveLength(1)

        await user.click(screen.getByLabelText('Enable Jellyfin'))
        expect(screen.getAllByLabelText('Server URL')).toHaveLength(2)
    })

    it('renders a stored secret as a placeholder and omits it when untouched', async () => {
        installFetch()
        // The library section is source-affecting, so the save is gated on the
        // confirmation; accept it and let the request through.
        vi.spyOn(window, 'confirm').mockReturnValue(true)
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Token')).toBeTruthy())

        const tokenField = screen.getByLabelText('Token') as HTMLInputElement
        expect(tokenField.value).toBe(SECRET_PLACEHOLDER)

        await user.clear(screen.getByLabelText('Library section (optional)'))
        await user.type(screen.getByLabelText('Library section (optional)'), '3')
        await user.click(screen.getByRole('button', { name: 'Save settings' }))

        await waitFor(() => expect(posted).toHaveLength(1))
        const body = posted[0].body as { plex: Record<string, unknown> }
        expect(body.plex.token).toBeUndefined()
        expect(body.plex.clearToken).toBeUndefined()
        expect(body.plex.librarySection).toBe('3')
    })

    it('keeps password managers out of the credential fields', async () => {
        installFetch()
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Token')).toBeTruthy())

        // These are credentials, but none is an account password and there is no
        // username on the page. A manager filling one offers the wrong secret;
        // one saving it invents a login. autocomplete="off" alone does not stop
        // them, so each manager's opt-out has to be present.
        const field = screen.getByLabelText('Token')
        expect(field.getAttribute('autocomplete')).toBe('new-password')
        expect(field.hasAttribute('data-1p-ignore')).toBe(true)
        expect(field.getAttribute('data-lpignore')).toBe('true')
        expect(field.hasAttribute('data-bwignore')).toBe(true)
    })

    it('badges a field whose environment variable is being ignored', async () => {
        installFetch({
            settings: settingsFixture({
                provenance: {
                    'plex.url': { source: 'config file', envVar: 'PLEX_URL', envIgnored: true },
                },
            }),
        })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        await waitFor(() => expect(screen.getByText(/PLEX_URL is set but ignored/)).toBeTruthy())
    })

    it('keeps the provider picker hidden until a token verifies', async () => {
        installFetch({ verifyValid: false })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Enable streaming services')).toBeTruthy())

        await user.click(screen.getByLabelText('Enable streaming services'))
        expect(screen.queryByRole('combobox')).toBeNull()

        await user.type(screen.getByLabelText('TMDB read token'), 'bad')
        await user.click(screen.getByRole('button', { name: 'Check token' }))

        await waitFor(() => expect(screen.getByText('nope')).toBeTruthy())
        expect(screen.queryByRole('combobox')).toBeNull()
    })

    it('shows the picker once the token verifies', async () => {
        installFetch({ verifyValid: true, providers: [{ id: 'netflix', name: 'Netflix' }] })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Enable streaming services')).toBeTruthy())

        await user.click(screen.getByLabelText('Enable streaming services'))
        await user.type(screen.getByLabelText('TMDB read token'), 'good')
        await user.click(screen.getByRole('button', { name: 'Check token' }))

        // The picker shows a search box rather than the whole catalogue.
        await waitFor(() => expect(screen.getByRole('combobox')).toBeTruthy())
        await user.type(screen.getByRole('combobox'), 'net')
        expect(screen.getByRole('option', { name: 'Netflix' })).toBeTruthy()
    })

    it('reports the three save outcomes distinctly', async () => {
        const cases: [string, RegExp][] = [
            ['no_change', /nothing changed/i],
            ['applied', /is live/i],
            ['restart_required', /restart is needed/i],
        ]
        for (const [outcome, pattern] of cases) {
            const settings = settingsFixture()
            installFetch({ settings, saveResponse: { ...settings, outcome } })
            const user = userEvent.setup()
            renderSettings()
            await enterToken(user)
            await waitFor(() => expect(screen.getByRole('button', { name: 'Save settings' })).toBeTruthy())

            await user.click(screen.getByRole('button', { name: 'Save settings' }))
            await waitFor(() => expect(screen.getByText(pattern)).toBeTruthy())
            cleanup()
            clearSetupToken()
        }
    })

    it('attaches a rejected field’s message to that field', async () => {
        installFetch({
            saveStatus: 400,
            saveBody: { errors: { 'plex.url': 'must use http or https' } },
        })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByRole('button', { name: 'Save settings' })).toBeTruthy())

        await user.click(screen.getByRole('button', { name: 'Save settings' }))
        await waitFor(() => expect(screen.getByText('must use http or https')).toBeTruthy())
        expect(screen.getByText(/Nothing was saved/i)).toBeTruthy()
    })

    it('warns before a source-affecting save and abandons it when declined', async () => {
        installFetch()
        const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeTruthy())

        await user.clear(screen.getByLabelText('Server URL'))
        await user.type(screen.getByLabelText('Server URL'), 'http://moved:32400')
        await user.click(screen.getByRole('button', { name: 'Save settings' }))

        expect(confirmSpy).toHaveBeenCalledOnce()
        expect(posted).toHaveLength(0)
    })

    it('does not warn for a change that touches no source', async () => {
        installFetch()
        const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Public URL')).toBeTruthy())

        await user.clear(screen.getByLabelText('Public URL'))
        await user.type(screen.getByLabelText('Public URL'), 'http://corrected:8080')
        await user.click(screen.getByRole('button', { name: 'Save settings' }))

        await waitFor(() => expect(posted).toHaveLength(1))
        expect(confirmSpy).not.toHaveBeenCalled()
    })

    it('links to the environment-variable guide for a first-time operator', async () => {
        installFetch()
        renderSettings()

        const link = screen.getByRole('link', { name: /environment variables instead/i })
        expect(link.getAttribute('href')).toBe('/setup')
    })

    it('offers an explicit control for removing a stored credential', async () => {
        installFetch()
        vi.spyOn(window, 'confirm').mockReturnValue(true)
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Token')).toBeTruthy())

        // A stored credential can be removed without the operator having to
        // guess that emptying the field is what does it.
        await user.click(screen.getByRole('button', { name: 'Remove stored value' }))
        expect(screen.getByText(/removed when you save/i)).toBeTruthy()

        await user.click(screen.getByRole('button', { name: 'Save settings' }))
        await waitFor(() => expect(posted).toHaveLength(1))
        const body = posted[0].body as { plex: Record<string, unknown> }
        expect(body.plex.clearToken).toBe(true)
        expect(body.plex.token).toBeUndefined()
    })

    it('offers no removal control for a credential that is not set', async () => {
        installFetch()
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Enable Jellyfin')).toBeTruthy())

        await user.click(screen.getByLabelText('Enable Jellyfin'))
        // Jellyfin has no stored API key in the fixture, so only Plex's
        // credential offers removal.
        expect(screen.getAllByRole('button', { name: 'Remove stored value' })).toHaveLength(1)
    })

})

describe('buildUpdate', () => {
    it('clears a secret only when it was set and has been emptied', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        draft.plex.token = ''

        const update = buildUpdate(draft, from)
        expect(update.plex?.clearToken).toBe(true)
        expect(update.plex?.token).toBeUndefined()
    })

    it('sends a replacement secret as a value, not a clear', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        draft.plex.token = 'new-token'

        const update = buildUpdate(draft, from)
        expect(update.plex?.token).toBe('new-token')
        expect(update.plex?.clearToken).toBeUndefined()
    })

    it('does not clear a secret that was never set', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        // jellyfin.apiKey starts unset, so an empty field is not a clear.
        expect(draft.jellyfin.apiKey).toBe('')

        const update = buildUpdate(draft, from)
        expect(update.jellyfin?.clearApiKey).toBeUndefined()
        expect(update.jellyfin?.apiKey).toBeUndefined()
    })
})

describe('sourceAffecting', () => {
    it('is false for an untouched draft', () => {
        const from = settingsFixture()
        expect(sourceAffecting(draftFrom(from), from)).toBe(false)
    })

    it('is false for a change to a harmless setting', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        draft.publicUrl = 'http://corrected:8080'
        expect(sourceAffecting(draft, from)).toBe(false)
    })

    it('is true when a stored secret is replaced', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        draft.plex.token = 'rotated'
        expect(sourceAffecting(draft, from)).toBe(true)
    })

    it('is true when a provider selection changes', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        draft.streaming.providers = ['netflix']
        expect(sourceAffecting(draft, from)).toBe(true)
    })
})
