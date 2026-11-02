import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate, useParams } from '@tanstack/react-router';
import { useAtom } from 'jotai';
import {
  ArrowLeft,
  Bookmark,
  Calendar,
  Check,
  ChevronLeft,
  ChevronRight,
  Clock,
  Film,
  Heart,
  Info,
  ListPlus,
  MoreHorizontal,
  Pencil,
  Play,
  RefreshCw,
  Star,
  Volume2,
} from 'lucide-react';
import { api, type Item, type ItemDetail } from '../lib/api';
import { useCurrentUser } from '../lib/auth';
import { audioSummary, certLabel, episodeLabel, formatClock, formatReleaseDate, resolutionLabel } from '../lib/format';
import { playerAtom } from '../atoms/ui';
import { ErrorState, Loading } from '../components/State';
import { MediaInfoPanel } from '../components/MediaInfo';
import { Dialog, DialogButton } from '../components/Dialog';
import { EditVideoInfoDialog } from '../components/EditVideoInfoDialog';
import { Artwork, progressPercent } from '../components/Poster';

export function DetailPage() {
  const params = useParams({ strict: false }) as { itemId?: string };
  const id = Number(params.itemId ?? 0);
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [, setPlayer] = useAtom(playerAtom);
  const [editing, setEditing] = useState<ItemDetail | null>(null);
  const [seasonId, setSeasonId] = useState<number | null>(null);
  const [expanded, setExpanded] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const openEpisodeEdit = async (id: number) => {
    try {
      setEditing(await api.item(id));
    } catch {
      /* ignore */
    }
  };
  const [mediaInfoOpen, setMediaInfoOpen] = useState(false);
  const { user } = useCurrentUser();
  const isAdmin = user?.role === 'admin';

  const item = useQuery({ queryKey: ['item', id], queryFn: () => api.item(id), enabled: id > 0 });
  const neighbors = useQuery({
    queryKey: ['neighbors', id],
    queryFn: () => api.neighbors(id),
    enabled: id > 0,
  });
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

  const playback = useQuery({
    queryKey: ['playback', id],
    queryFn: () => api.playback(id),
    enabled: id > 0 && (item.data?.fileCount ?? 0) > 0,
    staleTime: 30_000,
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

  const refreshMeta = useMutation({
    mutationFn: () => api.refreshMetadata(id, window.localStorage.getItem('mvs.tmdbLanguage') || 'en-US'),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['item', id] });
      void qc.invalidateQueries({ queryKey: ['browse'] });
      void qc.invalidateQueries({ queryKey: ['home'] });
    },
  });

  if (item.isLoading) return <Loading />;
  if (item.isError) return <ErrorState error={item.error} />;

  const detail = item.data!;
  const playable = detail.files.find((f) => !f.missing);
  const file = detail.files.find((f) => !f.missing);
  const rawEpisodes: Item[] = isShow ? (seasonDetail.data?.children ?? []) : detail.children;
  // Episode lists are ordered by episode number (the API returns them by title).
  const episodes = [...rawEpisodes].sort(
    (a, b) =>
      (a.episodeNumber ?? 0) - (b.episodeNumber ?? 0) ||
      (a.seasonNumber ?? 0) - (b.seasonNumber ?? 0) ||
      a.title.localeCompare(b.title),
  );
  const isEpisode = detail.kind === 'episode';

  const favoritesId = collections.data?.collections.find((c) => c.systemKey === 'favorites')?.id ?? 0;
  const watchlistId = collections.data?.collections.find((c) => c.systemKey === 'watchlist')?.id ?? 0;
  const isFavorite = favoritesId > 0 && detail.collectionIds.includes(favoritesId);
  const isWatchlisted = watchlistId > 0 && detail.collectionIds.includes(watchlistId);
  const userCollections = (collections.data?.collections ?? []).filter((c) => !c.systemKey);

  const progress = progressPercent(detail);
  const subtitles = playback.data?.subtitles ?? [];
  const audio = playback.data?.audio ?? [];

  const play = () => {
    if (!playable) return;
    setPlayer({ itemId: detail.id, title: detail.title });
  };

  const breadcrumb: { label: string; to?: string }[] = [];
  if (detail.libraryName) breadcrumb.push({ label: detail.libraryName });
  if (grandParent.data) breadcrumb.push({ label: grandParent.data.title, to: `/items/${grandParent.data.id}` });
  if (parent.data) breadcrumb.push({ label: parent.data.title, to: `/items/${parent.data.id}` });
  if (isEpisode) breadcrumb.push({ label: episodeLabel(detail.seasonNumber, detail.episodeNumber, detail.episodeEnd) });
  else if (detail.kind === 'season') breadcrumb.push({ label: detail.title });

  const cast = detail.people.filter((p) => p.role === 'cast');
  const directors = detail.people.filter((p) => p.role === 'director');
  const writers = detail.people.filter((p) => p.role === 'writer');

  return (
    <div className="relative min-h-[calc(100vh-3.5rem)] pb-16">
      {/* backdrop covers the whole page, like Video Station */}
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        {/* instant blurred poster so the page never renders empty */}
        {(detail.kind === 'episode' ? detail.showPosterUrl || detail.posterUrl : detail.posterUrl) ? (
          <img
            src={(detail.kind === 'episode' ? detail.showPosterUrl || detail.posterUrl : detail.posterUrl)!}
            alt=""
            className="absolute inset-0 h-full w-full scale-110 object-cover blur-2xl brightness-[0.35]"
          />
        ) : (
          <div className="absolute inset-0 bg-surface" />
        )}
        {(detail.kind === 'episode' ? detail.showBackdropUrl || detail.backdropUrl : detail.backdropUrl) ? (
          <BackdropImage src={(detail.kind === 'episode' ? detail.showBackdropUrl || detail.backdropUrl : detail.backdropUrl)!} cover />
        ) : null}
        {/* overall dim + left-to-right scrim so text stays readable over the art */}
        <div className="absolute inset-0 bg-bg/40" />
        <div className="absolute inset-0 bg-gradient-to-r from-bg/95 via-bg/50 to-bg/10" />
        <div className="absolute inset-x-0 bottom-0 h-64 bg-gradient-to-t from-bg to-transparent" />
      </div>

      <div className="relative px-6 pt-4 lg:px-8">
        {/* breadcrumb + overflow menu */}
        <div className="flex items-center justify-between gap-4">
          <div className="flex min-w-0 flex-wrap items-center gap-1.5 text-[12px] text-fg-dim">
            <button
              type="button"
              onClick={() => window.history.back()}
              className="mr-1 inline-flex items-center gap-1 rounded-md px-1.5 py-1 hover:bg-surface-2 hover:text-fg"
            >
              <ArrowLeft className="size-3.5" aria-hidden /> Back
            </button>
            {breadcrumb.map((c, i) => (
              <span key={`${c.label}-${i}`} className="flex min-w-0 items-center gap-1.5">
                {i > 0 ? <span className="text-fg-dim/60">/</span> : null}
                {c.to ? (
                  <Link to={c.to as never} className="truncate hover:text-fg">
                    {c.label}
                  </Link>
                ) : (
                  <span className="truncate text-fg-muted">{c.label}</span>
                )}
              </span>
            ))}
          </div>

          <div className="relative flex items-center gap-1">
            {detail.files.length > 0 ? (
              <button
                type="button"
                onClick={() => setMediaInfoOpen(true)}
                aria-label="View media info"
                title="View media info"
                className="grid size-8 place-items-center rounded-lg text-fg-muted hover:bg-surface-2 hover:text-fg"
              >
                <Info className="size-4" aria-hidden />
              </button>
            ) : null}
            {detail.locked ? (
              <span className="rounded-full border border-hairline px-2 py-0.5 text-[10.5px] text-fg-dim">Locked</span>
            ) : null}
            <button
              type="button"
              onClick={() => setMenuOpen((v) => !v)}
              aria-label="More actions"
              aria-expanded={menuOpen}
              className="rounded-lg p-1.5 text-fg-muted hover:bg-surface-2 hover:text-fg"
            >
              <MoreHorizontal className="size-4" aria-hidden />
            </button>
            {menuOpen ? (
              <>
                <button type="button" className="fixed inset-0 z-30 cursor-default" aria-hidden onClick={() => setMenuOpen(false)} />
                <div className="absolute right-0 top-full z-40 mt-1 w-56 rounded-lg border border-hairline bg-surface-2 p-1 shadow-2xl">
                  {isAdmin ? (
                    <MenuItem onClick={() => { setMenuOpen(false); setEditing(detail); }} icon={<Pencil className="size-3.5" aria-hidden />}>
                      Edit video info
                    </MenuItem>
                  ) : null}
                  {isAdmin && detail.providerId ? (
                    <MenuItem onClick={() => { setMenuOpen(false); refreshMeta.mutate(); }} icon={<RefreshCw className="size-3.5" aria-hidden />}>
                      Refresh metadata
                    </MenuItem>
                  ) : null}
                  {detail.files.length > 0 ? (
                    <MenuItem onClick={() => { setMenuOpen(false); setMediaInfoOpen(true); }} icon={<Info className="size-3.5" aria-hidden />}>
                      View media info
                    </MenuItem>
                  ) : null}
                  <MenuItem
                    onClick={() => { setMenuOpen(false); markWatched.mutate(!detail.played); }}
                    icon={<Check className="size-3.5" aria-hidden />}
                  >
                    {detail.played ? 'Mark unwatched' : 'Mark watched'}
                  </MenuItem>
                </div>
              </>
            ) : null}
          </div>
        </div>

        {/* poster + info */}
        <div className="mt-4 flex flex-col gap-7 sm:flex-row sm:gap-8">
          <div className={['w-full shrink-0', detail.kind === 'episode' ? 'sm:w-[400px]' : 'sm:w-[270px]'].join(' ')}>
            <div className={['group relative w-full', detail.kind === 'episode' ? 'sm:w-[400px]' : 'sm:w-[270px]'].join(' ')}>
              <div
                className={[
                  'relative w-full overflow-hidden rounded-sm border border-white/15',
                  detail.kind === 'episode' ? 'sm:w-[400px]' : 'sm:w-[270px]',
                ].join(' ')}
                style={{ aspectRatio: detail.kind === 'episode' ? '16 / 9' : '2 / 3' }}
              >
                <Artwork item={detail} />
                {playable ? (
                  <button
                    type="button"
                    onClick={play}
                    aria-label={`Play ${detail.title}`}
                    className="absolute left-1/2 top-1/2 grid size-[76px] -translate-x-1/2 -translate-y-1/2 place-items-center rounded-full bg-black/35 ring-1 ring-white/40 backdrop-blur-sm transition-colors hover:bg-black/55"
                  >
                    <Play className="ml-1 size-7 text-white" aria-hidden />
                  </button>
                ) : null}
                {progress > 0 ? (
                  <div className="absolute inset-x-0 bottom-0 h-[3px] bg-black/60">
                    <div className="h-full bg-progress" style={{ width: `${progress}%` }} />
                  </div>
                ) : null}
              </div>

              {/* status chip + icon actions (VS puts these under the poster) */}
              <div className="mt-3 flex items-center gap-2">
                <span
                  className={[
                    'rounded-sm px-2 py-0.5 text-[11.5px] font-bold',
                    detail.played ? 'bg-surface-3 text-fg-muted' : 'bg-rating text-[#333639]',
                  ].join(' ')}
                >
                  {detail.played ? 'Watched' : 'Unwatched'}
                </span>
                <div className="ml-auto flex items-center gap-1">
                  <ActionIcon
                    label={isFavorite ? 'Remove from favorites' : 'Add to favorites'}
                    active={isFavorite}
                    disabled={favoritesId === 0}
                    onClick={() => favoritesId > 0 && toggleCollection.mutate({ collectionId: favoritesId, member: isFavorite })}
                  >
                    <Heart className={['size-4', isFavorite ? 'fill-active text-active' : ''].join(' ')} aria-hidden />
                  </ActionIcon>
                  <ActionIcon
                    label={isWatchlisted ? 'Remove from watchlist' : 'Add to watchlist'}
                    active={isWatchlisted}
                    disabled={watchlistId === 0}
                    onClick={() => watchlistId > 0 && toggleCollection.mutate({ collectionId: watchlistId, member: isWatchlisted })}
                  >
                    <Bookmark className={['size-4', isWatchlisted ? 'fill-progress text-progress' : ''].join(' ')} aria-hidden />
                  </ActionIcon>
                  <ActionIcon
                    label="Add to collection"
                    disabled={userCollections.length === 0}
                    onClick={() => {
                      if (userCollections.length > 0) addToCollection.mutate(userCollections[0]!.id);
                    }}
                  >
                    <ListPlus className="size-4" aria-hidden />
                  </ActionIcon>
                  {isAdmin ? (
                    <ActionIcon label="Edit video info" onClick={() => setEditing(detail)}>
                      <Pencil className="size-4" aria-hidden />
                    </ActionIcon>
                  ) : null}
                </div>
              </div>
            </div>
          </div>

          <div className="min-w-0 flex-1 pb-2">
            <h1 className="text-[28px] font-normal leading-tight tracking-tight text-fg-heading sm:text-[34px] lg:text-[38px]">
              {detail.title}
            </h1>
            {detail.tagline ? <p className="mt-1 text-[15px] text-fg-muted">{detail.tagline}</p> : null}

            {/* info line: date · stars · duration · codecs · age classification */}
            <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 text-[15px] text-fg-muted">
              <span className="flex items-center gap-1.5">
                <Calendar className="size-4 text-fg-dim" aria-hidden />
                {detail.releaseDate ? formatReleaseDate(detail.releaseDate) : detail.year || new Date(detail.addedAt).getFullYear()}
              </span>
              {detail.rating ? (
                <span className="flex items-center gap-0.5" aria-label={`${detail.rating.toFixed(1)} of 10`}>
                  {[0, 1, 2, 3, 4].map((i) => (
                    <Star
                      key={i}
                      className={['size-4', i < Math.round(detail.rating! / 2) ? 'fill-rating text-rating' : 'text-white/20'].join(' ')}
                      aria-hidden
                    />
                  ))}
                </span>
              ) : null}
              {detail.runtimeMs ? (
                <span className="flex items-center gap-1.5">
                  <Clock className="size-4 text-fg-dim" aria-hidden />
                  {formatClock(detail.runtimeMs)}
                </span>
              ) : null}
              {isEpisode ? (
                <span className="rounded-sm border border-white/25 px-2 py-0.5 text-[13px]">
                  {episodeLabel(detail.seasonNumber, detail.episodeNumber, detail.episodeEnd)}
                </span>
              ) : null}
              {file ? (
                <span className="flex items-center gap-1.5">
                  <Film className="size-4 text-fg-dim" aria-hidden />
                  {[resolutionLabel(file.width, file.height), file.videoCodec?.toUpperCase(), file.hdr].filter(Boolean).join(' · ')}
                </span>
              ) : null}
              {file ? (
                <span className="flex items-center gap-1.5">
                  <Volume2 className="size-4 text-fg-dim" aria-hidden />
                  {audioSummary(file)}
                </span>
              ) : null}
              {detail.contentRating ? (
                <span className="ml-auto rounded-sm border border-white/50 px-2 py-0.5 text-[12.5px] font-medium text-fg">
                  {certLabel(detail.contentRating)}
                </span>
              ) : null}
            </div>

            {/* definition rows */}
            <dl className="mt-4 space-y-1.5 text-[13px]">
              {detail.genres.length > 0 ? <DefRow label="Genre">{detail.genres.join(', ')}</DefRow> : null}
              {detail.runtimeMs ? <DefRow label="Duration">{formatClock(detail.runtimeMs)}</DefRow> : null}
              {cast.length > 0 ? (
                <DefRow label="Cast">
                  <span className={expanded ? '' : 'line-clamp-3'}>{cast.map((p) => p.name).join(', ')}</span>
                </DefRow>
              ) : null}
              {writers.length > 0 ? <DefRow label="Writer">{writers.map((p) => p.name).join(', ')}</DefRow> : null}
              {directors.length > 0 ? <DefRow label="Director">{directors.map((p) => p.name).join(', ')}</DefRow> : null}
              {detail.provider ? (
                <DefRow label="Reference">
                  {providerLabel(detail.provider)}
                  {detail.metadataStatus === 'matched' && detail.matchConfidence > 0 && detail.matchConfidence < 0.55 ? (
                    <span
                      className="ml-2 rounded-sm border border-new/50 px-1.5 py-0.5 text-[10px] text-new"
                      title={`Automatic match confidence ${(detail.matchConfidence * 100).toFixed(0)}%`}
                    >
                      low confidence
                    </span>
                  ) : null}
                </DefRow>
              ) : null}
              {subtitles.length > 0 ? (
                <DefRow label="Subtitles">
                  <select
                    disabled
                    title="Subtitle selection happens in the player"
                    defaultValue={subtitles.find((s) => s.default)?.id ?? subtitles[0]?.id}
                    className="max-w-full bg-transparent text-[13px] text-fg-muted"
                  >
                    {subtitles.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.label}
                        {s.language ? ` - ${s.language}` : ''}
                      </option>
                    ))}
                  </select>
                </DefRow>
              ) : null}
              {audio.length > 0 ? (
                <DefRow label="Audio Tracks">
                  <select
                    disabled
                    title="Audio switching arrives with transcoding (M5)"
                    defaultValue={String((audio.find((a) => a.default) ?? audio[0])?.index ?? 0)}
                    className="max-w-full bg-transparent text-[13px] text-fg-muted"
                  >
                    {audio.map((a) => (
                      <option key={a.index} value={String(a.index)}>
                        {a.codec.toUpperCase()} · {a.channels}ch{a.language ? ` · ${a.language}` : ''}
                      </option>
                    ))}
                  </select>
                </DefRow>
              ) : null}
            </dl>

            <div className="mt-4 h-px w-full bg-white/20" />

            <h2 className="mt-3 text-[15px] text-fg-heading">Summary</h2>
            {detail.overview ? (
              <>
                <p
                  className={[
                    'mt-1 whitespace-pre-wrap break-words text-[13.5px] leading-relaxed text-fg-muted',
                    expanded ? '' : 'line-clamp-5',
                  ].join(' ')}
                >
                  {detail.overview}
                </p>
                {detail.overview.length > 420 && !expanded ? (
                  <button
                    type="button"
                    onClick={() => setExpanded(true)}
                    className="mt-2 rounded-sm border border-white/30 px-2.5 py-1 text-[11.5px] text-fg-muted hover:border-white/50 hover:text-fg"
                  >
                    View all
                  </button>
                ) : null}
              </>
            ) : (
              <p className="mt-1 text-[12.5px] text-fg-dim">No overview yet.</p>
            )}
          </div>
        </div>

        {/* show seasons + episodes */}
        {isShow ? (
          <section className="mt-8">
            <div className="flex flex-wrap items-center gap-2 border-y border-white/15 py-2.5">
              <span className="mr-2 text-[15px] text-fg">Seasons</span>
              {seasons.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => setSeasonId(s.id)}
                  aria-pressed={currentSeasonId === s.id}
                  className={[
                    'min-w-8 rounded-sm px-2 py-0.5 text-[14px] font-medium transition-colors',
                    currentSeasonId === s.id ? 'bg-active text-white' : 'text-fg-muted hover:text-fg',
                  ].join(' ')}
                >
                  {s.seasonNumber === 0 ? 'S' : s.seasonNumber}
                </button>
              ))}
            </div>
            {seasonDetail.isLoading ? <Loading label="Loading episodes…" /> : null}
            {episodes.length > 0 ? (
              <div className="mt-4 flex flex-wrap gap-4">
                {episodes.map((e) => (
                  <EpisodeCard key={e.id} item={e} onEdit={isAdmin ? () => void openEpisodeEdit(e.id) : undefined} />
                ))}
              </div>
            ) : null}
          </section>
        ) : null}

        {!isShow && episodes.length > 0 ? (
          <section className="mt-8">
            <h2 className="mb-3 text-[17px] font-semibold text-fg-heading">Episodes</h2>
            <div className="flex flex-wrap gap-4">
              {episodes.map((e) => (
                <EpisodeCard key={e.id} item={e} onEdit={isAdmin ? () => void openEpisodeEdit(e.id) : undefined} />
              ))}
            </div>
          </section>
        ) : null}

        {detail.extras.length > 0 ? (
          <section className="mt-9">
            <h2 className="mb-3 text-[17px] font-semibold text-fg-heading">Extras</h2>
            <div className="flex flex-wrap gap-4">
              {detail.extras.map((e) => (
                <Link key={e.id} to="/items/$itemId" params={{ itemId: String(e.id) }} className="w-[120px]">
                  <div className="aspect-[2/3] overflow-hidden rounded-sm border border-white/15">
                    <Artwork item={e} />
                  </div>
                  <p className="mt-1.5 truncate text-[12.5px] text-fg">{e.title}</p>
                </Link>
              ))}
            </div>
          </section>
        ) : null}
      </div>

      {/* prev/next, VS style bottom corners */}
      <button
        type="button"
        disabled={!neighbors.data?.prev}
        onClick={() => neighbors.data?.prev && void navigate({ to: '/items/$itemId', params: { itemId: String(neighbors.data.prev.id) } })}
        aria-label="Previous item"
        className="fixed bottom-5 left-3 z-20 grid size-9 place-items-center rounded-full bg-black/45 text-fg-muted ring-1 ring-white/15 backdrop-blur transition-colors hover:text-fg disabled:opacity-25 sm:left-[252px]"
      >
        <ChevronLeft className="size-5" aria-hidden />
      </button>
      <button
        type="button"
        disabled={!neighbors.data?.next}
        onClick={() => neighbors.data?.next && void navigate({ to: '/items/$itemId', params: { itemId: String(neighbors.data.next.id) } })}
        aria-label="Next item"
        className="fixed bottom-5 right-3 z-20 grid size-9 place-items-center rounded-full bg-black/45 text-fg-muted ring-1 ring-white/15 backdrop-blur transition-colors hover:text-fg disabled:opacity-25"
      >
        <ChevronRight className="size-5" aria-hidden />
      </button>

      {mediaInfoOpen ? <MediaInfoDialog title={detail.title} files={detail.files} onClose={() => setMediaInfoOpen(false)} /> : null}
      {editing ? <EditVideoInfoDialog item={editing} onClose={() => setEditing(null)} /> : null}
    </div>
  );
}

