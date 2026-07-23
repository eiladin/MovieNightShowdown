import { BrowserRouter, Route, Routes } from 'react-router-dom'
import AdminSetup from './pages/AdminSetup'
import Landing from './pages/Landing'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Landing />} />
        <Route path="/admin" element={<AdminSetup />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
