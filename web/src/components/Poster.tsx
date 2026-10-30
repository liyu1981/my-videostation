import { Link } from '@tanstack/react-router';
import { Film, Play } from 'lucide-react';
import type { Item } from '../lib/api';
import { episodeLabel, formatRuntime, posterGradient } from '../lib/format';

export function progressPercent(item: Item): number {
  if (!item.durationMs || item.durationMs <= 0 || item.positionMs <= 0) return 0;
  return Math.min(100, (item.positionMs / item.durationMs) * 100);
}

export function itemSubtitle(item: Item): string {
  if (item.kind === 'episode') {
    return [episodeLabel(item.seasonNumber, item.episodeNumber, item.episodeEnd), formatRuntime(item.durationMs)].filter(Boolean).join(' · ');
  }
  if (item.kind === 'season') return `${item.seasonNumber === 0 ? 'Specials' : `Season ${item.seasonNumber}`}`;
  if (item.kind === 'home_video') return new Date(item.addedAt).toLocaleDateString();
  if (item.kind === 'show') return item.year ? String(item.year) : 'TV Show';
  return [item.year, formatRuntime(item.runtimeMs)].filter(Boolean).join(' · ');
}

export function Artwork({ item, className = '' }: { item: Item; className?: string }) {
  if (item.posterUrl) {
    return (
      <img
        src={item.posterUrl}
        alt=""
        loading="lazy"
        decoding="async"
        className={`h-full w-full object-cover ${className}`}
      />
    );
  }
  return (
    <div
      className={`flex h-full w-full items-center justify-center ${className}`}
      style={{ background: posterGradient(item.title) }}
    >
      <Film className="size-7 text-white/20" aria-hidden />
    </div>
  );
}

export function PosterCard({ item }: { item: Item }) {
  const progress = progressPercent(item);
  return (
    <Link
      to="/items/$itemId"
      params={{ itemId: String(item.id) }}
      className="group card-hover block rounded-[var(--radius-card)] focus-visible:outline-none"
      title={item.title}
    >
      <div className="relative aspect-[2/3] overflow-hidden rounded-[var(--radius-card)] border border-hairline bg-surface-2">
        <Artwork item={item} className="transition-transform duration-300 group-hover:scale-[1.02]" />

        <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/75 via-black/0 to-black/0 opacity-0 transition-opacity duration-200 group-hover:opacity-100" />
        <div className="pointer-events-none absolute inset-x-0 bottom-0 flex items-center justify-center pb-3 opacity-0 transition-opacity duration-200 group-hover:opacity-100">
          <span className="grid size-9 place-items-center rounded-full bg-black/60 ring-1 ring-white/20 backdrop-blur">
            <Play className="ml-0.5 size-4 text-white" aria-hidden />
          </span>
        </div>

        {item.played ? (
          <span className="absolute left-2 top-2 rounded-full bg-black/65 px-2 py-0.5 text-[10px] font-medium tracking-wide text-fg backdrop-blur">
            Watched
          </span>
        ) : null}

        {item.kind === 'episode' && item.episodeNumber ? (
          <span className="absolute right-2 top-2 rounded-full bg-black/65 px-2 py-0.5 text-[10px] font-medium text-fg backdrop-blur">
            {episodeLabel(item.seasonNumber, item.episodeNumber, item.episodeEnd)}
          </span>
        ) : null}

        {progress > 0 ? (
          <div className="absolute inset-x-0 bottom-0 h-[3px] bg-black/60">
            <div className="h-full bg-progress" style={{ width: `${progress}%` }} />
          </div>
        ) : null}
      </div>

      <div className="mt-2 space-y-0.5">
        <p className="truncate text-[13.5px] font-medium text-fg">{item.title}</p>
        <p className="truncate text-[12px] text-fg-dim">{itemSubtitle(item)}</p>
      </div>
    </Link>
  );
}

export function TileCard({ item }: { item: Item }) {
  const progress = progressPercent(item);
  const firstFileBadge = item.kind === 'episode' ? episodeLabel(item.seasonNumber, item.episodeNumber, item.episodeEnd) : undefined;
  return (
    <Link
      to="/items/$itemId"
      params={{ itemId: String(item.id) }}
      className="group flex gap-4 rounded-[var(--radius-panel)] border border-hairline bg-surface p-3 transition-colors hover:border-hairline-strong hover:bg-surface-2"
    >
      <div className="relative h-[120px] w-20 shrink-0 overflow-hidden rounded-lg bg-surface-3">
        <Artwork item={item} />
        {progress > 0 ? (
          <div className="absolute inset-x-0 bottom-0 h-[3px] bg-black/60">
            <div className="h-full bg-progress" style={{ width: `${progress}%` }} />
          </div>
        ) : null}
      </div>
      <div className="min-w-0 flex-1 py-0.5">
        <div className="flex items-baseline gap-2">
          <p className="truncate text-[14px] font-medium text-fg">{item.title}</p>
          {firstFileBadge ? <span className="text-[11px] text-fg-dim">{firstFileBadge}</span> : null}
        </div>
        <p className="mt-0.5 text-[12px] text-fg-dim">{itemSubtitle(item)}</p>
        {item.overview ? (
          <p className="mt-2 line-clamp-2 text-[12.5px] leading-relaxed text-fg-muted">{item.overview}</p>
        ) : null}
        <div className="mt-2 flex items-center gap-2 text-[11px] text-fg-dim">
          {item.rating ? <span>★ {item.rating.toFixed(1)}</span> : null}
          {item.contentRating ? (
            <span className="rounded border border-hairline px-1.5 py-px">{item.contentRating}</span>
          ) : null}
        </div>
      </div>
    </Link>
  );
}

export function ListRow({ item }: { item: Item }) {
  const progress = progressPercent(item);
  return (
    <Link
      to="/items/$itemId"
      params={{ itemId: String(item.id) }}
      className="group flex items-center gap-4 border-b border-hairline px-3 py-2.5 transition-colors hover:bg-surface-2"
    >
      <div className="relative h-14 w-10 shrink-0 overflow-hidden rounded-md bg-surface-3">
        <Artwork item={item} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-[13.5px] font-medium text-fg">{item.title}</p>
          {item.kind === 'episode' && item.episodeNumber ? (
            <span className="shrink-0 text-[11px] text-fg-dim">{episodeLabel(item.seasonNumber, item.episodeNumber, item.episodeEnd)}</span>
          ) : null}
        </div>
        <p className="truncate text-[11.5px] text-fg-dim">{itemSubtitle(item)}</p>
        {progress > 0 ? (
          <div className="mt-1.5 h-[2px] w-full max-w-xs overflow-hidden rounded bg-black/50">
            <div className="h-full bg-progress" style={{ width: `${progress}%` }} />
          </div>
        ) : null}
      </div>
      <div className="hidden shrink-0 items-center gap-3 text-[11.5px] text-fg-dim sm:flex">
        {item.rating ? <span>★ {item.rating.toFixed(1)}</span> : null}
        {item.contentRating ? <span className="rounded border border-hairline px-1.5 py-px">{item.contentRating}</span> : null}
      </div>
    </Link>
  );
}