function MediaInfoDialog({ title, files, onClose }: { title: string; files: import('../lib/api').MediaFile[]; onClose: () => void }) {
  return (
    <Dialog title="Media info" onClose={onClose} footer={
        <div className="ml-auto">
          <DialogButton onClick={onClose}>Close</DialogButton>
        </div>
      }>
      <div className="p-4">
        <p className="mb-2 truncate text-[11.5px] text-fg-dim">{title}</p>
        <MediaInfoPanel files={files} />
      </div>
    </Dialog>
  );
}

function BackdropImage({ src, cover = false }: { src: string; cover?: boolean }) {
  const [loaded, setLoaded] = useState(false);
  return (
    <img
      src={src}
      alt=""
      decoding="async"
      onLoad={() => setLoaded(true)}
      className={[
        'absolute inset-0 h-full w-full object-cover transition-opacity duration-500',
        cover ? 'opacity-55' : 'opacity-70',
        loaded ? '' : 'opacity-0',
      ].join(' ')}
    />
  );
}

function DefRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-2">
      <dt className="w-[92px] shrink-0 font-bold text-fg">{label}:</dt>
      <dd className="min-w-0 flex-1 text-fg-muted">{children}</dd>
    </div>
  );
}

function ActionIcon({
  label,
  active,
  disabled,
  onClick,
  children,
}: {
  label: string;
  active?: boolean;
  disabled?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      aria-pressed={active}
      className="grid size-8 place-items-center rounded-md text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg disabled:opacity-30"
    >
      {children}
    </button>
  );
}

