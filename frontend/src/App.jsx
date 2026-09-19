import { Navigate, Route, Routes } from 'react-router-dom'
import { ProtectedRoute } from './routes/ProtectedRoute'
import { AppShell } from './layouts/AppShell'
import { DashboardPage } from './pages/DashboardPage'
import { LoginPage } from './pages/LoginPage'
import { NodeDetailsPage } from './pages/NodeDetailsPage'
import { NodesPage } from './pages/NodesPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { SettingsPage } from './pages/SettingsPage'

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="nodes" element={<NodesPage />} />
          <Route path="nodes/:id" element={<NodeDetailsPage />} />
          <Route path="settings" element={<Navigate to="/settings/profile" replace />} />
          <Route path="settings/:tab" element={<SettingsPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Route>
    </Routes>
  )
}
