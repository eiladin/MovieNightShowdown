#!/usr/bin/env node
// Generates placeholder poster art for the screenshot pipeline's fixture
// movies (see mock-jellyfin/main.go). Each of the 15 posters is a bespoke
// 400x600 (2:3) one-sheet assigned to one of five visual styles (synthwave,
// cartoon, vintage print, minimalist, painterly), each with its own subject
// and typography, built from procedural SVG primitives (skies, silhouettes,
// starfields, halftone/scanline textures) with per-style grain and vignette.
// A seeded PRNG keeps every run deterministic. Cards are rendered at 3x
// device scale (1200x1800 output) so they stay crisp on the full-size
// swipe/result cards, then losslessly optimized with oxipng. All titles and
// artwork are invented; none of this represents a real film.
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
// (used as the output filename), titles, years, and genres.
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

// --- deterministic PRNG (mulberry32) -------------------------------------
function mulberry32(seed) {
    let a = seed >>> 0
    return function () {
        a = (a + 0x6d2b79f5) | 0
        let t = Math.imul(a ^ (a >>> 15), 1 | a)
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296
    }
}
const rand = (rng, lo, hi) => lo + rng() * (hi - lo)

// --- shared SVG primitives (400x600 coordinate space) --------------------
function stars(rng, n, maxY) {
    let s = ''
    for (let i = 0; i < n; i++) {
        const x = rand(rng, 0, WIDTH).toFixed(1)
        const y = rand(rng, 0, maxY).toFixed(1)
        const r = rand(rng, 0.3, 1.6).toFixed(2)
        const o = rand(rng, 0.25, 0.95).toFixed(2)
        s += `<circle cx="${x}" cy="${y}" r="${r}" fill="#fff" opacity="${o}"/>`
    }
    return s
}

// A ragged mountain/hill ridge filling from baseY down to the bottom.
function ridge(rng, baseY, amp, fill, steps = 7) {
    const pts = []
    for (let i = 0; i <= steps; i++) {
        const x = ((WIDTH / steps) * i).toFixed(1)
        const y = (baseY - rand(rng, 0, amp)).toFixed(1)
        pts.push(`${x},${y}`)
    }
    return `<polygon points="0,${HEIGHT} ${pts.join(' ')} ${WIDTH},${HEIGHT}" fill="${fill}"/>`
}

// A distant lone figure; feet at (x, y), total height h.
function figure(x, y, h, fill, opacity = 1) {
    const headR = h * 0.13
    const headCy = y - h + headR
    const shoulder = headCy + headR * 1.6
    const w = h * 0.34
    return `<g fill="${fill}" opacity="${opacity}">
    <circle cx="${x}" cy="${headCy.toFixed(1)}" r="${headR.toFixed(1)}"/>
    <path d="M ${(x - w * 0.5).toFixed(1)} ${y.toFixed(1)}
      C ${(x - w * 0.5).toFixed(1)} ${shoulder.toFixed(1)}, ${(x - w * 0.4).toFixed(1)} ${shoulder.toFixed(1)}, ${x} ${shoulder.toFixed(1)}
      C ${(x + w * 0.4).toFixed(1)} ${shoulder.toFixed(1)}, ${(x + w * 0.5).toFixed(1)} ${shoulder.toFixed(1)}, ${(x + w * 0.5).toFixed(1)} ${y.toFixed(1)} Z"/>
  </g>`
}

// A bare, forking tree; trunk base at (x, y), height h.
function deadTree(rng, x, y, h, fill) {
    const top = y - h
    let branches = ''
    const forks = Math.floor(rand(rng, 3, 6))
    for (let i = 0; i < forks; i++) {
        const by = rand(rng, top, y - h * 0.3)
        const dir = rng() < 0.5 ? -1 : 1
        const len = rand(rng, h * 0.16, h * 0.32)
        const bx = x + dir * rand(rng, 2, 6)
        branches += `<line x1="${bx.toFixed(1)}" y1="${by.toFixed(1)}" x2="${(bx + dir * len).toFixed(1)}" y2="${(by - len * 0.7).toFixed(1)}" stroke="${fill}" stroke-width="${rand(rng, 1, 2.4).toFixed(1)}" stroke-linecap="round"/>`
    }
    return `<g>
    <line x1="${x}" y1="${y}" x2="${x}" y2="${top.toFixed(1)}" stroke="${fill}" stroke-width="${(h * 0.03 + 2).toFixed(1)}" stroke-linecap="round"/>
    ${branches}
  </g>`
}

function saguaro(x, y, h, fill) {
    const w = h * 0.16
    const armY = y - h * 0.55
    return `<g fill="${fill}">
    <rect x="${(x - w / 2).toFixed(1)}" y="${(y - h).toFixed(1)}" width="${w.toFixed(1)}" height="${h.toFixed(1)}" rx="${(w / 2).toFixed(1)}"/>
    <path d="M ${x} ${armY.toFixed(1)} q ${(-h * 0.22).toFixed(1)} 0 ${(-h * 0.22).toFixed(1)} ${(h * 0.22).toFixed(1)}" fill="none" stroke="${fill}" stroke-width="${w.toFixed(1)}" stroke-linecap="round"/>
    <path d="M ${x} ${(armY + h * 0.12).toFixed(1)} q ${(h * 0.2).toFixed(1)} 0 ${(h * 0.2).toFixed(1)} ${(h * 0.2).toFixed(1)}" fill="none" stroke="${fill}" stroke-width="${w.toFixed(1)}" stroke-linecap="round"/>
  </g>`
}

