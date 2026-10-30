import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { useAtom } from 'jotai';
import { CheckSquare, LayoutGrid, List, Rows3, Search, SlidersHorizontal, X } from 'lucide-react';
import { api, type Library } from '../lib/api';
import { kindLabel } from '../lib/format';
import { viewModeAtom } from '../atoms/ui';
import type { ViewMode } from '../atoms/ui';
import { EmptyState, ErrorState, Loading } from '../components/State';
import { ItemViews, Pagination } from '../components/ItemViews';

type BrowseSearch = {
  kind: string;
  libraryId?: number;
  sort: string;
  page: number;
  q: string;
  status?: string;
  category?: string;
  genre?: string;
  year?: number;
  watched?: boolean;
  folder?: string;
};

const CATEGORIES = [
  { value: 'recent_added', label: 'Recently added' },
  { value: 'recent_watched', label: 'Recently watched' },
  { value: 'recent_released', label: 'Recently released' },
  { value: 'all', label: 'All (A–Z)' },
] as const;

const SORTS = [
  { value: '', label: 'Default' },
  { value: 'title', label: 'Title' },
  { value: 'year', label: 'Year' },
  { value: 'rating', label: 'Rating' },
  { value: 'runtime', label: 'Runtime' },
] as const;

const VIEW_ICONS: { value: ViewMode; label: string; Icon: typeof LayoutGrid }[] = [
  { value: 'poster', label: 'Poster', Icon: LayoutGrid },
  { value: 'tile', label: 'Tile', Icon: Rows3 },
  { value: 'list', label: 'List', Icon: List },
];

