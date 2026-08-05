import '../styles/chip-group.css'

// ChipToggleOption is one selectable thing: an opaque identifier and a name to show.
export interface ChipToggleOption {
    id: string
    name: string
}

interface ChipToggleGroupProps {
    legend: string
    options: ChipToggleOption[]
    selected: string[]
    onChange: (next: string[]) => void
    // emptyNote explains what selecting nothing means, when it means something. For
    // libraries it means "use every one", which is a state worth naming rather than
    // leaving as an empty box.
    emptyNote?: string
    // noneNote is shown when there are no options at all — a different situation with
    // a different fix.
    noneNote: string
    // missingIds are options the source no longer offers, rendered marked so they can
    // be switched off rather than persisting silently.
    //
    // The caller decides, because only the caller knows whether its option list is
    // authoritative. Deriving this here — "selected but not in options" — claimed
    // every saved value was missing whenever the list had simply not been fetched
    // yet, which is the state the screen loads in.
    missingIds?: string[]
}

// ChipToggleGroup is the multi-select control for a handful of options.
//
// It is a fieldset of checkboxes styled as chips, which is the idiom the host screen
// already uses for sources and genres. That matters more than it sounds: the
// alternative here was a chips row plus a list of pills to click, and it read as a
// search-and-chips widget because it looked exactly like the search-and-chips widget
// further down the same page. Checked and unchecked in one group cannot be misread.
//
// Use this when the options are few and all of them can be shown. When there are
// several hundred — TMDB's watch providers — use ChipPicker, which hides them behind
// a query.
export default function ChipToggleGroup({
    legend,
    options,
    selected,
    onChange,
    emptyNote,
    noneNote,
    missingIds = [],
}: ChipToggleGroupProps) {
    function toggle(id: string) {
        onChange(selected.includes(id) ? selected.filter((s) => s !== id) : [...selected, id])
    }

    return (
        <fieldset className="chip-group">
            <legend>{legend}</legend>

            {options.length === 0 && <p className="chip-group-note">{noneNote}</p>}

            {options.map((o) => {
                const missing = missingIds.includes(o.id)
                return (
                    <label
                        key={o.id}
                        className={`chip ${selected.includes(o.id) ? 'checked' : ''} ${
                            missing ? 'orphaned' : ''
                        }`}
                    >
                        <input
                            type="checkbox"
                            className="sr-only"
                            checked={selected.includes(o.id)}
                            onChange={() => toggle(o.id)}
                        />
                        {missing ? `${o.name} — not on this server` : o.name}
                    </label>
                )
            })}

            {emptyNote && selected.length === 0 && (
                <p className="chip-group-note">{emptyNote}</p>
            )}
        </fieldset>
    )
}
