import { useEffect, useMemo, useRef, useState } from 'react'
import '../styles/chip-picker.css'

// ChipOption is one selectable thing. Both callers happen to name their entities
// this way — a streaming provider and a media library are each an opaque id with a
// display name — so the picker needs no knowledge of either.
export interface ChipOption {
    id: string
    name: string
}

// ChipPickerVariant decides how an option is added, and the choice is about
// cardinality rather than taste.
//
//   - 'search' is for a list too long to render. TMDB returns several hundred watch
//     providers for a region; the only usable control there is a search box.
//   - 'list' is for a handful. A media server has three or four movie libraries, and
//     asking somebody to guess at a name to reveal the three things they already own
//     is a search box solving a problem nobody had.
//
// Both share everything that matters: the chips, the add and remove behaviour, and
// the rule that a selected id absent from the options still renders.
export type ChipPickerVariant = 'search' | 'list'

interface ChipPickerProps {
    options: ChipOption[]
    selected: string[]
    onChange: (next: string[]) => void
    // labelId points at the heading that names this control, so the combobox — or in
    // list form, the group — is announced rather than reaching a screen reader
    // unnamed.
    labelId: string
    // emptyLabel is shown when nothing is selected, and noneLabel when the option
    // list itself came back empty. They are different states with different fixes,
    // so they are different messages.
    emptyLabel: string
    noneLabel: string
    variant?: ChipPickerVariant
    // placeholder is the search field's, and is unused by the list variant.
    placeholder?: string
}

// MAX_VISIBLE caps how many search results are rendered at once.
//
// Rendering every match makes the overlay a wall of services nobody has, which is
// the same problem the search box was added to solve — a shorter list is easier to
// read than a complete one you have to skim. The remainder is counted rather than
// dropped silently, so the list never looks exhaustive when it is not.
//
// It applies to the search variant only. The list variant renders everything,
// because everything is four things.
const MAX_VISIBLE = 5

export default function ChipPicker({
    options,
    selected,
    onChange,
    labelId,
    emptyLabel,
    noneLabel,
    variant = 'search',
    placeholder = '',
}: ChipPickerProps) {
    const [query, setQuery] = useState('')
    const [open, setOpen] = useState(false)
    const [active, setActive] = useState(0)
    const containerRef = useRef<HTMLDivElement>(null)
    const inputRef = useRef<HTMLInputElement>(null)

    const byId = useMemo(() => new Map(options.map((o) => [o.id, o])), [options])

    const searching = variant === 'search'
    const trimmed = query.trim().toLowerCase()

    // Anything not already chosen. Selecting an option removes it from here and adds
    // a chip, so neither variant can offer the same thing twice.
    const unselected = useMemo(
        () => options.filter((o) => !selected.includes(o.id)),
        [options, selected],
    )

    const matches = useMemo(() => {
        if (!searching || trimmed === '') return []
        return unselected.filter((o) => o.name.toLowerCase().includes(trimmed))
    }, [searching, unselected, trimmed])

    // visible is what the keyboard walks as well as what renders. Highlighting a
    // row that was sliced off would make Enter select something invisible.
    const visible = matches.slice(0, MAX_VISIBLE)
    const hidden = matches.length - visible.length

    // The overlay needs a query, not just focus. Gating on `open` alone is what
    // made clicking the field cover the form with an unfiltered list.
    const showOptions = searching && open && trimmed !== ''

    // Reset the highlight whenever the result set changes, so Enter never
    // selects something that scrolled out from under the cursor.
    useEffect(() => {
        setActive(0)
    }, [query, open])

    // Close on a click outside. Without this the overlay survives interaction
    // with the rest of the form and covers the fields below it. The list variant has
    // no overlay, so it never opens and this never runs.
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
        if (searching) inputRef.current?.focus()
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
        <div className="chip-picker" ref={containerRef}>
            <ul className="chip-picker-selected">
                {selected.length === 0 && <li className="chip-picker-empty">{emptyLabel}</li>}
                {selected.map((id) => (
                    <li key={id} className="chip-picker-chip">
                        {/* A selected id the current option list does not contain
                            still renders, by its id, so changing region — or losing a
                            library on the server — cannot silently drop a
                            selection. */}
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

            {searching ? (
                <input
                    ref={inputRef}
                    type="text"
                    role="combobox"
                    // The heading above the chips is a span, not a label — it names
                    // the whole control, chips included, not just this input.
                    // Pointing at it is what stops the combobox being announced
                    // unnamed.
                    aria-labelledby={labelId}
                    aria-expanded={showOptions}
                    aria-controls={`${labelId}-options`}
                    aria-autocomplete="list"
                    placeholder={placeholder}
                    value={query}
                    onChange={(e) => {
                        setQuery(e.target.value)
                        setOpen(true)
                    }}
                    onFocus={() => setOpen(true)}
                    onKeyDown={onKeyDown}
                    autoComplete="off"
                />
            ) : (
                // The list variant: everything not already chosen, rendered. No
                // query, no overlay, nothing to reflow — the list is short enough
                // that hiding it behind a search box only hides it.
                <ul className="chip-picker-available" aria-labelledby={labelId}>
                    {options.length === 0 && <li className="chip-picker-none">{noneLabel}</li>}
                    {unselected.map((o) => (
                        <li key={o.id}>
                            <button
                                type="button"
                                className="chip-picker-add"
                                onClick={() => add(o.id)}
                            >
                                {o.name}
                            </button>
                        </li>
                    ))}
                </ul>
            )}

            {showOptions && (
                <ul className="chip-picker-options" id={`${labelId}-options`} role="listbox">
                    {matches.length === 0 && (
                        <li className="chip-picker-none">
                            {options.length === 0 ? noneLabel : 'No matches.'}
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
                        <li className="chip-picker-more" aria-live="polite">
                            {hidden} more — keep typing to narrow.
                        </li>
                    )}
                </ul>
            )}
        </div>
    )
}
