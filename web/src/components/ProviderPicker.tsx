import { useEffect, useMemo, useRef, useState } from 'react'
import type { ProviderOption } from '../api'
import '../styles/provider-picker.css'

interface ProviderPickerProps {
    options: ProviderOption[]
    selected: string[]
    onChange: (next: string[]) => void
}

// ProviderPicker selects a handful of streaming services out of the several
// hundred TMDB lists for a region.
//
// It shows what is selected and nothing else until you search. Rendering every
// option as a checkbox spent a screen of space to express a choice that is
// usually three or four items, and made the rest of the form unreachable
// without scrolling past services nobody has.
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

    const matches = useMemo(() => {
        const q = query.trim().toLowerCase()
        return options.filter(
            (o) => !selected.includes(o.id) && (q === '' || o.name.toLowerCase().includes(q)),
        )
    }, [options, selected, query])

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
                if (matches.length === 0) return 0
                const next = e.key === 'ArrowDown' ? i + 1 : i - 1
                return (next + matches.length) % matches.length
            })
            return
        }
        if (e.key === 'Enter') {
            // The picker lives inside the settings form; Enter here must choose
            // an option, not submit everything.
            e.preventDefault()
            if (open && matches[active]) add(matches[active].id)
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
                aria-expanded={open}
                aria-controls="provider-options"
                aria-autocomplete="list"
                placeholder="Search services to add…"
                value={query}
                onChange={(e) => {
                    setQuery(e.target.value)
                    setOpen(true)
                }}
                onFocus={() => setOpen(true)}
                onKeyDown={onKeyDown}
                autoComplete="off"
            />

            {open && (
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
                    {matches.map((o, i) => (
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
                </ul>
            )}
        </div>
    )
}
