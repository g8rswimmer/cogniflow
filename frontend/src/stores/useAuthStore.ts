import { create } from 'zustand'
import type { UserResponse } from '../api/types'

const TOKEN_KEY = 'cogniflow_token'

interface AuthState {
  token: string | null
  user: UserResponse | null
  login: (token: string, user: UserResponse) => void
  logout: () => void
  initialize: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  token: null,
  user: null,

  login: (token, user) => {
    localStorage.setItem(TOKEN_KEY, token)
    set({ token, user })
  },

  logout: () => {
    localStorage.removeItem(TOKEN_KEY)
    set({ token: null, user: null })
  },

  // Reads stored token; the API client will validate it on first use.
  // If the first authenticated request gets a 401, client.ts calls logout().
  initialize: () => {
    const token = localStorage.getItem(TOKEN_KEY)
    if (token) {
      set({ token })
    }
  },
}))
