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
const STEP_TIMEOUT = 15_000
const ADMIN_NAME = 'Alex'

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

// startShowdown drives Landing -> createSession -> /admin?code=... as the
// admin, common to both passes.
async function startShowdown(page) {
  await step('goto landing', async () => {
    await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded' })
    await page.waitForSelector('.landing-panel', { timeout: STEP_TIMEOUT })
    await page.getByPlaceholder('Your name').waitFor({ timeout: STEP_TIMEOUT })
  })

  return page
}

async function fillAndStart(page) {
  await step('start a showdown as admin', async () => {
    await page.getByPlaceholder('Your name').fill(ADMIN_NAME)
    await page.getByRole('button', { name: 'Start a Showdown' }).click()
    await page.waitForURL(/\/admin\?code=/, { timeout: STEP_TIMEOUT })
  })
}

async function goToLobby(page) {
  await step('go to the lobby', async () => {
    await page.getByRole('link', { name: 'Go to the Lobby →' }).click()
    await page.waitForURL(/\/join\//, { timeout: STEP_TIMEOUT })
    await page.waitForSelector('.qr-join-code', { timeout: STEP_TIMEOUT })
    await page.getByText(`${ADMIN_NAME} (admin)`).waitFor({ timeout: STEP_TIMEOUT })
  })
}

async function main() {
  const browser = await chromium.launch()

  try {
    await runPass1(browser)
    await runPass2(browser)
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
      // admin), which is exactly what we want for an immediate match.
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

// --- Pass 2: 02-admin (filtered preview, full-page) ---
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

    await step('pick filters on the admin screen', async () => {
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

    await step('capture 02-admin (full page)', async () => {
      await page.screenshot({ path: outPath('02-admin.png'), fullPage: true })
    })
  } finally {
    await context.close()
  }
}

main().catch((err) => {
  console.error('capture.mjs failed:', err)
  process.exit(1)
})
