import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ChipPicker, { type ChipOption } from './ChipPicker'

const OPTIONS: ChipOption[] = [
    { id: 'netflix', name: 'Netflix' },
    { id: 'prime', name: 'Amazon Prime Video' },
    { id: 'disney', name: 'Disney Plus' },
    { id: 'apple', name: 'Apple TV+' },
]

function renderPicker(selected: string[] = [], options = OPTIONS) {
    const onChange = vi.fn()
    render(
        <ChipPicker
            options={options}
            selected={selected}
            onChange={onChange}
            labelId="services"
            placeholder="Type to find a service…"
            emptyLabel="No services selected yet."
            noneLabel="No services were returned for this region."
        />,
    )
    return onChange
}

// renderList is the list variant: every unselected option rendered, no search box.
// It is what a handful of options wants — a media server has three or four movie
// libraries, and hiding those behind a query only hides them.
function renderList(selected: string[] = [], options = OPTIONS) {
    const onChange = vi.fn()
    render(
        <ChipPicker
            variant="list"
            options={options}
            selected={selected}
            onChange={onChange}
            labelId="libraries"
            emptyLabel="Every library."
            noneLabel="No movie libraries were found on this server."
        />,
    )
    return onChange
}

describe('ChipPicker, list variant', () => {
    afterEach(cleanup)

    it('renders every option with nothing typed', () => {
        renderList()

        expect(screen.queryByRole('combobox')).toBeNull()
        for (const o of OPTIONS) {
            expect(screen.getByRole('button', { name: o.name })).toBeTruthy()
        }
    })

    it('adds an option and removes it from the available list', async () => {
        const user = userEvent.setup()
        const onChange = renderList()

        await user.click(screen.getByRole('button', { name: 'Netflix' }))

        expect(onChange).toHaveBeenCalledWith(['netflix'])
    })

    // Selecting something must not leave it offered, or it could be chosen twice.
    it('does not offer a selected option', () => {
        renderList(['netflix'])

        expect(screen.queryByRole('button', { name: 'Netflix' })).toBeNull()
        expect(screen.getByRole('button', { name: 'Remove Netflix' })).toBeTruthy()
        expect(screen.getByRole('button', { name: 'Apple TV+' })).toBeTruthy()
    })

    it('removes a selected option and offers it again', async () => {
        const user = userEvent.setup()
        const onChange = renderList(['netflix', 'apple'])

        await user.click(screen.getByRole('button', { name: 'Remove Netflix' }))

        expect(onChange).toHaveBeenCalledWith(['apple'])
    })

    it('says so when nothing is selected', () => {
        renderList()

        expect(screen.getByText('Every library.')).toBeTruthy()
    })

    it('says so when the server returned nothing', () => {
        renderList([], [])

        expect(screen.getByText('No movie libraries were found on this server.')).toBeTruthy()
    })

    // A selected id the option list does not contain still renders, so a library
    // removed on the server can still be seen and unselected.
    it('keeps a selection the option list does not contain', () => {
        renderList(['retired'], [{ id: 'netflix', name: 'Netflix' }])

        expect(screen.getByText('retired')).toBeTruthy()
        expect(screen.getByRole('button', { name: 'Remove retired' })).toBeTruthy()
    })
})

