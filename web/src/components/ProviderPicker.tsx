import { useEffect, useMemo, useRef, useState } from 'react'
import type { ProviderOption } from '../api'
import '../styles/provider-picker.css'

interface ProviderPickerProps {
    options: ProviderOption[]
    selected: string[]
    onChange: (next: string[]) => void
}

// MAX_VISIBLE caps how many results are rendered at once.
//
// TMDB lists several hundred watch providers for a region. Rendering every match
// makes the overlay a wall of services nobody has, which is the same problem the
// search box was added to solve — a shorter list is easier to read than a
// complete one you have to skim. The remainder is counted rather than dropped
// silently, so the list never looks exhaustive when it is not.
const MAX_VISIBLE = 5

// ProviderPicker selects a handful of streaming services out of the several
// hundred TMDB lists for a region.
//
// It shows what is selected and nothing else until you type. An empty query
// deliberately renders no results at all — not even on focus — because "every
// service, unfiltered" is not a list anyone reads, and showing it on focus meant
// clicking into the field buried the rest of the form under an overlay.
//
// The results appear in an overlay rather than inline so opening them does not
// reflow the form underneath.
export default function ProviderPicker({ options, selected, onChange }: ProviderPickerProps) {
    const [query, setQuery] = useState('')
    const [open, setOpen] = useState(false)
    const [active, setActive] = useState(0)
    const containerRef = useRef<HTMLDivElement>(null)
    const inputRef = useRef<HTMLInputElement>(null)

    const byId = useMemo(() => new Map(options.map((o) => [o.id, o])), [options])

    const trimmed = query.trim().toLowerCase()

    const matches = useMemo(() => {
        if (trimmed === '') return []
        return options.filter(
            (o) => !selected.includes(o.id) && o.name.toLowerCase().includes(trimmed),
        )
    }, [options, selected, trimmed])

    // visible is what the keyboard walks as well as what renders. Highlighting a
    // row that was sliced off would make Enter select something invisible.
    const visible = matches.slice(0, MAX_VISIBLE)
    const hidden = matches.length - visible.length

    // The overlay needs a query, not just focus. Gating on `open` alone is what
    // made clicking the field cover the form with an unfiltered list.
    const showOptions = open && trimmed !== ''

    // Reset the highlight whenever the result set changes, so Enter never
    // selects something that scrolled out from under the cursor.
    useEffect(() => {
        setActive(0)
    }, [query, open])

    // Close on a click outside. Without this the overlay survives interaction
    // with the rest of the form and covers the fields below it.
    useEffect(() => {
        if (!open) return
        function onPointerDown(e: MouseEvent) {
            if (!containerRef.current?.contains(e.target as Node)) setOpen(false)
        }
        document.addEventListener('mousedown', onPointerDown)
        return () => document.removeEventListener('mousedown', onPointerDown)
    }, [open])

    function add(id: string) {
        if (!selected.includes(id)) onChange([...selected, id])
        setQuery('')
        setOpen(false)
        inputRef.current?.focus()
    }

    function remove(id: string) {
        onChange(selected.filter((s) => s !== id))
    }

    function onKeyDown(e: React.KeyboardEvent) {
        if (e.key === 'Escape') {
            setOpen(false)
            return
        }
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault()
            setOpen(true)
            setActive((i) => {
                if (visible.length === 0) return 0
                const next = e.key === 'ArrowDown' ? i + 1 : i - 1
                return (next + visible.length) % visible.length
            })
            return
        }
        if (e.key === 'Enter') {
            // The picker lives inside the settings form; Enter here must choose
            // an option, not submit everything.
            e.preventDefault()
            if (showOptions && visible[active]) add(visible[active].id)
        }
    }

    return (
        <div className="provider-picker" ref={containerRef}>
            <ul className="provider-chips">
                {selected.length === 0 && (
                    <li className="provider-empty">No services selected yet.</li>
                )}
                {selected.map((id) => (
                    <li key={id} className="provider-chip">
                        {/* A saved service that the current region's list does not
                            contain still renders, by its id, so changing region
                            cannot silently drop a selection. */}
                        {byId.get(id)?.name ?? id}
                        <button
                            type="button"
                            aria-label={`Remove ${byId.get(id)?.name ?? id}`}
                            onClick={() => remove(id)}
                        >
                            ×
                        </button>
                    </li>
                ))}
            </ul>

            <input
                ref={inputRef}
                type="text"
                role="combobox"
                aria-expanded={showOptions}
                aria-controls="provider-options"
                aria-autocomplete="list"
                placeholder="Type to find a service…"
                value={query}
                onChange={(e) => {
                    setQuery(e.target.value)
                    setOpen(true)
                }}
                onFocus={() => setOpen(true)}
                onKeyDown={onKeyDown}
                autoComplete="off"
            />

            {showOptions && (
                <ul className="provider-options" id="provider-options" role="listbox">
                    {matches.length === 0 && (
                        <li className="provider-none">
                            {options.length === 0
                                ? 'No services were returned for this region.'
                                : 'No matches.'}
                        </li>
                    )}
                    {/* The option is the interactive element itself rather than a
                        button inside it: a listbox option that only responds when
                        an inner control is hit is a target users miss. */}
                    {visible.map((o, i) => (
                        <li
                            key={o.id}
                            role="option"
                            aria-selected={i === active}
                            className={i === active ? 'active' : undefined}
                            onMouseEnter={() => setActive(i)}
                            onMouseDown={(e) => e.preventDefault()}
                            onClick={() => add(o.id)}
                        >
                            {o.name}
                        </li>
                    ))}
                    {hidden > 0 && (
                        <li className="provider-more" aria-live="polite">
                            {hidden} more — keep typing to narrow.
                        </li>
                    )}
                </ul>
            )}
        </div>
    )
}
