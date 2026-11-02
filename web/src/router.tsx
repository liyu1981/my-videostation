import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router';
import { Shell } from './components/Shell';
import { HomePage } from './routes/HomePage';
import { BrowsePage } from './routes/BrowsePage';
import { DetailPage } from './routes/DetailPage';
import { SearchPage } from './routes/SearchPage';
import { LibrariesPage } from './routes/LibrariesPage';
import { SettingsPage } from './routes/SettingsPage';
import { PlaylistsPage } from './routes/PlaylistsPage';
import { CollectionPage } from './routes/CollectionPage';
import { TasksPage } from './routes/TasksPage';
import { LoginPage } from './routes/LoginPage';
import { SetupPage } from './routes/SetupPage';
import { UsersPage } from './routes/UsersPage';
import { ActivityPage } from './routes/ActivityPage';

const toNum = (v: unknown): number | undefined => {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return undefined;
};
const toStr = (v: unknown, fallback = ''): string => (typeof v === 'string' ? v : fallback);
const toBool = (v: unknown): boolean | undefined => {
  if (v === true || v === 'true') return true;
  if (v === false || v === 'false') return false;
  return undefined;
};

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
    sort: toStr(search.sort),
    page: toNum(search.page) ?? 1,
    q: toStr(search.q),
    status: toStr(search.status),
    category: toStr(search.category, 'recent_added'),
    genre: toStr(search.genre),
    year: toNum(search.year),
    watched: toBool(search.watched),
    folder: toStr(search.folder),
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

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsPage,
});

const playlistsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/playlists',
  component: PlaylistsPage,
});

const collectionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/collections/$collectionId',
  component: CollectionPage,
});

const tasksRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/tasks',
  component: TasksPage,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  component: LoginPage,
});

const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/setup',
  component: SetupPage,
});

const usersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/users',
  component: UsersPage,
});

const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/activity',
  component: ActivityPage,
});

const routeTree = rootRoute.addChildren([
  homeRoute,
  browseRoute,
  detailRoute,
  searchRoute,
  librariesRoute,
  settingsRoute,
  playlistsRoute,
  collectionRoute,
  tasksRoute,
  loginRoute,
  setupRoute,
  usersRoute,
  activityRoute,
]);

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
