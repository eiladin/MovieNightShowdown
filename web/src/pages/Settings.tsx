import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router'
import {
    getSettings,
    listProviders,
    saveSettings,
    SettingsAuthError,
    ValidationError,
    verifyTmdbToken,
    type ProviderOption,
    type Settings as SettingsData,
} from '../api'
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
// They are credentials, so they are type="password" — but none of them is an
// account password, and there is no username on this page. A manager that
// offers to fill one is offering the wrong secret, and one that offers to save
// it stores a server credential under a login it invented. `autocomplete="off"`
// alone does not stop them; every major manager needs its own opt-out, so they
// are all set. `new-password` is the value browsers honour for "do not fill".
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

    const [tmdbVerified, setTmdbVerified] = useState(false)
    const [verifying, setVerifying] = useState(false)
    const [verifyMessage, setVerifyMessage] = useState('')
    const [providers, setProviders] = useState<ProviderOption[]>([])

    const load = useCallback(async (value: string) => {
        try {
            const data = await getSettings(value)
            setSettings(data)
            setDraft(draftFrom(data))
            setNeedsToken(false)
            setFailure('')
            // A token already stored means it verified at some point; the
            // picker can be populated without asking again.
            setTmdbVerified(data.streaming.tmdbReadTokenSet)
        } catch (err) {
            if (err instanceof SettingsAuthError) {
                setNeedsToken(true)
                setFailure('That setup token was not accepted.')
                return
            }
            setFailure('Could not load the current settings.')
        }
    }, [])

    useEffect(() => {
        if (token) void load(token)
    }, [token, load])

    // Populate the picker once a token is known to work. Without a verified
    // token the list cannot be fetched, so an empty picker would appear with no
    // explanation.
    useEffect(() => {
        if (!token || !tmdbVerified || !draft) return
        let cancelled = false
        listProviders(token, draft.streaming.watchRegion)
            .then((list) => {
                if (!cancelled) setProviders(list.providers)
            })
            .catch(() => {
                if (!cancelled) setProviders([])
            })
        return () => {
            cancelled = true
        }
    }, [token, tmdbVerified, draft?.streaming.watchRegion, draft])

    function submitToken(e: React.FormEvent) {
        e.preventDefault()
        setSetupToken(tokenInput)
        setToken(tokenInput.trim())
    }

    async function handleVerify() {
        if (!draft || !token) return
        setVerifying(true)
        setVerifyMessage('')
        try {
            const candidate =
                draft.streaming.tmdbReadToken === SECRET_PLACEHOLDER
                    ? ''
                    : draft.streaming.tmdbReadToken
            const result = await verifyTmdbToken(token, candidate, draft.streaming.watchRegion)
            setTmdbVerified(result.valid)
            setVerifyMessage(result.valid ? 'Token accepted.' : (result.message ?? 'Token rejected.'))
            if (result.valid && candidate) {
                const list = await listProviders(token, draft.streaming.watchRegion, candidate)
                setProviders(list.providers)
            }
        } catch {
            setTmdbVerified(false)
            setVerifyMessage('Could not check the token.')
        } finally {
            setVerifying(false)
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
            <div className="settings-page">
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
                            <input
                                id="jf-url"
                                value={draft.jellyfin.url}
                                onChange={(e) =>
                                    setDraft({
                                        ...draft,
                                        jellyfin: { ...draft.jellyfin, url: e.target.value },
                                    })
                                }
                            />
                            {badge('jellyfin.url')}
                            {fieldError('jellyfin.url')}

                            <label htmlFor="jf-key">API key</label>
                            <input
                                id="jf-key"
                                type="password"
                                value={draft.jellyfin.apiKey}
                                onChange={(e) =>
                                    setDraft({
                                        ...draft,
                                        jellyfin: { ...draft.jellyfin, apiKey: e.target.value },
                                    })
                                }
                                {...noAutofill}
                            />
                            {clearControl(settings.jellyfin.apiKeySet, draft.jellyfin.apiKey, () =>
                                setDraft({ ...draft, jellyfin: { ...draft.jellyfin, apiKey: '' } }),
                            )}
                            {badge('jellyfin.apiKey')}
                            {fieldError('jellyfin.apiKey')}

                            <label htmlFor="jf-user">User ID (optional)</label>
                            <input
                                id="jf-user"
                                value={draft.jellyfin.userId}
                                onChange={(e) =>
                                    setDraft({
                                        ...draft,
                                        jellyfin: { ...draft.jellyfin, userId: e.target.value },
                                    })
                                }
                            />
                            {badge('jellyfin.userId')}
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
                                value={draft.plex.url}
                                onChange={(e) =>
                                    setDraft({ ...draft, plex: { ...draft.plex, url: e.target.value } })
                                }
                            />
                            {badge('plex.url')}
                            {fieldError('plex.url')}

                            <label htmlFor="plex-token">Token</label>
                            <input
                                id="plex-token"
                                type="password"
                                value={draft.plex.token}
                                onChange={(e) =>
                                    setDraft({ ...draft, plex: { ...draft.plex, token: e.target.value } })
                                }
                                {...noAutofill}
                            />
                            {clearControl(settings.plex.tokenSet, draft.plex.token, () =>
                                setDraft({ ...draft, plex: { ...draft.plex, token: '' } }),
                            )}
                            {badge('plex.token')}
                            {fieldError('plex.token')}

                            <label htmlFor="plex-section">Library section (optional)</label>
                            <input
                                id="plex-section"
                                value={draft.plex.librarySection}
                                onChange={(e) =>
                                    setDraft({
                                        ...draft,
                                        plex: { ...draft.plex, librarySection: e.target.value },
                                    })
                                }
                            />
                            {badge('plex.librarySection')}
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
                                    setTmdbVerified(false)
                                    setVerifyMessage('')
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
                                    setTmdbVerified(false)
                                    setVerifyMessage('')
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
                                value={draft.streaming.watchRegion}
                                onChange={(e) =>
                                    setDraft({
                                        ...draft,
                                        streaming: { ...draft.streaming, watchRegion: e.target.value },
                                    })
                                }
                            />
                            {badge('streaming.watchRegion')}

                            <button type="button" onClick={handleVerify} disabled={verifying}>
                                {verifying ? 'Checking…' : 'Check token'}
                            </button>
                            {verifyMessage && <p className="settings-verify">{verifyMessage}</p>}

                            {/* The picker appears only once a token works: without
                                one the list cannot be fetched, and an empty picker
                                would look like a deployment with no services. */}
                            {tmdbVerified ? (
                                <fieldset className="settings-providers">
                                    <legend>Services</legend>
                                    {providers.length === 0 && <p>No services were returned for this region.</p>}
                                    {providers.map((p) => (
                                        <label key={p.id}>
                                            <input
                                                type="checkbox"
                                                checked={draft.streaming.providers.includes(p.id)}
                                                onChange={(e) => {
                                                    const next = e.target.checked
                                                        ? [...draft.streaming.providers, p.id]
                                                        : draft.streaming.providers.filter((id) => id !== p.id)
                                                    setDraft({
                                                        ...draft,
                                                        streaming: { ...draft.streaming, providers: next },
                                                    })
                                                }}
                                            />
                                            {p.name}
                                        </label>
                                    ))}
                                </fieldset>
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
                            value={draft.publicUrl}
                            onChange={(e) => setDraft({ ...draft, publicUrl: e.target.value })}
                        />
                        {badge('publicUrl')}
                        {fieldError('publicUrl')}

                        <label htmlFor="session-ttl">Session lifetime</label>
                        <input
                            id="session-ttl"
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
