import { useEffect, useState } from 'react';
import { Link, Outlet, useNavigate, useRouterState } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Clapperboard, Film, Home, LibraryBig, Search, Settings, Tv, Video } from 'lucide-react';

type NavItem = { label: string; to: string; search?: Record<string, string>; icon: typeof Home };

const navItems: NavItem[] = [
  { label: 'Home', to: '/', icon: Home },
  { label: 'Movies', to: '/browse', search: { kind: 'movie' }, icon: Clapperboard },
  { label: 'TV Shows', to: '/browse', search: { kind: 'show' }, icon: Tv },
  { label: 'Home Videos', to: '/browse', search: { kind: 'home_video' }, icon: Video },
];

export function Shell() {
  const navigate = useNavigate();
  const location = useRouterState({ select: (s) => s.location });
  const [query, setQuery] = useState('');

  const currentKind =
    location.pathname === '/browse' ? ((location.search as { kind?: string }).kind ?? 'movie') : null;

  useEffect(() => {
    if (location.pathname === '/search') {
      setQuery(((location.search as { q?: string }).q ?? '') as string);
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

  const submitSearch = (e: React.FormEvent) => {
    e.preventDefault();
    const q = query.trim();
    if (q) void navigate({ to: '/search', search: { q } });
  };

  return (
    <div className="flex h-full min-h-0 bg-bg text-fg">
      <aside className="hidden w-60 shrink-0 flex-col border-r border-hairline bg-surface md:flex">
        <Link to="/" className="flex h-14 items-center gap-3 px-5">
          <span className="grid size-8 place-items-center rounded-lg bg-active/15 ring-1 ring-active/30">
            <Film className="size-4 text-active" aria-hidden />
          </span>
          <span className="text-[15px] font-semibold tracking-tight">
            My<span className="text-fg-muted">VideoStation</span>
          </span>
        </Link>

        <nav className="flex flex-1 flex-col gap-1 overflow-y-auto px-3 py-3" aria-label="Main">
          <p className="px-2 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wider text-fg-dim">Library</p>
          {navItems.map((nav) => {
            const { label, to, icon: Icon } = nav;
            const active =
              to === '/'
                ? location.pathname === '/'
                : location.pathname === '/browse' && currentKind === nav.search?.kind;
            return (
              <Link
                key={label}
                to={to as never}
                search={nav.search as never}
                className={[
                  'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
                  active
                    ? 'bg-surface-3 text-fg'
                    : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
                ].join(' ')}
                aria-current={active ? 'page' : undefined}
              >
                <Icon className={['size-[18px]', active ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted'].join(' ')} aria-hidden />
                {label}
              </Link>
            );
          })}

          <p className="px-2 pb-1 pt-5 text-[11px] font-medium uppercase tracking-wider text-fg-dim">Manage</p>
          <Link
            to="/libraries"
            className={[
              'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
              location.pathname === '/libraries'
                ? 'bg-surface-3 text-fg'
                : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
            ].join(' ')}
            aria-current={location.pathname === '/libraries' ? 'page' : undefined}
          >
            <LibraryBig
              className={[
                'size-[18px]',
                location.pathname === '/libraries' ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted',
              ].join(' ')}
              aria-hidden
            />
            Libraries
          </Link>
          <Link
            to="/settings"
            className={[
              'group flex items-center gap-3 rounded-lg px-2.5 py-2 text-[13.5px] transition-colors',
              location.pathname === '/settings'
                ? 'bg-surface-3 text-fg'
                : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
            ].join(' ')}
            aria-current={location.pathname === '/settings' ? 'page' : undefined}
          >
            <Settings
              className={[
                'size-[18px]',
                location.pathname === '/settings' ? 'text-active' : 'text-fg-dim group-hover:text-fg-muted',
              ].join(' ')}
              aria-hidden
            />
            Settings
          </Link>
        </nav>

        <div className="border-t border-hairline px-5 py-3 text-[11px] text-fg-dim">
          v{config.data?.version ?? 'dev'} · local
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b border-hairline bg-surface/70 px-4 backdrop-blur">
          <Link to="/" className="grid size-8 place-items-center rounded-lg bg-surface-2 md:hidden" aria-label="Home">
            <Film className="size-4 text-active" aria-hidden />
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

          <div className="ml-auto hidden items-center gap-2 text-[12px] text-fg-dim sm:flex">
            <Link
              to="/libraries"
              className="rounded-lg border border-hairline px-3 py-1.5 text-fg-muted transition-colors hover:border-hairline-strong hover:text-fg"
            >
              Libraries
            </Link>
          </div>
        </header>

        <main className="min-h-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

