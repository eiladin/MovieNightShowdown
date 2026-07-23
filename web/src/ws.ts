import type { Movie } from './api'
import type { Participant, Status } from './store'

// Envelope mirrors server.Envelope (see server/messages.go).
interface Envelope {
  type: string
  payload: unknown
}

// --- Server -> Client payload shapes (see server/messages.go) ---

export interface SessionStatePayload {
  status: Status
  code: string
  requiredCount: number
  participants: Participant[]
  yourParticipantId: string
  yourToken: string
}

export interface ParticipantUpdatePayload {
  participants: Participant[]
}

// DeckPayload mirrors server.DeckPayload: the ordered, capped deck dealt at
// admin:start (Phase 4). Every client receives the same order.
export interface DeckPayload {
  movies: Movie[]
}

// ProgressPayload mirrors server.ProgressPayload: a HUD summary of swipe
// progress that never reveals who voted which way (Phase 4).
export interface ProgressPayload {
  participantsSwiped: number
  participantsTotal: number
  cardsRemaining: number
}

export interface MatchPayload {
  movie: Movie
}

export interface LeaderboardEntry {
  movie: Movie
  yesCount: number
}

export interface SessionEndedPayload {
  leaderboard: LeaderboardEntry[]
}

export interface ErrorPayload {
  message: string
}

type Listener = (payload: unknown) => void

const MAX_BACKOFF_MS = 10_000
const BASE_BACKOFF_MS = 1000

function tokenKey(code: string): string {
  return `mns:token:${code}`
}

// SessionSocket wraps the native WebSocket for one session: it connects to
// /ws?code=&token=, persists the resume token in localStorage, replays
// `join` on every (re)connect, and auto-reconnects with exponential backoff
// when the connection drops unexpectedly.
export class SessionSocket {
  private readonly code: string
  private readonly name: string
  private ws: WebSocket | null = null
  private reconnectAttempts = 0
  private manuallyClosed = false
  private readonly listeners = new Map<string, Set<Listener>>()

  constructor(code: string, name: string) {
    this.code = code
    this.name = name
  }

  static getToken(code: string): string {
    return localStorage.getItem(tokenKey(code)) ?? ''
  }

  static setToken(code: string, token: string): void {
    localStorage.setItem(tokenKey(code), token)
  }

  on(type: string, listener: Listener): () => void {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type)?.add(listener)
    return () => this.listeners.get(type)?.delete(listener)
  }

  connect(): void {
    this.manuallyClosed = false
    const token = SessionSocket.getToken(this.code)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const params = new URLSearchParams({ code: this.code, token })
    const url = `${protocol}//${window.location.host}/ws?${params.toString()}`

    const ws = new WebSocket(url)
    this.ws = ws

    ws.onopen = () => {
      this.reconnectAttempts = 0
      this.send('join', { code: this.code, name: this.name })
    }

    ws.onmessage = (event: MessageEvent<string>) => {
      let env: Envelope
      try {
        env = JSON.parse(event.data) as Envelope
      } catch {
        return
      }
      if (env.type === 'session_state') {
        const payload = env.payload as SessionStatePayload
        if (payload.yourToken) {
          SessionSocket.setToken(this.code, payload.yourToken)
        }
      }
      this.emit(env.type, env.payload)
    }

    ws.onclose = () => {
      if (!this.manuallyClosed) {
        this.scheduleReconnect()
      }
    }

    ws.onerror = () => {
      ws.close()
    }
  }

  send(type: string, payload: unknown): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type, payload }))
    }
  }

  close(): void {
    this.manuallyClosed = true
    this.ws?.close()
  }

  private scheduleReconnect(): void {
    const delay = Math.min(BASE_BACKOFF_MS * 2 ** this.reconnectAttempts, MAX_BACKOFF_MS)
    this.reconnectAttempts += 1
    setTimeout(() => {
      if (!this.manuallyClosed) {
        this.connect()
      }
    }, delay)
  }

  private emit(type: string, payload: unknown): void {
    this.listeners.get(type)?.forEach((listener) => listener(payload))
  }
}
