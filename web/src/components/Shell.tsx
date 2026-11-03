import { useEffect, useState } from 'react';
import { Link, Outlet, useNavigate, useRouterState } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  KeyRound,
  ChevronDown,
  Clapperboard,
  DatabaseBackup,
  Film,
  Home,
  LibraryBig,
  ListVideo,
  LogOut,
  ScrollText,
  Search,
  Settings,
  Tv,
  Users,
  Video,
} from 'lucide-react';
import { api } from '../lib/api';
import { useI18n, type I18nKey, type Locale } from '../lib/i18n';
import { ChangePasswordDialog } from './ChangePasswordDialog';
import { PlayerModal } from './Player';
import { Loading } from './State';

type NavItem = { key: I18nKey; to: string; search?: Record<string, string>; icon: typeof Home };

const navItems: NavItem[] = [
  { key: 'nav.home', to: '/', icon: Home },
  { key: 'nav.movies', to: '/browse', search: { kind: 'movie' }, icon: Clapperboard },
  { key: 'nav.tv', to: '/browse', search: { kind: 'show' }, icon: Tv },
  { key: 'nav.homeVideos', to: '/browse', search: { kind: 'home_video' }, icon: Video },
  { key: 'nav.playlists', to: '/playlists', icon: ListVideo },
];

const adminPaths = ['/libraries', '/settings', '/tasks', '/users', '/activity', '/backup'];

