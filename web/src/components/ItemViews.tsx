import { useAtomValue } from 'jotai';
import type { Item } from '../lib/api';
import { densityAtom } from '../atoms/ui';
import type { ViewMode } from '../atoms/ui';
import { ListRow, PosterCard, TileCard } from './Poster';

export function ItemViews({ items, view, bare = false }: { items: Item[]; view: ViewMode; bare?: boolean }) {
  const density = useAtomValue(densityAtom);
  const compact = density === 'compact';
  const gridClass = compact
    ? 'grid grid-cols-3 gap-x-3 gap-y-5 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-7 xl:grid-cols-8 2xl:grid-cols-9'
    : 'grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7';

  if (view === 'tile') {
    return (
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {items.map((item) => (
          <TileCard key={item.id} item={item} />
        ))}
      </div>
    );
  }

  if (view === 'list') {
    return (
      <div className="overflow-hidden rounded-[var(--radius-panel)] border border-hairline bg-surface/40">
        {items.map((item) => (
          <ListRow key={item.id} item={item} />
        ))}
      </div>
    );
  }

  return (
    <div className={bare ? 'flex gap-4 overflow-x-auto pb-2 no-scrollbar' : gridClass}>
      {items.map((item) => (
        <div key={item.id} className={bare ? 'w-[150px] shrink-0' : undefined}>
          <PosterCard item={item} />
        </div>
      ))}
    </div>
  );
}

export function Pagination({
  page,
  totalPages,
  total,
  onChange,
}: {
  page: number;
  totalPages: number;
  total: number;
  onChange: (page: number) => void;
}) {
  if (totalPages <= 1) {
    return <p className="text-[12px] text-fg-dim">{total} items</p>;
  }
  const pages: number[] = [];
  const start = Math.max(1, Math.min(page - 2, totalPages - 4));
  for (let p = start; p <= Math.min(totalPages, start + 4); p++) pages.push(p);

  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-[12px] text-fg-dim">
        {total} items · page {page} of {totalPages}
      </p>
      <div className="flex items-center gap-1">
        <button
          type="button"
          onClick={() => onChange(Math.max(1, page - 1))}
          disabled={page <= 1}
          className="h-8 rounded-lg border border-hairline px-3 text-[12px] text-fg-muted disabled:opacity-40 enabled:hover:border-hairline-strong enabled:hover:text-fg"
        >
          Prev
        </button>
        {pages.map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => onChange(p)}
            aria-current={p === page ? 'page' : undefined}
            className={[
              'h-8 min-w-8 rounded-lg border px-2 text-[12px]',
              p === page
                ? 'border-active/60 bg-active/15 text-fg'
                : 'border-hairline text-fg-muted hover:border-hairline-strong hover:text-fg',
            ].join(' ')}
          >
            {p}
          </button>
        ))}
        <button
          type="button"
          onClick={() => onChange(Math.min(totalPages, page + 1))}
          disabled={page >= totalPages}
          className="h-8 rounded-lg border border-hairline px-3 text-[12px] text-fg-muted disabled:opacity-40 enabled:hover:border-hairline-strong enabled:hover:text-fg"
        >
          Next
        </button>
      </div>
    </div>
  );
}
