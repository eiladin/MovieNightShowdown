import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it } from 'vitest'
import { AppRoutes } from './App'
import {
    configuredSetup,
    filtersResponse,
    installFetchMock,
    jsonResponse,
    unconfiguredSetup,
} from './test/fetchMock'
import { useSessionStore } from './store'

// Smoke tests for the route tree. App itself hardcodes a BrowserRouter, so the
// routes are exported separately as AppRoutes and mounted here under a
// MemoryRouter at a chosen path. Assertions stay on headings, which are the
// stable part of each page.

function renderAt(path: string) {
    return render(
        <MemoryRouter initialEntries={[path]}>
            <AppRoutes />
        </MemoryRouter>,
    )
}

function mockServer(setup = configuredSetup) {
    return installFetchMock({
        '/api/setup': () => jsonResponse(setup),
        '/api/library/filters': () => jsonResponse(filtersResponse()),
    })
}

afterEach(() => {
    cleanup()
    localStorage.clear()
    useSessionStore.getState().reset()
})

describe('routes on a configured deployment', () => {
    it('renders the landing page at /', async () => {
        mockServer()
        renderAt('/')
        expect(await screen.findByRole('heading', { name: /movie night showdown/i })).toBeInTheDocument()
    })

    it('renders the host setup page at /host', async () => {
        mockServer()
        renderAt('/host')
        expect(await screen.findByRole('heading', { name: /start a showdown/i })).toBeInTheDocument()
    })

    it('renders the lobby at /join/:code', async () => {
        mockServer()
        renderAt('/join/abcd')
        // No saved token, so the lobby shows its join form. The code is
        // uppercased from the path param.
        expect(await screen.findByRole('heading', { name: /join session ABCD/i })).toBeInTheDocument()
    })

    it('renders the setup guide at /setup', async () => {
        mockServer()
        renderAt('/setup')
        expect(await screen.findByRole('heading', { name: /finish setting up/i })).toBeInTheDocument()
    })
})

describe('the setup gate', () => {
    it('keeps /setup reachable on an unconfigured deployment', async () => {
        mockServer(unconfiguredSetup)
        renderAt('/setup')
        expect(await screen.findByRole('heading', { name: /finish setting up/i })).toBeInTheDocument()
    })

    it('redirects a gated route to /setup when no source is configured', async () => {
        mockServer(unconfiguredSetup)
        renderAt('/')
        expect(await screen.findByRole('heading', { name: /finish setting up/i })).toBeInTheDocument()
        expect(screen.queryByRole('heading', { name: /movie night showdown/i })).not.toBeInTheDocument()
    })
})
