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
import AdminStats from './pages/AdminStats.jsx'

function Private({ children }) {
  const { user } = useAuth()
  if (user === undefined) return <div className="loading">Loading...</div>
  if (user === null) return <Navigate to="/login" replace />
  return children
}

// Super admin has no personal-finance data of their own - send them
// straight to the platform pages instead of an empty Dashboard/Spending/etc.
function AdminGuard({ children }) {
  const { user } = useAuth()
  if (user?.is_admin) return <Navigate to="/admin" replace />
  return children
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/" element={<Private><Layout /></Private>}>
        <Route index element={<AdminGuard><Dashboard /></AdminGuard>} />
        <Route path="budgeting" element={<AdminGuard><Budgeting /></AdminGuard>} />
        <Route path="spending" element={<AdminGuard><Spending /></AdminGuard>} />
        <Route path="assets" element={<AdminGuard><Assets /></AdminGuard>} />
        <Route path="setup" element={<AdminGuard><Setup /></AdminGuard>} />
        <Route path="guide" element={<AdminGuard><Guide /></AdminGuard>} />
        <Route path="integrations" element={<AdminGuard><Integrations /></AdminGuard>} />
        <Route path="admin" element={<Admin />} />
        <Route path="admin/stats" element={<AdminStats />} />
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
