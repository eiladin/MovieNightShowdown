import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ChipToggleGroup, { type ChipToggleOption } from './ChipToggleGroup'

const LIBRARIES: ChipToggleOption[] = [
    { id: 'aaa', name: 'Movies' },
    { id: 'bbb', name: 'Kids Movies' },
]

function renderGroup(selected: string[] = [], options = LIBRARIES) {
    const onChange = vi.fn()
    render(
        <ChipToggleGroup
            legend="Libraries"
            options={options}
            selected={selected}
            onChange={onChange}
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

    // A library removed on the server, or saved before this screen could enumerate
    // them, has to stay visible so it can be switched off — otherwise it persists
    // through every later save with nothing on screen admitting it exists.
    it('keeps a selection the option list does not contain, and marks it', async () => {
        const user = userEvent.setup()
        const onChange = renderGroup(['retired'], LIBRARIES)

        const orphan = screen.getByRole('checkbox', { name: /retired/ })
        expect((orphan as HTMLInputElement).checked).toBe(true)
        expect(screen.getByText(/retired — not on this server/)).toBeTruthy()

        await user.click(orphan)
        expect(onChange).toHaveBeenCalledWith([])
    })
})
