import { useQuery } from '@tanstack/react-query';
import { api, type User } from './api';

/** Cached current user. The Shell only renders authenticated pages. */
export function useCurrentUser(): { user?: User; isLoading: boolean } {
  const q = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: api.me,
    retry: false,
    staleTime: 30_000,
  });
  return { user: q.data?.user, isLoading: q.isLoading };
}
