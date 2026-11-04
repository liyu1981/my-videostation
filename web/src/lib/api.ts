export type Kind = 'movie' | 'show' | 'season' | 'episode' | 'home_video' | 'extra';
export type LibraryType = 'movie' | 'tv' | 'home_video' | 'tv_recording';

export interface Item {
  id: number;
  libraryId: number;
  parentId: number;
  kind: Kind;
  title: string;
  sortTitle: string;
  originalTitle?: string;
  year?: number;
  releaseDate?: string;
  overview?: string;
  tagline?: string;
  runtimeMs?: number;
  rating?: number;
  contentRating?: string;
  provider?: string;
  providerId?: string;
  metadataStatus: 'unmatched' | 'matched' | 'manual' | 'failed' | string;
  matchConfidence: number;
  seasonNumber?: number;
  episodeNumber?: number;
  episodeEnd?: number;
  addedAt: string;
  updatedAt: string;
  locked: boolean;
  hidden: boolean;
  positionMs: number;
  played: boolean;
  durationMs?: number;
  fileCount: number;
  childCount?: number;
  posterUrl?: string;
  backdropUrl?: string;
}

export interface AudioStream {
  index: number;
  codec: string;
  channels: number;
  layout?: string;
  sampleRate?: number;
  bitrate?: number;
  language?: string;
  title?: string;
  default?: boolean;
}

export interface SubtitleStream {
  index: number;
  codec: string;
  language?: string;
  title?: string;
  forced?: boolean;
  external?: boolean;
}

export interface MediaFile {
  id: number;
  libraryId: number;
  itemId: number;
  path: string;
  relPath: string;
  size: number;
  mtime: number;
  container?: string;
  durationMs?: number;
  bitrate?: number;
  width?: number;
  height?: number;
  videoCodec?: string;
  hdr?: string;
  audioStreams: AudioStream[];
  subtitleStreams: SubtitleStream[];
  probedAt?: string;
  missing: boolean;
  addedAt: string;
  streamUrl: string;
}

export interface Person {
  name: string;
  role: 'cast' | 'director' | 'writer' | 'producer';
  character?: string;
  ordering: number;
}

export interface ItemDetail extends Item {
  genres: string[];
  people: Person[];
  files: MediaFile[];
  locks: string[];
  children: Item[];
  extras: Item[];
  collectionIds: number[];
  libraryName?: string;
  showTitle?: string;
  showPosterUrl?: string;
  showBackdropUrl?: string;
}

export type UserRole = 'admin' | 'user';

export interface User {
  id: number;
  username: string;
  displayName: string;
  role: UserRole;
  active: boolean;
  ratingCeiling?: string;
  effectiveRatingCeiling?: string;
  birthYear?: number;
  createdAt: string;
  updatedAt: string;
}

export interface AuditEvent {
  id: number;
  userId?: number;
  username: string;
  action: string;
  target?: string;
  detail?: string;
  ip?: string;
  createdAt: string;
}

export interface LibraryAccess {
  id: number;
  name: string;
  allowed: boolean;
}

export interface AuthStatus {
  needsSetup: boolean;
}

export interface LibraryPath {
  id: number;
  path: string;
  recursive: boolean;
  lastScanAt?: string;
}

