// Registers the jest-dom matchers (toBeInTheDocument, toHaveTextContent, …)
// with vitest's expect, for every test file in the suite.
import '@testing-library/jest-dom/vitest'

// Node ≥22 defines an experimental `globalThis.localStorage` that throws
// without --localstorage-file, and vitest's jsdom environment drops jsdom's own
// implementation rather than shadow it. Neither `window.localStorage` nor the
// global one is usable as a result, so install a plain in-memory Storage for
// the code that reaches for it (zustand's persist middleware, SessionSocket).
class MemoryStorage implements Storage {
    private entries = new Map<string, string>()

    get length(): number {
        return this.entries.size
    }

    key(index: number): string | null {
        return [...this.entries.keys()][index] ?? null
    }

    getItem(key: string): string | null {
        return this.entries.get(key) ?? null
    }

    setItem(key: string, value: string): void {
        this.entries.set(key, String(value))
    }

    removeItem(key: string): void {
        this.entries.delete(key)
    }

    clear(): void {
        this.entries.clear()
    }
}

if (typeof globalThis.localStorage?.getItem !== 'function') {
    const storage = new MemoryStorage()
    for (const target of [globalThis, window]) {
        Object.defineProperty(target, 'localStorage', { configurable: true, value: storage })
    }
}
