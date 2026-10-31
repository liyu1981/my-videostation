import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import type { Item } from '../lib/api';
import { PosterCard } from './Poster';

/**
 * Horizontally scrollable poster shelf, Video-Station style: small posters,
 * arrow paging, "View all" link.
 */
export function MediaRow({
  title,
  items,
  icon,
  viewAll,
  showCount = true,
  posterWidth = 120,
}: {
  title: string;
  items: Item[];
  icon?: React.ReactNode;
  viewAll?: { kind: string; libraryId?: number; status?: string };
  showCount?: boolean;
  posterWidth?: number;
}) {
  const scroller = useRef<HTMLDivElement | null>(null);
  const [canLeft, setCanLeft] = useState(false);
  const [canRight, setCanRight] = useState(false);

  const update = useCallback(() => {
    const el = scroller.current;
    if (!el) return;
    setCanLeft(el.scrollLeft > 4);
    setCanRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 4);
  }, []);

  useEffect(() => {
    update();
    const el = scroller.current;
    if (!el) return;
    el.addEventListener('scroll', update, { passive: true });
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => {
      el.removeEventListener('scroll', update);
      ro.disconnect();
    };
  }, [update, items.length]);

  if (items.length === 0) return null;

  const page = (dir: number) => {
    const el = scroller.current;
    if (!el) return;
    el.scrollBy({ left: dir * el.clientWidth * 0.85, behavior: 'smooth' });
  };

  return (
    <section aria-label={title}>
      <div className="mb-2 flex items-center justify-between gap-3">
        <h2 className="flex min-w-0 items-center gap-2 text-[17px] font-semibold tracking-tight text-fg-heading">
          {icon ? <span className="text-fg-dim">{icon}</span> : null}
          <span className="truncate">{title}</span>
          {showCount ? <span className="shrink-0 text-[11.5px] font-normal text-fg-dim">{items.length}</span> : null}
        </h2>
        <div className="flex shrink-0 items-center gap-1">
          {viewAll ? (
            <Link
              to="/browse"
              search={{
                kind: viewAll.kind,
                libraryId: viewAll.libraryId,
                sort: '',
                page: 1,
                q: '',
                category: 'recent_added',
                status: viewAll.status ?? '',
                genre: '',
                year: undefined,
                watched: undefined,
                folder: '',
              }}
              className="mr-1 text-[11.5px] text-fg-dim hover:text-fg"
            >
              View all
            </Link>
          ) : null}
          <button
            type="button"
            onClick={() => page(-1)}
            disabled={!canLeft}
            aria-label={`Scroll ${title} left`}
            className="grid size-7 place-items-center rounded-md border border-hairline text-fg-dim enabled:hover:border-hairline-strong enabled:hover:text-fg disabled:opacity-30"
          >
            <ChevronLeft className="size-4" aria-hidden />
          </button>
          <button
            type="button"
            onClick={() => page(1)}
            disabled={!canRight}
            aria-label={`Scroll ${title} right`}
            className="grid size-7 place-items-center rounded-md border border-hairline text-fg-dim enabled:hover:border-hairline-strong enabled:hover:text-fg disabled:opacity-30"
          >
            <ChevronRight className="size-4" aria-hidden />
          </button>
        </div>
      </div>

      <div
        ref={scroller}
        className="no-scrollbar flex gap-4 overflow-x-auto scroll-smooth pb-1"
        style={{ scrollSnapType: 'x proximity' }}
      >
        {items.map((item) => (
          <div key={item.id} className="shrink-0" style={{ width: posterWidth, scrollSnapAlign: 'start' }}>
            <PosterCard item={item} />
          </div>
        ))}
      </div>
    </section>
  );
}