export interface Library {
  id: number;
  name: string;
  type: LibraryType;
  language: string;
  paths: LibraryPath[];
  itemCount: number;
  unmatchedCount: number;
  autoScanMinutes: number;
  watchEnabled: boolean;
  /** Glob patterns matched relative to each library folder; no "/" matches any segment. */
  excludeGlobs: string[];
  /** When set, only files matching one of these patterns are indexed. */
  includeGlobs: string[];
  minFileSizeMb: number;
  /** Do not index trailers/featurettes (samples are always skipped). */
  skipExtras: boolean;
  lastAutoScanAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface LibraryRulesInput {
  excludeGlobs?: string[];
  includeGlobs?: string[];
  minFileSizeMb?: number;
  skipExtras?: boolean;
}

export interface FSEntry {
  name: string;
  path: string;
}

export interface FSListing {
  path: string;
  parent?: string;
  roots: FSEntry[];
  entries: FSEntry[];
}

export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled';

export interface Job {
  id: number;
  type: string;
  libraryId?: number;
  status: JobStatus;
  progress: number;
  message?: string;
  error?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface JobLog {
  id: number;
  jobId: number;
  level: 'info' | 'warn' | 'error';
  message: string;
  createdAt: string;
}

export interface Home {
  continue: Item[];
  recent: Item[];
  shelves: { title: string; kind: Kind; items: Item[] }[];
}

export interface BrowseResponse {
  items: Item[];
  page: number;
  perPage: number;
  total: number;
  totalPages: number;
}

export interface Candidate {
  providerId: string;
  title: string;
  originalTitle?: string;
  year?: number;
  releaseDate?: string;
  overview?: string;
  posterUrl?: string;
  backdropUrl?: string;
  rating?: number;
  mediaType: 'movie' | 'tv';
  provider?: string;
}

export interface ProviderInfo {
  name: string;
  configured: boolean;
  kinds: string[];
}

export interface Collection {
  id: number;
  name: string;
  systemKey?: 'favorites' | 'watchlist';
  itemCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface Facets {
  genres: { value: string; count: number }[];
  years: { value: string; count: number }[];
}

export interface FolderEntry {
  path: string;
  count: number;
}

export interface PlaybackSubtitle {
  id: string;
  label: string;
  language?: string;
  url: string;
  source: 'external' | 'embedded';
  /** Image subtitles (PGS/VobSub) can only be burned in by a transcode. */
  burnIn?: boolean;
  default?: boolean;
}

export interface PlaybackAudio {
  index: number;
  codec: string;
  channels: number;
  language?: string;
  title?: string;
  default?: boolean;
  /** Playable without transcoding. */
  direct: boolean;
}

export interface QualityOption {
  label: string;
  height: number;
}

export interface TranscodeRequest {
  fileId?: number;
  startMs?: number;
  audioIndex?: number;
  subtitleIndex?: number;
  externalSubtitleId?: number;
  maxHeight?: number;
  mode?: 'remux' | 'transcode';
}

export interface TranscodeSession {
  sessionId: string;
  playlistUrl: string;
  startMs: number;
  durationMs: number;
  mode: 'remux' | 'transcode';
  height: number;
  videoCodec?: string;
  audioCodec?: string;
  audioIndex: number;
  reasons?: string[];
}

export interface TranscodeStatus extends TranscodeSession {
  encodedMs: number;
  speed: number;
  running: boolean;
  error?: string;
}

export interface PlaybackCapabilities {
  directPlay: {
    containers: string[];
    videoCodecs: string[];
    audioCodecs: string[];
  };
  transcoding: {
    available: boolean;
    encoder: string;
    encoders: { name: string; label: string }[];
    qualities: QualityOption[];
  };
}

export interface CastInfo {
  url?: string;
  compatible: boolean;
  reachable: boolean;
  reason?: string;
}

export interface BackupArchive {
  name: string;
  path?: string;
  size: number;
  createdAt: string;
}

export interface BackupStatus {
  maintenance: boolean;
  pendingImport: boolean;
}

export interface DLNADevice {
  id: string;
  name: string;
  controlUrl: string;
}

export interface PlaybackInfo {
  itemId: number;
  fileId: number;
  streamUrl: string;
  resumeMs: number;
  durationMs: number;
  subtitles: PlaybackSubtitle[];
  audio: PlaybackAudio[];
  container?: string;
  videoCodec?: string;
  width?: number;
  height?: number;
  directPlay: boolean;
  hls: boolean;
  reasons?: string[];
  qualities: QualityOption[];
  next?: { itemId: number; title: string; seasonNumber?: number; episodeNumber?: number };
  cast: CastInfo;
}

export interface Settings {
  metadata: {
    apiKeySet: boolean;
    language: string;
    region: string;
  };
}

export interface ItemPatch {
  title?: string;
  originalTitle?: string;
  overview?: string;
  tagline?: string;
  contentRating?: string;
  releaseDate?: string;
  year?: number;
  rating?: number;
  genres?: string[];
  cast?: { name: string; character?: string }[];
  locked?: boolean;
  locks?: string[];
}

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function responseError(res: Response): Promise<ApiError> {
  let code = 'error';
  let message = res.statusText || 'request failed';
  try {
    const body = (await res.json()) as { error?: { code?: string; message?: string } };
    code = body.error?.code ?? code;
    message = body.error?.message ?? message;
  } catch {
    /* non-JSON error */
  }
  return new ApiError(res.status, code, message);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    ...init,
    headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
  });
  if (!res.ok) throw await responseError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

async function requestForm<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(`/api/v1${path}`, { method: 'POST', body: form, headers: { Accept: 'application/json' } });
  if (!res.ok) throw await responseError(res);
  return (await res.json()) as T;
}

function qs(params: Record<string, string | number | boolean | undefined>): string {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : '';
}

export const api = {
  home: () => request<Home>('/home'),
  browse: (p: {
    libraryId?: number;
    kind?: string;
    sort?: string;
    page?: number;
    perPage?: number;
    q?: string;
    metadataStatus?: string;
    category?: string;
    genre?: string;
    year?: number;
    watched?: boolean;
    folder?: string;
  }) =>
    request<BrowseResponse>(`/browse${qs(p)}`),
  search: (q: string, limit = 60) => request<{ items: Item[]; query: string }>(`/search${qs({ q, limit })}`),
  item: (id: number) => request<ItemDetail>(`/items/${id}`),
  patchItem: (id: number, patch: ItemPatch) =>
    request<ItemDetail>(`/items/${id}`, { method: 'PATCH', body: JSON.stringify(patch) }),
  playback: (id: number) => request<PlaybackInfo>(`/items/${id}/playback`),
  transcode: (id: number, body: TranscodeRequest) =>
    request<TranscodeSession>(`/playback/${id}/transcode`, { method: 'POST', body: JSON.stringify(body) }),
  transcodeStatus: (sessionId: string) => request<TranscodeStatus>(`/playback/sessions/${sessionId}`),
  stopTranscode: (sessionId: string) =>
    request<{ ok: boolean }>(`/playback/sessions/${sessionId}`, { method: 'DELETE' }).catch(() => ({ ok: false })),
  capabilities: () => request<PlaybackCapabilities>('/playback/capabilities'),
  neighbors: (id: number) => request<{ prev?: Item; next?: Item }>(`/items/${id}/neighbors`),
  progress: (id: number, body: { positionMs: number; durationMs: number; played: boolean }) =>
    request<{ ok: boolean }>(`/items/${id}/progress`, { method: 'POST', body: JSON.stringify(body) }),
  castDLNADevices: () => request<{ devices: DLNADevice[] }>('/cast/dlna/discover', { method: 'POST' }),
  castDLNAPlay: (fileId: number, controlUrl: string, title: string) =>
    request<{ ok: boolean }>('/cast/dlna/play', {
      method: 'POST',
      body: JSON.stringify({ fileId, controlUrl, title }),
    }),
  authStatus: () => request<AuthStatus>('/auth/status'),
  me: () => request<{ user: User }>('/auth/me'),
  login: (username: string, password: string) =>
    request<{ user: User }>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  setup: (username: string, displayName: string, password: string) =>
    request<{ user: User }>('/auth/setup', { method: 'POST', body: JSON.stringify({ username, displayName, password }) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<void>('/auth/password', {
      method: 'POST',
      body: JSON.stringify({ currentPassword, newPassword }),
    }),
  users: () => request<{ users: User[] }>('/users'),
  createUser: (body: { username: string; displayName: string; password: string; role: UserRole; birthYear?: number }) =>
    request<User>('/users', { method: 'POST', body: JSON.stringify(body) }),
  updateUser: (id: number, body: { displayName?: string; role?: UserRole; active?: boolean; password?: string; ratingCeiling?: string; birthYear?: number }) =>
    request<User>(`/users/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteUser: (id: number) => request<void>(`/users/${id}`, { method: 'DELETE' }),
  userLibraries: (id: number) => request<{ libraries: LibraryAccess[] }>(`/users/${id}/libraries`),
  setUserLibraries: (id: number, libraryIds: number[], unrestricted = false) =>
    request<{ libraries: LibraryAccess[] }>(`/users/${id}/libraries`, {
      method: 'PUT',
      body: JSON.stringify({ libraryIds, unrestricted }),
    }),
  audit: (limit = 100) => request<{ events: AuditEvent[] }>(`/audit${qs({ limit })}`),
  backupStatus: () => request<BackupStatus>('/backup/status'),
  backupExports: () => request<{ exports: BackupArchive[] }>('/backup/exports'),
  backupImports: () => request<{ imports: BackupArchive[] }>('/backup/imports'),
  createExport: () => request<BackupArchive>('/backup/export', { method: 'POST' }),
  importBackup: (name: string) =>
    request<{ status: string; name: string; export: string }>('/backup/import', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  libraries: () => request<{ libraries: Library[] }>('/libraries'),
  listFS: (path?: string) => request<FSListing>(`/fs${qs({ path })}`),
  library: (id: number) => request<Library>(`/libraries/${id}`),
  createLibrary: (body: { name: string; type: LibraryType; language?: string; paths: string[]; autoScanMinutes?: number; watchEnabled?: boolean } & LibraryRulesInput) =>
    request<Library>('/libraries', { method: 'POST', body: JSON.stringify(body) }),
  updateLibrary: (id: number, body: { name?: string; language?: string; paths?: string[]; autoScanMinutes?: number; watchEnabled?: boolean } & LibraryRulesInput) =>
    request<Library>(`/libraries/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteLibrary: (id: number) => request<void>(`/libraries/${id}`, { method: 'DELETE' }),
  scanLibrary: (id: number, force = false) =>
    request<Job>(`/libraries/${id}/scan`, { method: 'POST', body: JSON.stringify({ force }) }),
  jobs: (limit = 20) => request<{ jobs: Job[] }>(`/jobs${qs({ limit })}`),
  job: (id: number) => request<Job>(`/jobs/${id}`),
  jobLogs: (id: number, limit = 5000) => request<{ logs: JobLog[] }>(`/jobs/${id}/logs${qs({ limit })}`),
  deleteJob: (id: number) => request<void>(`/jobs/${id}`, { method: 'DELETE' }),
  clearFinishedJobs: () => request<{ deleted: number }>('/jobs', { method: 'DELETE' }),

  // metadata
  settings: () => request<Settings>('/settings'),
  saveSettings: (body: { tmdbApiKey?: string; language?: string; region?: string }) =>
    request<Settings>('/settings', { method: 'PUT', body: JSON.stringify(body) }),
  providers: () => request<{ providers: ProviderInfo[] }>('/providers'),
  matches: (id: number, q?: string, year?: number, language?: string, provider?: string) =>
    request<{ candidates: Candidate[] }>(`/items/${id}/matches${qs({ q, year, language, provider })}`),
  duplicate: (id: number) => request<{ item?: Item }>(`/items/${id}/duplicate`),
  applyLocal: (id: number, sourceId: number) =>
    request<ItemDetail>(`/items/${id}/apply-local`, { method: 'POST', body: JSON.stringify({ sourceId }) }),
  mergeItems: (id: number, fromId: number) =>
    request<ItemDetail>(`/items/${id}/merge`, { method: 'POST', body: JSON.stringify({ fromId }) }),
  keepSeparate: (id: number, otherId: number) =>
    request<{ ok: boolean }>(`/items/${id}/keep-separate`, { method: 'POST', body: JSON.stringify({ otherId }) }),
  match: (id: number, providerId: string, language?: string, season?: number, episode?: number, provider?: string) =>
    request<ItemDetail>(`/items/${id}/match`, {
      method: 'POST',
      body: JSON.stringify({ providerId, provider, language, season, episode }),
    }),
  refreshMetadata: (id: number, language?: string) =>
    request<ItemDetail>(`/items/${id}/refresh${qs({ language })}`, { method: 'POST' }),
  setArt: (id: number, kind: 'poster' | 'backdrop', url: string) =>
    request<ItemDetail>(`/items/${id}/art`, { method: 'POST', body: JSON.stringify({ kind, url }) }),
  uploadArt: (id: number, kind: 'poster' | 'backdrop', file: File) => {
    const form = new FormData();
    form.append('kind', kind);
    form.append('file', file);
    return requestForm<ItemDetail>(`/items/${id}/art/upload`, form);
  },
  enrichLibrary: (id: number) => request<Job>(`/libraries/${id}/enrich`, { method: 'POST' }),

  // collections
  collections: () => request<{ collections: Collection[] }>('/collections'),
  createCollection: (name: string) =>
    request<Collection>('/collections', { method: 'POST', body: JSON.stringify({ name }) }),
  renameCollection: (id: number, name: string) =>
    request<Collection>(`/collections/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) }),
  deleteCollection: (id: number) => request<void>(`/collections/${id}`, { method: 'DELETE' }),
  collectionItems: (id: number, page = 1) =>
    request<{ items: Item[]; total: number; page: number }>(`/collections/${id}/items${qs({ page, perPage: 60 })}`),
  addToCollection: (id: number, itemIds: number[]) =>
    request<Collection>(`/collections/${id}/items`, { method: 'POST', body: JSON.stringify({ itemIds }) }),
  removeFromCollection: (id: number, itemId: number) =>
    request<void>(`/collections/${id}/items/${itemId}`, { method: 'DELETE' }),

  // bulk + facets
  batch: (body: { action: 'watched' | 'unwatched' | 'add_collection' | 'remove_collection'; itemIds: number[]; collectionId?: number }) =>
    request<{ ok: boolean; applied: number }>('/items/batch', { method: 'POST', body: JSON.stringify(body) }),
  facets: (libraryId: number, kind: string) =>
    request<Facets>(`/libraries/${libraryId}/facets${qs({ kind })}`),
  folders: (libraryId: number) => request<{ folders: FolderEntry[] }>(`/libraries/${libraryId}/folders`),
};
