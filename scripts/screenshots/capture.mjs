#!/usr/bin/env node
// Playwright driver for the README screenshot pipeline. Drives the real app
// (server/ + web/) against the mock Jellyfin server (see
// mock-jellyfin/main.go) through the actual user flow: landing -> start a
// session -> lobby -> swipe -> match. All WS-driven screen transitions are
// awaited via real signals (selectors/text), never fixed sleeps.
//
// Run with: node capture.mjs
// (invoked by run.sh, which also starts the app + mock server first)

import { chromium, devices } from 'playwright'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const REPO_ROOT = path.resolve(__dirname, '..', '..')
const OUT_DIR = path.join(REPO_ROOT, 'docs', 'screenshots')

const BASE_URL = process.env.CAPTURE_BASE_URL || 'http://localhost:8080'
// BARE_URL serves an app with no sources configured, which is the only state the
// /setup guide describes; see run.sh.
const BARE_URL = process.env.CAPTURE_BARE_URL || 'http://localhost:8081'
// The settings screen is behind the setup token, which the server prints to its log
// on first start. run.sh reads it from there and passes it in.
const SETUP_TOKEN = process.env.CAPTURE_SETUP_TOKEN || ''
const STEP_TIMEOUT = 15_000
const HOST_NAME = 'Alex'

function outPath(name) {
    return path.join(OUT_DIR, name)
}

// step wraps one capture step with a clear label so a timeout/assertion
// failure says exactly which screen it happened on.
async function step(label, fn) {
    try {
        await fn()
        console.log(`ok: ${label}`)
    } catch (err) {
        console.error(`FAILED at step "${label}": ${err.message || err}`)
        throw err
    }
}

// startShowdown drives Landing -> createSession -> /host?code=... as the
// host, common to both passes.
async function startShowdown(page) {
    await step('goto landing', async () => {
        await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded' })
        await page.waitForSelector('.landing-panel', { timeout: STEP_TIMEOUT })
        await page.getByPlaceholder('Your name').waitFor({ timeout: STEP_TIMEOUT })
    })

    return page
}

async function fillAndStart(page) {
    await step('start a showdown as host', async () => {
        await page.getByPlaceholder('Your name').fill(HOST_NAME)
        await page.getByRole('button', { name: 'Start a Showdown' }).click()
        await page.waitForURL(/\/host\?code=/, { timeout: STEP_TIMEOUT })
    })
}