function MenuItem({ icon, children, onClick }: { icon: React.ReactNode; children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
    >
      {icon}
      {children}
    </button>
  );
}

function EpisodeCard({ item, onEdit }: { item: Item; onEdit?: () => void }) {
  return (
    <Link to="/items/$itemId" params={{ itemId: String(item.id) }} className="group w-[180px] shrink-0">
      <div className="relative aspect-video overflow-hidden rounded-sm border border-white/15 bg-surface-2">
        <Artwork item={item} />
        {item.played ? (
          <span className="absolute left-1.5 top-1.5 rounded-full bg-black/70 px-1.5 py-0.5 text-[9.5px] text-fg">Watched</span>
        ) : null}
        <div className="absolute inset-0 bg-black/0 transition-colors group-hover:bg-black/20" />
        {onEdit ? (
          <button
            type="button"
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              onEdit();
            }}
            aria-label={`Edit ${item.title}`}
            className="absolute right-1.5 top-1.5 grid size-7 place-items-center rounded-full bg-black/60 text-white opacity-0 transition-opacity hover:bg-black/80 focus-visible:opacity-100 group-hover:opacity-100"
          >
            <Pencil className="size-3.5" aria-hidden />
          </button>
        ) : null}
        <span className="absolute bottom-1.5 right-1.5 grid size-7 place-items-center rounded-full bg-black/60 opacity-0 transition-opacity group-hover:opacity-100">
          <Play className="ml-0.5 size-3.5 text-white" aria-hidden />
        </span>
      </div>
      <p className="mt-1.5 truncate text-[13px] text-fg">{item.title}</p>
      <p className="text-[11.5px] text-fg-dim">
        {episodeLabel(item.seasonNumber, item.episodeNumber, item.episodeEnd) || 'Episode'}
      </p>
    </Link>
  );
}

function providerLabel(provider: string): string {
  switch (provider) {
    case 'tmdb':
      return 'TMDB';
    case 'nfo':
    case 'vsmeta':
    case 'nfo+vsmeta':
      return `Local (${provider.toUpperCase()})`;
    default:
      return provider;
  }
}
