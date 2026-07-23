import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { createSession } from '../api'
import { SessionSocket } from '../ws'
import '../styles/landing.css'

// Landing is the entry page: start a new session as host, or join an
// existing one by code.
export default function Landing() {
    const navigate = useNavigate()

    const [hostName, setHostName] = useState('')
    const [starting, setStarting] = useState(false)
    const [startError, setStartError] = useState<string | null>(null)

    const [joinCode, setJoinCode] = useState('')

    async function handleStart(e: FormEvent) {
        e.preventDefault()
        const name = hostName.trim()
        if (!name || starting) return

        setStarting(true)
        setStartError(null)
        try {
            const session = await createSession(name)
            // Persist the host's resume token before Lobby ever connects, so its
            // first /ws?token= attaches to this same participant instead of
            // creating a second one.
            SessionSocket.setToken(session.code, session.token)
            navigate(`/host?code=${session.code}`)
        } catch (err) {
            setStartError(err instanceof Error ? err.message : 'Failed to start session')
        } finally {
            setStarting(false)
        }
    }

    function handleJoin(e: FormEvent) {
        e.preventDefault()
        const code = joinCode.trim().toUpperCase()
        if (code) navigate(`/join/${code}`)
    }

    return (
        <div className="landing">
            <h1>Movie Night Showdown</h1>

            <form className="landing-panel" onSubmit={handleStart}>
                <h2>Start a Showdown</h2>
                <input
                    type="text"
                    placeholder="Your name"
                    value={hostName}
                    onChange={(e) => setHostName(e.target.value)}
                />
                <button type="submit" className="btn-primary" disabled={starting}>
                    {starting ? 'Starting…' : 'Start a Showdown'}
                </button>
                {startError && <p className="landing-error">{startError}</p>}
            </form>

            <form className="landing-panel" onSubmit={handleJoin}>
                <h2>Join a Showdown</h2>
                <input
                    type="text"
                    placeholder="Session code"
                    value={joinCode}
                    onChange={(e) => setJoinCode(e.target.value)}
                    maxLength={4}
                />
                <button type="submit">Join</button>
            </form>
        </div>
    )
}
