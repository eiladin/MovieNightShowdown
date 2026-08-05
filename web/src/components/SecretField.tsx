import { useState } from 'react'
import '../styles/secret-field.css'

interface SecretFieldProps {
    id: string
    value: string
    onChange: (value: string) => void
    // storedMarker is the value that means "a secret is saved on the server",
    // rather than a secret itself. Revealing it would show the marker's bullet
    // characters, so the toggle is withheld while the field still holds it.
    storedMarker?: string
    // rest carries the password-manager opt-out attributes, which have to reach
    // the input rather than this wrapper.
    inputProps?: Record<string, string>
    'aria-describedby'?: string
}

// SecretField is a credential input with a reveal toggle.
//
// It is a toggle rather than press-and-hold on purpose. These values are pasted
// or transcribed from somewhere else — an API key out of a Jellyfin dashboard, a
// token out of a log — and checking that what landed in the field matches the
// source means reading both, which takes two hands and more than a moment.
//
// The toggle only appears once the field holds something to reveal. An empty
// field has nothing, and a field still showing the stored-value marker has only
// that marker: the server never sends a saved credential back, so "show" there
// would display bullet characters and read as a bug.
export default function SecretField({
    id,
    value,
    onChange,
    storedMarker,
    inputProps,
    'aria-describedby': describedBy,
}: SecretFieldProps) {
    const [shown, setShown] = useState(false)

    const canReveal = value !== '' && value !== storedMarker
    // Revealing while nothing is revealable would leave the field readable the
    // next time it does hold something, which is not what was asked for.
    const revealed = shown && canReveal

    return (
        <div className="secret-field">
            <input
                id={id}
                // Switching the type is what reveals the value. The
                // password-manager opt-outs still apply in either state; a text
                // input beside a labelled credential is no less interesting to
                // them than a password one.
                type={revealed ? 'text' : 'password'}
                value={value}
                aria-describedby={describedBy}
                onChange={(e) => onChange(e.target.value)}
                {...inputProps}
            />
            {canReveal && (
                <button
                    type="button"
                    className="secret-toggle"
                    // A toggle reports its state; a button that merely changes its
                    // own label leaves a screen reader announcing the action and
                    // never the result.
                    aria-pressed={revealed}
                    aria-label={revealed ? 'Hide the value' : 'Show the value'}
                    title={revealed ? 'Hide the value' : 'Show the value'}
                    onClick={() => setShown((s) => !s)}
                >
                    {revealed ? <EyeOffIcon /> : <EyeIcon />}
                </button>
            )}
        </div>
    )
}

// The icons are inline SVG rather than a font or an image so they inherit
// currentColor and need no network request. aria-hidden because the button
// already carries the label; announcing the glyph as well would say it twice.
function EyeIcon() {
    return (
        <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false">
            <path
                d="M1.5 12S5.5 5 12 5s10.5 7 10.5 7-4 7-10.5 7S1.5 12 1.5 12Z"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
            <circle cx="12" cy="12" r="3.2" fill="none" stroke="currentColor" strokeWidth="1.8" />
        </svg>
    )
}

function EyeOffIcon() {
    return (
        <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false">
            <path
                d="M3 3l18 18"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
            />
            <path
                d="M10.6 6.2A9.9 9.9 0 0 1 12 6c6 0 9.5 6 9.5 6a17 17 0 0 1-2.5 3.2M6.6 8.1A17 17 0 0 0 2.5 12s3.5 6 9.5 6a9.6 9.6 0 0 0 3.6-.7"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
            <path
                d="M9.9 9.9a3.2 3.2 0 0 0 4.3 4.3"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
            />
        </svg>
    )
}
