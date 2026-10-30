import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { useAtom } from 'jotai';
import { LayoutGrid, List, Rows3, Search, SlidersHorizontal } from 'lucide-react';
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
};

const SORTS = [
  { value: 'added', label: 'Recently added' },
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
  const [view, setView] = useAtom(viewModeAtom);

  const kind = search.kind || 'movie';
  const page = search.page || 1;
  const sort = search.sort || 'added';
  const libraryId = search.libraryId;
  const status = search.status ?? '';

  const libs = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const items = useQuery({
    queryKey: ['browse', kind, libraryId ?? 0, sort, page, search.q, status],
    queryFn: () =>
      api.browse({ kind, libraryId, sort, page, perPage: 48, q: search.q, metadataStatus: status || undefined }),
  });

  const kindLibs = (libs.data?.libraries ?? []).filter((l: Library) => {
    const want = kind === 'show' ? 'tv' : kind === 'home_video' ? 'home_video' : 'movie';
    return l.type === want;
  });

  const update = (patch: Partial<BrowseSearch>) => {
    void navigate({
      to: '/browse',
      search: {
        kind,
        libraryId,
        sort,
        page: 1,
        q: search.q ?? '',
        status: status || undefined,
        ...patch,
      } as never,
    });
  };

  return (
    <div className="px-6 py-6 lg:px-8">
      <header className="mb-5 flex flex-wrap items-end justify-between gap-4">
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
              aria-label="Filter by library"
            >
              <option value="">All libraries</option>
              {kindLibs.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.name}
                </option>
              ))}
            </select>
          ) : null}

          <select value={sort} onChange={(e) => update({ sort: e.target.value })} className={selectClass} aria-label="Sort">
            {SORTS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>

          <div className="flex items-center gap-1 rounded-lg border border-hairline p-1">
            {VIEW_ICONS.map(({ value, label, Icon }) => (
              <button
                key={value}
                type="button"
                onClick={() => setView(value)}
                aria-label={`${label} view`}
                aria-pressed={view === value}
                className={[
                  'grid size-7 place-items-center rounded-md transition-colors',
                  view === value ? 'bg-surface-3 text-fg' : 'text-fg-dim hover:text-fg-muted',
                ].join(' ')}
              >
                <Icon className="size-4" aria-hidden />
              </button>
            ))}
          </div>

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
                : 'This library is empty or the filter excludes everything.'
          }
        />
      ) : null}

      {items.data && items.data.items.length > 0 ? (
        <div className="space-y-6">
          <ItemViews items={items.data.items} view={view} />
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

const selectClass =
  'h-8 rounded-lg border border-hairline bg-surface-2 px-2.5 text-[12px] text-fg-muted outline-none focus:border-hairline-strong hover:text-fg';
