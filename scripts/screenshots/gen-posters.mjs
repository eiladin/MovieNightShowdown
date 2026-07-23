#!/usr/bin/env node
// Generates placeholder poster art for the screenshot pipeline's fixture
// movies (see mock-jellyfin/main.go). Each poster is a 400x600 (2:3) title
// card: a distinct dark gradient background, the movie title large and
// centered, and small year/genre text below. Cards are rendered at 3x device
// scale (1200x1800 output) so they stay crisp on the full-size swipe/result
// cards, then losslessly optimized with oxipng. All titles below are invented;
// none of this artwork represents a real film.
//
// Run with: node scripts/screenshots/gen-posters.mjs

import { chromium } from 'playwright'
import { mkdir } from 'node:fs/promises'
import { execFileSync } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const OUT_DIR = path.join(__dirname, 'fixtures', 'posters')

// Keep in sync with the fixture list in mock-jellyfin/main.go: same IDs
// (used as the output filename), titles, years, and lead genre.
const MOVIES = [
  { id: 'movie-01', title: 'The Last Signal', year: 2019, genre: 'Science Fiction / Thriller' },
  { id: 'movie-02', title: 'Paper Moons', year: 1994, genre: 'Drama / Romance' },
  { id: 'movie-03', title: 'Neon Alley Cats', year: 2021, genre: 'Action / Comedy' },
  { id: 'movie-04', title: 'Whispering Pines', year: 2003, genre: 'Horror / Thriller' },
  { id: 'movie-05', title: "Captain Fizzbucket's Grand Voyage", year: 1998, genre: 'Family / Adventure' },
  { id: 'movie-06', title: 'Midnight Cartographers', year: 2016, genre: 'Mystery / Drama' },
  { id: 'movie-07', title: 'The Clockwork Hearts', year: 2011, genre: 'Romance / Science Fiction' },
  { id: 'movie-08', title: 'Iron Harvest Blues', year: 1992, genre: 'Drama / Western' },
  { id: 'movie-09', title: 'Static & Fury', year: 2023, genre: 'Action / Thriller' },
  { id: 'movie-10', title: 'The Quiet Algorithm', year: 2020, genre: 'Science Fiction / Drama' },
  { id: 'movie-11', title: "Grandma's Rocket", year: 2001, genre: 'Comedy / Family' },
  { id: 'movie-12', title: 'Dust and Starlight', year: 2018, genre: 'Adventure / Drama' },
  { id: 'movie-13', title: 'Bone Orchard Nights', year: 2007, genre: 'Horror' },
  { id: 'movie-14', title: 'The Umbrella Conspiracy', year: 1996, genre: 'Thriller / Mystery' },
  { id: 'movie-15', title: 'Sunday Static', year: 2022, genre: 'Comedy / Drama' },
]

const WIDTH = 400
const HEIGHT = 600

// Deterministic per-poster hue pair spread evenly around the color wheel so
// every card reads as visually distinct.
function gradientFor(index) {
  const hue = (index * 47) % 360
  const hue2 = (hue + 46) % 360
  return `linear-gradient(160deg, hsl(${hue} 55% 16%) 0%, hsl(${hue2} 60% 8%) 100%)`
}

function cardHTML({ title, year, genre }, index) {
  // Font size steps down for longer titles so nothing overflows the card.
  const fontSize = title.length > 28 ? '34px' : title.length > 18 ? '42px' : '52px'
  return `<!doctype html>
<html><head><meta charset="utf-8"><style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  html, body { width: ${WIDTH}px; height: ${HEIGHT}px; overflow: hidden; }
  body {
    background: ${gradientFor(index)};
    font-family: -apple-system, 'Segoe UI', Helvetica, Arial, sans-serif;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 32px;
    position: relative;
  }
  body::after {
    content: '';
    position: absolute;
    inset: 0;
    background:
      radial-gradient(circle at 30% 20%, rgba(255,255,255,0.08), transparent 45%),
      radial-gradient(circle at 80% 85%, rgba(0,0,0,0.35), transparent 55%);
  }
  .title {
    position: relative;
    color: #fff;
    font-size: ${fontSize};
    font-weight: 700;
    text-align: center;
    line-height: 1.15;
    text-shadow: 0 2px 12px rgba(0,0,0,0.55);
    max-width: 320px;
  }
  .meta {
    position: relative;
    margin-top: 22px;
    color: rgba(255,255,255,0.72);
    font-size: 16px;
    letter-spacing: 0.04em;
    text-align: center;
  }
  .meta .year { font-weight: 600; color: rgba(255,255,255,0.9); }
  .frame {
    position: absolute;
    inset: 14px;
    border: 1px solid rgba(255,255,255,0.14);
    border-radius: 6px;
  }
</style></head>
<body>
  <div class="frame"></div>
  <div class="title">${escapeHTML(title)}</div>
  <div class="meta"><span class="year">${year}</span> &middot; ${escapeHTML(genre)}</div>
</body></html>`
}

function escapeHTML(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

async function main() {
  await mkdir(OUT_DIR, { recursive: true })

  const browser = await chromium.launch()
  try {
    // Render at 3x so posters stay crisp when the swipe/result card upscales
    // them to full width; the admin grid uses the same assets at small size.
    const page = await browser.newPage({ viewport: { width: WIDTH, height: HEIGHT }, deviceScaleFactor: 3 })
    for (const [index, movie] of MOVIES.entries()) {
      await page.setContent(cardHTML(movie, index), { waitUntil: 'load' })
      const outPath = path.join(OUT_DIR, `${movie.id}.png`)
      await page.screenshot({ path: outPath })
      console.log(`wrote ${path.relative(process.cwd(), outPath)}`)
    }
  } finally {
    await browser.close()
  }

  // Losslessly shrink the fixtures so they don't bloat the repo. oxipng is
  // the same optimizer run.sh uses on the final screenshots; skip gracefully
  // if it is not installed.
  try {
    execFileSync('oxipng', ['-o', 'max', '--strip', 'safe', ...MOVIES.map((m) => path.join(OUT_DIR, `${m.id}.png`))], {
      stdio: 'inherit',
    })
  } catch (err) {
    console.warn('oxipng not run (is it installed?):', err.message)
  }

  console.log(`done: ${MOVIES.length} posters in ${path.relative(process.cwd(), OUT_DIR)}`)
}

main().catch((err) => {
  console.error('gen-posters failed:', err)
  process.exit(1)
})
