// The setup token authorizes configuration changes. The server generates it on
// first start and prints it to its log, which is the only delivery channel an
// application without accounts has.
//
// It is held in memory for the tab's lifetime and deliberately not persisted.
// store.ts persists exactly one thing — the host's filter picks — and a
// credential in localStorage outlives its usefulness by months, survives the
// tab closing, and is readable by anything that can run script on this origin.
// Re-entering it after a reload is the correct trade.

let token = ''

export function getSetupToken(): string {
    return token
}

export function setSetupToken(value: string): void {
    token = value.trim()
}

export function clearSetupToken(): void {
    token = ''
}
