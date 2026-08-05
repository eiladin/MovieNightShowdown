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
        jellyfin: { enabled: false, url: '', apiKeySet: false, userId: '', libraries: [] },
        plex: {
            enabled: true,
            url: 'http://plex.local:32400',
            tokenSet: true,
            librarySection: '2',
            libraries: [],
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
    jellyfinCheck?: { valid: boolean; message?: string; libraries?: { id: string; name: string }[] }
    plexCheck?: { valid: boolean; message?: string; libraries?: { id: string; name: string }[] }
    providerListStatus?: number
    // requireCandidateToken makes the provider endpoint answer 400 unless the
    // request carries a token, which is how the real server behaves when nothing
    // is stored to fall back to.
    requireCandidateToken?: boolean
    // users undefined means the list endpoint fails, which is the state of a
    // Jellyfin the server could not read accounts from.
    users?: { id: string; name: string }[]
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
            if (opts.requireCandidateToken && !(body as { token?: string })?.token) {
                return jsonResponse(
                    { message: 'a TMDB read token is required to list providers' },
                    { ok: false, status: 400 },
                )
            }
            if (opts.providerListStatus) {
                return jsonResponse(
                    { message: 'could not reach TMDB to list providers' },
                    { ok: false, status: opts.providerListStatus },
                )
            }
            return jsonResponse({ region: 'US', providers: opts.providers ?? [] })
        }
        if (url === '/api/settings/verify/jellyfin') {
            return jsonResponse(opts.jellyfinCheck ?? { valid: false, message: 'no' })
        }
        if (url === '/api/settings/verify/plex') {
            return jsonResponse(opts.plexCheck ?? { valid: false, message: 'no' })
        }
        if (url === '/api/settings/jellyfin/users') {
            if (!opts.users) {
                return jsonResponse({}, { ok: false, status: 502 })
            }
            return jsonResponse({ users: opts.users })
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

        // Any source-affecting edit will do; the point of the test is the secret.
        await user.type(screen.getByLabelText('Server URL'), '/x')
        await user.click(screen.getByRole('button', { name: 'Save settings' }))

        await waitFor(() => expect(posted).toHaveLength(1))
        const body = posted[0].body as { plex: Record<string, unknown> }
        expect(body.plex.token).toBeUndefined()
        expect(body.plex.clearToken).toBeUndefined()
        expect(body.plex.url).toBe('http://plex.local:32400/x')
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
        expect(field.hasAttribute('data-1p-ignore')).toBe(true)
        expect(field.getAttribute('data-lpignore')).toBe('true')
        expect(field.hasAttribute('data-bwignore')).toBe(true)

        // The autocomplete value must not be a token anything recognizes. It was
        // "new-password", which stops a saved password being filled but also means
        // "a new password goes here" — the signal that offers a password
        // generator, which is what a manager was observed doing on the API key
        // field. Every standard token describes a credential this page does not
        // have, so the correct value is one that matches nothing.
        const autocomplete = field.getAttribute('autocomplete') ?? ''
        expect(autocomplete).not.toBe('')
        expect([
            'new-password',
            'current-password',
            'password',
            'username',
            'email',
            'one-time-code',
            'on',
        ]).not.toContain(autocomplete)
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

    // Without this, the diagnostic for a URL typo is a movie night: the failure
    // otherwise surfaces as an empty deck with everyone already swiping.
    it('reports what a Jellyfin check found', async () => {
        // Plex is switched off so "Server URL" and "Check connection" name exactly
        // one control each; both sections render the same labels.
        const wired = settingsFixture({
            jellyfin: { enabled: true, url: 'http://nas:8096', apiKeySet: true, userId: '', libraries: [] },
            plex: { enabled: false, url: '', tokenSet: false, librarySection: '', libraries: [] },
        })
        installFetch({
            settings: wired,
            users: [],
            jellyfinCheck: { valid: true, message: 'Connected to Anton — 1284 movies.' },
        })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeTruthy())

        await user.click(screen.getByRole('button', { name: 'Check connection' }))

        await waitFor(() =>
            expect(screen.getByText('Connected to Anton — 1284 movies.')).toBeTruthy(),
        )
    })

    // A green "Connected" sitting beside a URL that has since been retyped is a
    // claim about a configuration that no longer exists.
    it('discards a check result when the field it checked is edited', async () => {
        const wired = settingsFixture({
            jellyfin: { enabled: true, url: 'http://nas:8096', apiKeySet: true, userId: '', libraries: [] },
            plex: { enabled: false, url: '', tokenSet: false, librarySection: '', libraries: [] },
        })
        installFetch({ settings: wired, users: [], jellyfinCheck: { valid: true, message: 'Connected.' } })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeTruthy())

        await user.click(screen.getByRole('button', { name: 'Check connection' }))
        await waitFor(() => expect(screen.getByText('Connected.')).toBeTruthy())

        await user.type(screen.getByLabelText('Server URL'), 'x')

        expect(screen.queryByText('Connected.')).toBeNull()
    })

    // A Jellyfin user id is a 32-character hex string out of the admin
    // dashboard's URL bar. Transcribing it is how the unwatched filter ends up
    // pointed at nothing: a wrong id is never rejected, it just returns no films.
    it('offers the Jellyfin accounts as a list', async () => {
        const wired = settingsFixture({
            jellyfin: { enabled: true, url: 'http://nas:8096', apiKeySet: true, userId: 'bbb', libraries: [] },
        })
        installFetch({
            settings: wired,
            users: [
                { id: 'aaa', name: 'Alex' },
                { id: 'bbb', name: 'Sami' },
            ],
        })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        // The field is a text input until the accounts arrive, so wait for the
        // select rather than for mere presence.
        await waitFor(() =>
            expect(screen.getByLabelText('Account for “unwatched only”').tagName).toBe('SELECT'),
        )
        const field = screen.getByLabelText('Account for “unwatched only”')
        expect((field as HTMLSelectElement).value).toBe('bbb')
        expect(screen.getByRole('option', { name: 'Alex' })).toBeTruthy()

        // The field has to say what it is for. "User ID (optional)" gave no clue
        // that it is the only thing enabling the unwatched filter.
        expect(screen.getByText(/unwatched films needs an account/)).toBeTruthy()
    })

    // A saved id this server does not list must survive being looked at. Dropping
    // it from the select would silently blank it on the next save.
    it('keeps a stored account id the server does not list', async () => {
        const wired = settingsFixture({
            jellyfin: { enabled: true, url: 'http://nas:8096', apiKeySet: true, userId: 'ghost', libraries: [] },
        })
        installFetch({ settings: wired, users: [{ id: 'aaa', name: 'Alex' }] })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        const field = await waitFor(() =>
            screen.getByLabelText('Account for “unwatched only”'),
        )
        expect((field as HTMLSelectElement).value).toBe('ghost')
        expect(screen.getByText(/ghost — not an account on this server/)).toBeTruthy()
    })

    // With no account list and nothing stored there is nothing to offer. An empty
    // text box would be a request to go and transcribe a 32-character id out of
    // Jellyfin's admin URL bar, so the field is not shown at all and the hint
    // points at the control that can populate it.
    it('does not offer the account field until the accounts are known', async () => {
        const wired = settingsFixture({
            jellyfin: { enabled: true, url: 'http://nas:8096', apiKeySet: true, userId: '', libraries: [] },
            plex: { enabled: false, url: '', tokenSet: false, librarySection: '', libraries: [] },
        })
        installFetch({ settings: wired })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeTruthy())

        expect(screen.queryByLabelText('Account for “unwatched only”')).toBeNull()
        expect(screen.getByText(/Check the connection to choose which account/)).toBeTruthy()
    })

    // A stored id is a different case: it has to stay visible even with no list,
    // or it could never be seen or cleared.
    it('still shows a stored account id when the accounts cannot be read', async () => {
        const wired = settingsFixture({
            jellyfin: { enabled: true, url: 'http://nas:8096', apiKeySet: true, userId: 'ghost', libraries: [] },
        })
        installFetch({ settings: wired })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        const field = await waitFor(() => screen.getByLabelText('Account for “unwatched only”'))
        expect(field.tagName).toBe('INPUT')
        expect((field as HTMLInputElement).value).toBe('ghost')
    })

    // A library identifier is opaque — a numeric key for Plex, a hexadecimal id for
    // Jellyfin — so it is offered as a list or not at all. A media server has a
    // handful of libraries, so they are all enumerated rather than hidden behind a
    // search box.
    it('enumerates the Plex libraries once a check has run', async () => {
        const noSection = settingsFixture({
            plex: {
                enabled: true,
                url: 'http://plex:32400',
                tokenSet: true,
                librarySection: '',
                libraries: [],
            },
        })
        installFetch({
            settings: noSection,
            plexCheck: {
                valid: true,
                message: 'Connected.',
                libraries: [
                    { id: '1', name: 'Films' },
                    { id: '3', name: 'Kids Films' },
                ],
            },
        })
        // Choosing a library rebuilds the source set and ends every session, so the
        // save is gated on the confirmation. Accept it and let the request through.
        vi.spyOn(window, 'confirm').mockReturnValue(true)
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Token')).toBeTruthy())

        expect(screen.queryByRole('checkbox', { name: 'Films' })).toBeNull()
        expect(screen.getByText(/Check the connection to choose which libraries/)).toBeTruthy()

        await user.click(screen.getByRole('button', { name: 'Check connection' }))

        // Every library is on screen as a toggle, with nothing typed and no search
        // field anywhere near it.
        await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Films' })).toBeTruthy())
        expect(screen.getByRole('checkbox', { name: 'Kids Films' })).toBeTruthy()

        await user.click(screen.getByRole('checkbox', { name: 'Kids Films' }))

        // Checked and unchecked, not a chip and a pill: one control in two states.
        expect(
            (screen.getByRole('checkbox', { name: 'Kids Films' }) as HTMLInputElement).checked,
        ).toBe(true)
        expect(
            (screen.getByRole('checkbox', { name: 'Films' }) as HTMLInputElement).checked,
        ).toBe(false)

        // Chosen libraries are sent as id and name pairs, so a later start has
        // nothing to resolve.
        await user.click(screen.getByRole('button', { name: 'Save settings' }))
        await waitFor(() => expect(posted.some((p) => p.url === '/api/settings')).toBe(true))
        const save = posted.find((p) => p.url === '/api/settings')
        const body = save?.body as { plex: { libraries: unknown } } | undefined
        expect(body?.plex.libraries).toEqual([{ id: '3', name: 'Kids Films' }])
    })

    // The state after a restart: libraries are saved, no check has run yet. They have
    // to render under their own names and unmarked. Before the merge they rendered as
    // bare identifiers labelled "not on this server", which was a lie — nothing had
    // been fetched to judge them against.
    it('shows saved libraries by name on load, before any check', async () => {
        const stored = settingsFixture({
            plex: {
                enabled: true,
                url: 'http://plex:32400',
                tokenSet: true,
                librarySection: '',
                libraries: [
                    { id: '1', name: 'Films' },
                    { id: '3', name: 'Kids Films' },
                ],
            },
        })
        installFetch({ settings: stored })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Films' })).toBeTruthy())
        for (const name of ['Films', 'Kids Films']) {
            const box = screen.getByRole('checkbox', { name }) as HTMLInputElement
            expect(box.checked).toBe(true)
        }
        // No check has run, so nothing may be claimed missing.
        expect(screen.queryByText(/not on this server/)).toBeNull()
    })

    // Once a check has enumerated the libraries, a saved one absent from that list
    // genuinely is gone, and saying so is the point.
    it('marks a saved library the server no longer lists, after a check', async () => {
        const stored = settingsFixture({
            plex: {
                enabled: true,
                url: 'http://plex:32400',
                tokenSet: true,
                librarySection: '',
                libraries: [{ id: '9', name: 'Retired Films' }],
            },
        })
        installFetch({
            settings: stored,
            plexCheck: {
                valid: true,
                message: 'Connected.',
                libraries: [{ id: '1', name: 'Films' }],
            },
        })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() =>
            expect(screen.getByRole('checkbox', { name: 'Retired Films' })).toBeTruthy(),
        )
        expect(screen.queryByText(/not on this server/)).toBeNull()

        await user.click(screen.getByRole('button', { name: 'Check connection' }))

        await waitFor(() =>
            expect(screen.getByText(/Retired Films — not on this server/)).toBeTruthy(),
        )
        // Still checked, so it can be switched off rather than persisting silently.
        expect(
            (screen.getByRole('checkbox', { name: /Retired Films/ }) as HTMLInputElement).checked,
        ).toBe(true)
    })

    // An empty picker reported an unreachable TMDB, a rejected token, and a region
    // that genuinely lists nothing all the same way — the least actionable of the
    // three. The server's own explanation is shown instead.
    it('says why the streaming service list is empty', async () => {
        const streaming = settingsFixture({
            streaming: {
                enabled: true,
                tmdbReadTokenSet: true,
                watchRegion: 'US',
                providers: [],
            },
        })
        installFetch({ settings: streaming, providerListStatus: 502 })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)

        await waitFor(() =>
            expect(screen.getByText('could not reach TMDB to list providers')).toBeTruthy(),
        )
    })

    it('checks Plex against the candidate library section', async () => {
        installFetch({ plexCheck: { valid: true, message: 'Connected — using the "Films" library.' } })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByLabelText('Token')).toBeTruthy())

        await user.click(screen.getByRole('button', { name: 'Check connection' }))

        await waitFor(() =>
            expect(screen.getByText('Connected — using the "Films" library.')).toBeTruthy(),
        )
        const check = posted.find((p) => p.url === '/api/settings/verify/plex')
        // The stored token is not on this page, so the check has to send an empty
        // secret and let the server fall back to what it has.
        expect(check?.body).toEqual({ url: 'http://plex.local:32400', secret: '', librarySection: '2' })
    })

    // Checking a stored TMDB token has to submit an empty token, because the
    // screen never receives the stored one. Sending the placeholder would have the
    // server check a credential made of bullet characters, and reporting "no token
    // was supplied" made the screen hide the provider picker for a working token.
    it('checks a stored TMDB token without inventing a value for it', async () => {
        const streaming = settingsFixture({
            streaming: {
                enabled: true,
                tmdbReadTokenSet: true,
                watchRegion: 'US',
                providers: ['netflix'],
            },
        })
        installFetch({ settings: streaming, verifyValid: true, providers: [] })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() => expect(screen.getByRole('button', { name: 'Check token' })).toBeTruthy())

        await user.click(screen.getByRole('button', { name: 'Check token' }))

        await waitFor(() => expect(screen.getByText('Token accepted.')).toBeTruthy())
        const check = posted.find((p) => p.url === '/api/settings/verify/tmdb')
        expect(check?.body).toEqual({ token: '', region: 'US' })
    })

    // The provider list was re-requested on every keystroke anywhere in the form,
    // because the effect depended on the whole draft object and every edit
    // produces a new one.
    it('does not re-request the provider list when an unrelated field changes', async () => {
        const streaming = settingsFixture({
            streaming: {
                enabled: true,
                tmdbReadTokenSet: true,
                watchRegion: 'US',
                providers: [],
            },
        })
        installFetch({ settings: streaming, providers: [{ id: 'netflix', name: 'Netflix' }] })
        const user = userEvent.setup()
        renderSettings()
        await enterToken(user)
        await waitFor(() =>
            expect(posted.filter((p) => p.url === '/api/settings/providers')).toHaveLength(1),
        )

        await user.type(screen.getByLabelText('Public URL'), 'abcdef')

        expect(posted.filter((p) => p.url === '/api/settings/providers')).toHaveLength(1)
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

    // A library change rebuilds the source set and ends every session, so the
    // confirmation has to fire for it.
    it('is true when a library selection changes', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        draft.plex.libraries = [{ id: '1', name: 'Films' }]
        expect(sourceAffecting(draft, from)).toBe(true)
    })

    it('is true when a provider selection changes', () => {
        const from = settingsFixture()
        const draft = draftFrom(from)
        draft.streaming.providers = ['netflix']
        expect(sourceAffecting(draft, from)).toBe(true)
    })
})
