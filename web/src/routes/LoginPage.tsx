import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { LogIn } from 'lucide-react';
import { api } from '../lib/api';
import { useI18n } from '../lib/i18n';
import { AuthCard, authButtonClass, authInputClass } from '../components/AuthCard';
import { Loading } from '../components/State';

export function LoginPage() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const { t } = useI18n();
  const status = useQuery({ queryKey: ['auth', 'status'], queryFn: api.authStatus, retry: false });
  const me = useQuery({ queryKey: ['auth', 'me'], queryFn: api.me, retry: false });

  useEffect(() => {
    if (status.data?.needsSetup) {
      void navigate({ to: '/setup' });
    } else if (me.data?.user) {
      void navigate({ to: '/' });
    }
  }, [status.data, me.data, navigate]);

  const login = useMutation({
    mutationFn: () => api.login(username.trim(), password),
    onSuccess: async (data) => {
      qc.setQueryData(['auth', 'me'], data);
      qc.setQueryData(['auth', 'status'], { needsSetup: false });
      await navigate({ to: '/' });
    },
  });

  if (status.isLoading || me.isLoading) return <Loading />;
  if (status.data?.needsSetup || me.data?.user) return <Loading />;

  return (
    <AuthCard title={t('auth.signIn')} subtitle={t('auth.signInSubtitle')}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          login.mutate();
        }}
        className="space-y-3"
      >
        <label className="block">
          <span className="mb-1 block text-[12px] text-fg-muted">{t('auth.username')}</span>
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
          <span className="mb-1 block text-[12px] text-fg-muted">{t('auth.password')}</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
            className={authInputClass}
          />
        </label>
        {login.isError ? <p className="text-[12px] text-active">{(login.error as Error).message}</p> : null}
        <button type="submit" disabled={login.isPending} className={`${authButtonClass} inline-flex items-center justify-center gap-2`}>
          <LogIn className="size-4" aria-hidden />
          {login.isPending ? t('auth.signingIn') : t('auth.signIn')}
        </button>
      </form>
    </AuthCard>
  );
}
