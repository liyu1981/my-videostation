import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router';
import { Shell } from './components/Shell';
import { HomePage } from './routes/HomePage';
import { BrowsePage } from './routes/BrowsePage';
import { DetailPage } from './routes/DetailPage';
import { SearchPage } from './routes/SearchPage';
import { LibrariesPage } from './routes/LibrariesPage';

const toNum = (v: unknown): number | undefined => {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return undefined;
};
const toStr = (v: unknown, fallback = ''): string => (typeof v === 'string' ? v : fallback);

const rootRoute = createRootRoute({ component: Shell });

const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: HomePage,
});

const browseRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/browse',
  validateSearch: (search: Record<string, unknown>) => ({
    kind: toStr(search.kind, 'movie'),
    libraryId: toNum(search.libraryId),
    sort: toStr(search.sort, 'added'),
    page: toNum(search.page) ?? 1,
    q: toStr(search.q),
  }),
  component: BrowsePage,
});

const detailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/items/$itemId',
  component: DetailPage,
});

const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/search',
  validateSearch: (search: Record<string, unknown>) => ({ q: toStr(search.q) }),
  component: SearchPage,
});

const librariesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/libraries',
  component: LibrariesPage,
});

const routeTree = rootRoute.addChildren([homeRoute, browseRoute, detailRoute, searchRoute, librariesRoute]);

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 5_000,
});

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
