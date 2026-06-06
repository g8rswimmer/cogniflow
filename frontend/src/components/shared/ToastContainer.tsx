import { useToastStore } from '../../stores/useToastStore'
import { Toast } from './Toast'

export function ToastContainer() {
  const toasts = useToastStore(s => s.toasts)

  if (toasts.length === 0) return null

  return (
    <div
      className="fixed top-4 right-4 z-50 flex flex-col gap-2 pointer-events-none"
      aria-live="polite"
      aria-label="Notifications"
    >
      {toasts.map(t => <Toast key={t.id} toast={t} />)}
    </div>
  )
}
