import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import QRJoin from '../components/QRJoin'
import { useSessionStore } from '../store'
import { SessionSocket, type ErrorPayload, type ParticipantUpdatePayload, type SessionStatePayload } from '../ws'
import '../styles/lobby.css'

// Lobby is reached at /join/:code (guest link, and the admin arrives here
// too via a link from /admin). It connects over WebSocket, joins (either
// resuming via a saved token or by submitting a name), and shows the live
// roster. The admin additionally sees the join code + QR.
export default function Lobby() {
  const { code = '' } = useParams<{ code: string }>()
  const upperCode = code.toUpperCase()

  const status = useSessionStore((s) => s.status)
  const participants = useSessionStore((s) => s.participants)
  const myParticipantId = useSessionStore((s) => s.myParticipantId)
  const applySessionState = useSessionStore((s) => s.applySessionState)
  const setParticipants = useSessionStore((s) => s.setParticipants)
  const reset = useSessionStore((s) => s.reset)

  const [name, setName] = useState('')
  const [joined, setJoined] = useState(() => SessionSocket.getToken(upperCode) !== '')
  const [socketError, setSocketError] = useState<string | null>(null)
  const socketRef = useRef<SessionSocket | null>(null)

  // Reset session state when navigating to a different code.
  useEffect(() => {
    return () => reset()
  }, [upperCode, reset])

  useEffect(() => {
    if (!joined) return

    const socket = new SessionSocket(upperCode, name)
    socketRef.current = socket

    const offState = socket.on('session_state', (payload) =>
      applySessionState(payload as SessionStatePayload),
    )
    const offParticipants = socket.on('participant_update', (payload) =>
      setParticipants((payload as ParticipantUpdatePayload).participants),
    )
    const offError = socket.on('error', (payload) => setSocketError((payload as ErrorPayload).message))

    socket.connect()

    return () => {
      offState()
      offParticipants()
      offError()
      socket.close()
    }
    // name is intentionally captured only at the moment `joined` flips true.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [joined, upperCode])

  function handleJoinSubmit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setJoined(true)
  }

  const me = participants.find((p) => p.id === myParticipantId)
  const isAdmin = me?.isAdmin ?? false
  const joinURL = `${window.location.origin}/join/${upperCode}`

  if (!joined) {
    return (
      <div className="lobby lobby-join-form">
        <h1>Join session {upperCode}</h1>
        <form onSubmit={handleJoinSubmit}>
          <input
            type="text"
            placeholder="Your name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
          <button type="submit">Join</button>
        </form>
      </div>
    )
  }

  return (
    <div className="lobby">
      <h1>Session {upperCode}</h1>
      <p className="lobby-status">Status: {status}</p>

      {isAdmin && <QRJoin joinURL={joinURL} />}

      {socketError && <p className="lobby-error">{socketError}</p>}
      {participants.length === 0 && !socketError && <p>Connecting…</p>}

      <ul className="participant-list">
        {participants.map((p) => (
          <li key={p.id} className={p.connected ? 'connected' : 'disconnected'}>
            <span className="participant-name">
              {p.name}
              {p.isAdmin ? ' (admin)' : ''}
            </span>
            <span className="participant-status">{p.connected ? 'online' : 'offline'}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
