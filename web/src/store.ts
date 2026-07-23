import { create } from 'zustand'
import type { Movie } from './api'

// Status mirrors server.Status (see server/session.go).
export type Status = 'lobby' | 'active' | 'matched' | 'ended'

// Participant mirrors server.Participant's JSON shape. Token is never sent
// to other participants, so it never appears here.
export interface Participant {
  id: string
  name: string
  isAdmin: boolean
  connected: boolean
}

export type Vote = 'yes' | 'no'

interface SessionSnapshot {
  status: Status
  code: string
  requiredCount: number
  participants: Participant[]
  yourParticipantId: string
}

interface SessionStore {
  code: string | null
  status: Status
  requiredCount: number
  participants: Participant[]
  deck: Movie[]
  myParticipantId: string | null
  // myVoteState is local UI state only (never another participant's votes):
  // movieID -> the vote I last cast, so the swipe screen can render the
  // right state after an undo or a reconnect replay.
  myVoteState: Record<string, Vote>

  applySessionState: (snapshot: SessionSnapshot) => void
  setParticipants: (participants: Participant[]) => void
  setDeck: (deck: Movie[]) => void
  setStatus: (status: Status) => void
  recordVote: (movieId: string, vote: Vote) => void
  clearVote: (movieId: string) => void
  reset: () => void
}

const initialState = {
  code: null,
  status: 'lobby' as Status,
  requiredCount: 0,
  participants: [],
  deck: [],
  myParticipantId: null,
  myVoteState: {},
}

export const useSessionStore = create<SessionStore>((set) => ({
  ...initialState,

  applySessionState: (snapshot) =>
    set({
      status: snapshot.status,
      code: snapshot.code,
      requiredCount: snapshot.requiredCount,
      participants: snapshot.participants,
      myParticipantId: snapshot.yourParticipantId,
    }),

  setParticipants: (participants) => set({ participants }),
  setDeck: (deck) => set({ deck }),
  setStatus: (status) => set({ status }),

  recordVote: (movieId, vote) =>
    set((s) => ({ myVoteState: { ...s.myVoteState, [movieId]: vote } })),

  clearVote: (movieId) =>
    set((s) => {
      const next = { ...s.myVoteState }
      delete next[movieId]
      return { myVoteState: next }
    }),

  reset: () => set(initialState),
}))
