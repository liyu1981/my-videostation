import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { ShieldCheck } from 'lucide-react';
import { api } from '../lib/api';
import { AuthCard, authButtonClass, authInputClass } from '../components/AuthCard';
import { Loading } from '../components/State';

export function SetupPage() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [username, setUsername] = useState('admin');
  const [displayName, setDisplayName] = useState('Admin');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [localError, setLocalError] = useState('');
  const status = useQuery({ queryKey: ['auth', 'status'], queryFn: api.authStatus, retry: false });
  const me = useQuery({ queryKey: ['auth', 'me'], queryFn: api.me, retry: false });

  useEffect(() => {
    if (status.isLoading || me.isLoading) return;
    if (me.data?.user) {
      void navigate({ to: '/' });
    } else if (status.data && !status.data.needsSetup && me.isError && !me.isFetching) {
      void navigate({ to: '/login' });
    }
  }, [status.data, status.isLoading, me.data, me.isError, me.isFetching, me.isLoading, navigate]);

  const setup = useMutation({
    mutationFn: () => api.setup(username.trim(), displayName.trim(), password),
    onSuccess: async (data) => {
      qc.setQueryData(['auth', 'me'], data);
      qc.setQueryData(['auth', 'status'], { needsSetup: false });
      await navigate({ to: '/' });
    },
  });

  if (status.isLoading || me.isLoading) return <Loading />;
  if (me.data?.user || (status.data && !status.data.needsSetup)) return <Loading />;

  return (
    <AuthCard title="Set up MyVideoStation" subtitle="Create the first admin account. This prompt appears once.">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setLocalError('');
          if (password !== confirm) {
            setLocalError('Passwords do not match');
            return;
          }
          if (password.length < 8) {
            setLocalError('Password must be at least 8 characters');
            return;
          }
          setup.mutate();
        }}
        className="space-y-3"
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Username</span>
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
              required
              className={authInputClass}
            />
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Display name</span>
            <input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              autoComplete="name"
              className={authInputClass}
            />
          </label>
        </div>
        <label className="block">
          <span className="mb-1 block text-[12px] text-fg-muted">Password</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            required
            minLength={8}
            className={authInputClass}
          />
          <span className="mt-1 block text-[10.5px] text-fg-dim">At least 8 characters.</span>
        </label>
        <label className="block">
          <span className="mb-1 block text-[12px] text-fg-muted">Confirm password</span>
          <input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            required
            className={authInputClass}
          />
        </label>
        {localError || setup.isError ? (
          <p className="text-[12px] text-active">{localError || (setup.error as Error).message}</p>
        ) : null}
        <button type="submit" disabled={setup.isPending} className={`${authButtonClass} inline-flex items-center justify-center gap-2`}>
          <ShieldCheck className="size-4" aria-hidden />
          {setup.isPending ? 'Creating…' : 'Create admin account'}
        </button>
      </form>
    </AuthCard>
  );
}
