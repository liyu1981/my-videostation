import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from '@tanstack/react-router';
import { useAtom } from 'jotai';
import { ArrowLeft, Bookmark, Check, Heart, Lock, Pencil, Play, RefreshCw, SearchCheck } from 'lucide-react';
import { api } from '../lib/api';
import type { Item } from '../lib/api';
import { episodeLabel, formatRuntime } from '../lib/format';
import { playerAtom } from '../atoms/ui';
import { ErrorState, Loading } from '../components/State';
import { ItemViews } from '../components/ItemViews';
import { MediaInfoPanel } from '../components/MediaInfo';
import { EditDialog } from '../components/EditDialog';
import { PlayerModal } from '../components/Player';
import { IdentifyDialog } from '../components/IdentifyDialog';
import { Artwork } from '../components/Poster';

export function DetailPage() {
  const params = useParams({ strict: false }) as { itemId?: string };
  const id = Number(params.itemId ?? 0);
  const qc = useQueryClient();
  const [, setPlayer] = useAtom(playerAtom);
  const [editing, setEditing] = useState(false);
  const [identifying, setIdentifying] = useState(false);
  const [seasonId, setSeasonId] = useState<number | null>(null);

  const item = useQuery({ queryKey: ['item', id], queryFn: () => api.item(id), enabled: id > 0 });
  const collections = useQuery({ queryKey: ['collections'], queryFn: api.collections });
  const parentId = item.data?.parentId ?? 0;
  const parent = useQuery({ queryKey: ['item', parentId], queryFn: () => api.item(parentId), enabled: parentId > 0 });
  const grandParentId = parent.data?.parentId ?? 0;
  const grandParent = useQuery({
    queryKey: ['item', grandParentId],
    queryFn: () => api.item(grandParentId),
    enabled: grandParentId > 0,
  });

  const isShow = item.data?.kind === 'show';
  const seasons = useMemo(() => (isShow ? (item.data?.children ?? []) : []), [isShow, item.data?.children]);
  const currentSeasonId = seasonId ?? seasons[0]?.id ?? 0;
  const seasonDetail = useQuery({
    queryKey: ['item', currentSeasonId],
    queryFn: () => api.item(currentSeasonId),
    enabled: isShow && currentSeasonId > 0,
  });

  const toggleCollection = useMutation({
    mutationFn: async ({ collectionId, member }: { collectionId: number; member: boolean }) =>
      member ? api.removeFromCollection(collectionId, id) : api.addToCollection(collectionId, [id]),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['item', id] });
      void qc.invalidateQueries({ queryKey: ['collections'] });
      void qc.invalidateQueries({ queryKey: ['home'] });
    },
  });

  const addToCollection = useMutation({
    mutationFn: (collectionId: number) => api.addToCollection(collectionId, [id]),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['item', id] });
      void qc.invalidateQueries({ queryKey: ['collections'] });
    },
  });

  const markWatched = useMutation({
    mutationFn: (played: boolean) =>
      api.progress(id, {
        positionMs: played ? (item.data?.durationMs ?? item.data?.runtimeMs ?? 0) : 0,
        durationMs: item.data?.durationMs ?? item.data?.runtimeMs ?? 0,
        played,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['item', id] });
      void qc.invalidateQueries({ queryKey: ['home'] });
      void qc.invalidateQueries({ queryKey: ['browse'] });
    },
  });

  const refreshMeta = useMutation({
    mutationFn: () => api.refreshMetadata(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['item', id] });
      void qc.invalidateQueries({ queryKey: ['browse'] });
      void qc.invalidateQueries({ queryKey: ['home'] });
    },
  });

  if (item.isLoading) return <Loading />;
  if (item.isError) return <ErrorState error={item.error} />;

  const detail = item.data!;
  const favoritesId = collections.data?.collections.find((c) => c.systemKey === 'favorites')?.id ?? 0;
  const watchlistId = collections.data?.collections.find((c) => c.systemKey === 'watchlist')?.id ?? 0;
  const isFavorite = favoritesId > 0 && detail.collectionIds.includes(favoritesId);
  const isWatchlisted = watchlistId > 0 && detail.collectionIds.includes(watchlistId);
  const userCollections = (collections.data?.collections ?? []).filter((c) => !c.systemKey);
  const playable = detail.files.find((f) => !f.missing);
  const episodes: Item[] = isShow ? (seasonDetail.data?.children ?? []) : detail.children;

  const play = () => {
    if (!playable) return;
    setPlayer({ itemId: detail.id, title: detail.title });
  };

  const breadcrumb: { label: string; to?: string; search?: Record<string, unknown> }[] = [];
  if (detail.libraryName) breadcrumb.push({ label: detail.libraryName, to: '/browse', search: { kind: kindFor(detail.kind) } });
  if (grandParent.data) breadcrumb.push({ label: grandParent.data.title, to: `/items/${grandParent.data.id}` as string });
  if (parent.data) breadcrumb.push({ label: parent.data.title, to: `/items/${parent.data.id}` as string });
  if (detail.kind === 'episode' || detail.kind === 'season') {
    breadcrumb.push({ label: detail.kind === 'episode' ? episodeLabel(detail.seasonNumber, detail.episodeNumber, detail.episodeEnd) : detail.title });
  }

  return (
    <div className="pb-16">
      <div className="relative">
        {detail.backdropUrl ? (
          <div className="pointer-events-none absolute inset-x-0 top-0 h-[340px] overflow-hidden">
            <img src={detail.backdropUrl} alt="" className="h-full w-full object-cover opacity-30" />
            <div className="absolute inset-0 bg-gradient-to-b from-bg/40 via-bg/70 to-bg" />
          </div>
        ) : null}

        <div className="relative px-6 pt-5 lg:px-8">
          <div className="flex flex-wrap items-center gap-2 text-[11.5px] text-fg-dim">
            <button
              type="button"
              onClick={() => window.history.back()}
              className="mr-1 inline-flex items-center gap-1 rounded-md px-1.5 py-1 hover:bg-surface-2 hover:text-fg"
            >
              <ArrowLeft className="size-3.5" aria-hidden /> Back
            </button>
            {breadcrumb.map((c, i) => (
              <span key={`${c.label}-${i}`} className="flex items-center gap-2">
                {i > 0 ? <span className="text-fg-dim/60">/</span> : null}
                {c.to ? (
                  <Link to={c.to as never} className="hover:text-fg">
                    {c.label}
                  </Link>
                ) : (
                  <span className="text-fg-muted">{c.label}</span>
                )}
              </span>
            ))}
          </div>

          <div className="mt-5 flex flex-col gap-6 sm:flex-row">
            <div className="group relative w-40 shrink-0 self-start sm:w-48">
              <div className="aspect-[2/3] overflow-hidden rounded-xl border border-hairline bg-surface-2 shadow-2xl">
                <Artwork item={detail} />
              </div>
              {playable ? (
                <button
                  type="button"
                  onClick={play}
                  aria-label={`Play ${detail.title}`}
                  className="absolute inset-0 m-auto grid size-14 place-items-center rounded-full bg-black/55 ring-1 ring-white/25 backdrop-blur transition-colors hover:bg-black/70"
                >
                  <Play className="ml-0.5 size-6 text-white" aria-hidden />
                </button>
              ) : null}
              {detail.locked || detail.locks.length > 0 ? (
                <span className="absolute -right-2 -top-2 grid size-7 place-items-center rounded-full border border-hairline bg-surface-3 text-fg-muted" title="Metadata locked">
                  <Lock className="size-3.5" aria-hidden />
                </span>
              ) : null}
            </div>

            <div className="min-w-0 flex-1">
              <h1 className="text-[26px] font-semibold leading-tight tracking-tight text-fg-heading sm:text-[32px]">
                {detail.title}
              </h1>
              {detail.tagline ? <p className="mt-1 text-[13.5px] italic text-fg-muted">{detail.tagline}</p> : null}

              <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-[12.5px] text-fg-muted">
                {detail.year ? <span>{detail.year}</span> : null}
                {detail.kind === 'episode' ? (
                  <span>{episodeLabel(detail.seasonNumber, detail.episodeNumber, detail.episodeEnd)}</span>
                ) : null}
                {detail.runtimeMs ? <span>{formatRuntime(detail.runtimeMs)}</span> : null}
                {detail.rating ? <span className="text-rating">★ {detail.rating.toFixed(1)}</span> : null}
                {detail.contentRating ? (
                  <span className="rounded border border-hairline-strong px-1.5 py-px text-[11px] text-fg-muted">{detail.contentRating}</span>
                ) : null}
                {detail.played ? <span className="rounded bg-surface-3 px-2 py-0.5 text-[11px] text-fg-muted">Watched</span> : null}
                <MetaStatus status={detail.metadataStatus} confidence={detail.matchConfidence} provider={detail.provider} />
              </div>

              {detail.genres.length > 0 ? (
                <div className="mt-2.5 flex flex-wrap gap-1.5">
                  {detail.genres.map((g) => (
                    <span key={g} className="rounded-full border border-hairline px-2.5 py-0.5 text-[11.5px] text-fg-muted">
                      {g}
                    </span>
                  ))}
                </div>
              ) : null}

              <div className="mt-5 flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  onClick={play}
                  disabled={!playable}
                  className="inline-flex items-center gap-2 rounded-lg bg-progress/90 px-4 py-2 text-[13px] font-medium text-[#062027] transition-colors hover:bg-progress disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <Play className="size-4" aria-hidden /> Play
                </button>
                <button
                  type="button"
                  onClick={() => markWatched.mutate(!detail.played)}
                  disabled={markWatched.isPending}
                  className="inline-flex items-center gap-2 rounded-lg border border-hairline px-3.5 py-2 text-[12.5px] text-fg-muted transition-colors hover:border-hairline-strong hover:text-fg disabled:opacity-60"
                >
                  <Check className="size-4" aria-hidden />
                  {detail.played ? 'Mark unwatched' : 'Mark watched'}
                </button>
                <button
                  type="button"
                  onClick={() => favoritesId > 0 && toggleCollection.mutate({ collectionId: favoritesId, member: isFavorite })}
                  disabled={favoritesId === 0}
                  aria-pressed={isFavorite}
                  className={[
                    'inline-flex items-center gap-2 rounded-lg border px-3.5 py-2 text-[12.5px] transition-colors disabled:opacity-50',
                    isFavorite ? 'border-active/50 bg-active/10 text-fg' : 'border-hairline text-fg-muted hover:border-hairline-strong hover:text-fg',
                  ].join(' ')}
                >
                  <Heart className={['size-4', isFavorite ? 'fill-active text-active' : ''].join(' ')} aria-hidden />
                  {isFavorite ? 'Favorited' : 'Favorite'}
                </button>
                <button
                  type="button"
                  onClick={() => watchlistId > 0 && toggleCollection.mutate({ collectionId: watchlistId, member: isWatchlisted })}
                  disabled={watchlistId === 0}
                  aria-pressed={isWatchlisted}
                  className={[
                    'inline-flex items-center gap-2 rounded-lg border px-3.5 py-2 text-[12.5px] transition-colors disabled:opacity-50',
                    isWatchlisted ? 'border-progress/50 bg-progress/10 text-fg' : 'border-hairline text-fg-muted hover:border-hairline-strong hover:text-fg',
                  ].join(' ')}
                >
                  <Bookmark className={['size-4', isWatchlisted ? 'fill-progress text-progress' : ''].join(' ')} aria-hidden />
                  {isWatchlisted ? 'In watchlist' : 'Watchlist'}
                </button>
                {userCollections.length > 0 ? (
                  <select
                    value=""
                    onChange={(e) => {
                      if (e.target.value) addToCollection.mutate(Number(e.target.value));
                    }}
                    aria-label="Add to collection"
                    className="h-[38px] rounded-lg border border-hairline bg-surface-2 px-2.5 text-[12.5px] text-fg-muted hover:text-fg focus:border-hairline-strong"
                  >
                    <option value="">Add to collection…</option>
                    {userCollections.map((c) => (
                      <option key={c.id} value={c.id}>{c.name}</option>
                    ))}
                  </select>
                ) : null}
                <button
                  type="button"
                  onClick={() => setEditing(true)}
                  className="inline-flex items-center gap-2 rounded-lg border border-hairline px-3.5 py-2 text-[12.5px] text-fg-muted transition-colors hover:border-hairline-strong hover:text-fg"
                >
                  <Pencil className="size-4" aria-hidden /> Edit metadata
                </button>
                <button
                  type="button"
                  onClick={() => setIdentifying(true)}
                  className="inline-flex items-center gap-2 rounded-lg border border-hairline px-3.5 py-2 text-[12.5px] text-fg-muted transition-colors hover:border-hairline-strong hover:text-fg"
                >
                  <SearchCheck className="size-4" aria-hidden /> Identify
                </button>
                {detail.providerId ? (
                  <button
                    type="button"
                    onClick={() => refreshMeta.mutate()}
                    disabled={refreshMeta.isPending}
                    className="inline-flex items-center gap-2 rounded-lg border border-hairline px-3.5 py-2 text-[12.5px] text-fg-muted transition-colors hover:border-hairline-strong hover:text-fg disabled:opacity-60"
                  >
                    <RefreshCw className={`size-4 ${refreshMeta.isPending ? 'animate-spin' : ''}`} aria-hidden /> Refresh
                  </button>
                ) : null}
              </div>

              {detail.overview ? (
                <p className="mt-5 max-w-3xl text-[13.5px] leading-relaxed text-fg-muted">{detail.overview}</p>
              ) : (
                <p className="mt-5 text-[12.5px] text-fg-dim">No overview yet — use “Edit metadata” to add one.</p>
              )}

              {detail.people.length > 0 ? (
                <div className="mt-5 grid gap-x-8 gap-y-3 sm:grid-cols-2">
                  {(['cast', 'director', 'writer'] as const).map((role) => {
                    const people = detail.people.filter((p) => p.role === role);
                    if (people.length === 0) return null;
                    return (
                      <div key={role}>
                        <p className="mb-1 text-[10.5px] uppercase tracking-wide text-fg-dim">
                          {role === 'cast' ? 'Cast' : role === 'director' ? 'Director' : 'Writer'}
                        </p>
                        <p className="text-[12.5px] leading-relaxed text-fg-muted">
                          {people.slice(0, role === 'cast' ? 12 : 4).map((p) => p.name).join(', ')}
                        </p>
                      </div>
                    );
                  })}
                </div>
              ) : null}
            </div>
          </div>
        </div>
      </div>

      {isShow ? (
        <section className="mt-8 px-6 lg:px-8">
          <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
            <h2 className="text-[15px] font-semibold text-fg-heading">Episodes</h2>
            <div className="flex flex-wrap gap-1">
              {seasons.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => setSeasonId(s.id)}
                  aria-pressed={currentSeasonId === s.id}
                  className={[
                    'rounded-lg px-2.5 py-1 text-[12px] transition-colors',
                    currentSeasonId === s.id
                      ? 'bg-active/15 text-fg ring-1 ring-active/40'
                      : 'text-fg-dim hover:bg-surface-2 hover:text-fg-muted',
                  ].join(' ')}
                >
                  {s.seasonNumber === 0 ? 'Specials' : `Season ${s.seasonNumber}`}
                </button>
              ))}
            </div>
          </div>
          {seasonDetail.isLoading ? <Loading label="Loading episodes…" /> : null}
          {episodes.length > 0 ? <ItemViews items={episodes} view="poster" /> : null}
          {!seasonDetail.isLoading && episodes.length === 0 ? (
            <p className="text-[12.5px] text-fg-dim">No episodes indexed in this season.</p>
          ) : null}
        </section>
      ) : null}

      {!isShow && episodes.length > 0 ? (
        <section className="mt-8 px-6 lg:px-8">
          <h2 className="mb-3 text-[15px] font-semibold text-fg-heading">Episodes</h2>
          <ItemViews items={episodes} view="poster" />
        </section>
      ) : null}

      {detail.extras?.length > 0 ? (
        <section className="mt-9 px-6 lg:px-8">
          <h2 className="mb-3 text-[15px] font-semibold text-fg-heading">Extras</h2>
          <ItemViews items={detail.extras} view="poster" />
        </section>
      ) : null}

      {detail.files.length > 0 ? (
        <section className="mt-9 px-6 lg:px-8">
          <h2 className="mb-3 text-[15px] font-semibold text-fg-heading">Media info</h2>
          <MediaInfoPanel files={detail.files} />
        </section>
      ) : null}

      {editing ? <EditDialog item={detail} onClose={() => setEditing(false)} /> : null}
      {identifying ? <IdentifyDialog item={detail} onClose={() => setIdentifying(false)} /> : null}
      <PlayerModal />
    </div>
  );
}