describe('ChipPicker, search variant', () => {
    afterEach(cleanup)

    it('shows only the selection until the search is used', () => {
        renderPicker(['netflix'])

        expect(screen.getByText('Netflix')).toBeTruthy()
        // The point of the component: several hundred options must not be on
        // screen by default.
        expect(screen.queryByRole('listbox')).toBeNull()
        expect(screen.queryByText('Apple TV+')).toBeNull()
    })

    it('filters as you type', async () => {
        const user = userEvent.setup()
        renderPicker()

        await user.type(screen.getByRole('combobox'), 'dis')
        expect(screen.getByRole('option', { name: 'Disney Plus' })).toBeTruthy()
        expect(screen.queryByRole('option', { name: 'Netflix' })).toBeNull()
    })

    it('adds a service and clears the search', async () => {
        const user = userEvent.setup()
        const onChange = renderPicker(['netflix'])

        await user.type(screen.getByRole('combobox'), 'apple')
        await user.click(screen.getByRole('option', { name: 'Apple TV+' }))

        expect(onChange).toHaveBeenCalledWith(['netflix', 'apple'])
        expect((screen.getByRole('combobox') as HTMLInputElement).value).toBe('')
    })

    it('does not offer a service that is already selected', async () => {
        const user = userEvent.setup()
        renderPicker(['netflix'])

        await user.type(screen.getByRole('combobox'), 'net')
        expect(screen.queryByRole('option', { name: 'Netflix' })).toBeNull()
    })

    it('removes a selected service', async () => {
        const user = userEvent.setup()
        const onChange = renderPicker(['netflix', 'apple'])

        await user.click(screen.getByRole('button', { name: 'Remove Netflix' }))
        expect(onChange).toHaveBeenCalledWith(['apple'])
    })

    it('keeps a selection the current region does not list', () => {
        // Changing region changes the option list. A saved service missing from
        // it must still render and be removable, or saving would silently drop
        // it.
        renderPicker(['some-regional-service'], [{ id: 'netflix', name: 'Netflix' }])

        expect(screen.getByText('some-regional-service')).toBeTruthy()
        expect(screen.getByRole('button', { name: 'Remove some-regional-service' })).toBeTruthy()
    })

    it('selects the highlighted option with the keyboard', async () => {
        const user = userEvent.setup()
        const onChange = renderPicker()

        await user.type(screen.getByRole('combobox'), 'apple')
        await user.keyboard('{ArrowDown}{Enter}')

        // Enter must choose an option rather than submitting the settings form
        // the picker sits inside.
        expect(onChange).toHaveBeenCalledTimes(1)
        expect(onChange).toHaveBeenCalledWith(['apple'])
    })

    it('closes the results on Escape', async () => {
        const user = userEvent.setup()
        renderPicker()

        await user.type(screen.getByRole('combobox'), 'net')
        expect(screen.getByRole('listbox')).toBeTruthy()

        await user.keyboard('{Escape}')
        expect(screen.queryByRole('listbox')).toBeNull()
    })

    it('says so when a region returns nothing', async () => {
        const user = userEvent.setup()
        renderPicker([], [])

        await user.type(screen.getByRole('combobox'), 'net')
        expect(screen.getByText('No services were returned for this region.')).toBeTruthy()
    })

    // Focus alone used to open the overlay, which meant clicking into the field
    // covered the rest of the form with an unfiltered list of every service TMDB
    // returns. Nobody reads that list; it is the problem the search box exists to
    // solve.
    it('shows no results on focus alone', async () => {
        const user = userEvent.setup()
        renderPicker()

        await user.click(screen.getByRole('combobox'))

        expect(screen.queryByRole('listbox')).toBeNull()
    })

    // Backspacing to an empty field has to close the overlay, not fall back to
    // showing everything.
    it('hides the results again when the query is cleared', async () => {
        const user = userEvent.setup()
        renderPicker()

        const input = screen.getByRole('combobox')
        await user.type(input, 'net')
        expect(screen.getByRole('listbox')).toBeTruthy()

        await user.clear(input)

        expect(screen.queryByRole('listbox')).toBeNull()
    })

    // A region can match dozens of services on a two-letter query. The overlay
    // caps what it renders, and counts the rest rather than dropping them
    // silently — a truncated list that looks complete is worse than a short one
    // that admits it.
    it('caps the results and reports how many are hidden', async () => {
        const user = userEvent.setup()
        const many: ChipOption[] = Array.from({ length: 9 }, (_, i) => ({
            id: `channel-${i}`,
            name: `Channel ${i}`,
        }))
        renderPicker([], many)

        await user.type(screen.getByRole('combobox'), 'channel')

        expect(screen.getAllByRole('option')).toHaveLength(5)
        expect(screen.getByText('4 more — keep typing to narrow.')).toBeTruthy()
    })

    // Narrowing to a single match must leave no remainder note behind.
    it('drops the remainder note once everything fits', async () => {
        const user = userEvent.setup()
        renderPicker()

        await user.type(screen.getByRole('combobox'), 'net')

        expect(screen.getAllByRole('option')).toHaveLength(1)
        expect(screen.queryByText(/more — keep typing/)).toBeNull()
    })
})
