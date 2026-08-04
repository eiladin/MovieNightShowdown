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
}: ChipToggleGroupProps) {
    function toggle(id: string) {
        onChange(selected.includes(id) ? selected.filter((s) => s !== id) : [...selected, id])
    }

    // A selected id the option list does not contain still renders, so a library
    // removed on the server — or one saved before this screen could enumerate them —
    // can be seen and switched off instead of silently persisting.
    const orphaned = selected.filter((id) => !options.some((o) => o.id === id))

    return (
        <fieldset className="chip-group">
            <legend>{legend}</legend>

            {options.length === 0 && orphaned.length === 0 && (
                <p className="chip-group-note">{noneNote}</p>
            )}

            {options.map((o) => (
                <label
                    key={o.id}
                    className={`chip ${selected.includes(o.id) ? 'checked' : ''}`}
                >
                    <input
                        type="checkbox"
                        className="sr-only"
                        checked={selected.includes(o.id)}
                        onChange={() => toggle(o.id)}
                    />
                    {o.name}
                </label>
            ))}

            {orphaned.map((id) => (
                <label key={id} className="chip checked orphaned">
                    <input
                        type="checkbox"
                        className="sr-only"
                        checked
                        onChange={() => toggle(id)}
                    />
                    {id} — not on this server
                </label>
            ))}

            {emptyNote && selected.length === 0 && (
                <p className="chip-group-note">{emptyNote}</p>
            )}
        </fieldset>
    )
}