export function Shell() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const location = useRouterState({ select: (s) => s.location });
  const [query, setQuery] = useState('');
  const [userMenu, setUserMenu] = useState(false);
  const [passwordOpen, setPasswordOpen] = useState(false);
  const { locale, setLocale, t } = useI18n();

  const currentKind =
    location.pathname === '/browse' ? ((location.search as { kind?: string }).kind ?? 'movie') : null;

  useEffect(() => {
    if (location.pathname === '/search') {
      setQuery(((location.search as { q?: string })?.q ?? '') as string);
    } else {
      setQuery('');
    }
  }, [location.pathname, location.search]);

  const config = useQuery({
    queryKey: ['config'],
    queryFn: () => fetch('/api/v1/config').then((r) => r.json() as Promise<{ version: string }>),
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
  const auth = useQuery({ queryKey: ['auth', 'me'], queryFn: api.me, retry: false, staleTime: 30_000 });
  const status = useQuery({ queryKey: ['auth', 'status'], queryFn: api.authStatus, retry: false, staleTime: 10_000 });

  const isMaintenance = location.pathname === '/maintenance';
  const isAuthPage = location.pathname === '/login' || location.pathname === '/setup' || isMaintenance;
  const user = auth.data?.user;
  const isAdmin = user?.role === 'admin';
  const userName = user?.displayName || user?.username || '';

  useEffect(() => {
    if (isAuthPage || status.isLoading || auth.isLoading) return;
    if (status.data?.needsSetup) {
      void navigate({ to: '/setup' });
      return;
    }
    if (auth.isError || !auth.data) {
      void navigate({ to: '/login' });
      return;
    }
    if (auth.data.user.role !== 'admin' && adminPaths.some((p) => location.pathname === p || location.pathname.startsWith(p + '/'))) {
      void navigate({ to: '/' });
    }
  }, [isAuthPage, status.isLoading, status.data, auth.isLoading, auth.isError, auth.data, location.pathname, navigate]);

  const submitSearch = (e: React.FormEvent) => {
    e.preventDefault();
    const q = query.trim();
    if (q) void navigate({ to: '/search', search: { q } });
  };

  const logout = async () => {
    setUserMenu(false);
    try {
      await api.logout();
    } catch {
      /* the session may already be gone */
    }
    qc.clear();
    await navigate({ to: '/login' });
  };

  if (isAuthPage) return <Outlet />;
  if (status.isLoading || auth.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg">
        <Loading label="Checking session…" />
      </div>
    );
  }
  if (!user) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg">
        <Loading label="Redirecting…" />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-bg text-fg">
      {/* header: logo · home · search · user · settings (Video Station layout) */}
      <header className="relative z-30 flex h-14 shrink-0 items-center gap-2 border-b border-hairline bg-surface/70 pr-3 backdrop-blur">
        <Link to="/" className="flex w-60 shrink-0 items-center gap-2.5 pl-3">
          <span className="grid size-8 place-items-center rounded-lg bg-active/15 ring-1 ring-active/30">
            <Film className="size-4 text-active" aria-hidden />
          </span>
          <span className="hidden text-[15px] font-semibold tracking-tight sm:inline">
            My<span className="text-fg-muted">VideoStation</span>
          </span>
        </Link>

        <Link
          to="/"
          aria-label="Home"
          className="grid size-9 place-items-center rounded-lg text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
        >
          <Home className="size-[18px]" aria-hidden />
        </Link>

        <form onSubmit={submitSearch} className="relative w-full max-w-xl" role="search">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-fg-dim" aria-hidden />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search titles, people, overviews…"
            aria-label="Search"
            className="h-9 w-full rounded-[2px] border border-hairline bg-surface-2 pl-9 pr-3 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3"
          />
        </form>

        <div className="ml-auto flex items-center gap-1">
          {isAdmin ? (
            <Link
              to="/settings"
              aria-label={t('nav.settings')}
              title={t('nav.settings')}
              className={[
                'grid size-9 place-items-center rounded-lg transition-colors',
                location.pathname === '/settings' ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
              ].join(' ')}
            >
              <Settings className="size-[18px]" aria-hidden />
            </Link>
          ) : null}

          <div className="relative">
            <button
              type="button"
              onClick={() => setUserMenu((v) => !v)}
              aria-expanded={userMenu}
              aria-label="Account"
              className="flex h-9 items-center gap-2 rounded-lg px-2 text-[12.5px] text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
            >
              <span className="grid size-6 place-items-center rounded-full bg-surface-3 text-[10.5px] font-medium text-fg">
                {userName.slice(0, 1).toUpperCase()}
              </span>
              <span className="hidden sm:inline">{userName}</span>
              <ChevronDown className="size-3.5 text-fg-dim" aria-hidden />
            </button>
            {userMenu ? (
              <>
                <button type="button" aria-hidden className="fixed inset-0 z-30 cursor-default" onClick={() => setUserMenu(false)} />
                <div className="absolute right-0 top-full z-40 mt-1 w-56 rounded-lg border border-hairline bg-surface-2 p-1 shadow-2xl">
                  <div className="px-2.5 py-2">
                    <p className="text-[12.5px] font-medium text-fg">{userName}</p>
                    <p className="text-[10.5px] uppercase tracking-wide text-fg-dim">
                      {user.role} · v{config.data?.version ?? 'dev'}
                    </p>
                  </div>
                  <div className="my-1 h-px bg-hairline" />
                  {isAdmin ? (
                    <>
                      <Link
                        to="/settings"
                        onClick={() => setUserMenu(false)}
                        className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                      >
                        <Settings className="size-3.5" aria-hidden /> {t('nav.settings')}
                      </Link>
                      <Link
                        to="/users"
                        onClick={() => setUserMenu(false)}
                        className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                      >
                        <Users className="size-3.5" aria-hidden /> {t('nav.users')}
                      </Link>
                      <Link
                        to="/activity"
                        onClick={() => setUserMenu(false)}
                        className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                      >
                        <ScrollText className="size-3.5" aria-hidden /> {t('nav.activity')}
                      </Link>
                      <Link
                        to="/backup"
                        onClick={() => setUserMenu(false)}
                        className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                      >
                        <DatabaseBackup className="size-3.5" aria-hidden /> {t('nav.backup')}
                      </Link>
                      <Link
                        to="/tasks"
                        onClick={() => setUserMenu(false)}
                        className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                      >
                        <Activity className="size-3.5" aria-hidden /> {t('nav.tasks')}
                      </Link>
                    </>
                  ) : null}
                  <div className="my-1 h-px bg-hairline" />
                  <label className="flex items-center justify-between gap-2 px-2.5 py-1.5 text-[11.5px] text-fg-dim">
                    {t('nav.language')}
                    <select
                      value={locale}
                      onChange={(e) => setLocale(e.target.value as Locale)}
                      className="rounded border border-hairline bg-surface-3 px-1.5 py-0.5 text-[11px] text-fg"
                    >
                      <option value="en">English</option>
                      <option value="de">Deutsch</option>
                      <option value="zh">中文</option>
                    </select>
                  </label>
                  <button
                    type="button"
                    onClick={() => {
                      setUserMenu(false);
                      setPasswordOpen(true);
                    }}
                    className="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                  >
                    <KeyRound className="size-3.5" aria-hidden /> {t('nav.changePassword')}
                  </button>
                  <button
                    type="button"
                    onClick={() => void logout()}
                    className="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                  >
                    <LogOut className="size-3.5" aria-hidden /> {t('nav.signOut')}
                  </button>
                </div>
              </>
            ) : null}
          </div>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <aside className="hidden w-60 shrink-0 flex-col border-r border-hairline bg-surface md:flex">
          <nav className="flex flex-1 flex-col gap-1 overflow-y-auto px-3 py-3" aria-label="Main">
            <p className="px-2 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wider text-fg-dim">{t('nav.library')}</p>
            {navItems.map((nav) => {
              const { key, to, icon: Icon } = nav;
              const label = t(key);
              const active =
                to === '/'
                  ? location.pathname === '/'
                  : to === '/browse'
                    ? location.pathname === '/browse' && currentKind === nav.search?.kind
                    : location.pathname === to;
              return (
                <Link
                  key={label}
                  to={to as never}
                  search={nav.search as never}
                  className={[
                    'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
                    active ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
                  ].join(' ')}
                  aria-current={active ? 'page' : undefined}
                >
                  <Icon
                    className={['size-[18px]', active ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted'].join(' ')}
                    aria-hidden
                  />
                  {label}
                </Link>
              );
            })}

            {isAdmin ? (
              <>
                <p className="px-2 pb-1 pt-5 text-[11px] font-medium uppercase tracking-wider text-fg-dim">{t('nav.manage')}</p>
                <Link
                  to="/libraries"
                  className={[
                    'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
                    location.pathname === '/libraries' ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
                  ].join(' ')}
                  aria-current={location.pathname === '/libraries' ? 'page' : undefined}
                >
                  <LibraryBig
                    className={['size-[18px]', location.pathname === '/libraries' ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted'].join(' ')}
                    aria-hidden
                  />
                  {t('nav.libraries')}
                </Link>
                <Link
                  to="/users"
                  className={[
                    'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
                    location.pathname === '/users' ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
                  ].join(' ')}
                  aria-current={location.pathname === '/users' ? 'page' : undefined}
                >
                  <Users
                    className={['size-[18px]', location.pathname === '/users' ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted'].join(' ')}
                    aria-hidden
                  />
                  {t('nav.users')}
                </Link>
                <Link
                  to="/activity"
                  className={[
                    'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
                    location.pathname === '/activity' ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
                  ].join(' ')}
                  aria-current={location.pathname === '/activity' ? 'page' : undefined}
                >
                  <ScrollText
                    className={['size-[18px]', location.pathname === '/activity' ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted'].join(' ')}
                    aria-hidden
                  />
                  {t('nav.activity')}
                </Link>
                <Link
                  to="/backup"
                  className={[
                    'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
                    location.pathname === '/backup' ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
                  ].join(' ')}
                  aria-current={location.pathname === '/backup' ? 'page' : undefined}
                >
                  <DatabaseBackup
                    className={['size-[18px]', location.pathname === '/backup' ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted'].join(' ')}
                    aria-hidden
                  />
                  {t('nav.backup')}
                </Link>
                <Link
                  to="/tasks"
                  className={[
                    'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
                    location.pathname === '/tasks' ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
                  ].join(' ')}
                  aria-current={location.pathname === '/tasks' ? 'page' : undefined}
                >
                  <Activity
                    className={['size-[18px]', location.pathname === '/tasks' ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted'].join(' ')}
                    aria-hidden
                  />
                  {t('nav.tasks')}
                </Link>
              </>
            ) : null}
          </nav>

          <div className="border-t border-hairline px-5 py-3 text-[11px] text-fg-dim">
            v{config.data?.version ?? 'dev'} · local
          </div>
        </aside>

        <main className="min-h-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
      <PlayerModal />
      {passwordOpen ? <ChangePasswordDialog onClose={() => setPasswordOpen(false)} /> : null}
    </div>
  );
}