// A stacked-triangle conifer; trunk base at (x, yBase), height h.
function pineTree(x, yBase, h, fill, tiers = 3) {
    const trunkH = h * 0.08
    const trunkW = h * 0.05
    const tierH = (h - trunkH) / tiers
    let tiersSVG = ''
    for (let i = 0; i < tiers; i++) {
        const topY = yBase - trunkH - tierH * (tiers - i) - i * tierH * 0.1
        const botY = yBase - trunkH - tierH * (tiers - i - 1) + tierH * 0.28
        const w = h * 0.46 * (1 - i * 0.22)
        tiersSVG += `<polygon points="${x.toFixed(1)},${topY.toFixed(1)} ${(x - w / 2).toFixed(1)},${botY.toFixed(1)} ${(x + w / 2).toFixed(1)},${botY.toFixed(1)}" fill="${fill}"/>`
    }
    return `<g>${tiersSVG}<rect x="${(x - trunkW / 2).toFixed(1)}" y="${(yBase - trunkH).toFixed(1)}" width="${trunkW.toFixed(1)}" height="${trunkH.toFixed(1)}" fill="${fill}"/></g>`
}

// A toothed cog; centered at (cx, cy).
function gear(cx, cy, rOuter, rInner, teeth, fill) {
    const step = (Math.PI * 2) / (teeth * 2)
    const pts = []
    for (let i = 0; i < teeth * 2; i++) {
        const r = i % 2 === 0 ? rOuter : rInner
        const a = i * step - Math.PI / 2
        pts.push(`${(cx + Math.cos(a) * r).toFixed(1)},${(cy + Math.sin(a) * r).toFixed(1)}`)
    }
    return `<polygon points="${pts.join(' ')}" fill="${fill}"/>`
}

// A finned rocket; nose tip at (cx, topY), total height h. Optional outline
// for the cartoon thick-stroke look.
function rocket(cx, topY, h, bodyFill, accent, outline = null) {
    const w = h * 0.34
    const noseH = h * 0.22
    const bodyH = h * 0.62
    const finH = h * 0.2
    const bodyTop = topY + noseH
    const bodyBottom = bodyTop + bodyH
    const strokeAttr = outline ? `stroke="${outline}" stroke-width="4" stroke-linejoin="round"` : ''
    return `<g>
    <path d="M ${cx} ${topY} L ${(cx + w / 2).toFixed(1)} ${bodyTop.toFixed(1)} L ${(cx - w / 2).toFixed(1)} ${bodyTop.toFixed(1)} Z" fill="${accent}" ${strokeAttr}/>
    <rect x="${(cx - w / 2).toFixed(1)}" y="${bodyTop.toFixed(1)}" width="${w.toFixed(1)}" height="${bodyH.toFixed(1)}" rx="${(w * 0.3).toFixed(1)}" fill="${bodyFill}" ${strokeAttr}/>
    <circle cx="${cx}" cy="${(bodyTop + bodyH * 0.36).toFixed(1)}" r="${(w * 0.26).toFixed(1)}" fill="#7ec8e3" ${strokeAttr}/>
    <path d="M ${(cx - w / 2).toFixed(1)} ${bodyBottom.toFixed(1)} L ${(cx - w / 2 - finH * 0.6).toFixed(1)} ${(bodyBottom + finH).toFixed(1)} L ${(cx - w / 2).toFixed(1)} ${(bodyBottom - finH * 0.3).toFixed(1)} Z" fill="${accent}" ${strokeAttr}/>
    <path d="M ${(cx + w / 2).toFixed(1)} ${bodyBottom.toFixed(1)} L ${(cx + w / 2 + finH * 0.6).toFixed(1)} ${(bodyBottom + finH).toFixed(1)} L ${(cx + w / 2).toFixed(1)} ${(bodyBottom - finH * 0.3).toFixed(1)} Z" fill="${accent}" ${strokeAttr}/>
  </g>`
}

// A small hulled sailboat with a single mast and sail; waterline at (cx, waterY).
function sailboat(cx, waterY, w, h, hullFill, sailFill, outline = null) {
    const hullH = h * 0.16
    const mastH = h * 0.66
    const strokeAttr = outline ? `stroke="${outline}" stroke-width="4" stroke-linejoin="round"` : ''
    return `<g>
    <path d="M ${(cx - w / 2).toFixed(1)} ${(waterY - hullH).toFixed(1)} Q ${cx} ${(waterY + hullH * 0.7).toFixed(1)} ${(cx + w / 2).toFixed(1)} ${(waterY - hullH).toFixed(1)} Z" fill="${hullFill}" ${strokeAttr}/>
    <line x1="${cx}" y1="${(waterY - hullH).toFixed(1)}" x2="${cx}" y2="${(waterY - hullH - mastH).toFixed(1)}" stroke="${outline || hullFill}" stroke-width="4" stroke-linecap="round"/>
    <path d="M ${cx} ${(waterY - hullH - mastH).toFixed(1)} L ${cx} ${(waterY - hullH).toFixed(1)} L ${(cx - w * 0.4).toFixed(1)} ${(waterY - hullH - mastH * 0.12).toFixed(1)} Z" fill="${sailFill}" ${strokeAttr}/>
  </g>`
}

// A rounded wave band filling from baseY down to the bottom.
function waves(baseY, amp, fill, cycles = 3, outline = null) {
    const step = WIDTH / cycles
    let d = `M 0 ${HEIGHT} L 0 ${baseY.toFixed(1)} `
    for (let i = 0; i < cycles; i++) {
        const x1 = step * i + step * 0.5
        const x2 = step * (i + 1)
        d += `Q ${x1.toFixed(1)} ${(baseY - amp).toFixed(1)} ${x2.toFixed(1)} ${baseY.toFixed(1)} `
    }
    d += `L ${WIDTH} ${HEIGHT} Z`
    const strokeAttr = outline ? `stroke="${outline}" stroke-width="4" stroke-linejoin="round"` : ''
    return `<path d="${d}" fill="${fill}" ${strokeAttr}/>`
}

// A soft rounded cloud made of overlapping circles plus a base pad.
function puffyCloud(cx, cy, r, fill) {
    return `<g fill="${fill}">
    <circle cx="${(cx - r * 0.6).toFixed(1)}" cy="${cy.toFixed(1)}" r="${(r * 0.55).toFixed(1)}"/>
    <circle cx="${cx.toFixed(1)}" cy="${(cy - r * 0.25).toFixed(1)}" r="${(r * 0.7).toFixed(1)}"/>
    <circle cx="${(cx + r * 0.6).toFixed(1)}" cy="${cy.toFixed(1)}" r="${(r * 0.55).toFixed(1)}"/>
    <rect x="${(cx - r * 0.7).toFixed(1)}" y="${cy.toFixed(1)}" width="${(r * 1.4).toFixed(1)}" height="${(r * 0.5).toFixed(1)}" rx="${(r * 0.25).toFixed(1)}"/>
  </g>`
}

