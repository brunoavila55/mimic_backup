import { createContext, useContext, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const queryClient = useQueryClient()
  const authQuery = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: api.me,
    retry: false,
    staleTime: 60_000,
  })
  const loginMutation = useMutation({
    mutationFn: api.login,
    onSuccess: (user) => queryClient.setQueryData(['auth', 'me'], user),
  })
  const logoutMutation = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.setQueryData(['auth', 'me'], null)
      queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'auth' })
    },
  })

  const value = useMemo(() => ({
    user: authQuery.data || null,
    isLoading: authQuery.isLoading,
    isAuthenticated: Boolean(authQuery.data),
    error: authQuery.error,
    login: loginMutation.mutateAsync,
    loginPending: loginMutation.isPending,
    logout: logoutMutation.mutateAsync,
  }), [authQuery.data, authQuery.error, authQuery.isLoading, loginMutation.isPending, loginMutation.mutateAsync, logoutMutation.mutateAsync])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used inside AuthProvider')
  return context
}