async function goToLobby(page) {
    await step('go to the lobby', async () => {
        await page.getByRole('link', { name: 'Go to the Lobby →' }).click()
        await page.waitForURL(/\/join\//, { timeout: STEP_TIMEOUT })
        await page.waitForSelector('.qr-join-code', { timeout: STEP_TIMEOUT })
        await page.getByText(`${HOST_NAME} (host)`).waitFor({ timeout: STEP_TIMEOUT })
    })
}

async function main() {
    const browser = await chromium.launch()

    try {
        await runPass1(browser)
        await runPass2(browser)
        await runPass3(browser)
    } finally {
        await browser.close()
    }

    console.log('capture complete')
}

// --- Pass 1: 01-landing, 03-lobby, 04-swipe, 05-result ---
// deviceScaleFactor 3, iPhone-class mobile UA/viewport, dark mode.
async function runPass1(browser) {
    const context = await browser.newContext({
        ...devices['iPhone 12 Pro'],
        // Playwright's iPhone 12 Pro preset reserves space for mobile Safari's
        // chrome (viewport 390x664); we want the full device viewport (390x844)
        // since this app isn't rendered inside a real mobile browser frame.
        viewport: { width: 390, height: 844 },
        colorScheme: 'dark',
    })
    const page = await context.newPage()
    page.setDefaultTimeout(STEP_TIMEOUT)

    try {
        await startShowdown(page)

        await step('capture 01-landing', async () => {
            await page.screenshot({ path: outPath('01-landing.png') })
        })

        await fillAndStart(page)
        await goToLobby(page)

        await step('capture 03-lobby', async () => {
            await page.screenshot({ path: outPath('03-lobby.png') })
        })

        await step('begin the session', async () => {
            // "Required to agree" defaults to the current roster size (1, just the
            // host), which is exactly what we want for an immediate match.
            const required = page.getByLabel('Required to agree')
            await required.waitFor()
            await required.fill('1')
            await page.getByRole('button', { name: 'Begin' }).click()
            await page.waitForSelector('.swipe-deck .swipe-card', { timeout: STEP_TIMEOUT })
        })

        await step('capture 04-swipe', async () => {
            await page.screenshot({ path: outPath('04-swipe.png') })
        })

        await step('swipe yes to force a match', async () => {
            await page.getByRole('button', { name: 'Yes', exact: true }).click()
            await page.waitForSelector('.result-screen', { timeout: STEP_TIMEOUT })
            await page.getByText("It's a match!").waitFor({ timeout: STEP_TIMEOUT })
            await page.waitForSelector('.result-home-btn', { timeout: STEP_TIMEOUT })
        })

        await step('capture 05-result', async () => {
            // The confetti (canvas-confetti) fires its first burst 250ms after the
            // result screen mounts and repeats every 250ms. There is no state signal
            // to await, so a short fixed delay is the correct tool here: it lets a
            // few bursts accumulate on screen so the celebratory splash is captured.
            await page.waitForTimeout(800)
            await page.screenshot({ path: outPath('05-result.png') })
        })
    } finally {
        await context.close()
    }
}

// --- Pass 2: 02-host (filtered preview, full-page) ---
// deviceScaleFactor 2, same mobile UA/viewport, dark mode.
async function runPass2(browser) {
    const context = await browser.newContext({
        ...devices['iPhone 12 Pro'],
        viewport: { width: 390, height: 844 },
        deviceScaleFactor: 2,
        colorScheme: 'dark',
    })
    const page = await context.newPage()
    page.setDefaultTimeout(STEP_TIMEOUT)

    try {
        await startShowdown(page)
        await fillAndStart(page)

        await step('pick filters on the host screen', async () => {
            const genreGroup = page.locator('fieldset.chip-group', { has: page.locator('legend', { hasText: 'Genres' }) })
            for (const genre of ['Action', 'Adventure', 'Comedy']) {
                await genreGroup.getByText(genre, { exact: true }).click()
            }
            const ratingGroup = page.locator('fieldset.chip-group', {
                has: page.locator('legend', { hasText: 'Parental Rating' }),
            })
            await ratingGroup.getByText('PG-13', { exact: true }).click()
        })

        await step('preview the filtered library', async () => {
            await page.getByRole('button', { name: 'Preview' }).click()
            await page.waitForSelector('.poster-grid', { timeout: STEP_TIMEOUT })
            await page.getByText(/movies match/).waitFor({ timeout: STEP_TIMEOUT })
            await page.waitForLoadState('networkidle', { timeout: STEP_TIMEOUT })
        })

        await step('capture 02-host (full page)', async () => {
            await page.screenshot({ path: outPath('02-host.png'), fullPage: true })
        })
    } finally {
        await context.close()
    }
}

// --- Pass 3: 06-setup, 07-settings (operator screens, desktop) ---
//
// Desktop rather than the mobile viewport the other passes use, and deliberately so:
// these two are configured from a laptop, the settings screen is 48rem wide, and at
// 390px it renders as a very tall column with its read-only Container list collapsed
// to one field per row. Nobody sets this up from a phone.
async function runPass3(browser) {
    const context = await browser.newContext({
        viewport: { width: 1100, height: 900 },
        deviceScaleFactor: 2,
        colorScheme: 'dark',
    })
    const page = await context.newPage()
    page.setDefaultTimeout(STEP_TIMEOUT)

    try {
        await step('capture 06-setup (unconfigured app, full page)', async () => {
            await page.goto(BARE_URL + '/setup', { waitUntil: 'domcontentloaded' })
            await page.waitForSelector('.setup-page', { timeout: STEP_TIMEOUT })
            // The status list is fetched, so wait for it rather than for the shell.
            await page.locator('.setup-status').waitFor({ timeout: STEP_TIMEOUT })
            await page.screenshot({ path: outPath('06-setup.png'), fullPage: true })
        })

        await step('unlock the settings screen', async () => {
            if (!SETUP_TOKEN) {
                throw new Error('CAPTURE_SETUP_TOKEN is empty; run.sh reads it from the app log')
            }
            await page.goto(BASE_URL + '/settings', { waitUntil: 'domcontentloaded' })
            await page.getByLabel('Setup token').fill(SETUP_TOKEN)
            await page.getByRole('button', { name: 'Continue' }).click()
            // Past the gate. Waiting for the save button rather than a heading, since
            // the gate has headings of its own.
            await page.getByRole('button', { name: 'Save settings' }).waitFor({
                timeout: STEP_TIMEOUT,
            })
        })

        await step('check the connection so the discovered fields appear', async () => {
            // The account and library pickers are not rendered until a check has
            // enumerated the options — the identifiers are opaque, so the screen never
            // invites anyone to type one. Capturing before the check would document a
            // screen with half its controls missing.
            await page.getByRole('button', { name: 'Check connection' }).first().click()
            await page.getByText(/Connected to Movie Closet/).waitFor({ timeout: STEP_TIMEOUT })
            await page.locator('fieldset.chip-group').first().waitFor({ timeout: STEP_TIMEOUT })
        })

        await step('choose a library so the control shows both states', async () => {
            // One checked and one not, so the screenshot shows what a selection looks
            // like rather than an untouched group.
            await page
                .locator('fieldset.chip-group .chip', { hasText: 'Family Movies' })
                .first()
                .click()
        })

        await step('capture 07-settings (full page)', async () => {
            // The chip fill transitions over 0.2s. Reading the page before it settles
            // captures the unchecked colour, which is the state the shot exists to
            // contrast against.
            await page.waitForTimeout(500)
            await page.screenshot({ path: outPath('07-settings.png'), fullPage: true })
        })
    } finally {
        await context.close()
    }
}

main().catch((err) => {
    console.error('capture.mjs failed:', err)
    process.exit(1)
})
