import { Navigate } from 'react-router-dom'
import { useAuthStore } from '../../stores/useAuthStore'

interface Props {
  children: React.ReactNode
  requiredRole?: 'org_admin' | 'system_admin'
}

const ROLE_ORDER: Record<string, number> = {
  member: 0,
  org_admin: 1,
  system_admin: 2,
}

export function ProtectedRoute({ children, requiredRole }: Props) {
  const token = useAuthStore(s => s.token)
  const user = useAuthStore(s => s.user)

  if (!token) {
    return <Navigate to="/login" replace />
  }

  if (requiredRole) {
    // Render nothing while the user profile is still loading after page reload.
    // App.tsx calls getMe() after initialize(); once user is set the check runs.
    if (!user) return null

    const userLevel = ROLE_ORDER[user.role] ?? 0
    const requiredLevel = ROLE_ORDER[requiredRole] ?? 0
    if (userLevel < requiredLevel) {
      return <Navigate to="/" replace />
    }
  }

  return <>{children}</>
}
