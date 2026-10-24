import { Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './AuthContext.jsx'
import { YearProvider } from './YearContext.jsx'
import { OfflineProvider } from './OfflineContext.jsx'
import { ThemeProvider } from './ThemeContext.jsx'
import Layout from './Layout.jsx'
import Login from './pages/Login.jsx'
import Dashboard from './pages/Dashboard.jsx'
import Budgeting from './pages/Budgeting.jsx'
import Spending from './pages/Spending.jsx'
import Assets from './pages/Assets.jsx'
import Setup from './pages/Setup.jsx'
import Guide from './pages/Guide.jsx'
import Integrations from './pages/Integrations.jsx'
import Admin from './pages/Admin.jsx'

function Private({ children }) {
  const { user } = useAuth()
  if (user === undefined) return <div className="loading">Loading...</div>
  if (user === null) return <Navigate to="/login" replace />
  return children
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/" element={<Private><Layout /></Private>}>
        <Route index element={<Dashboard />} />
        <Route path="budgeting" element={<Budgeting />} />
        <Route path="spending" element={<Spending />} />
        <Route path="assets" element={<Assets />} />
        <Route path="setup" element={<Setup />} />
        <Route path="guide" element={<Guide />} />
        <Route path="integrations" element={<Integrations />} />
        <Route path="admin" element={<Admin />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <YearProvider>
          <OfflineProvider>
            <AppRoutes />
          </OfflineProvider>
        </YearProvider>
      </AuthProvider>
    </ThemeProvider>
  )
}
