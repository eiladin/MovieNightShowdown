import { BrowserRouter, Route, Routes } from 'react-router-dom'
import HostSetup from './pages/HostSetup'
import Landing from './pages/Landing'
import Lobby from './pages/Lobby'

function App() {
    return (
        <BrowserRouter>
            <Routes>
                <Route path="/" element={<Landing />} />
                <Route path="/host" element={<HostSetup />} />
                <Route path="/join/:code" element={<Lobby />} />
            </Routes>
        </BrowserRouter>
    )
}

export default App
