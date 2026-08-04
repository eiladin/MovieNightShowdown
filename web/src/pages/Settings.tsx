import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router'
import {
    getSettings,
    listJellyfinUsers,
    listProviders,
    saveSettings,
    SettingsAuthError,
    ValidationError,
    verifyJellyfin,
    verifyPlex,
    verifyTmdbToken,
    type JellyfinUser,
    type LibrarySection,
    type ProviderOption,
    type Settings as SettingsData,
    type SourceCheck,
} from '../api'
import ProviderPicker from '../components/ProviderPicker'
import { getSetupToken, setSetupToken } from '../setupToken'
import {
    buildUpdate,
    draftFrom,
    SECRET_PLACEHOLDER,
    sourceAffecting,
    type Draft,
} from '../settingsDraft'
import '../styles/settings.css'

// noAutofill keeps password managers out of these fields.
//
// Most of them are credentials, so they are type="password" — but none is an
// account password, and there is no login on this page. A manager that offers to
// fill one is offering the wrong secret, and one that offers to save it stores a
// server credential under a login it invented. `autocomplete="off"` alone does
// not stop them; every major manager needs its own opt-out, so they are all set.
// `new-password` is the value browsers honour for "do not fill".
//
// The Jellyfin account field gets it too despite being plain text: it sits
// directly above an API key field, and a text input followed by a password input
// is exactly the shape a manager reads as a login form.
const noAutofill = {
    autoComplete: 'new-password',
    'data-1p-ignore': '',
    'data-lpignore': 'true',
    'data-bwignore': '',
    'data-protonpass-ignore': '',
    'data-form-type': 'other',
} as const

const OUTCOME_TEXT: Record<string, string> = {
    no_change: 'Saved. Nothing changed, so nothing was restarted.',
    applied: 'Saved and applied. The new configuration is live.',
    restart_required: 'Saved, but a restart is needed before it takes effect.',
}

// SourceKey names the three checkable sections. The state for a check is the
// same shape in each, so it is held in one map rather than three sets of
// near-identical variables.
type SourceKey = 'jellyfin' | 'plex' | 'streaming'

interface CheckState {
    checking: boolean
    valid: boolean
    message: string
}

const NO_CHECK: CheckState = { checking: false, valid: false, message: '' }

// candidateSecret turns a secret field into what a check should submit.
//
// The placeholder means "unchanged", and the server reads an empty secret as
// "use the stored one" — the same thing. Submitting the placeholder itself would
// have the server check a credential made of bullet characters.
function candidateSecret(value: string): string {
    return value === SECRET_PLACEHOLDER ? '' : value
}

