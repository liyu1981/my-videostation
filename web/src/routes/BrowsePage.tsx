import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { useAtom } from 'jotai';
import { CheckSquare, LayoutGrid, List, Rows3 } from 'lucide-react';
import { api } from '../lib/api';
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
  { value: 'recent_added', label: 'Recently Added' },
  { value: 'recent_watched', label: 'Recently Watched' },
  { value: 'recent_released', label: 'Recently Released' },
  { value: 'all', label: 'All' },
] as const;

const SORTS = [
  { value: '', label: 'Default' },
  { value: 'title', label: 'Title' },
  { value: 'year', label: 'Year' },
  { value: 'rating', label: 'Rating' },
  { value: 'runtime', label: 'Runtime' },
] as const;

const VIEW_ICONS: { value: ViewMode; label: string; Icon: typeof LayoutGrid }[] = [
  { value: 'poster', label: 'Thumbnail view', Icon: LayoutGrid },
  { value: 'tile', label: 'Tile view', Icon: Rows3 },
  { value: 'list', label: 'List view', Icon: List },
];

/** Library browser with Video Station's minimal toolbar: category, sort, view, selection. */
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

  const collections = useQuery({ queryKey: ['collections'], queryFn: api.collections });

  const items = useQuery({
    queryKey: ['browse', kind, search.libraryId ?? 0, search.sort, page, search.q, status, category],
    queryFn: () =>
      api.browse({
        kind,
        libraryId: search.libraryId,
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
        libraryId: search.libraryId,
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
      <header className="mb-5 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">
            {status === 'unmatched' ? 'Needs attention' : kindLabel(kind)}
          </h1>
          <p className="mt-1 text-[12px] text-fg-dim">{items.data ? `${items.data.total} items` : 'Loading…'}</p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <select
            value={category}
            onChange={(e) => update({ category: e.target.value })}
            className={selectClass}
            aria-label="Category"
          >
            {CATEGORIES.map((c) => (
              <option key={c.value} value={c.value}>{c.label}</option>
            ))}
          </select>

          <select value={search.sort} onChange={(e) => update({ sort: e.target.value })} className={selectClass} aria-label="Sort">
            {SORTS.map((s) => (
              <option key={s.value} value={s.value}>{s.label}</option>
            ))}
          </select>

          <div className="flex items-center gap-1 rounded-lg border border-hairline p-1">
            {VIEW_ICONS.map(({ value, label, Icon }) => (
              <button
                key={value}
                type="button"
                onClick={() => setView(value)}
                disabled={selecting}
                aria-label={label}
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
        </div>
      </header>

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
                  {userCollections.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
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
          title="Nothing here yet"
          description={
            status === 'unmatched'
              ? 'Everything is matched. New scans will surface items that need a manual match.'
              : 'This library is empty. Add a folder and scan it from Libraries.'
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
