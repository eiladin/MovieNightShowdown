import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ChipToggleGroup, { type ChipToggleOption } from './ChipToggleGroup'

const LIBRARIES: ChipToggleOption[] = [
    { id: 'aaa', name: 'Movies' },
    { id: 'bbb', name: 'Kids Movies' },
]

function renderGroup(selected: string[] = [], options = LIBRARIES, missingIds: string[] = []) {
    const onChange = vi.fn()
    render(
        <ChipToggleGroup
            legend="Libraries"
            options={options}
            selected={selected}
            onChange={onChange}
            missingIds={missingIds}
            emptyNote="None chosen, so every library is used."
            noneNote="No movie libraries were found on this server."
        />,
    )
    return onChange
}

describe('ChipToggleGroup', () => {
    afterEach(cleanup)

    // The reason this exists rather than a search box: a media server has a handful
    // of libraries, so every one of them is on screen with nothing typed.
    it('renders every option as a toggle, with no search field', () => {
        renderGroup()

        expect(screen.queryByRole('combobox')).toBeNull()
        expect(screen.queryByRole('textbox')).toBeNull()
        for (const o of LIBRARIES) {
            expect(screen.getByRole('checkbox', { name: o.name })).toBeTruthy()
        }
    })

    // Checked and unchecked are the same control in two states, which is what stops
    // the group being misread as a row of things already chosen.
    it('reports each option as checked or not', () => {
        renderGroup(['aaa'])

        expect((screen.getByRole('checkbox', { name: 'Movies' }) as HTMLInputElement).checked).toBe(
            true,
        )
        expect(
            (screen.getByRole('checkbox', { name: 'Kids Movies' }) as HTMLInputElement).checked,
        ).toBe(false)
    })

    it('selects an option', async () => {
        const user = userEvent.setup()
        const onChange = renderGroup()

        await user.click(screen.getByRole('checkbox', { name: 'Kids Movies' }))

        expect(onChange).toHaveBeenCalledWith(['bbb'])
    })

    it('deselects an option', async () => {
        const user = userEvent.setup()
        const onChange = renderGroup(['aaa', 'bbb'])

        await user.click(screen.getByRole('checkbox', { name: 'Movies' }))

        expect(onChange).toHaveBeenCalledWith(['bbb'])
    })

    // Selecting nothing means "use every library", which is a state worth naming
    // rather than leaving as an empty box.
    it('says what selecting nothing means', () => {
        renderGroup()

        expect(screen.getByText('None chosen, so every library is used.')).toBeTruthy()
    })

    it('drops that note once something is selected', () => {
        renderGroup(['aaa'])

        expect(screen.queryByText('None chosen, so every library is used.')).toBeNull()
    })

    it('says so when the server offered nothing', () => {
        renderGroup([], [])

        expect(screen.getByText('No movie libraries were found on this server.')).toBeTruthy()
    })

    // A library the source no longer offers is marked, so it can be switched off
    // rather than persisting through every later save with nothing admitting it
    // exists. The caller says which — this component never guesses.
    it('marks an option the caller reported missing', async () => {
        const user = userEvent.setup()
        const onChange = renderGroup(
            ['retired'],
            [...LIBRARIES, { id: 'retired', name: 'Retired Films' }],
            ['retired'],
        )

        const orphan = screen.getByRole('checkbox', { name: /Retired Films/ })
        expect((orphan as HTMLInputElement).checked).toBe(true)
        expect(screen.getByText(/Retired Films — not on this server/)).toBeTruthy()

        await user.click(orphan)
        expect(onChange).toHaveBeenCalledWith([])
    })

    // Nothing is marked unless the caller says so. Deriving it here from "selected but
    // not in options" claimed every saved value was missing while the option list had
    // simply not been fetched — which is the state the settings screen loads in.
    it('marks nothing when the caller reports nothing missing', () => {
        renderGroup(['aaa', 'bbb'])

        expect(screen.queryByText(/not on this server/)).toBeNull()
        expect(document.querySelectorAll('.chip.orphaned')).toHaveLength(0)
    })
})
