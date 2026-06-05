import { create } from 'zustand'

export type ToastType = 'error' | 'success' | 'warning'

export interface Toast {
  id: string
  type: ToastType
  title: string
  message?: string
  duration: number // ms; 0 = persist until dismissed
}

interface ToastStore {
  toasts: Toast[]
  addToast: (type: ToastType, title: string, message?: string, duration?: number) => void
  removeToast: (id: string) => void
}

export const useToastStore = create<ToastStore>((set) => ({
  toasts: [],

  addToast: (type, title, message, duration) =>
    set(s => ({
      toasts: [
        ...s.toasts,
        {
          id: `${Date.now()}-${Math.random()}`,
          type,
          title,
          message,
          duration: duration ?? (type === 'error' ? 8000 : 4000),
        },
      ],
    })),

  removeToast: (id) =>
    set(s => ({ toasts: s.toasts.filter(t => t.id !== id) })),
}))
