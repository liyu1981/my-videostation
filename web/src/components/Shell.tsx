import { useEffect, useState } from 'react';
import { Link, Outlet, useNavigate, useRouterState } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import {
  Activity,
  ChevronDown,
  Clapperboard,
  Film,
  Home,
  LibraryBig,
  ListVideo,
  Search,
  Settings,
  Tv,
  Video,
} from 'lucide-react';
import { PlayerModal } from './Player';

type NavItem = { label: string; to: string; search?: Record<string, string>; icon: typeof Home };

const navItems: NavItem[] = [
  { label: 'Home', to: '/', icon: Home },
  { label: 'Movies', to: '/browse', search: { kind: 'movie' }, icon: Clapperboard },
  { label: 'TV Shows', to: '/browse', search: { kind: 'show' }, icon: Tv },
  { label: 'Home Videos', to: '/browse', search: { kind: 'home_video' }, icon: Video },
  { label: 'Playlists', to: '/playlists', icon: ListVideo },
];

export function Shell() {
  const navigate = useNavigate();
  const location = useRouterState({ select: (s) => s.location });
  const [query, setQuery] = useState('');
  const [userMenu, setUserMenu] = useState(false);

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
    queryFn: () => fetch('/api/v1/config').then((r) => r.json() as Promise<{ version: string; user: { name: string; role: string } }>),
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  });

  const submitSearch = (e: React.FormEvent) => {
    e.preventDefault();
    const q = query.trim();
    if (q) void navigate({ to: '/search', search: { q } });
  };

  const userName = config.data?.user?.name ?? 'Admin';

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
            className="h-9 w-full rounded-full border border-hairline bg-surface-2 pl-9 pr-3 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3"
          />
        </form>

        <div className="ml-auto flex items-center gap-1">
          <Link
            to="/settings"
            aria-label="Settings"
            title="Settings"
            className={[
              'grid size-9 place-items-center rounded-lg transition-colors',
              location.pathname === '/settings' ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
            ].join(' ')}
          >
            <Settings className="size-[18px]" aria-hidden />
          </Link>

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
                      {config.data?.user?.role ?? 'admin'} · v{config.data?.version ?? 'dev'}
                    </p>
                  </div>
                  <div className="my-1 h-px bg-hairline" />
                  <Link
                    to="/settings"
                    onClick={() => setUserMenu(false)}
                    className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                  >
                    <Settings className="size-3.5" aria-hidden /> Settings
                  </Link>
                  <Link
                    to="/tasks"
                    onClick={() => setUserMenu(false)}
                    className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                  >
                    <Activity className="size-3.5" aria-hidden /> Tasks
                  </Link>
                </div>
              </>
            ) : null}
          </div>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <aside className="hidden w-60 shrink-0 flex-col border-r border-hairline bg-surface md:flex">
          <nav className="flex flex-1 flex-col gap-1 overflow-y-auto px-3 py-3" aria-label="Main">
            <p className="px-2 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wider text-fg-dim">Library</p>
            {navItems.map((nav) => {
              const { label, to, icon: Icon } = nav;
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

            <p className="px-2 pb-1 pt-5 text-[11px] font-medium uppercase tracking-wider text-fg-dim">Manage</p>
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
              Libraries
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
              Tasks
            </Link>
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
    </div>
  );
}
