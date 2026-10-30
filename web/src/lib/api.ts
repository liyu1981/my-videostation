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
  libraryName?: string;
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
  createdAt: string;
  updatedAt: string;
}

export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled';

export interface Job {
  id: number;
  type: string;
  status: JobStatus;
  progress: number;
  message?: string;
  error?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
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
  overview?: string;
  posterUrl?: string;
  backdropUrl?: string;
  rating?: number;
  mediaType: 'movie' | 'tv';
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
  year?: number;
  rating?: number;
  genres?: string[];
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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    ...init,
    headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
  });
  if (!res.ok) {
    let code = 'error';
    let message = res.statusText || 'request failed';
    try {
      const body = (await res.json()) as { error?: { code?: string; message?: string } };
      code = body.error?.code ?? code;
      message = body.error?.message ?? message;
    } catch {
      /* non-JSON error */
    }
    throw new ApiError(res.status, code, message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

function qs(params: Record<string, string | number | undefined>): string {
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
  }) =>
    request<BrowseResponse>(`/browse${qs(p)}`),
  search: (q: string, limit = 60) => request<{ items: Item[]; query: string }>(`/search${qs({ q, limit })}`),
  item: (id: number) => request<ItemDetail>(`/items/${id}`),
  patchItem: (id: number, patch: ItemPatch) =>
    request<ItemDetail>(`/items/${id}`, { method: 'PATCH', body: JSON.stringify(patch) }),
  progress: (id: number, body: { positionMs: number; durationMs: number; played: boolean }) =>
    request<{ ok: boolean }>(`/items/${id}/progress`, { method: 'POST', body: JSON.stringify(body) }),
  libraries: () => request<{ libraries: Library[] }>('/libraries'),
  library: (id: number) => request<Library>(`/libraries/${id}`),
  createLibrary: (body: { name: string; type: LibraryType; language?: string; paths: string[] }) =>
    request<Library>('/libraries', { method: 'POST', body: JSON.stringify(body) }),
  updateLibrary: (id: number, body: { name?: string; language?: string; paths?: string[] }) =>
    request<Library>(`/libraries/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteLibrary: (id: number) => request<void>(`/libraries/${id}`, { method: 'DELETE' }),
  scanLibrary: (id: number) => request<Job>(`/libraries/${id}/scan`, { method: 'POST' }),
  jobs: (limit = 20) => request<{ jobs: Job[] }>(`/jobs${qs({ limit })}`),
  job: (id: number) => request<Job>(`/jobs/${id}`),

  // metadata
  settings: () => request<Settings>('/settings'),
  saveSettings: (body: { tmdbApiKey?: string; language?: string; region?: string }) =>
    request<Settings>('/settings', { method: 'PUT', body: JSON.stringify(body) }),
  matches: (id: number, q?: string, year?: number) =>
    request<{ candidates: Candidate[] }>(`/items/${id}/matches${qs({ q, year })}`),
  match: (id: number, providerId: string) =>
    request<ItemDetail>(`/items/${id}/match`, { method: 'POST', body: JSON.stringify({ providerId }) }),
  refreshMetadata: (id: number) => request<ItemDetail>(`/items/${id}/refresh`, { method: 'POST' }),
  setArt: (id: number, kind: 'poster' | 'backdrop', url: string) =>
    request<ItemDetail>(`/items/${id}/art`, { method: 'POST', body: JSON.stringify({ kind, url }) }),
  enrichLibrary: (id: number) => request<Job>(`/libraries/${id}/enrich`, { method: 'POST' }),
};
