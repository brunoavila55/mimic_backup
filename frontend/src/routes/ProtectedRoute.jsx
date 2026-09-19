import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { LoadingScreen } from '../components/LoadingState'

export function ProtectedRoute() {
  const { isAuthenticated, isLoading } = useAuth()
  const location = useLocation()
  if (isLoading) return <LoadingScreen label="Checking your session" />
  if (!isAuthenticated) return <Navigate to="/login" replace state={{ from: location }} />
  return <Outlet />
}
