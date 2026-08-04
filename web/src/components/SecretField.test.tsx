import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SecretField from './SecretField'

const MARKER = '••••••••'

function renderField(value: string, storedMarker?: string) {
    const onChange = vi.fn()
    render(
        <SecretField id="secret" value={value} onChange={onChange} storedMarker={storedMarker} />,
    )
    return onChange
}

const field = () => document.getElementById('secret') as HTMLInputElement
const toggle = () => screen.queryByRole('button')

describe('SecretField', () => {
    afterEach(cleanup)

    it('masks the value until the toggle is used', async () => {
        const user = userEvent.setup()
        renderField('tmdb-v4-token')

        expect(field().type).toBe('password')

        await user.click(screen.getByRole('button', { name: 'Show the value' }))

        expect(field().type).toBe('text')
        expect(field().value).toBe('tmdb-v4-token')
    })

    // A toggle, not press-and-hold: checking a pasted credential against its
    // source means reading both, which takes longer than a button can be held.
    it('stays revealed until it is toggled back', async () => {
        const user = userEvent.setup()
        renderField('tmdb-v4-token')

        await user.click(screen.getByRole('button', { name: 'Show the value' }))
        // A click is a press and a release; the release must not re-mask it.
        expect(field().type).toBe('text')

        await user.click(screen.getByRole('button', { name: 'Hide the value' }))
        expect(field().type).toBe('password')
    })

    it('reports its state rather than just its next action', async () => {
        const user = userEvent.setup()
        renderField('tmdb-v4-token')

        expect(toggle()?.getAttribute('aria-pressed')).toBe('false')
        await user.click(screen.getByRole('button', { name: 'Show the value' }))
        expect(toggle()?.getAttribute('aria-pressed')).toBe('true')
    })

    // The marker is not a masked secret — the server never sends a stored
    // credential back. Revealing it would show the marker's bullet characters and
    // read as a bug, so the toggle is withheld until there is something real.
    it('offers no toggle while the field only holds the stored marker', () => {
        renderField(MARKER, MARKER)

        expect(field().type).toBe('password')
        expect(toggle()).toBeNull()
    })

    it('offers no toggle for an empty field', () => {
        renderField('', MARKER)

        expect(toggle()).toBeNull()
    })

    it('offers the toggle once a value is typed over the marker', async () => {
        const user = userEvent.setup()
        const onChange = vi.fn()
        const { rerender } = render(
            <SecretField id="secret" value={MARKER} onChange={onChange} storedMarker={MARKER} />,
        )
        expect(toggle()).toBeNull()

        // The parent owns the value, so simulate what it does on an edit.
        rerender(
            <SecretField id="secret" value="typed" onChange={onChange} storedMarker={MARKER} />,
        )

        await user.click(screen.getByRole('button', { name: 'Show the value' }))
        expect(field().type).toBe('text')
    })

    // Revealing and then clearing must not leave the next value readable.
    it('re-masks when the value goes back to the marker', async () => {
        const user = userEvent.setup()
        const onChange = vi.fn()
        const { rerender } = render(
            <SecretField id="secret" value="typed" onChange={onChange} storedMarker={MARKER} />,
        )
        await user.click(screen.getByRole('button', { name: 'Show the value' }))
        expect(field().type).toBe('text')

        rerender(
            <SecretField id="secret" value={MARKER} onChange={onChange} storedMarker={MARKER} />,
        )

        expect(field().type).toBe('password')
        expect(toggle()).toBeNull()
    })

    it('passes typed characters up unchanged', async () => {
        const user = userEvent.setup()
        const onChange = renderField('')

        await user.type(field(), 'a')

        expect(onChange).toHaveBeenCalledWith('a')
    })

    it('applies the password-manager opt-outs to the input, not the wrapper', () => {
        render(
            <SecretField
                id="secret"
                value="x"
                onChange={vi.fn()}
                inputProps={{ 'data-lpignore': 'true' }}
            />,
        )

        expect(field().getAttribute('data-lpignore')).toBe('true')
    })
})
