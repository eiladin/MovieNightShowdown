import { BrowserRouter, Route, Routes } from 'react-router-dom'
import AdminSetup from './pages/AdminSetup'
import Landing from './pages/Landing'
import Lobby from './pages/Lobby'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Landing />} />
        <Route path="/admin" element={<AdminSetup />} />
        <Route path="/join/:code" element={<Lobby />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