// A row of building silhouettes with randomly lit windows, base at baseY.
function citySkyline(rng, baseY, fill, litColor) {
    let out = ''
    let x = -10
    while (x < WIDTH + 10) {
        const w = rand(rng, 26, 55)
        const h = rand(rng, 90, 260)
        const y = baseY - h
        out += `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${w.toFixed(1)}" height="${h.toFixed(1)}" fill="${fill}"/>`
        const cols = Math.max(1, Math.floor(w / 10))
        const rows = Math.max(1, Math.floor(h / 14))
        for (let r = 0; r < rows; r++) {
            for (let c = 0; c < cols; c++) {
                if (rng() < 0.35) {
                    const wx = x + 4 + c * 10
                    const wy = y + 6 + r * 14
                    out += `<rect x="${wx.toFixed(1)}" y="${wy.toFixed(1)}" width="4" height="6" fill="${litColor}" opacity="${rand(rng, 0.5, 1).toFixed(2)}"/>`
                }
            }
        }
        x += w + rand(rng, 2, 8)
    }
    return out
}

// A scalloped-canopy umbrella, crown at (cx, cy), radius r.
function umbrella(cx, cy, r, fill, segments = 6) {
    let d = `M ${(cx - r).toFixed(1)} ${cy.toFixed(1)} A ${r} ${r} 0 0 1 ${(cx + r).toFixed(1)} ${cy.toFixed(1)} `
    for (let i = segments; i >= 1; i--) {
        const x1 = cx - r + ((2 * r) / segments) * i
        const x0 = cx - r + ((2 * r) / segments) * (i - 1)
        const midX = (x0 + x1) / 2
        d += `Q ${midX.toFixed(1)} ${(cy + 8).toFixed(1)} ${x0.toFixed(1)} ${cy.toFixed(1)} `
    }
    d += 'Z'
    return `<g>
    <path d="${d}" fill="${fill}"/>
    <line x1="${cx}" y1="${cy}" x2="${cx}" y2="${(cy + r * 1.6).toFixed(1)}" stroke="${fill}" stroke-width="4"/>
    <path d="M ${cx} ${(cy + r * 1.6).toFixed(1)} q 10 6 10 16" fill="none" stroke="${fill}" stroke-width="4"/>
  </g>`
}

// A crescent moon, built with a subtractive mask so it works over any
// background (gradient or flat).
function crescentMoon(cx, cy, r, fill) {
    const maskId = `moonmask${Math.round(cx)}${Math.round(cy)}${Math.round(r)}`
    return `<mask id="${maskId}"><rect x="0" y="0" width="${WIDTH}" height="${HEIGHT}" fill="#fff"/><circle cx="${(cx + r * 0.4).toFixed(1)}" cy="${(cy - r * 0.15).toFixed(1)}" r="${(r * 0.92).toFixed(1)}" fill="#000"/></mask>
  <circle cx="${cx}" cy="${cy}" r="${r}" fill="${fill}" mask="url(#${maskId})"/>`
}

// A simple sun disc, optionally outlined (cartoon) and/or given a face.
function sunDisc(cx, cy, r, fill, { outline = null, outlineWidth = 4, smile = false } = {}) {
    let face = ''
    if (smile && outline) {
        const eyeR = r * 0.07
        face = `<circle cx="${(cx - r * 0.28).toFixed(1)}" cy="${(cy - r * 0.1).toFixed(1)}" r="${eyeR.toFixed(1)}" fill="${outline}"/>
      <circle cx="${(cx + r * 0.28).toFixed(1)}" cy="${(cy - r * 0.1).toFixed(1)}" r="${eyeR.toFixed(1)}" fill="${outline}"/>
      <path d="M ${(cx - r * 0.3).toFixed(1)} ${(cy + r * 0.18).toFixed(1)} Q ${cx} ${(cy + r * 0.42).toFixed(1)} ${(cx + r * 0.3).toFixed(1)} ${(cy + r * 0.18).toFixed(1)}" fill="none" stroke="${outline}" stroke-width="${(r * 0.06).toFixed(1)}" stroke-linecap="round"/>`
    }
    const strokeAttr = outline ? `stroke="${outline}" stroke-width="${outlineWidth}"` : ''
    return `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${fill}" ${strokeAttr}/>${face}`
}

// A ring of radiating wedge rays between rInner and rOuter around (cx, cy).
function sunburstRays(cx, cy, rInner, rOuter, count, fill, opacity) {
    let out = ''
    for (let i = 0; i < count; i++) {
        const a0 = (i / count) * Math.PI * 2
        const a1 = a0 + ((Math.PI / count) * 2) / 3
        const x0i = cx + Math.cos(a0) * rInner
        const y0i = cy + Math.sin(a0) * rInner
        const x1i = cx + Math.cos(a1) * rInner
        const y1i = cy + Math.sin(a1) * rInner
        const x0o = cx + Math.cos(a0) * rOuter
        const y0o = cy + Math.sin(a0) * rOuter
        const x1o = cx + Math.cos(a1) * rOuter
        const y1o = cy + Math.sin(a1) * rOuter
        out += `<polygon points="${x0i.toFixed(1)},${y0i.toFixed(1)} ${x0o.toFixed(1)},${y0o.toFixed(1)} ${x1o.toFixed(1)},${y1o.toFixed(1)} ${x1i.toFixed(1)},${y1i.toFixed(1)}" fill="${fill}" opacity="${opacity}"/>`
    }
    return out
}

