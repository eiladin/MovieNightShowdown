import { BrowserRouter, Route, Routes } from 'react-router'
import RequireSetup from './components/RequireSetup'
import HostSetup from './pages/HostSetup'
import Landing from './pages/Landing'
import Lobby from './pages/Lobby'
import Setup from './pages/Setup'

// AppRoutes is the route tree on its own, without a router around it. App
// wraps it in a BrowserRouter for production; keeping the two separate lets
// tests mount the same tree under a MemoryRouter at a chosen path.
export function AppRoutes() {
    return (
        <Routes>
            {/* /setup sits outside the gate: it is where the gate sends an
                unconfigured deployment, and stays reachable as a reference
                once configuration is done. */}
            <Route path="/setup" element={<Setup />} />
            <Route
                path="/"
                element={
                    <RequireSetup>
                        <Landing />
                    </RequireSetup>
                }
            />
            <Route
                path="/host"
                element={
                    <RequireSetup>
                        <HostSetup />
                    </RequireSetup>
                }
            />
            <Route
                path="/join/:code"
                element={
                    <RequireSetup>
                        <Lobby />
                    </RequireSetup>
                }
            />
        </Routes>
    )
}

function App() {
    return (
        <BrowserRouter>
            <AppRoutes />
        </BrowserRouter>
    )
}

export default App