export function BrowsePage() {
  const search = useSearch({ strict: false }) as BrowseSearch;
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [view, setView] = useAtom(viewModeAtom);

  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkCollection, setBulkCollection] = useState('');

  const kind = search.kind || 'movie';
  const page = search.page || 1;
  const status = search.status ?? '';
  const category = search.category ?? 'recent_added';
  const libraryId = search.libraryId;

  const libs = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const collections = useQuery({ queryKey: ['collections'], queryFn: api.collections });

  const kindLibs = (libs.data?.libraries ?? []).filter((l: Library) => {
    const want = kind === 'show' ? 'tv' : kind === 'home_video' ? 'home_video' : 'movie';
    return l.type === want;
  });
  const activeLib = libraryId ? kindLibs.find((l) => l.id === libraryId) : kindLibs[0];

  const facets = useQuery({
    queryKey: ['facets', activeLib?.id, kind],
    queryFn: () => api.facets(activeLib!.id, kind),
    enabled: Boolean(activeLib) && kind !== 'home_video',
  });
  const folders = useQuery({
    queryKey: ['folders', activeLib?.id],
    queryFn: () => api.folders(activeLib!.id),
    enabled: Boolean(activeLib),
  });

  const items = useQuery({
    queryKey: [
      'browse', kind, libraryId ?? 0, search.sort, page, search.q, status,
      category, search.genre, search.year, search.watched, search.folder,
    ],
    queryFn: () =>
      api.browse({
        kind,
        libraryId,
        sort: search.sort,
        page,
        perPage: 48,
        q: search.q,
        metadataStatus: status || undefined,
        category,
        genre: search.genre,
        year: search.year,
        watched: search.watched,
        folder: search.folder,
      }),
  });

  const bulk = useMutation({
    mutationFn: (action: 'watched' | 'unwatched' | 'add_collection') =>
      api.batch({
        action,
        itemIds: [...selected],
        collectionId: bulkCollection ? Number(bulkCollection) : undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['browse'] });
      void qc.invalidateQueries({ queryKey: ['home'] });
      void qc.invalidateQueries({ queryKey: ['collections'] });
      if (bulk.variables !== 'add_collection') setSelected(new Set());
    },
  });

  const update = (patch: Partial<BrowseSearch>) => {
    void navigate({
      to: '/browse',
      search: {
        kind,
        libraryId,
        sort: search.sort ?? '',
        page: 1,
        q: search.q ?? '',
        status: status || undefined,
        category,
        genre: search.genre,
        year: search.year,
        watched: search.watched,
        folder: search.folder,
        ...patch,
      } as never,
    });
  };

  const toggleSelected = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const userCollections = (collections.data?.collections ?? []).filter((c) => !c.systemKey);

  return (
    <div className="px-6 py-6 lg:px-8">
      <header className="mb-4 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">
            {status === 'unmatched' ? 'Needs attention' : kindLabel(kind)}
          </h1>
          <p className="mt-1 text-[12px] text-fg-dim">
            {items.data ? `${items.data.total} items` : 'Loading…'}
            {kindLibs.length > 1 ? ` · ${kindLibs.length} libraries` : ''}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {kindLibs.length > 1 ? (
            <select
              value={libraryId ?? ''}
              onChange={(e) => update({ libraryId: e.target.value ? Number(e.target.value) : undefined })}
              className={selectClass}
              aria-label="Library"
            >
              <option value="">All libraries</option>
              {kindLibs.map((l) => (
                <option key={l.id} value={l.id}>{l.name}</option>
              ))}
            </select>
          ) : null}

          <select value={search.sort} onChange={(e) => update({ sort: e.target.value })} className={selectClass} aria-label="Sort">
            {SORTS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
          </select>

          <div className="flex items-center gap-1 rounded-lg border border-hairline p-1">
            {VIEW_ICONS.map(({ value, label, Icon }) => (
              <button
                key={value}
                type="button"
                onClick={() => setView(value)}
                disabled={selecting}
                aria-label={`${label} view`}
                aria-pressed={view === value}
                className={[
                  'grid size-7 place-items-center rounded-md transition-colors disabled:opacity-40',
                  view === value ? 'bg-surface-3 text-fg' : 'text-fg-dim hover:text-fg-muted',
                ].join(' ')}
              >
                <Icon className="size-4" aria-hidden />
              </button>
            ))}
          </div>

          <button
            type="button"
            onClick={() => {
              setSelecting((v) => !v);
              setSelected(new Set());
            }}
            aria-pressed={selecting}
            className={[
              'grid size-8 place-items-center rounded-lg border transition-colors',
              selecting ? 'border-active/50 bg-active/15 text-fg' : 'border-hairline text-fg-dim hover:text-fg',
            ].join(' ')}
            aria-label="Selection mode"
            title="Selection mode"
          >
            <CheckSquare className="size-4" aria-hidden />
          </button>

          <label className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-fg-dim" aria-hidden />
            <input
              defaultValue={search.q ?? ''}
              onKeyDown={(e) => {
                if (e.key === 'Enter') update({ q: (e.target as HTMLInputElement).value });
              }}
              placeholder="Filter…"
              aria-label="Filter items"
              className="h-8 w-36 rounded-lg border border-hairline bg-surface-2 pl-8 pr-2 text-[12px] outline-none placeholder:text-fg-dim focus:border-hairline-strong"
            />
          </label>
        </div>
      </header>

      <div className="mb-5 flex flex-wrap items-center gap-2">
        <select value={category} onChange={(e) => update({ category: e.target.value })} className={selectClass} aria-label="Category">
          {CATEGORIES.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
        </select>

        {facets.data && facets.data.genres.length > 0 ? (
          <select value={search.genre ?? ''} onChange={(e) => update({ genre: e.target.value || undefined })} className={selectClass} aria-label="Genre">
            <option value="">All genres</option>
            {facets.data.genres.map((g) => <option key={g.value} value={g.value}>{g.value} ({g.count})</option>)}
          </select>
        ) : null}

        {facets.data && facets.data.years.length > 0 ? (
          <select
            value={search.year ?? ''}
            onChange={(e) => update({ year: e.target.value ? Number(e.target.value) : undefined })}
            className={selectClass}
            aria-label="Year"
          >
            <option value="">All years</option>
            {facets.data.years.map((y) => <option key={y.value} value={y.value}>{y.value} ({y.count})</option>)}
          </select>
        ) : null}

        <select
          value={search.watched === undefined ? '' : String(search.watched)}
          onChange={(e) => update({ watched: e.target.value === '' ? undefined : e.target.value === 'true' })}
          className={selectClass}
          aria-label="Watch state"
        >
          <option value="">Any state</option>
          <option value="false">Unwatched</option>
          <option value="true">Watched</option>
        </select>

        {folders.data && folders.data.folders.length > 1 ? (
          <select value={search.folder ?? ''} onChange={(e) => update({ folder: e.target.value || undefined })} className={selectClass} aria-label="Folder">
            <option value="">All folders</option>
            {folders.data.folders.map((f) => (
              <option key={f.path || '.'} value={f.path}>{f.path || '(root)'} ({f.count})</option>
            ))}
          </select>
        ) : null}

        {search.genre || search.year || search.watched !== undefined || search.folder || status ? (
          <button
            type="button"
            onClick={() => update({ genre: undefined, year: undefined, watched: undefined, folder: undefined, status: undefined, category: 'recent_added' })}
            className="inline-flex items-center gap-1 rounded-lg border border-hairline px-2.5 py-1.5 text-[11.5px] text-fg-dim hover:text-fg"
          >
            <X className="size-3" aria-hidden /> Clear filters
          </button>
        ) : null}
      </div>

      {selecting ? (
        <div className="sticky top-0 z-10 mb-4 flex flex-wrap items-center gap-2 rounded-[var(--radius-panel)] border border-active/40 bg-surface/95 px-3 py-2 backdrop-blur">
          <span className="text-[12.5px] font-medium">{selected.size} selected</span>
          <div className="ml-auto flex flex-wrap items-center gap-1.5">
            <button type="button" disabled={!selected.size || bulk.isPending} onClick={() => bulk.mutate('watched')} className={bulkBtn}>
              Mark watched
            </button>
            <button type="button" disabled={!selected.size || bulk.isPending} onClick={() => bulk.mutate('unwatched')} className={bulkBtn}>
              Mark unwatched
            </button>
            {userCollections.length > 0 ? (
              <>
                <select value={bulkCollection} onChange={(e) => setBulkCollection(e.target.value)} className={selectClass} aria-label="Collection">
                  <option value="">Collection…</option>
                  {userCollections.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                </select>
                <button
                  type="button"
                  disabled={!selected.size || !bulkCollection || bulk.isPending}
                  onClick={() => bulk.mutate('add_collection')}
                  className={bulkBtn}
                >
                  Add
                </button>
              </>
            ) : null}
            <button type="button" onClick={() => setSelected(new Set())} className={bulkBtn}>
              Clear
            </button>
            <button
              type="button"
              onClick={() => {
                setSelecting(false);
                setSelected(new Set());
              }}
              className="rounded-lg bg-surface-3 px-2.5 py-1.5 text-[12px] font-medium hover:bg-surface-3/80"
            >
              Done
            </button>
          </div>
        </div>
      ) : null}

      {items.isLoading ? <Loading /> : null}
      {items.isError ? <ErrorState error={items.error} /> : null}

      {items.data && items.data.items.length === 0 ? (
        <EmptyState
          icon={<SlidersHorizontal className="size-7" />}
          title="Nothing here yet"
          description={
            status === 'unmatched'
              ? 'Everything is matched. New scans will surface items that need a manual match.'
              : kindLibs.length === 0
                ? `No ${kindLabel(kind).toLowerCase()} library is configured. Add one and scan it.`
                : 'This library is empty or the filters exclude everything.'
          }
        />
      ) : null}

      {items.data && items.data.items.length > 0 ? (
        <div className="space-y-6">
          <ItemViews
            items={items.data.items}
            view={view}
            selection={selecting ? { selected, onToggle: toggleSelected } : undefined}
          />
          <Pagination
            page={items.data.page}
            totalPages={items.data.totalPages}
            total={items.data.total}
            onChange={(p) => update({ page: p })}
          />
        </div>
      ) : null}
    </div>
  );
}

const bulkBtn =
  'rounded-lg border border-hairline px-2.5 py-1.5 text-[12px] text-fg-muted hover:border-hairline-strong hover:text-fg disabled:cursor-not-allowed disabled:opacity-40';

const selectClass =
  'h-8 rounded-lg border border-hairline bg-surface-2 px-2.5 text-[12px] text-fg-muted outline-none hover:text-fg focus:border-hairline-strong';