// A jagged lightning-bolt polygon, top at (x, y), total height h.
function lightningBolt(x, y, h, fill) {
    const w = h * 0.5
    const pts = [
        [x + w * 0.5, y],
        [x, y + h * 0.42],
        [x + w * 0.28, y + h * 0.42],
        [x - w * 0.1, y + h],
        [x + w * 0.62, y + h * 0.5],
        [x + w * 0.32, y + h * 0.5],
    ]
    return `<polygon points="${pts.map((p) => `${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ')}" fill="${fill}"/>`
}

// A synthwave-style perspective grid converging on the horizon.
function synthGrid(horizonY, color = '#2de2ff', count = 7) {
    let out = ''
    const vanishX = WIDTH / 2
    for (let i = -count; i <= count; i++) {
        const xBase = vanishX + i * (WIDTH / count) * 1.4
        out += `<line x1="${vanishX}" y1="${horizonY}" x2="${xBase.toFixed(1)}" y2="${HEIGHT}" stroke="${color}" stroke-width="0.8" opacity="0.32"/>`
    }
    for (let i = 1; i <= 6; i++) {
        const y = horizonY + i * i * 5
        if (y > HEIGHT) break
        out += `<line x1="0" y1="${y.toFixed(1)}" x2="${WIDTH}" y2="${y.toFixed(1)}" stroke="${color}" stroke-width="0.8" opacity="${Math.max(0.08, 0.4 - i * 0.06).toFixed(2)}"/>`
    }
    return out
}

// Thin colored streaks on wet pavement, reflecting the skyline above.
function wetStreakReflections(rng, baseY) {
    let out = ''
    const colors = ['#ff5ce0', '#2de2ff', '#a259ff']
    for (let i = 0; i < 10; i++) {
        const x = rand(rng, 20, WIDTH - 20)
        const c = colors[Math.floor(rng() * colors.length)]
        const h = rand(rng, 40, 110)
        out += `<rect x="${x.toFixed(1)}" y="${baseY.toFixed(1)}" width="${rand(rng, 2, 4).toFixed(1)}" height="${h.toFixed(1)}" fill="${c}" opacity="${rand(rng, 0.12, 0.3).toFixed(2)}"/>`
    }
    return out
}

// --- shared texture defs ---------------------------------------------------
function grainFilter(id) {
    return `<filter id="${id}"><feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/></filter>`
}

function vignette(id, color, innerStop = 0.55, maxOpacity = 0.5) {
    return `<radialGradient id="${id}" cx="0.5" cy="0.42" r="0.75"><stop offset="${innerStop}" stop-color="${color}" stop-opacity="0"/><stop offset="1" stop-color="${color}" stop-opacity="${maxOpacity}"/></radialGradient>`
}

function scanlinePattern(id) {
    return `<pattern id="${id}" width="1" height="3" patternUnits="userSpaceOnUse"><rect width="1" height="1" fill="#000"/></pattern>`
}

function halftonePattern(id, color, size = 6, dot = 1.1) {
    return `<pattern id="${id}" width="${size}" height="${size}" patternUnits="userSpaceOnUse" patternTransform="rotate(18)"><circle cx="${size / 2}" cy="${size / 2}" r="${dot}" fill="${color}"/></pattern>`
}

// --- style -> subject matrix -----------------------------------------------
// Each movie id is assigned to exactly one of five visual styles and gets a
// bespoke scene function (no shared per-genre templates). Style-level finish
// (grain / vignette / scanlines) is applied uniformly by posterSVG below.
const STYLE_BY_ID = {
    'movie-01': 'synthwave',
    'movie-03': 'synthwave',
    'movie-10': 'synthwave',
    'movie-05': 'cartoon',
    'movie-11': 'cartoon',
    'movie-15': 'cartoon',
    'movie-02': 'vintage',
    'movie-07': 'vintage',
    'movie-08': 'vintage',
    'movie-06': 'minimalist',
    'movie-12': 'minimalist',
    'movie-14': 'minimalist',
    'movie-04': 'painterly',
    'movie-09': 'painterly',
    'movie-13': 'painterly',
}

const STYLE = {
    synthwave: { grain: 0.05, vignetteColor: '#000', vignetteOpacity: 0.42, scanlines: 0.1 },
    cartoon: { grain: 0, vignetteColor: null, vignetteOpacity: 0, scanlines: 0 },
    vintage: { grain: 0.1, vignetteColor: '#2a1608', vignetteOpacity: 0.5, scanlines: 0 },
    minimalist: { grain: 0, vignetteColor: null, vignetteOpacity: 0, scanlines: 0 },
    painterly: { grain: 0.06, vignetteColor: '#000', vignetteOpacity: 0.5, scanlines: 0 },
}

// Each scene function returns { defs, layers } authored in the 400x600 space.
const SCENES_BY_ID = {
    // --- SYNTHWAVE -----------------------------------------------------------

    // movie-01: a tall transmission tower on a neon grid horizon, emitting
    // concentric signal arcs from its tip.
    'movie-01'(rng) {
        const horizon = rand(rng, 400, 430)
        const towerX = rand(rng, 170, 230)
        const towerH = rand(rng, 220, 260)
        const towerTopY = horizon - towerH
        let arcs = ''
        for (let i = 1; i <= 5; i++) {
            const r = i * rand(rng, 26, 34)
            arcs += `<circle cx="${towerX}" cy="${towerTopY.toFixed(1)}" r="${r.toFixed(1)}" fill="none" stroke="#ff5ce0" stroke-width="1.4" opacity="${Math.max(0.05, 0.4 - i * 0.06).toFixed(2)}"/>`
        }
        let lattice = ''
        const segs = 6
        for (let i = 0; i < segs; i++) {
            const y0 = towerTopY + (towerH / segs) * i
            const y1 = towerTopY + (towerH / segs) * (i + 1)
            const w0 = 3 + (i / segs) * 22
            const w1 = 3 + ((i + 1) / segs) * 22
            lattice += `<line x1="${(towerX - w0).toFixed(1)}" y1="${y0.toFixed(1)}" x2="${(towerX + w1).toFixed(1)}" y2="${y1.toFixed(1)}" stroke="#0a0418" stroke-width="1.6"/>`
            lattice += `<line x1="${(towerX + w0).toFixed(1)}" y1="${y0.toFixed(1)}" x2="${(towerX - w1).toFixed(1)}" y2="${y1.toFixed(1)}" stroke="#0a0418" stroke-width="1.6"/>`
        }
        return {
            defs: `<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#0a0420"/><stop offset="0.5" stop-color="#341060"/><stop offset="1" stop-color="#7a1f6e"/></linearGradient>
        <radialGradient id="sun" cx="0.5" cy="0.5" r="0.5"><stop offset="0" stop-color="#ffb0e6"/><stop offset="0.6" stop-color="#ff5ce0"/><stop offset="1" stop-color="#ff5ce0" stop-opacity="0"/></radialGradient>`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#sky)"/>
        ${stars(rng, 50, horizon - 40)}
        <circle cx="${towerX}" cy="${(horizon - 90).toFixed(1)}" r="120" fill="url(#sun)"/>
        ${synthGrid(horizon)}
        <rect x="0" y="${(horizon - 1).toFixed(1)}" width="${WIDTH}" height="2" fill="#2de2ff" opacity="0.8"/>
        <line x1="${towerX}" y1="${towerTopY.toFixed(1)}" x2="${towerX}" y2="${horizon.toFixed(1)}" stroke="#0a0418" stroke-width="3"/>
        ${lattice}
        <circle cx="${towerX}" cy="${towerTopY.toFixed(1)}" r="4.5" fill="#ff5ce0"/>
        ${arcs}`,
        }
    },

    // movie-03: a neon city skyline with lit windows over a wet street, colored
    // reflections streaking the pavement.
    'movie-03'(rng) {
        const baseY = rand(rng, 430, 450)
        return {
            defs: `<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#08021c"/><stop offset="0.55" stop-color="#2c0f52"/><stop offset="1" stop-color="#6e1868"/></linearGradient>
        <linearGradient id="street" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#160828"/><stop offset="1" stop-color="#020006"/></linearGradient>`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#sky)"/>
        ${stars(rng, 30, 140)}
        ${citySkyline(rng, baseY, '#0c0420', '#ff5ce0')}
        ${citySkyline(rng, baseY + 20, '#160730', '#2de2ff')}
        <rect x="0" y="${baseY.toFixed(1)}" width="${WIDTH}" height="${(HEIGHT - baseY).toFixed(1)}" fill="url(#street)"/>
        ${wetStreakReflections(rng, baseY)}`,
        }
    },

    // movie-10: an abstract circuit/grid field with a rising gradient moon and
    // scattered node dots.
    'movie-10'(rng) {
        const moonCx = rand(rng, 260, 320)
        const moonCy = rand(rng, 150, 210)
        const moonR = rand(rng, 60, 78)
        let circuit = ''
        for (let i = 0; i < 14; i++) {
            const horizontal = rng() < 0.5
            const x = rand(rng, 0, WIDTH)
            const y = rand(rng, 260, HEIGHT)
            const len = rand(rng, 30, 120)
            if (horizontal) {
                circuit += `<line x1="${x.toFixed(1)}" y1="${y.toFixed(1)}" x2="${(x + len).toFixed(1)}" y2="${y.toFixed(1)}" stroke="#2de2ff" stroke-width="1.2" opacity="0.35"/>`
                circuit += `<circle cx="${(x + len).toFixed(1)}" cy="${y.toFixed(1)}" r="2.4" fill="#2de2ff" opacity="0.7"/>`
            } else {
                circuit += `<line x1="${x.toFixed(1)}" y1="${y.toFixed(1)}" x2="${x.toFixed(1)}" y2="${(y + len).toFixed(1)}" stroke="#ff5ce0" stroke-width="1.2" opacity="0.35"/>`
                circuit += `<circle cx="${x.toFixed(1)}" cy="${(y + len).toFixed(1)}" r="2.4" fill="#ff5ce0" opacity="0.7"/>`
            }
        }
        return {
            defs: `<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#050014"/><stop offset="0.6" stop-color="#1c0a3e"/><stop offset="1" stop-color="#4a1470"/></linearGradient>
        <linearGradient id="moon" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ffe9a8"/><stop offset="0.5" stop-color="#ff8ac2"/><stop offset="1" stop-color="#2de2ff"/></linearGradient>
        <radialGradient id="mglow" cx="0.5" cy="0.5" r="0.5"><stop offset="0" stop-color="#ff8ac2" stop-opacity="0.5"/><stop offset="1" stop-color="#ff8ac2" stop-opacity="0"/></radialGradient>`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#sky)"/>
        ${synthGrid(470, '#2de2ff', 6)}
        <circle cx="${moonCx}" cy="${moonCy}" r="${(moonR * 1.7).toFixed(1)}" fill="url(#mglow)"/>
        <circle cx="${moonCx}" cy="${moonCy}" r="${moonR}" fill="url(#moon)"/>
        ${circuit}`,
        }
    },

    // --- CARTOON ---------------------------------------------------------------

    // movie-05: a small sailboat riding big rounded waves under a round sun and
    // a puffy cloud, flat bright bands, thick dark outlines.
    'movie-05'(rng) {
        const outline = '#1b1b2e'
        const sunX = rand(rng, 90, 150)
        return {
            defs: ``,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#7fd8f0"/>
        ${sunDisc(sunX, 120, 54, '#ffd84d', { outline, outlineWidth: 5 })}
        ${puffyCloud(rand(rng, 230, 320), rand(rng, 90, 140), 46, '#ffffff')}
        <rect x="0" y="380" width="${WIDTH}" height="${HEIGHT - 380}" fill="#2aa7c8"/>
        ${waves(420, 26, '#1f8fb0', 4, outline)}
        ${waves(470, 30, '#187a99', 3, outline)}
        ${sailboat(200, 430, 120, 170, '#e8a34d', '#fff7e6', outline)}
        ${waves(520, 34, '#0f5c78', 3, outline)}
        <rect x="0" y="560" width="${WIDTH}" height="40" fill="#0a3a4d"/>`,
        }
    },

    // movie-11: a rocket blasting upward with flame, puffy smoke, and stars.
    'movie-11'(rng) {
        const outline = '#1b1b2e'
        const cx = 200
        const topY = 90
        const h = 260
        let smoke = ''
        for (let i = 0; i < 5; i++) {
            smoke += puffyCloud(rand(rng, 130, 270), rand(rng, 470, 560), rand(rng, 26, 44), '#dfe6ef')
        }
        let starDots = ''
        for (let i = 0; i < 18; i++) {
            const x = rand(rng, 0, WIDTH)
            const y = rand(rng, 0, 340)
            starDots += `<path d="M ${x.toFixed(1)} ${(y - 5).toFixed(1)} l 1.4 3.6 3.6 1.4 -3.6 1.4 -1.4 3.6 -1.4 -3.6 -3.6 -1.4 3.6 -1.4 Z" fill="#fff"/>`
        }
        return {
            defs: ``,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#2a1a5e"/>
        ${starDots}
        ${rocket(cx, topY, h, '#e8e8f0', '#ff5e7e', outline)}
        <path d="M ${cx - 26} ${topY + h - 6} Q ${cx} ${topY + h + 70} ${cx + 26} ${topY + h - 6} Q ${cx + 14} ${topY + h + 30} ${cx} ${topY + h + 20} Q ${cx - 14} ${topY + h + 30} ${cx - 26} ${topY + h - 6} Z" fill="#ffb347" stroke="${outline}" stroke-width="4" stroke-linejoin="round"/>
        ${smoke}
        <rect x="0" y="560" width="${WIDTH}" height="40" fill="#180f3a"/>`,
        }
    },

    // movie-15: a retro TV set with antenna and rays radiating behind it,
    // evoking Sunday-morning static.
    'movie-15'(rng) {
        const outline = '#1b1b2e'
        const cx = 200
        const cy = 300
        const tvW = 220
        const tvH = 160
        return {
            defs: ``,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#ffb0c4"/>
        ${sunburstRays(cx, cy - 10, 140, 260, 12, '#ffd84d', 0.5)}
        <rect x="${cx - tvW / 2}" y="${cy - tvH / 2}" width="${tvW}" height="${tvH}" rx="18" fill="#f2e9d8" stroke="${outline}" stroke-width="5"/>
        <rect x="${cx - tvW / 2 + 18}" y="${cy - tvH / 2 + 18}" width="${tvW * 0.6}" height="${tvH - 36}" rx="8" fill="#8fd8e8" stroke="${outline}" stroke-width="4"/>
        <circle cx="${cx + tvW / 2 - 30}" cy="${cy - 8}" r="10" fill="#ff5e7e" stroke="${outline}" stroke-width="3"/>
        <circle cx="${cx + tvW / 2 - 30}" cy="${cy + 24}" r="7" fill="#ffd84d" stroke="${outline}" stroke-width="3"/>
        <line x1="${cx - 20}" y1="${cy - tvH / 2}" x2="${cx - 60}" y2="${cy - tvH / 2 - 70}" stroke="${outline}" stroke-width="4" stroke-linecap="round"/>
        <line x1="${cx + 20}" y1="${cy - tvH / 2}" x2="${cx + 60}" y2="${cy - tvH / 2 - 70}" stroke="${outline}" stroke-width="4" stroke-linecap="round"/>
        <rect x="${cx - tvW * 0.42}" y="${cy + tvH / 2}" width="${tvW * 0.84}" height="30" rx="6" fill="#c98f4a" stroke="${outline}" stroke-width="4"/>
        <rect x="0" y="520" width="${WIDTH}" height="80" fill="#f2e9d8"/>`,
        }
    },

    // --- VINTAGE PRINT -----------------------------------------------------------

    // movie-02: a crescent moon over layered rolling hills, cream paper base,
    // halftone dot texture.
    'movie-02'(rng) {
        return {
            defs: `${halftonePattern('halo2', '#8a5a2a', 5, 1)}`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#f2e6c8"/>
        ${crescentMoon(210, 150, 72, '#c98a3a')}
        ${ridge(rng, 380, 40, '#8a5a2a', 6)}
        ${ridge(rng, 460, 50, '#5c3a1e', 6)}
        ${ridge(rng, 540, 60, '#2e1c10', 7)}
        <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#halo2)" opacity="0.35"/>`,
        }
    },

    // movie-07: a large cog with a small heart at its center, limited two-ink
    // palette, halftone texture.
    'movie-07'(rng) {
        return {
            defs: `${halftonePattern('halo7', '#6e2a3a', 5, 1)}`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#eee0c4"/>
        ${gear(200, 300, 150, 118, 12, '#8a3040')}
        ${gear(200, 300, 96, 76, 10, '#eee0c4')}
        <path d="M 200 262 C 172 232 128 246 128 286 C 128 320 200 366 200 366 C 200 366 272 320 272 286 C 272 246 228 232 200 262 Z" fill="#8a3040"/>
        <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#halo7)" opacity="0.4"/>`,
        }
    },

    // movie-08: a western desert — saguaro, mesa, and a big setting sun with
    // sunburst rays, warm two-ink palette.
    'movie-08'(rng) {
        const sunX = rand(rng, 150, 250)
        let cacti = ''
        for (let i = 0; i < 3; i++) {
            cacti += saguaro(rand(rng, 40, 360), rand(rng, 510, 530), rand(rng, 90, 130), '#3a2010')
        }
        return {
            defs: `${halftonePattern('halo8', '#7a3a12', 5, 1)}`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#f2d9a0"/>
        ${sunburstRays(sunX, 330, 60, 280, 16, '#c9743a', 0.16)}
        <circle cx="${sunX}" cy="330" r="72" fill="#d9772f"/>
        ${ridge(rng, 470, 40, '#a35a24', 6)}
        ${ridge(rng, 520, 50, '#5c3212', 6)}
        ${cacti}
        <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#halo8)" opacity="0.4"/>`,
        }
    },

    // --- MINIMALIST --------------------------------------------------------------

    // movie-06: a compass/crosshair on a flat dark field, generous negative
    // space.
    'movie-06'(rng) {
        const cx = 200
        const cy = 260
        const r = 90
        return {
            defs: ``,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#0e1a2b"/>
        <circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="#e8c76a" stroke-width="1.4"/>
        <line x1="${cx - r - 20}" y1="${cy}" x2="${cx + r + 20}" y2="${cy}" stroke="#e8c76a" stroke-width="1"/>
        <line x1="${cx}" y1="${cy - r - 20}" x2="${cx}" y2="${cy + r + 20}" stroke="#e8c76a" stroke-width="1"/>
        <polygon points="${cx},${cy - r - 38} ${cx - 9},${cy - r - 14} ${cx + 9},${cy - r - 14}" fill="#e8c76a"/>
        <circle cx="${cx}" cy="${cy}" r="3" fill="#e8c76a"/>`,
        }
    },

    // movie-12: a single triangular mountain peak with one bright star, flat
    // dark field.
    'movie-12'(rng) {
        return {
            defs: ``,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#161b2c"/>
        <polygon points="60,540 200,180 340,540" fill="#0c1120"/>
        <path d="M 300 140 l 3 9 9 3 -9 3 -3 9 -3 -9 -9 -3 9 -3 Z" fill="#f5e6b8"/>`,
        }
    },

    // movie-14: a single bold umbrella silhouette with a few minimal rain
    // dashes, flat light field.
    'movie-14'(rng) {
        let rain = ''
        for (let i = 0; i < 6; i++) {
            const x = rand(rng, 60, 340)
            const y = rand(rng, 300, 400)
            rain += `<line x1="${x.toFixed(1)}" y1="${y.toFixed(1)}" x2="${(x - 6).toFixed(1)}" y2="${(y + 18).toFixed(1)}" stroke="#8fa6c2" stroke-width="2" stroke-linecap="round"/>`
        }
        return {
            defs: ``,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="#e7e2d6"/>
        ${umbrella(200, 260, 110, '#1b1f2b')}
        ${rain}`,
        }
    },

    // --- PAINTERLY -----------------------------------------------------------

    // movie-04: a misty triangular-pine forest under a cold pale moon, fog
    // bands, cold teal palette.
    'movie-04'(rng) {
        let pines = ''
        for (let i = 0; i < 9; i++) {
            pines += pineTree(rand(rng, 10, 390), rand(rng, 540, 580), rand(rng, 120, 240), '#04211f')
        }
        let fog = ''
        for (let i = 0; i < 4; i++) {
            const y = rand(rng, 380, 520)
            fog += `<rect x="0" y="${y.toFixed(1)}" width="${WIDTH}" height="${rand(rng, 20, 40).toFixed(1)}" fill="#bfe3dd" opacity="${rand(rng, 0.08, 0.18).toFixed(2)}"/>`
        }
        return {
            defs: `<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#031816"/><stop offset="0.6" stop-color="#0a2e2c"/><stop offset="1" stop-color="#123f3c"/></linearGradient>
        <radialGradient id="moon" cx="0.5" cy="0.5" r="0.5"><stop offset="0.6" stop-color="#dff2ee"/><stop offset="1" stop-color="#a9cfc8"/></radialGradient>
        <radialGradient id="mglow" cx="0.5" cy="0.5" r="0.5"><stop offset="0" stop-color="#bfe3dd" stop-opacity="0.35"/><stop offset="1" stop-color="#bfe3dd" stop-opacity="0"/></radialGradient>`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#sky)"/>
        ${stars(rng, 30, 200)}
        <circle cx="200" cy="140" r="120" fill="url(#mglow)"/>
        <circle cx="200" cy="140" r="46" fill="url(#moon)"/>
        ${pines}
        ${fog}`,
        }
    },

    // movie-09: a turbulent storm sky, a jagged lightning bolt, a lone figure
    // in the rain; charcoal + electric blue/white palette.
    'movie-09'(rng) {
        let rain = ''
        for (let i = 0; i < 70; i++) {
            const x = rand(rng, -20, WIDTH)
            const y = rand(rng, 0, HEIGHT)
            const l = rand(rng, 14, 30)
            rain += `<line x1="${x.toFixed(1)}" y1="${y.toFixed(1)}" x2="${(x + l * 0.3).toFixed(1)}" y2="${(y + l).toFixed(1)}" stroke="#bcd6ff" stroke-width="0.8" opacity="${rand(rng, 0.08, 0.25).toFixed(2)}"/>`
        }
        const boltX = rand(rng, 140, 260)
        return {
            defs: `<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#05070a"/><stop offset="0.6" stop-color="#131a24"/><stop offset="1" stop-color="#1f2a38"/></linearGradient>
        <radialGradient id="flash" cx="0.5" cy="0.5" r="0.5"><stop offset="0" stop-color="#8fd0ff" stop-opacity="0.4"/><stop offset="1" stop-color="#8fd0ff" stop-opacity="0"/></radialGradient>`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#sky)"/>
        <circle cx="${boltX}" cy="200" r="180" fill="url(#flash)"/>
        ${lightningBolt(boltX, 60, 260, '#eaf4ff')}
        ${rain}
        ${figure(rand(rng, 150, 250), 560, 140, '#04070a')}`,
        }
    },

    // movie-13: rows of bare orchard trees under a large red blood moon with
    // low fog; deep red/orange palette.
    'movie-13'(rng) {
        let trees = ''
        for (let row = 0; row < 3; row++) {
            const y = 440 + row * 50
            for (let i = 0; i < 5; i++) {
                trees += deadTree(rng, 20 + i * 90 + rand(rng, -10, 10), y, rand(rng, 110, 170), '#1a0704')
            }
        }
        let fog = ''
        for (let i = 0; i < 3; i++) {
            const y = rand(rng, 420, 520)
            fog += `<rect x="0" y="${y.toFixed(1)}" width="${WIDTH}" height="30" fill="#4a1006" opacity="0.2"/>`
        }
        return {
            defs: `<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#0a0301"/><stop offset="0.55" stop-color="#4a0f06"/><stop offset="1" stop-color="#8a2410"/></linearGradient>
        <radialGradient id="moon" cx="0.5" cy="0.5" r="0.5"><stop offset="0.65" stop-color="#ff6a3a"/><stop offset="1" stop-color="#c8320f"/></radialGradient>
        <radialGradient id="mglow" cx="0.5" cy="0.5" r="0.5"><stop offset="0" stop-color="#ff4a1e" stop-opacity="0.45"/><stop offset="1" stop-color="#ff4a1e" stop-opacity="0"/></radialGradient>`,
            layers: `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#sky)"/>
        <circle cx="200" cy="160" r="150" fill="url(#mglow)"/>
        <circle cx="200" cy="160" r="66" fill="url(#moon)"/>
        ${trees}
        ${fog}`,
        }
    },
}

function posterSVG(movie, index) {
    const style = STYLE_BY_ID[movie.id]
    const rng = mulberry32((index + 1) * 2654435761)
    const scene = SCENES_BY_ID[movie.id](rng)
    const cfg = STYLE[style]

    let overlayDefs = ''
    let overlay = ''
    if (cfg.grain > 0) {
        overlayDefs += grainFilter('grain')
        overlay += `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#grain)" opacity="${cfg.grain}"/>`
    }
    if (cfg.scanlines > 0) {
        overlayDefs += scanlinePattern('scan')
        overlay += `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#scan)" opacity="${cfg.scanlines}"/>`
    }
    if (cfg.vignetteColor) {
        overlayDefs += vignette('vig', cfg.vignetteColor, 0.55, cfg.vignetteOpacity)
        overlay += `<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#vig)"/>`
    }

    return `<svg width="${WIDTH}" height="${HEIGHT}" viewBox="0 0 ${WIDTH} ${HEIGHT}" xmlns="http://www.w3.org/2000/svg" preserveAspectRatio="xMidYMid slice">
    <defs>
      ${scene.defs}
      ${overlayDefs}
    </defs>
    ${scene.layers}
    ${overlay}
  </svg>`
}

// --- per-style title typography --------------------------------------------
const TITLE_STYLE = {
    synthwave: {
        fontFamily: `'Arial Black', 'Helvetica Neue', Arial, sans-serif`,
        fontWeight: 800,
        letterSpacing: '0.02em',
        textShadow: '0 0 14px rgba(45,226,255,0.85), 0 0 28px rgba(255,92,224,0.55), 0 2px 10px rgba(0,0,0,0.6)',
        metaLetterSpacing: '0.22em',
        sizes: [46, 38, 30],
    },
    cartoon: {
        fontFamily: `'Trebuchet MS', 'Segoe UI', system-ui, sans-serif`,
        fontWeight: 800,
        letterSpacing: '0.01em',
        textShadow: '0 3px 0 rgba(27,27,46,0.9)',
        metaLetterSpacing: '0.14em',
        sizes: [48, 40, 32],
    },
    vintage: {
        fontFamily: `Georgia, 'Times New Roman', serif`,
        fontWeight: 700,
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
        textShadow: '0 2px 6px rgba(0,0,0,0.35)',
        metaLetterSpacing: '0.24em',
        sizes: [40, 33, 26],
    },
    minimalist: {
        fontFamily: `'Helvetica Neue', Helvetica, Arial, sans-serif`,
        fontWeight: 300,
        letterSpacing: '0.12em',
        textShadow: '0 1px 4px rgba(0,0,0,0.5)',
        metaLetterSpacing: '0.3em',
        sizes: [34, 28, 22],
    },
    painterly: {
        fontFamily: `Georgia, 'Times New Roman', serif`,
        fontWeight: 700,
        letterSpacing: '0.01em',
        textShadow: '0 4px 18px rgba(0,0,0,0.85)',
        metaLetterSpacing: '0.18em',
        sizes: [46, 38, 30],
    },
}

function titleFontSize(style, title) {
    const [big, med, small] = TITLE_STYLE[style].sizes
    return `${title.length > 28 ? small : title.length > 18 ? med : big}px`
}

function cardHTML(movie, index) {
    const { title, year, genre, id } = movie
    const style = STYLE_BY_ID[id]
    const t = TITLE_STYLE[style]
    const fontSize = titleFontSize(style, title)
    const transform = t.textTransform ? `text-transform: ${t.textTransform};` : ''
    return `<!doctype html>
<html><head><meta charset="utf-8"><style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  html, body { width: ${WIDTH}px; height: ${HEIGHT}px; overflow: hidden; }
  body { position: relative; font-family: Georgia, 'Times New Roman', serif; background: #000; }
  .art { position: absolute; inset: 0; }
  .scrim { position: absolute; inset: 0; background: linear-gradient(to bottom, transparent 42%, rgba(0,0,0,0.42) 68%, rgba(0,0,0,0.82) 100%); }
  .content { position: absolute; left: 0; right: 0; bottom: 0; padding: 0 28px 40px; text-align: center; }
  .title { color: #fff; font-size: ${fontSize}; font-weight: ${t.fontWeight}; line-height: 1.08; letter-spacing: ${t.letterSpacing}; text-shadow: ${t.textShadow}; font-family: ${t.fontFamily}; ${transform} }
  .rule { width: 46px; height: 2px; background: rgba(255,255,255,0.55); margin: 16px auto 12px; }
  .meta { color: rgba(255,255,255,0.82); font-family: -apple-system, 'Segoe UI', Helvetica, Arial, sans-serif; font-size: 13px; letter-spacing: ${t.metaLetterSpacing}; text-transform: uppercase; }
  .frame { position: absolute; inset: 13px; border: 1px solid rgba(255,255,255,0.16); border-radius: 5px; pointer-events: none; }
</style></head>
<body>
  <div class="art">${posterSVG(movie, index)}</div>
  <div class="scrim"></div>
  <div class="content">
    <div class="title">${escapeHTML(title)}</div>
    <div class="rule"></div>
    <div class="meta">${year} &middot; ${escapeHTML(genre)}</div>
  </div>
  <div class="frame"></div>
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
        // them to full width; the host grid uses the same assets at small size.
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
