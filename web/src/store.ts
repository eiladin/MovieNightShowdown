import { create } from 'zustand'
import type { Movie, PreviewFilters } from './api'
import type { LeaderboardEntry } from './ws'

// Status mirrors server.Status (see server/session.go).
export type Status = 'lobby' | 'active' | 'matched' | 'ended'

// Participant mirrors server.Participant's JSON shape. Token is never sent
// to other participants, so it never appears here.
export interface Participant {
    id: string
    name: string
    isHost: boolean
    connected: boolean
}

export type Vote = 'yes' | 'no'

interface SessionSnapshot {
    status: Status
    code: string
    requiredCount: number
    participants: Participant[]
    yourParticipantId: string
    yourVotes?: Record<string, Vote>
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
    // filters is the host's chosen library filters, carried from HostSetup
    // (where they're picked) to Lobby (where "Begin" sends them in
    // host:start). Not session state from the server — purely local UI state.
    filters: PreviewFilters
    winner: Movie | null
    leaderboard: LeaderboardEntry[] | null

    applySessionState: (snapshot: SessionSnapshot) => void
    setParticipants: (participants: Participant[]) => void
    setDeck: (deck: Movie[]) => void
    setStatus: (status: Status) => void
    recordVote: (movieId: string, vote: Vote) => void
    clearVote: (movieId: string) => void
    setFilters: (filters: PreviewFilters) => void
    setWinner: (movie: Movie) => void
    setLeaderboard: (lb: LeaderboardEntry[]) => void
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
    filters: {} as PreviewFilters,
    winner: null,
    leaderboard: null,
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
            myVoteState: snapshot.yourVotes || {},
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

    setFilters: (filters) => set({ filters }),
    setWinner: (winner) => set({ winner, status: 'matched' }),
    setLeaderboard: (leaderboard) => set({ leaderboard, status: 'ended' }),

    // Everything in initialState is server-derived and must not survive a move
    // to a different session — except filters, which are the host's local
    // picks. Those have to outlive the lobby unmount so "Change filters" can
    // return to HostSetup with the previous selection still in place.
    reset: () => set((s) => ({ ...initialState, filters: s.filters })),
}))