export default function Settings() {
    const [token, setToken] = useState(getSetupToken())
    const [tokenInput, setTokenInput] = useState('')
    const [settings, setSettings] = useState<SettingsData | null>(null)
    const [draft, setDraft] = useState<Draft | null>(null)
    const [errors, setErrors] = useState<Record<string, string>>({})
    const [message, setMessage] = useState('')
    const [failure, setFailure] = useState('')
    const [needsToken, setNeedsToken] = useState(!getSetupToken())
    const [saving, setSaving] = useState(false)

    const [checks, setChecks] = useState<Record<SourceKey, CheckState>>({
        jellyfin: NO_CHECK,
        plex: NO_CHECK,
        streaming: NO_CHECK,
    })
    const [providers, setProviders] = useState<ProviderOption[]>([])
    // providerError carries why the service list could not be fetched. Without
    // it, an unreachable TMDB, a rejected token, and a region that genuinely
    // lists nothing all rendered as an empty picker.
    const [providerError, setProviderError] = useState('')
    // users is null until the list has been asked for. Empty means it was asked
    // for and Jellyfin did not answer, which is a different state: the account
    // field is not offered at all rather than offered empty.
    const [users, setUsers] = useState<JellyfinUser[] | null>(null)
    // sections holds the Plex movie libraries a check found. Until one has run
    // there is nothing to choose from, and the key is an opaque number nobody
    // knows offhand, so the field is not shown.
    const [sections, setSections] = useState<LibrarySection[] | null>(null)

    // A stored token verified at some point, so the picker can be populated
    // without asking again.
    const tmdbVerified = checks.streaming.valid

    const setCheck = useCallback((key: SourceKey, state: CheckState) => {
        setChecks((prev) => ({ ...prev, [key]: state }))
    }, [])

    const load = useCallback(
        async (value: string) => {
            try {
                const data = await getSettings(value)
                setSettings(data)
                setDraft(draftFrom(data))
                setNeedsToken(false)
                setFailure('')
                setCheck('streaming', { ...NO_CHECK, valid: data.streaming.tmdbReadTokenSet })
            } catch (err) {
                if (err instanceof SettingsAuthError) {
                    setNeedsToken(true)
                    setFailure('That setup token was not accepted.')
                    return
                }
                setFailure('Could not load the current settings.')
            }
        },
        [setCheck],
    )

    useEffect(() => {
        if (token) void load(token)
    }, [token, load])

    // Populate the picker once a token is known to work. Without a verified
    // token the list cannot be fetched, so an empty picker would appear with no
    // explanation.
    //
    // The dependency is the region string, not the draft object. Depending on the
    // draft fired this request on every keystroke anywhere in the form, because
    // every edit produces a new object.
    const region = draft?.streaming.watchRegion
    useEffect(() => {
        if (!token || !tmdbVerified || region === undefined) return
        let cancelled = false
        listProviders(token, region)
            .then((list) => {
                if (cancelled) return
                setProviders(list.providers)
                setProviderError(
                    list.providers.length === 0
                        ? `TMDB lists no streaming services for region ${region}.`
                        : '',
                )
            })
            .catch((err: unknown) => {
                if (cancelled) return
                setProviders([])
                setProviderError(
                    err instanceof Error
                        ? err.message
                        : 'The list of streaming services could not be fetched.',
                )
            })
        return () => {
            cancelled = true
        }
    }, [token, tmdbVerified, region])

    // Read the Jellyfin accounts so the user id can be chosen rather than
    // transcribed. Same rule as above: the dependencies are the stored values
    // this needs, not the settings object they arrived in.
    const jellyfinReady =
        !!settings?.jellyfin.enabled && !!settings.jellyfin.url && settings.jellyfin.apiKeySet
    useEffect(() => {
        if (!token || !jellyfinReady) {
            setUsers(null)
            return
        }
        let cancelled = false
        // An empty request means "use the stored URL and key", which is exactly
        // what is wanted on load.
        listJellyfinUsers(token, {})
            .then((list) => {
                if (!cancelled) setUsers(list)
            })
            .catch(() => {
                if (!cancelled) setUsers([])
            })
        return () => {
            cancelled = true
        }
    }, [token, jellyfinReady])

    function submitToken(e: React.FormEvent) {
        e.preventDefault()
        setSetupToken(tokenInput)
        setToken(tokenInput.trim())
    }

    // runCheck wraps the three checks in one state transition, so a section can
    // never be left showing a stale result next to a spinner.
    async function runCheck(key: SourceKey, check: () => Promise<SourceCheck>) {
        setCheck(key, { checking: true, valid: false, message: '' })
        try {
            const result = await check()
            setCheck(key, {
                checking: false,
                valid: result.valid,
                message: result.message ?? (result.valid ? 'Connected.' : 'That did not work.'),
            })
            return result
        } catch {
            setCheck(key, {
                checking: false,
                valid: false,
                message: 'The check could not be run.',
            })
            return null
        }
    }

    async function handleCheckJellyfin() {
        if (!draft || !token) return
        const req = {
            url: draft.jellyfin.url,
            secret: candidateSecret(draft.jellyfin.apiKey),
        }
        const result = await runCheck('jellyfin', () => verifyJellyfin(token, req))
        // A successful check may have used credentials that are not saved yet, so
        // the user list is re-read through the same ones. Without this, choosing
        // an account would need a save first.
        if (result?.valid) {
            try {
                setUsers(await listJellyfinUsers(token, req))
            } catch {
                setUsers([])
            }
        }
    }

    async function handleCheckPlex() {
        if (!draft || !token) return
        const result = await runCheck('plex', () =>
            verifyPlex(token, {
                url: draft.plex.url,
                secret: candidateSecret(draft.plex.token),
                librarySection: draft.plex.librarySection,
            }),
        )
        // Sections arrive on a failing result too — an unknown configured key is
        // exactly when the available ones are needed.
        if (result?.sections) setSections(result.sections)
    }

    async function handleVerify() {
        if (!draft || !token) return
        const candidate = candidateSecret(draft.streaming.tmdbReadToken)
        const result = await runCheck('streaming', () =>
            verifyTmdbToken(token, candidate, draft.streaming.watchRegion).then((v) => ({
                valid: v.valid,
                message: v.valid ? 'Token accepted.' : (v.message ?? 'Token rejected.'),
            })),
        )
        if (result?.valid && candidate) {
            try {
                const list = await listProviders(token, draft.streaming.watchRegion, candidate)
                setProviders(list.providers)
            } catch {
                setProviders([])
            }
        }
    }

    async function handleSave(e: React.FormEvent) {
        e.preventDefault()
        if (!draft || !settings || !token) return

        if (sourceAffecting(draft, settings)) {
            const ok = window.confirm(
                'Changing a movie source rebuilds the deck sources. Any session currently in ' +
                    'progress will end, and everyone swiping will be sent to the leaderboard. Continue?',
            )
            if (!ok) return
        }

        setSaving(true)
        setErrors({})
        setMessage('')
        setFailure('')
        try {
            const saved = await saveSettings(token, buildUpdate(draft, settings))
            setSettings(saved)
            setDraft(draftFrom(saved))
            setMessage(OUTCOME_TEXT[saved.outcome ?? ''] ?? 'Saved.')
        } catch (err) {
            if (err instanceof ValidationError) {
                setErrors(err.fields)
                setFailure('Some settings were rejected. Nothing was saved.')
            } else if (err instanceof SettingsAuthError) {
                setNeedsToken(true)
                setFailure('The setup token was rejected.')
            } else {
                setFailure('Could not save the settings.')
            }
        } finally {
            setSaving(false)
        }
    }

    if (needsToken) {
        return (
            <div className="settings-page settings-gate">
                <h1>Settings</h1>
                <p className="settings-lede">
                    Changing this server&apos;s configuration needs its setup token. The server
                    prints the token to its log when it starts — with Docker Compose, run{' '}
                    <code>docker compose logs | grep &quot;setup: token&quot;</code>.
                </p>
                {failure && <p className="settings-error">{failure}</p>}
                <form onSubmit={submitToken} className="settings-token-form" data-form-type="other">
                    <label htmlFor="setup-token">Setup token</label>
                    <input
                        id="setup-token"
                        type="password"
                        value={tokenInput}
                        onChange={(e) => setTokenInput(e.target.value)}
                        {...noAutofill}
                    />
                    <button type="submit">Continue</button>
                </form>
                <p className="settings-footnote">
                    <Link to="/setup">Configuring with environment variables instead?</Link>
                </p>
            </div>
        )
    }

    if (!draft || !settings) {
        return (
            <div className="settings-page">
                <h1>Settings</h1>
                {failure ? <p className="settings-error">{failure}</p> : <p>Loading…</p>}
            </div>
        )
    }

    const badge = (key: string) => {
        const p = settings.provenance[key]
        if (!p?.envIgnored || !p.envVar) return null
        return (
            <span className="settings-badge" role="note">
                {p.envVar} is set but ignored — the saved value takes precedence
            </span>
        )
    }

    const fieldError = (key: string) =>
        errors[key] ? <span className="settings-field-error">{errors[key]}</span> : null

    // Two fields are only offered as a list, because both take a value nobody
    // knows offhand: a Jellyfin user id and a Plex section key are both opaque
    // identifiers that have to be read off the source itself. An empty text box
    // asking for either is a request to go rummaging.
    const hasUsers = !!users && users.length > 0
    const hasSections = !!sections && sections.length > 0

    // Whether the field is offered at all is decided by what the server holds,
    // never by the draft. Keying it on the draft made the field unmount the moment
    // its value was cleared — so an operator emptying it could not then retype,
    // and the control vanished out from under the cursor.
    const showUserField = hasUsers || settings.jellyfin.userId !== ''
    const showSectionField = hasSections || settings.plex.librarySection !== ''

    // clearControl is the explicit way to remove a stored credential. Emptying
    // the field alone would also work, but nothing on screen would say so —
    // and an operator who cannot find how to remove a credential will leave a
    // dead one in place.
    const clearControl = (stored: boolean, current: string, onClear: () => void) => {
        if (!stored) return null
        if (current === '') {
            return (
                <span className="settings-hint">
                    This will be removed when you save.
                </span>
            )
        }
        return (
            <button type="button" className="settings-clear" onClick={onClear}>
                Remove stored value
            </button>
        )
    }

    // checkControl is the "does this actually work" button for one section.
    //
    // It exists because the alternative diagnostic is a movie night: a URL typo
    // or a stale credential otherwise surfaces as an empty deck with four people
    // already holding their phones.
    const checkControl = (key: SourceKey, label: string, onRun: () => void) => {
        const state = checks[key]
        return (
            <div className="settings-check">
                <button
                    type="button"
                    className="settings-check-button"
                    onClick={onRun}
                    disabled={state.checking}
                >
                    {state.checking ? 'Checking…' : label}
                </button>
                {state.message && (
                    <p
                        className={state.valid ? 'settings-check-ok' : 'settings-check-bad'}
                        role="status"
                    >
                        {state.message}
                    </p>
                )}
            </div>
        )
    }

    return (
        <div className="settings-page">
            <h1>Settings</h1>
            <p className="settings-lede">
                These are stored on the server and take effect without a restart, except where
                noted. Credentials are never sent back to this page.
            </p>

            {message && <p className="settings-success">{message}</p>}
            {failure && <p className="settings-error">{failure}</p>}

            <form onSubmit={handleSave} data-form-type="other">
                <section className="settings-section">
                    <h2>Jellyfin</h2>
                    <label className="settings-toggle">
                        <input
                            type="checkbox"
                            checked={draft.jellyfin.enabled}
                            onChange={(e) =>
                                setDraft({
                                    ...draft,
                                    jellyfin: { ...draft.jellyfin, enabled: e.target.checked },
                                })
                            }
                        />
                        Enable Jellyfin
                    </label>
                    {draft.jellyfin.enabled && (
                        <div className="settings-fields">
                            <label htmlFor="jf-url">Server URL</label>
                            {/* Editing an input a check was run against discards the
                                result. A green "Connected" sitting beside a URL that
                                has since been retyped is worse than no result. */}
                            <input
                                id="jf-url"
                                type="text"
                                value={draft.jellyfin.url}
                                onChange={(e) => {
                                    setCheck('jellyfin', NO_CHECK)
                                    setDraft({
                                        ...draft,
                                        jellyfin: { ...draft.jellyfin, url: e.target.value },
                                    })
                                }}
                            />
                            {badge('jellyfin.url')}
                            {fieldError('jellyfin.url')}

                            <label htmlFor="jf-key">API key</label>
                            <input
                                id="jf-key"
                                type="password"
                                value={draft.jellyfin.apiKey}
                                onChange={(e) => {
                                    setCheck('jellyfin', NO_CHECK)
                                    setDraft({
                                        ...draft,
                                        jellyfin: { ...draft.jellyfin, apiKey: e.target.value },
                                    })
                                }}
                                {...noAutofill}
                            />
                            {clearControl(settings.jellyfin.apiKeySet, draft.jellyfin.apiKey, () =>
                                setDraft({ ...draft, jellyfin: { ...draft.jellyfin, apiKey: '' } }),
                            )}
                            {badge('jellyfin.apiKey')}
                            {fieldError('jellyfin.apiKey')}

                            {/* The account field appears only once the accounts are
                                known, or when one is already stored.

                                A Jellyfin user id is a 32-character hex string out of
                                the admin dashboard's URL bar. An empty text box asking
                                for it is a request to go and transcribe something, and
                                a mistyped id is never rejected — the unwatched filter
                                simply returns nothing. A stored id still renders even
                                with no list, so it can always be seen and cleared. */}
                            {showUserField && (
                                <>
                                    <label htmlFor="jf-user">
                                        Account for &ldquo;unwatched only&rdquo;
                                    </label>
                                    <p className="settings-hint" id="jf-user-hint">
                                        Optional. Filtering a deck to unwatched films needs an
                                        account to judge watched state against. Without one, the
                                        host cannot use that filter.
                                    </p>
                                    {hasUsers ? (
                                        <select
                                            id="jf-user"
                                            aria-describedby="jf-user-hint"
                                            value={draft.jellyfin.userId}
                                            onChange={(e) =>
                                                setDraft({
                                                    ...draft,
                                                    jellyfin: {
                                                        ...draft.jellyfin,
                                                        userId: e.target.value,
                                                    },
                                                })
                                            }
                                        >
                                            <option value="">
                                                Not set — no unwatched filtering
                                            </option>
                                            {users?.map((u) => (
                                                <option key={u.id} value={u.id}>
                                                    {u.name}
                                                </option>
                                            ))}
                                            {/* A stored id this server does not list
                                                still renders, so opening the screen
                                                cannot silently drop it on the next
                                                save. */}
                                            {draft.jellyfin.userId !== '' &&
                                                !users?.some(
                                                    (u) => u.id === draft.jellyfin.userId,
                                                ) && (
                                                    <option value={draft.jellyfin.userId}>
                                                        {draft.jellyfin.userId} — not an account
                                                        on this server
                                                    </option>
                                                )}
                                        </select>
                                    ) : (
                                        <input
                                            id="jf-user"
                                            type="text"
                                            aria-describedby="jf-user-hint"
                                            {...noAutofill}
                                            value={draft.jellyfin.userId}
                                            onChange={(e) =>
                                                setDraft({
                                                    ...draft,
                                                    jellyfin: {
                                                        ...draft.jellyfin,
                                                        userId: e.target.value,
                                                    },
                                                })
                                            }
                                        />
                                    )}
                                    {badge('jellyfin.userId')}
                                </>
                            )}

                            {checkControl('jellyfin', 'Check connection', handleCheckJellyfin)}
                            {!hasUsers && (
                                <p className="settings-hint">
                                    Check the connection to choose which account the &ldquo;unwatched
                                    only&rdquo; filter should follow.
                                </p>
                            )}
                        </div>
                    )}
                </section>

                <section className="settings-section">
                    <h2>Plex</h2>
                    <label className="settings-toggle">
                        <input
                            type="checkbox"
                            checked={draft.plex.enabled}
                            onChange={(e) =>
                                setDraft({ ...draft, plex: { ...draft.plex, enabled: e.target.checked } })
                            }
                        />
                        Enable Plex
                    </label>
                    {draft.plex.enabled && (
                        <div className="settings-fields">
                            <label htmlFor="plex-url">Server URL</label>
                            <input
                                id="plex-url"
                                type="text"
                                value={draft.plex.url}
                                onChange={(e) => {
                                    setCheck('plex', NO_CHECK)
                                    setDraft({ ...draft, plex: { ...draft.plex, url: e.target.value } })
                                }}
                            />
                            {badge('plex.url')}
                            {fieldError('plex.url')}

                            <label htmlFor="plex-token">Token</label>
                            <input
                                id="plex-token"
                                type="password"
                                value={draft.plex.token}
                                onChange={(e) => {
                                    setCheck('plex', NO_CHECK)
                                    setDraft({ ...draft, plex: { ...draft.plex, token: e.target.value } })
                                }}
                                {...noAutofill}
                            />
                            {clearControl(settings.plex.tokenSet, draft.plex.token, () =>
                                setDraft({ ...draft, plex: { ...draft.plex, token: '' } }),
                            )}
                            {badge('plex.token')}
                            {fieldError('plex.token')}

                            {/* The library section appears only once a check has
                                enumerated the libraries, or when one is already stored.

                                Its value is an opaque numeric key with no way to
                                discover it except by asking Plex, so an empty text box
                                asking for it is a dead end. It is optional in any
                                case: with nothing set, the first movie library is
                                discovered on first use. */}
                            {showSectionField && (
                                <>
                                    <label htmlFor="plex-section">Movie library (optional)</label>
                                    {hasSections ? (
                                        <select
                                            id="plex-section"
                                            value={draft.plex.librarySection}
                                            onChange={(e) =>
                                                setDraft({
                                                    ...draft,
                                                    plex: {
                                                        ...draft.plex,
                                                        librarySection: e.target.value,
                                                    },
                                                })
                                            }
                                        >
                                            <option value="">
                                                Discover automatically — uses the first one
                                            </option>
                                            {sections?.map((s) => (
                                                <option key={s.key} value={s.key}>
                                                    {s.title}
                                                </option>
                                            ))}
                                            {/* A stored key this server does not list
                                                still renders, so looking at the screen
                                                cannot silently drop it. */}
                                            {draft.plex.librarySection !== '' &&
                                                !sections?.some(
                                                    (s) => s.key === draft.plex.librarySection,
                                                ) && (
                                                    <option value={draft.plex.librarySection}>
                                                        {draft.plex.librarySection} — not a library
                                                        on this server
                                                    </option>
                                                )}
                                        </select>
                                    ) : (
                                        <input
                                            id="plex-section"
                                            type="text"
                                            value={draft.plex.librarySection}
                                            onChange={(e) => {
                                                setCheck('plex', NO_CHECK)
                                                setDraft({
                                                    ...draft,
                                                    plex: {
                                                        ...draft.plex,
                                                        librarySection: e.target.value,
                                                    },
                                                })
                                            }}
                                        />
                                    )}
                                    {badge('plex.librarySection')}
                                </>
                            )}

                            {checkControl('plex', 'Check connection', handleCheckPlex)}
                            {!hasSections && (
                                <p className="settings-hint">
                                    Check the connection to choose which movie library to deal
                                    from.
                                </p>
                            )}
                        </div>
                    )}
                </section>

                <section className="settings-section">
                    <h2>Streaming</h2>
                    <label className="settings-toggle">
                        <input
                            type="checkbox"
                            checked={draft.streaming.enabled}
                            onChange={(e) =>
                                setDraft({
                                    ...draft,
                                    streaming: { ...draft.streaming, enabled: e.target.checked },
                                })
                            }
                        />
                        Enable streaming services
                    </label>
                    {draft.streaming.enabled && (
                        <div className="settings-fields">
                            <label htmlFor="tmdb-token">TMDB read token</label>
                            <input
                                id="tmdb-token"
                                type="password"
                                value={draft.streaming.tmdbReadToken}
                                onChange={(e) => {
                                    setCheck('streaming', NO_CHECK)
                                    setDraft({
                                        ...draft,
                                        streaming: {
                                            ...draft.streaming,
                                            tmdbReadToken: e.target.value,
                                        },
                                    })
                                }}
                                {...noAutofill}
                            />
                            {clearControl(
                                settings.streaming.tmdbReadTokenSet,
                                draft.streaming.tmdbReadToken,
                                () => {
                                    setCheck('streaming', NO_CHECK)
                                    setDraft({
                                        ...draft,
                                        streaming: { ...draft.streaming, tmdbReadToken: '' },
                                    })
                                },
                            )}
                            {badge('streaming.tmdbReadToken')}
                            {fieldError('streaming.tmdbReadToken')}

                            <label htmlFor="tmdb-region">Watch region</label>
                            <input
                                id="tmdb-region"
                                type="text"
                                value={draft.streaming.watchRegion}
                                onChange={(e) =>
                                    setDraft({
                                        ...draft,
                                        streaming: { ...draft.streaming, watchRegion: e.target.value },
                                    })
                                }
                            />
                            {badge('streaming.watchRegion')}

                            {checkControl('streaming', 'Check token', handleVerify)}

                            {/* The picker appears only once a token works: without
                                one the list cannot be fetched, and an empty picker
                                would look like a deployment with no services. */}
                            {tmdbVerified ? (
                                <>
                                    <span className="settings-pseudo-label" id="provider-picker-label">
                                        Services
                                    </span>
                                    {/* Why the list is empty, when it is. An unreachable
                                        TMDB, a rejected token, and a region that genuinely
                                        lists nothing are three different problems, and an
                                        empty picker is the least actionable way to report
                                        any of them. */}
                                    {providerError && (
                                        <p className="settings-check-bad" role="status">
                                            {providerError}
                                        </p>
                                    )}
                                    <ProviderPicker
                                        options={providers}
                                        selected={draft.streaming.providers}
                                        onChange={(next) =>
                                            setDraft({
                                                ...draft,
                                                streaming: { ...draft.streaming, providers: next },
                                            })
                                        }
                                    />
                                </>
                            ) : (
                                <p className="settings-hint">
                                    Check the token to choose which services to offer.
                                </p>
                            )}
                        </div>
                    )}
                </section>

                <section className="settings-section">
                    <h2>Server</h2>
                    <div className="settings-fields">
                        <label htmlFor="public-url">Public URL</label>
                        <input
                            id="public-url"
                            type="text"
                            value={draft.publicUrl}
                            onChange={(e) => setDraft({ ...draft, publicUrl: e.target.value })}
                        />
                        {badge('publicUrl')}
                        {fieldError('publicUrl')}

                        <label htmlFor="session-ttl">Session lifetime</label>
                        <input
                            id="session-ttl"
                            type="text"
                            value={draft.sessionTtl}
                            onChange={(e) => setDraft({ ...draft, sessionTtl: e.target.value })}
                        />
                        {badge('sessionTtl')}

                    </div>
                </section>

                <section className="settings-section settings-readonly">
                    <h2>Container</h2>
                    <p className="settings-hint">
                        These are fixed when the container starts. Change them in your deployment
                        — a compose file or run command — and recreate it.
                    </p>
                    <dl className="settings-readonly-list">
                        <dt>Listen port</dt>
                        <dd>{settings.runtime.port}</dd>
                        <dt>Poster cache directory</dt>
                        <dd>{settings.runtime.cacheDir}</dd>
                        <dt>Config file</dt>
                        <dd>{settings.runtime.configPath}</dd>
                    </dl>
                </section>

                <button type="submit" className="settings-save" disabled={saving}>
                    {saving ? 'Saving…' : 'Save settings'}
                </button>
            </form>

            <p className="settings-footnote">
                <Link to="/setup">Configuring with environment variables instead?</Link>
            </p>
        </div>
    )
}
