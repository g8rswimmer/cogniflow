import { useEffect } from 'react'
import type { Toast as ToastType } from '../../stores/useToastStore'
import { useToastStore } from '../../stores/useToastStore'

const borderColor: Record<string, string> = {
  error:   'border-l-red-500',
  success: 'border-l-green-500',
  warning: 'border-l-amber-500',
}

const titleColor: Record<string, string> = {
  error:   'text-red-400',
  success: 'text-green-400',
  warning: 'text-amber-400',
}

interface Props {
  toast: ToastType
}

export function Toast({ toast }: Props) {
  const removeToast = useToastStore(s => s.removeToast)

  useEffect(() => {
    if (toast.duration === 0) return
    const timer = setTimeout(() => removeToast(toast.id), toast.duration)
    return () => clearTimeout(timer)
  }, [toast.id, toast.duration, removeToast])

  return (
    <div
      className={`
        flex items-start gap-3 w-80 rounded-lg shadow-xl
        bg-gray-800 border border-gray-700 border-l-4 ${borderColor[toast.type] ?? 'border-l-gray-500'}
        px-4 py-3 pointer-events-auto
      `}
      role="alert"
    >
      <div className="flex-1 min-w-0">
        <p className={`text-sm font-semibold ${titleColor[toast.type] ?? 'text-gray-200'}`}>
          {toast.title}
        </p>
        {toast.message && (
          <p className="text-xs text-gray-400 mt-0.5 break-words">{toast.message}</p>
        )}
      </div>
      <button
        onClick={() => removeToast(toast.id)}
        className="flex-shrink-0 text-gray-500 hover:text-gray-300 transition-colors text-lg leading-none"
        aria-label="Dismiss"
      >
        ×
      </button>
    </div>
  )
}