function MetaStatus({ status, confidence, provider }: { status: string; confidence: number; provider?: string }) {
  if (!status || status === 'unmatched') {
    return <span className="rounded-full border border-new/40 bg-new/10 px-2 py-0.5 text-[11px] text-new">Unmatched</span>;
  }
  if (status === 'local') {
    return (
      <span className="rounded-full border border-progress/40 bg-progress/10 px-2 py-0.5 text-[11px] text-progress" title={provider ? `from ${provider}` : undefined}>
        Local metadata{provider ? ` · ${provider === 'nfo+vsmeta' ? 'NFO+VS' : provider.toUpperCase()}` : ''}
      </span>
    );
  }
  if (status === 'failed') {
    return <span className="rounded-full border border-active/40 bg-active/10 px-2 py-0.5 text-[11px] text-active">Match failed</span>;
  }
  const label =
    status === 'manual'
      ? 'Manual match'
      : `Matched${confidence > 0 ? ` ${Math.round(confidence * 100)}%` : ''}`;
  return (
    <span
      className="rounded-full border border-hairline px-2 py-0.5 text-[11px] text-fg-dim"
      title={provider ? `via ${provider}` : undefined}
    >
      {label}
    </span>
  );
}

function kindFor(kind: string): string {
  switch (kind) {
    case 'show':
    case 'season':
    case 'episode':
      return 'show';
    case 'home_video':
      return 'home_video';
    default:
      return 'movie';
  }
}
