import { createRef } from 'react'
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Card, { type SwipeCardApi, type SwipeDirection } from './Card'
import type { Movie } from '../api'

// These tests cover the parts of Card that do not need a real pointer gesture:
// the imperative handle Swipe.tsx drives, the conditional inner markup, and the
// active class. Dragging itself is deliberately not simulated — a synthesised
// pointer sequence in jsdom exercises the test's own assumptions about motion's
// gesture recogniser rather than the behaviour a user gets, so the drag
// threshold is verified by hand in a browser instead.

const movie: Movie = {
    id: 'm1',
    title: 'The Placeholder',
    year: 1999,
    genres: ['Drama', 'Comedy'],
    overview: 'A film that exists only in this test.',
    runtime: 101,
    communityRating: 7.5,
    officialRating: 'PG',
    posterURL: '/img/m1.jpg',
    availability: [{ source: 'jellyfin', label: 'Jellyfin' }],
}

afterEach(cleanup)

function renderCard(props: Partial<Parameters<typeof Card>[0]> = {}) {
    const ref = createRef<SwipeCardApi>()
    const onSwipe = vi.fn<(dir: SwipeDirection) => void>()
    const { container } = render(
        <Card ref={ref} movie={movie} active onSwipe={onSwipe} {...props} />,
    )
    const card = container.querySelector('.swipe-card') as HTMLElement
    return { ref, onSwipe, card }
}

describe('Card', () => {
    it('renders the movie details', () => {
        renderCard()
        expect(screen.getByRole('heading', { name: /The Placeholder/ })).toBeInTheDocument()
        expect(screen.getByRole('img', { name: 'The Placeholder' })).toHaveAttribute('src', '/img/m1.jpg')
        expect(screen.getByText('Drama, Comedy')).toBeInTheDocument()
        expect(screen.getByText('101 min')).toBeInTheDocument()
    })

    it.each(['right', 'left'] as const)('swipe(%s) through the ref reports that direction', async (dir) => {
        const { ref, onSwipe } = renderCard()
        await act(async () => {
            await ref.current?.swipe(dir)
        })
        expect(onSwipe).toHaveBeenCalledExactlyOnceWith(dir)
    })

    it('restoreCard resolves and leaves the card rendered', async () => {
        const { ref, onSwipe } = renderCard()
        await act(async () => {
            await ref.current?.swipe('left')
        })
        await act(async () => {
            await ref.current?.restoreCard()
        })
        expect(onSwipe).toHaveBeenCalledTimes(1)
        expect(screen.getByRole('heading', { name: /The Placeholder/ })).toBeInTheDocument()
    })

    it('renders the wrapper without inner content when not visible', () => {
        const { card } = renderCard({ visible: false })
        expect(card).toBeInTheDocument()
        expect(card.querySelector('.swipe-card-inner')).toBeNull()
        expect(screen.queryByRole('heading', { name: /The Placeholder/ })).toBeNull()
    })

    it('marks only the active card with swipe-card-active', () => {
        const { card } = renderCard({ active: true })
        expect(card).toHaveClass('swipe-card', 'swipe-card-active')
        cleanup()

        const { card: idle } = renderCard({ active: false })
        expect(idle).toHaveClass('swipe-card')
        expect(idle).not.toHaveClass('swipe-card-active')
    })
})
