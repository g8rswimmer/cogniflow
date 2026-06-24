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

  if (requiredRole && user) {
    const userLevel = ROLE_ORDER[user.role] ?? 0
    const requiredLevel = ROLE_ORDER[requiredRole] ?? 0
    if (userLevel < requiredLevel) {
      return <Navigate to="/" replace />
    }
  }

  return <>{children}</>
}
