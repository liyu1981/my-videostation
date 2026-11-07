import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  CalendarDays,
  Check,
  ChevronDown,
  ExternalLink,
  Image as ImageIcon,
  Loader2,
  Lock,
  LockOpen,
  RefreshCw,
  Search,
  X,
} from 'lucide-react';
import { api, type Candidate, type Item, type ItemDetail, type ItemPatch, type Kind } from '../lib/api';
import { Dialog, DialogButton, DialogTab } from './Dialog';

type Tab = 'info' | 'poster' | 'backdrop';
type ArtChoice = 'original' | 'none' | 'url' | 'upload';

const LANG_KEY = 'mvs.tmdbLanguage';
const LANGUAGES: [string, string][] = [
  ['en-US', 'English'],
  ['zh-CN', '简体中文'],
  ['zh-TW', '繁體中文'],
  ['ja-JP', '日本語'],
  ['ko-KR', '한국어'],
  ['fr-FR', 'Français'],
  ['de-DE', 'Deutsch'],
  ['es-ES', 'Español'],
  ['it-IT', 'Italiano'],
  ['pt-BR', 'Português (Brasil)'],
  ['ru-RU', 'Русский'],
];

const LOCK_FIELDS = [
  ['title', 'Title'],
  ['original_title', 'Original title'],
  ['year', 'Year'],
  ['release_date', 'Release date'],
  ['overview', 'Overview'],
  ['tagline', 'Tagline'],
  ['rating', 'Rating'],
  ['content_rating', 'Certification'],
  ['genres', 'Genres'],
  ['poster', 'Poster'],
  ['backdrop', 'Backdrop'],
  ['cast', 'Cast'],
  ['crew', 'Crew'],
] as const;

type CastEntry = { name: string; character?: string };

/**
 * Video-Station-style "Edit video info" dialog: Video Info / Poster / Backdrop
 * tabs, chip editors, and the nested "Search from Video Info Plugin" dialog with
 * a language picker (the last used language is remembered).
 */
export function EditVideoInfoDialog({ item, onClose }: { item: ItemDetail; onClose: () => void }) {
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>('info');
  const [base, setBase] = useState<ItemDetail>(item);
  const [searchOpen, setSearchOpen] = useState(false);
  const [localOpen, setLocalOpen] = useState(false);
  const [duplicate, setDuplicate] = useState<Item | null>(null);
  const [mergeBusy, setMergeBusy] = useState(false);
  const [mergeError, setMergeError] = useState('');

  const [title, setTitle] = useState(item.title);
  const [tagline, setTagline] = useState(item.tagline ?? '');
  const [releaseDate, setReleaseDate] = useState(toDateInput(item.releaseDate));
  const [contentRating, setContentRating] = useState(item.contentRating ?? '');
  const [rating, setRating] = useState(item.rating != null ? String(item.rating) : '');
  const [year, setYear] = useState(item.year ? String(item.year) : '');
  const [bestEffort, setBestEffort] = useState('');
  const [overview, setOverview] = useState(item.overview ?? '');
  const [genres, setGenres] = useState<string[]>(item.genres);
  const [cast, setCast] = useState<CastEntry[]>(castOf(item));
  const [locks, setLocks] = useState<string[]>(item.locks);
  const [locked, setLocked] = useState(item.locked);
  const [advanced, setAdvanced] = useState(false);
  // Manual type assignment (movie / TV episode / home video).
  const [kind, setKind] = useState<Kind>(item.kind);
  const [showTitle, setShowTitle] = useState(item.showTitle ?? '');
  const [seasonNumber, setSeasonNumber] = useState(String(item.seasonNumber ?? 1));
  const [episodeNumber, setEpisodeNumber] = useState(String(item.episodeNumber ?? 1));

  const [posterChoice, setPosterChoice] = useState<ArtChoice>('original');
  const [backdropChoice, setBackdropChoice] = useState<ArtChoice>('original');
  const [posterUrl, setPosterUrl] = useState('');
  const [backdropUrl, setBackdropUrl] = useState('');
  const [posterFile, setPosterFile] = useState<File | null>(null);
  const [backdropFile, setBackdropFile] = useState<File | null>(null);
  const [posterPreview, setPosterPreview] = useState('');
  const [backdropPreview, setBackdropPreview] = useState('');

  const applyDetail = (d: ItemDetail) => {
    setBase(d);
    setTitle(d.title);
    setTagline(d.tagline ?? '');
    setReleaseDate(toDateInput(d.releaseDate));
    setContentRating(d.contentRating ?? '');
    setRating(d.rating != null ? String(d.rating) : '');
    setYear(d.year ? String(d.year) : '');
    setOverview(d.overview ?? '');
    setBestEffort('');
    setGenres(d.genres);
    setCast(castOf(d));
    setLocks(d.locks);
    setLocked(d.locked);
    setKind(d.kind);
    setShowTitle(d.showTitle ?? '');
    setSeasonNumber(String(d.seasonNumber ?? 1));
    setEpisodeNumber(String(d.episodeNumber ?? 1));
    if (posterPreview) URL.revokeObjectURL(posterPreview);
    if (backdropPreview) URL.revokeObjectURL(backdropPreview);
    setPosterChoice('original');
    setBackdropChoice('original');
    setPosterUrl('');
    setBackdropUrl('');
    setPosterFile(null);
    setBackdropFile(null);
    setPosterPreview('');
    setBackdropPreview('');
    setTab('info');
  };

  useEffect(() => () => {
    if (posterPreview) URL.revokeObjectURL(posterPreview);
  }, [posterPreview]);
  useEffect(() => () => {
    if (backdropPreview) URL.revokeObjectURL(backdropPreview);
  }, [backdropPreview]);

  const setArtFile = (kind: 'poster' | 'backdrop', file: File | null) => {
    const preview = file ? URL.createObjectURL(file) : '';
    if (kind === 'poster') {
      if (posterPreview) URL.revokeObjectURL(posterPreview);
      setPosterFile(file);
      setPosterPreview(preview);
      setPosterChoice(file ? 'upload' : 'original');
    } else {
      if (backdropPreview) URL.revokeObjectURL(backdropPreview);
      setBackdropFile(file);
      setBackdropPreview(preview);
      setBackdropChoice(file ? 'upload' : 'original');
    }
  };

  const reset = () => applyDetail(base);

  const applyBestEffort = (d: {
    title?: string;
    overview?: string;
    year?: number;
    rating?: number;
    posterUrl?: string;
    backdropUrl?: string;
  }) => {
    if (d.title) setTitle(d.title);
    if (d.overview) setOverview(d.overview);
    if (d.year) setYear(String(d.year));
    if (d.rating) setRating(String(d.rating));
    const abs = (u: string) => (u.startsWith('/') ? window.location.origin + u : u);
    if (d.posterUrl) {
      setPosterChoice('url');
      setPosterUrl(abs(d.posterUrl));
    }
    if (d.backdropUrl) {
      setBackdropChoice('url');
      setBackdropUrl(abs(d.backdropUrl));
    }
    setBestEffort('The provider had no record for that id — filled the available search info. Review and save.');
  };

  const finish = () => {
    // Invalidate every item query: an episode edit must refresh the season/show
    // episode cards (their art URLs are versioned by updatedAt).
    void qc.invalidateQueries({ queryKey: ['item'] });
    void qc.invalidateQueries({ queryKey: ['browse'] });
    void qc.invalidateQueries({ queryKey: ['home'] });
    void qc.invalidateQueries({ queryKey: ['search'] });
    onClose();
  };

  const checkDuplicate = async () => {
    void qc.invalidateQueries({ queryKey: ['item'] });
    try {
      const res = await api.duplicate(item.id);
      if (res.item) {
        setDuplicate(res.item);
        return;
      }
    } catch {
      /* duplicate check is best-effort */
    }
    finish();
  };

  const mergeDuplicate = async () => {
    if (!duplicate) return;
    setMergeBusy(true);
    setMergeError('');
    try {
      await api.mergeItems(item.id, duplicate.id);
      setDuplicate(null);
      finish();
    } catch (err) {
      setMergeError((err as Error).message);
      setMergeBusy(false);
    }
  };

  const keepSeparate = async () => {
    if (!duplicate) return;
    setMergeBusy(true);
    setMergeError('');
    try {
      await api.keepSeparate(item.id, duplicate.id);
      setDuplicate(null);
      finish();
    } catch (err) {
      setMergeError((err as Error).message);
      setMergeBusy(false);
    }
  };

  const save = useMutation({
    mutationFn: async () => {
      const patch: ItemPatch = {
        title: title.trim(),
        tagline: tagline.trim(),
        overview: overview.trim(),
        contentRating: contentRating.trim(),
        releaseDate,
        genres,
        cast: cast.map((c) => ({ name: c.name, character: c.character })),
        locks,
        locked,
      };
      const y = Number(year);
      if (year.trim() !== '' && Number.isFinite(y)) patch.year = y;
      else if (releaseDate) {
        const ry = Number(releaseDate.slice(0, 4));
        if (Number.isFinite(ry)) patch.year = ry;
      }
      const r = Number(rating);
      if (rating.trim() !== '' && Number.isFinite(r)) patch.rating = r;
      const typeChanged =
        kind !== item.kind ||
        (kind === 'episode' &&
          (showTitle.trim() !== (item.showTitle ?? '') ||
            Number(seasonNumber) !== (item.seasonNumber ?? 0) ||
            Number(episodeNumber) !== (item.episodeNumber ?? 0)));
      if (typeChanged) {
        if (kind === 'episode' && showTitle.trim() === '') {
          throw new Error('Show title is required for a TV episode.');
        }
        await api.reclassifyItem(item.id, {
          kind: kind as 'movie' | 'episode' | 'home_video',
          showTitle: showTitle.trim(),
          seasonNumber: Number(seasonNumber) || 0,
          episodeNumber: Number(episodeNumber) || 0,
        });
      }
      await api.patchItem(item.id, patch);
      if (posterChoice === 'upload' && posterFile) await api.uploadArt(item.id, 'poster', posterFile);
      else if (posterChoice === 'none') await api.setArt(item.id, 'poster', '');
      else if (posterChoice === 'url' && posterUrl.trim()) await api.setArt(item.id, 'poster', posterUrl.trim());
      if (backdropChoice === 'upload' && backdropFile) await api.uploadArt(item.id, 'backdrop', backdropFile);
      else if (backdropChoice === 'none') await api.setArt(item.id, 'backdrop', '');
      else if (backdropChoice === 'url' && backdropUrl.trim()) await api.setArt(item.id, 'backdrop', backdropUrl.trim());
    },
    onSuccess: () => {
      void checkDuplicate();
    },
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      if (searchOpen) setSearchOpen(false);
      else onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [searchOpen, onClose]);

  const toggleLock = (field: string) =>
    setLocks((prev) => (prev.includes(field) ? prev.filter((f) => f !== field) : [...prev, field]));

  return (
    <>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Dialog
          title="Edit video info"
          onClose={onClose}
          tabs={
            <>
              {(['info', 'poster', 'backdrop'] as const).map((t) => (
                <DialogTab key={t} active={tab === t} onClick={() => setTab(t)}>
                  {t === 'info' ? 'Video Info' : t === 'poster' ? 'Poster' : 'Backdrop'}
                </DialogTab>
              ))}
            </>
          }
          footer={
            <>
              <p className="min-w-0 truncate text-[11.5px] text-active">{save.isError ? (save.error as Error).message : ''}</p>
              <div className="flex shrink-0 items-center gap-2">
                <DialogButton onClick={onClose}>Cancel</DialogButton>
                <DialogButton type="submit" variant="primary" disabled={save.isPending}>
                  {save.isPending ? <Loader2 className="size-3.5 animate-spin" aria-hidden /> : null}
                  OK
                </DialogButton>
              </div>
            </>
          }
        >
            {tab === 'info' ? (
              <div className="p-5">
                {bestEffort ? (
                  <p className="mb-3 rounded-md border border-new/40 bg-new/10 px-3 py-2 text-[11.5px] text-new">{bestEffort}</p>
                ) : null}
                {item.kind === 'movie' || item.kind === 'episode' || item.kind === 'home_video' ? (
                  <div className="mb-4 rounded-md border border-hairline bg-surface-2/40 p-3">
                    <div className="grid grid-cols-1 gap-x-4 gap-y-2.5 sm:grid-cols-[118px_1fr] sm:items-center">
                      <Field label="Type">
                        <select
                          value={kind}
                          onChange={(e) => setKind(e.target.value as Kind)}
                          className={inputClass}
                        >
                          <option value="movie">Movie</option>
                          <option value="episode">TV episode</option>
                          <option value="home_video">Home video</option>
                        </select>
                      </Field>
                      {kind === 'episode' ? (
                        <>
                          <Field label="Show title">
                            <input value={showTitle} onChange={(e) => setShowTitle(e.target.value)} className={inputClass} placeholder="e.g. Breaking Bad" />
                          </Field>
                          <Field label="Season">
                            <input type="number" min={0} value={seasonNumber} onChange={(e) => setSeasonNumber(e.target.value)} className={inputClass} />
                          </Field>
                          <Field label="Episode">
                            <input type="number" min={0} value={episodeNumber} onChange={(e) => setEpisodeNumber(e.target.value)} className={inputClass} />
                          </Field>
                        </>
                      ) : null}
                    </div>
                    <p className="mt-2 text-[11px] text-fg-dim">
                      Changing the type clears the current provider match; search again after saving. The choice survives rescans.
                    </p>
                  </div>
                ) : null}
                <div className="mb-4 flex flex-wrap items-center gap-2">
                  {item.kind === 'movie' || item.kind === 'show' || item.kind === 'episode' || item.kind === 'season' || item.kind === 'home_video' ? (
                    <button
                      type="button"
                      onClick={() => setSearchOpen(true)}
                      className="rounded-md border border-hairline-strong bg-surface-2 px-3 py-1.5 text-[12px] text-fg hover:bg-surface-3"
                    >
                      Search from Video Info Plugin
                    </button>
                  ) : (
                    <span className="text-[11.5px] text-fg-dim">Metadata search is available for movies, TV shows, seasons, episodes and home videos.</span>
                  )}
                  {item.kind === 'movie' || item.kind === 'show' || item.kind === 'episode' || item.kind === 'season' || item.kind === 'home_video' ? (
                    <button
                      type="button"
                      onClick={() => setLocalOpen(true)}
                      className="rounded-md border border-hairline-strong bg-surface-2 px-3 py-1.5 text-[12px] text-fg hover:bg-surface-3"
                    >
                      Select from existing videos
                    </button>
                  ) : null}
                  <button
                    type="button"
                    onClick={reset}
                    title="Discard unsaved edits and restore the saved values"
                    className="rounded-md border border-hairline-strong bg-surface-2 px-3 py-1.5 text-[12px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                  >
                    Reset changes
                  </button>
                </div>

                <div className="grid grid-cols-1 gap-x-4 gap-y-2.5 sm:grid-cols-[118px_1fr] sm:items-center">
                  <Field label={item.kind === 'episode' ? 'Episode title' : 'Title'}>
                    <input value={title} onChange={(e) => setTitle(e.target.value)} className={inputClass} />
                  </Field>
                  <Field label="Tagline">
                    <input value={tagline} onChange={(e) => setTagline(e.target.value)} className={inputClass} />
                  </Field>
                  <Field label={item.kind === 'episode' ? 'Air date' : 'Release date'}>
                    <div className="relative">
                      <input
                        type="date"
                        value={releaseDate}
                        onChange={(e) => setReleaseDate(e.target.value)}
                        className={`${inputClass} pr-9 [color-scheme:dark]`}
                      />
                      <CalendarDays className="pointer-events-none absolute right-2.5 top-1/2 size-3.5 -translate-y-1/2 text-fg-dim" aria-hidden />
                    </div>
                  </Field>
                  <Field label="Year">
                    <input
                      value={year}
                      onChange={(e) => setYear(e.target.value)}
                      inputMode="numeric"
                      placeholder="2019"
                      className={`${inputClass} max-w-[120px]`}
                    />
                  </Field>
                  <Field label="Classification">
                    <input
                      value={contentRating}
                      onChange={(e) => setContentRating(e.target.value)}
                      placeholder="PG-13"
                      className={inputClass}
                    />
                  </Field>
                  <Field label="Rating">
                    <input
                      value={rating}
                      onChange={(e) => setRating(e.target.value)}
                      inputMode="decimal"
                      placeholder="7.7"
                      className={`${inputClass} max-w-[140px]`}
                    />
                  </Field>
                  <Field label="Genre" align="start">
                    <ChipEditor values={genres} onChange={setGenres} placeholder="Add genre…" />
                  </Field>
                  <Field label="Cast" align="start">
                    <CastEditor values={cast} onChange={setCast} />
                  </Field>
                  <Field label="Summary" align="start">
                    <textarea
                      value={overview}
                      onChange={(e) => setOverview(e.target.value)}
                      rows={4}
                      className={`${inputClass} resize-y`}
                    />
                  </Field>
                </div>

                <div className="mt-4 border-t border-hairline pt-3">
                  <button
                    type="button"
                    onClick={() => setAdvanced((v) => !v)}
                    aria-expanded={advanced}
                    className="inline-flex items-center gap-1.5 text-[11.5px] text-fg-dim hover:text-fg-muted"
                  >
                    <ChevronDown className={`size-3.5 transition-transform ${advanced ? '' : '-rotate-90'}`} aria-hidden />
                    Keep locked after refresh ({locks.length})
                  </button>
                  {advanced ? (
                    <div className="mt-2.5">
                      <div className="flex flex-wrap gap-1.5">
                        {LOCK_FIELDS.map(([field, label]) => {
                          const on = locks.includes(field);
                          return (
                            <button
                              key={field}
                              type="button"
                              onClick={() => toggleLock(field)}
                              aria-pressed={on}
                              className={[
                                'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] transition-colors',
                                on
                                  ? 'border-active/50 bg-active/12 text-fg'
                                  : 'border-hairline text-fg-muted hover:border-hairline-strong hover:text-fg',
                              ].join(' ')}
                            >
                              {on ? <Lock className="size-3" aria-hidden /> : <LockOpen className="size-3 opacity-60" aria-hidden />}
                              {label}
                            </button>
                          );
                        })}
                      </div>
                      <label className="mt-2.5 flex items-center gap-2 text-[12px] text-fg-muted">
                        <input
                          type="checkbox"
                          checked={locked}
                          onChange={(e) => setLocked(e.target.checked)}
                          className="size-4 rounded border-hairline bg-surface-2"
                        />
                        Lock all metadata for this item
                      </label>
                    </div>
                  ) : null}
                </div>
              </div>
            ) : (
              <ArtPanel
                kind={tab}
                currentUrl={tab === 'poster' ? base.posterUrl : base.backdropUrl}
                choice={tab === 'poster' ? posterChoice : backdropChoice}
                onChoice={tab === 'poster' ? setPosterChoice : setBackdropChoice}
                url={tab === 'poster' ? posterUrl : backdropUrl}
                onUrl={tab === 'poster' ? setPosterUrl : setBackdropUrl}
                file={tab === 'poster' ? posterFile : backdropFile}
                previewUrl={tab === 'poster' ? posterPreview : backdropPreview}
                onFile={(f) => setArtFile(tab, f)}
              />
            )}
        </Dialog>
      </form>

      {localOpen ? (
        <LocalSearchDialog
          item={base}
          onClose={() => setLocalOpen(false)}
          onApplied={(d) => {
            applyDetail(d);
            setLocalOpen(false);
            void qc.invalidateQueries({ queryKey: ['item'] });
          }}
          onBestEffort={(source) => {
            applyBestEffort(source);
            setLocalOpen(false);
          }}
        />
      ) : null}

      {searchOpen ? (
        <SearchPluginDialog
          item={base}
          query={base.kind === 'episode' || base.kind === 'season' ? base.showTitle || base.title : base.title}
          mode={base.kind === 'episode' ? 'episode' : base.kind === 'season' ? 'season' : undefined}
          onClose={() => setSearchOpen(false)}
          onMatched={(d) => {
            applyDetail(d);
            setSearchOpen(false);
            void qc.invalidateQueries({ queryKey: ['item'] });
            void checkDuplicate();
          }}
          onBestEffort={(c) => {
            applyBestEffort({
              title: c.title,
              overview: c.overview,
              year: c.year,
              rating: c.rating,
              posterUrl: c.posterUrl,
              backdropUrl: c.backdropUrl,
            });
            setSearchOpen(false);
          }}
        />
      ) : null}

      {duplicate ? (
        <Dialog
          title="Possible duplicate"
          onClose={() => setDuplicate(null)}
          z={60}
          size="max-w-xl"
          footer={
            <>
              <p className="min-w-0 truncate text-[11.5px] text-active">{mergeError}</p>
              <div className="flex shrink-0 items-center gap-2">
                <DialogButton onClick={() => setDuplicate(null)}>Cancel</DialogButton>
                <DialogButton disabled={mergeBusy} onClick={() => void keepSeparate()}>
                  Keep separate
                </DialogButton>
                <DialogButton variant="primary" disabled={mergeBusy} onClick={() => void mergeDuplicate()}>
                  {mergeBusy ? <Loader2 className="size-3.5 animate-spin" aria-hidden /> : null}
                  Merge
                </DialogButton>
              </div>
            </>
          }
        >
          <div className="flex gap-4 p-5">
            <div className="h-[108px] w-[72px] shrink-0 overflow-hidden rounded bg-surface-2">
              {duplicate.posterUrl ? (
                <img src={duplicate.posterUrl} alt="" className="h-full w-full object-cover" />
              ) : (
                <div className="grid h-full place-items-center text-fg-dim">
                  <ImageIcon className="size-5" aria-hidden />
                </div>
              )}
            </div>
            <div className="min-w-0 text-[12.5px] text-fg-muted">
              <p className="text-[14px] font-medium text-fg">
                {duplicate.title}
                {duplicate.year ? <span className="ml-1 font-normal text-fg-dim">({duplicate.year})</span> : null}
              </p>
              <p className="mt-1">
                Another item with the same title already exists in this library
                {duplicate.fileCount ? ` (${duplicate.fileCount} file${duplicate.fileCount === 1 ? '' : 's'})` : ''}.
              </p>
              <p className="mt-2">
                <strong className="text-fg">Merge</strong> moves its files into this item; <strong className="text-fg">Keep separate</strong> leaves both entries.
              </p>
            </div>
          </div>
        </Dialog>
      ) : null}
    </>
  );
}

function Field({ label, children, align = 'center' }: { label: string; children: React.ReactNode; align?: 'center' | 'start' }) {
  return (
    <>
      <span className={`text-[12px] text-fg-muted ${align === 'start' ? 'self-start pt-1.5' : ''}`}>{label}</span>
      <div className="min-w-0">{children}</div>
    </>
  );
}

function ChipEditor({ values, onChange, placeholder }: { values: string[]; onChange: (v: string[]) => void; placeholder: string }) {
  const [draft, setDraft] = useState('');
  const add = () => {
    const v = draft.trim();
    if (v && !values.includes(v)) onChange([...values, v]);
    setDraft('');
  };
  return (
    <div className="rounded-lg border border-hairline bg-surface-2 px-2 py-1.5">
      <div className="flex flex-wrap items-center gap-1.5">
        {values.map((v) => (
          <span key={v} className="inline-flex items-center gap-1 rounded bg-surface-3 px-2 py-0.5 text-[11.5px] text-fg">
            {v}
            <button
              type="button"
              onClick={() => onChange(values.filter((x) => x !== v))}
              aria-label={`Remove ${v}`}
              className="text-fg-dim hover:text-fg"
            >
              <X className="size-3" aria-hidden />
            </button>
          </span>
        ))}
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ',') {
              e.preventDefault();
              add();
            }
          }}
          onBlur={add}
          placeholder={placeholder}
          className="h-6 min-w-[7rem] flex-1 bg-transparent text-[12px] text-fg outline-none placeholder:text-fg-dim"
        />
      </div>
    </div>
  );
}

function CastEditor({ values, onChange }: { values: CastEntry[]; onChange: (v: CastEntry[]) => void }) {
  const [draft, setDraft] = useState('');
  const add = () => {
    const name = draft.trim();
    if (name && !values.some((c) => c.name === name)) onChange([...values, { name }]);
    setDraft('');
  };
  return (
    <div className="rounded-lg border border-hairline bg-surface-2 px-2 py-1.5">
      <div className="flex flex-wrap items-center gap-1.5">
        {values.map((c, i) => (
          <span
            key={`${c.name}-${i}`}
            title={c.character ? `${c.name} as ${c.character}` : c.name}
            className="inline-flex items-center gap-1 rounded bg-surface-3 px-2 py-0.5 text-[11.5px] text-fg"
          >
            {c.name}
            <button
              type="button"
              onClick={() => onChange(values.filter((_, j) => j !== i))}
              aria-label={`Remove ${c.name}`}
              className="text-fg-dim hover:text-fg"
            >
              <X className="size-3" aria-hidden />
            </button>
          </span>
        ))}
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ',') {
              e.preventDefault();
              add();
            }
          }}
          onBlur={add}
          placeholder="Add cast member…"
          className="h-6 min-w-[7rem] flex-1 bg-transparent text-[12px] text-fg outline-none placeholder:text-fg-dim"
        />
      </div>
    </div>
  );
}

function ArtPanel({
  kind,
  currentUrl,
  choice,
  onChoice,
  url,
  onUrl,
  file,
  previewUrl,
  onFile,
}: {
  kind: 'poster' | 'backdrop';
  currentUrl?: string;
  choice: ArtChoice;
  onChoice: (c: ArtChoice) => void;
  url: string;
  onUrl: (v: string) => void;
  file: File | null;
  previewUrl: string;
  onFile: (f: File | null) => void;
}) {
  const preview =
    choice === 'none'
      ? undefined
      : choice === 'upload' && previewUrl
        ? previewUrl
        : choice === 'url' && url.trim()
          ? url.trim()
          : currentUrl;
  return (
    <div className="flex flex-col gap-6 p-5 sm:flex-row">
      <div
        className={[
          'shrink-0 overflow-hidden rounded-md border border-hairline bg-surface-2',
          kind === 'poster' ? 'aspect-[2/3] w-full sm:w-[176px]' : 'aspect-video w-full sm:w-[300px]',
        ].join(' ')}
      >
        {preview ? (
          <img src={preview} alt="" className="h-full w-full object-cover" />
        ) : (
          <div className="grid h-full place-items-center text-fg-dim">
            <ImageIcon className="size-7" aria-hidden />
          </div>
        )}
      </div>

      <div className="min-w-0 flex-1 space-y-3">
        <label className="flex items-start gap-2.5 text-[12.5px]">
          <input type="radio" checked={choice === 'original'} onChange={() => onChoice('original')} className="mt-0.5 size-3.5 accent-[#e5484d]" />
          <span>Original</span>
        </label>
        <label className="flex items-start gap-2.5 text-[12.5px]">
          <input type="radio" checked={choice === 'none'} onChange={() => onChoice('none')} className="mt-0.5 size-3.5 accent-[#e5484d]" />
          <span>No images</span>
        </label>
        <label className="flex items-start gap-2.5 text-[12.5px]">
          <input type="radio" checked={choice === 'url'} onChange={() => onChoice('url')} className="mt-0.5 size-3.5 accent-[#e5484d]" />
          <span className="min-w-0 flex-1">
            <span className="block">Enter URL</span>
            <input
              value={url}
              onChange={(e) => onUrl(e.target.value)}
              onFocus={() => onChoice('url')}
              placeholder="https://…/image.jpg"
              className={`${inputClass} mt-1.5`}
            />
          </span>
        </label>
        <label className="flex items-start gap-2.5 text-[12.5px]">
          <input type="radio" checked={choice === 'upload'} onChange={() => onChoice('upload')} className="mt-0.5 size-3.5 accent-[#e5484d]" />
          <span className="min-w-0 flex-1">
            <span className="block">Upload image</span>
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              onChange={(e) => onFile(e.target.files?.[0] ?? null)}
              className="mt-1.5 block w-full text-[11.5px] text-fg-muted file:mr-3 file:rounded-md file:border file:border-hairline-strong file:bg-surface-2 file:px-3 file:py-1.5 file:text-[11.5px] file:text-fg-muted hover:file:text-fg"
            />
            {file ? (
              <span className="mt-1 block truncate text-[11px] text-fg-dim">
                {file.name} · {(file.size / 1024).toFixed(0)} KB
              </span>
            ) : null}
          </span>
        </label>
        <p className="pt-1 text-[11.5px] leading-relaxed text-fg-dim">
          {kind === 'poster'
            ? 'JPEG, PNG, WebP or GIF up to 20 MB. The poster is shown on cards, rows and the detail page.'
            : 'JPEG, PNG, WebP or GIF up to 20 MB. The backdrop is shown behind the detail page and in the home hero.'}
        </p>
      </div>
    </div>
  );
}

function LocalSearchDialog({
  item,
  onClose,
  onApplied,
  onBestEffort,
}: {
  item: ItemDetail;
  onClose: () => void;
  onApplied: (d: ItemDetail) => void;
  onBestEffort: (source: Item) => void;
}) {
  const seriesTarget = item.kind === 'episode' || item.kind === 'season';
  const defaultQuery = seriesTarget ? item.showTitle || item.title : item.title;
  const [query, setQuery] = useState(defaultQuery);
  const [searchFor, setSearchFor] = useState(defaultQuery);
  const [selected, setSelected] = useState<number | null>(null);

  const results = useQuery({
    queryKey: ['local-search', item.libraryId, searchFor],
    queryFn: () => api.search(searchFor, 60),
    enabled: searchFor.trim().length > 0,
    retry: false,
  });

  const apply = useMutation({
    mutationFn: async (source: Item) => {
      if (seriesTarget && source.kind === 'show') {
        if (!source.providerId) {
          throw new Error('This series has no TMDb match yet — match the series first.');
        }
        const lang = window.localStorage.getItem('mvs.tmdbLanguage') || 'en-US';
        if (item.kind === 'episode') {
          return api.match(item.id, source.providerId, lang, item.seasonNumber, item.episodeNumber);
        }
        return api.match(item.id, source.providerId, lang, item.seasonNumber);
      }
      return api.applyLocal(item.id, source.id);
    },
    onSuccess: (d) => onApplied(d),
    onError: (err, source) => {
      const e = err as { code?: string; status?: number };
      if (e.code === 'not_found' || e.status === 404) onBestEffort(source);
    },
  });

  const targetKind = seriesTarget ? 'show' : item.kind;
  const candidates = (results.data?.items ?? []).filter(
    (c) => c.kind === targetKind && c.id !== item.id && c.libraryId === item.libraryId,
  );
  const selectedItem = candidates.find((c) => c.id === selected) ?? null;

  return (
    <Dialog
      title="Select from existing videos"
      onClose={onClose}
      z={50}
      overlay="bg-black/60"
      size="max-w-4xl"
      footer={
        <>
          <p className="text-[11px] text-fg-dim">
            {seriesTarget
              ? 'Select the TV series; the episode/season is fetched from TMDb using the series match.'
              : 'Copies title, overview, genres, cast and artwork from the selected item.'}
          </p>
          <div className="flex items-center gap-2">
            <DialogButton onClick={onClose}>Cancel</DialogButton>
            <DialogButton
              variant="primary"
              disabled={!selectedItem || apply.isPending}
              onClick={() => selectedItem && apply.mutate(selectedItem)}
            >
              {apply.isPending ? <Loader2 className="size-3.5 animate-spin" aria-hidden /> : <Check className="size-3.5" aria-hidden />}
              Apply
            </DialogButton>
          </div>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const q = query.trim();
          if (!q) return;
          if (q === searchFor) void results.refetch();
          else setSearchFor(q);
        }}
        className="flex items-center gap-2 border-b border-hairline px-5 py-3"
      >
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-fg-dim" aria-hidden />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={seriesTarget ? 'Search TV series in your library…' : 'Search your library…'}
            aria-label="Search your library"
            className="h-9 w-full rounded-md border border-hairline-strong bg-surface-2 pl-8 pr-3 text-[12.5px] text-fg outline-none placeholder:text-fg-dim"
          />
        </div>
        <button
          type="submit"
          className="inline-flex h-9 items-center gap-1.5 rounded-md bg-[#2f7de1] px-3.5 text-[12.5px] font-medium text-white hover:bg-[#3b8bea]"
        >
          <Search className="size-3.5" aria-hidden /> Search
        </button>
      </form>

      {apply.isError ? <p className="px-5 py-2 text-[12px] text-active">{(apply.error as Error).message}</p> : null}

      <div className="max-h-[52vh] overflow-y-auto">
        {results.isFetching ? (
          <p className="px-4 py-8 text-center text-[12.5px] text-fg-dim">Searching…</p>
        ) : candidates.length === 0 ? (
          <p className="px-4 py-8 text-center text-[12.5px] text-fg-dim">
            No matching {seriesTarget ? 'TV series' : item.kind === 'show' ? 'TV shows' : 'movies'} in this library.
          </p>
        ) : (
          <table className="w-full text-left text-[12px]">
            <thead>
              <tr className="border-b border-hairline text-[11px] uppercase tracking-wide text-fg-dim">
                <th className="w-14 px-4 py-2 font-medium">Art</th>
                <th className="px-3 py-2 font-medium">Title</th>
                <th className="w-24 px-3 py-2 font-medium">Year</th>
                <th className="px-3 py-2 font-medium">Summary</th>
              </tr>
            </thead>
            <tbody>
              {candidates.map((c) => (
                <tr
                  key={c.id}
                  onClick={() => setSelected(c.id)}
                  onDoubleClick={() => apply.mutate(c)}
                  className={['cursor-pointer border-b border-hairline/60', selected === c.id ? 'bg-active/10' : 'hover:bg-surface-2'].join(' ')}
                >
                  <td className="px-4 py-1.5">
                    <div className="h-[54px] w-9 overflow-hidden rounded bg-surface-3">
                      {c.posterUrl || c.backdropUrl ? (
                        <img src={c.posterUrl || c.backdropUrl} alt="" loading="lazy" className="h-full w-full object-cover" />
                      ) : (
                        <div className="grid h-full w-full place-items-center text-fg-dim">
                          <ImageIcon className="size-3.5" aria-hidden />
                        </div>
                      )}
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <span className="font-medium text-fg">{c.title}</span>
                    {c.kind === 'episode' ? (
                      <span className="ml-2 text-fg-dim">
                        {c.seasonNumber ? `S${String(c.seasonNumber).padStart(2, '0')}` : ''}
                        {c.episodeNumber ? `E${String(c.episodeNumber).padStart(2, '0')}` : ''}
                      </span>
                    ) : null}
                  </td>
                  <td className="px-3 py-2 tabular-nums text-fg-muted">{c.year ?? '—'}</td>
                  <td className="px-3 py-2 text-fg-muted">
                    <span className="line-clamp-1">{c.overview || '—'}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </Dialog>
  );
}

function SearchPluginDialog({
  item,
  query: initialQuery,
  mode,
  onClose,
  onMatched,
  onBestEffort,
}: {
  item: ItemDetail;
  query: string;
  mode?: 'episode' | 'season';
  onClose: () => void;
  onMatched: (d: ItemDetail) => void;
  onBestEffort: (c: Candidate) => void;
}) {
  const [query, setQuery] = useState(initialQuery);
  const [language, setLanguage] = useState(() => {
    const saved = typeof window !== 'undefined' ? window.localStorage.getItem(LANG_KEY) : null;
    return saved && LANGUAGES.some(([v]) => v === saved) ? saved : 'en-US';
  });
  const [searchFor, setSearchFor] = useState({ q: initialQuery, language });
  const [season, setSeason] = useState(item.seasonNumber ? String(item.seasonNumber) : '');
  const [episodeNo, setEpisodeNo] = useState(item.episodeNumber ? String(item.episodeNumber) : '');
  const [selected, setSelected] = useState<string | null>(null);
  const [provider, setProvider] = useState(() => {
    const saved = typeof window !== 'undefined' ? window.localStorage.getItem('mvs.metadataProvider') : null;
    return saved || 'tmdb';
  });
  const providers = useQuery({ queryKey: ['providers'], queryFn: api.providers, staleTime: 60_000 });
  // Providers that are configured and support this item kind. With one provider
  // the selector is hidden; future plugins appear here automatically.
  const available = (providers.data?.providers ?? []).filter(
    (p) => p.configured && p.kinds.includes(item.kind),
  );

  useEffect(() => {
    if (!providers.data) return;
    if (!available.some((p) => p.name === provider)) {
      setProvider(available[0]?.name ?? 'tmdb');
    }
  }, [providers.data, provider, item.kind]);

  const matches = useQuery({
    queryKey: ['matches', item.id, searchFor.q, searchFor.language, provider],
    queryFn: () => api.matches(item.id, searchFor.q, undefined, searchFor.language, provider),
    enabled: searchFor.q.trim().length > 0 && !providers.isLoading,
    retry: false,
  });

  const match = useMutation({
    mutationFn: (c: Candidate) => {
      if (mode === 'episode') {
        return api.match(
          item.id,
          c.providerId,
          searchFor.language,
          Number(season) || undefined,
          Number(episodeNo) || undefined,
          provider,
        );
      }
      if (mode === 'season') {
        return api.match(item.id, c.providerId, searchFor.language, Number(season) || undefined, undefined, provider);
      }
      return api.match(item.id, c.providerId, searchFor.language, undefined, undefined, provider);
    },
    onSuccess: (d) => onMatched(d),
    onError: (err, c) => {
      const e = err as { code?: string; status?: number };
      if (e.code === 'not_found' || e.status === 404) onBestEffort(c);
    },
  });

  const search = () => {
    const q = query.trim();
    if (!q) return;
    if (q === searchFor.q && language === searchFor.language) void matches.refetch();
    else setSearchFor({ q, language });
  };

  const candidates = matches.data?.candidates ?? [];
  const selectedCandidate = candidates.find((c) => c.providerId === selected) ?? null;

  return (
    <Dialog
      title="Search from Video Info Plugin"
      onClose={onClose}
      z={50}
      overlay="bg-black/60"
      size="max-w-4xl"
      footer={
        <>
          <a
            href="https://www.themoviedb.org/"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-[11px] text-fg-dim hover:text-fg-muted"
          >
            Metadata by TMDB <ExternalLink className="size-3" aria-hidden />
          </a>
          <div className="flex items-center gap-2">
            <DialogButton onClick={onClose}>Cancel</DialogButton>
            <DialogButton
              variant="primary"
              disabled={!selectedCandidate || match.isPending}
              onClick={() => selectedCandidate && match.mutate(selectedCandidate)}
            >
              {match.isPending ? <Loader2 className="size-3.5 animate-spin" aria-hidden /> : <Check className="size-3.5" aria-hidden />}
              OK
            </DialogButton>
          </div>
        </>
      }
    >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            search();
          }}
          className="flex flex-wrap items-center gap-2 border-b border-hairline px-5 py-3"
        >
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-fg-dim" aria-hidden />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Title"
              aria-label="Search title"
              className="h-9 w-full rounded-md border border-hairline-strong bg-surface-2 pl-8 pr-8 text-[12.5px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong"
            />
            {query ? (
              <button
                type="button"
                onClick={() => setQuery('')}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-fg-dim hover:text-fg"
                aria-label="Clear search"
              >
                <X className="size-3.5" aria-hidden />
              </button>
            ) : null}
          </div>
          {available.length > 1 ? (
            <label className="flex items-center gap-2 text-[12px] text-fg-muted">
              Provider:
              <select
                value={provider}
                onChange={(e) => {
                  const v = e.target.value;
                  setProvider(v);
                  window.localStorage.setItem('mvs.metadataProvider', v);
                }}
                className="h-9 rounded-md border border-hairline-strong bg-surface-2 px-2 text-[12.5px] text-fg outline-none"
              >
                {available.map((p) => (
                  <option key={p.name} value={p.name}>
                    {providerLabel(p.name)}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          {mode ? (
            <>
              <label className="flex items-center gap-1.5 text-[12px] text-fg-muted">
                S
                <input
                  type="number"
                  min={1}
                  value={season}
                  onChange={(e) => setSeason(e.target.value)}
                  aria-label="Season number"
                  className="h-9 w-16 rounded-md border border-hairline-strong bg-surface-2 px-2 text-[12.5px] text-fg outline-none"
                />
              </label>
              {mode === 'episode' ? (
                <label className="flex items-center gap-1.5 text-[12px] text-fg-muted">
                  E
                  <input
                    type="number"
                    min={1}
                    value={episodeNo}
                    onChange={(e) => setEpisodeNo(e.target.value)}
                    aria-label="Episode number"
                    className="h-9 w-16 rounded-md border border-hairline-strong bg-surface-2 px-2 text-[12.5px] text-fg outline-none"
                  />
                </label>
              ) : null}
            </>
          ) : null}
          <label className="flex items-center gap-2 text-[12px] text-fg-muted">
            Language:
            <select
              value={language}
              onChange={(e) => {
                const v = e.target.value;
                setLanguage(v);
                window.localStorage.setItem(LANG_KEY, v);
              }}
              className="h-9 rounded-md border border-hairline-strong bg-surface-2 px-2 text-[12.5px] text-fg outline-none"
            >
              {LANGUAGES.map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <button
            type="submit"
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-[#2f7de1] px-3.5 text-[12.5px] font-medium text-white hover:bg-[#3b8bea]"
          >
            <Search className="size-3.5" aria-hidden /> Search
          </button>
        </form>
        {mode === 'episode' ? (
          <p className="border-b border-hairline px-5 py-2 text-[11.5px] text-fg-dim">
            Select the TV show — the episode is fetched for season {season || '—'} / episode {episodeNo || '—'}.
          </p>
        ) : mode === 'season' ? (
          <p className="border-b border-hairline px-5 py-2 text-[11.5px] text-fg-dim">
            Select the TV show — the season is fetched for season {season || '—'}.
          </p>
        ) : null}

        <div className="max-h-[52vh] overflow-y-auto">
          {matches.isFetching ? (
            <p className="px-4 py-8 text-center text-[12.5px] text-fg-dim">Searching…</p>
          ) : matches.isError ? (
            <div className="mx-4 my-3 rounded-md border border-active/30 bg-active/5 px-3 py-2.5 text-[12px] text-fg-muted">
              {(matches.error as Error).message}
              {(matches.error as Error).message.includes('TMDB API key') ? ' (Settings → Metadata)' : ''}
            </div>
          ) : (
            <table className="w-full text-left text-[12px]">
              <thead>
                <tr className="border-b border-hairline text-[11px] uppercase tracking-wide text-fg-dim">
                  <th className="w-14 px-4 py-2 font-medium">Poster</th>
                  <th className="px-3 py-2 font-medium">Title</th>
                  <th className="w-28 px-3 py-2 font-medium">Release date</th>
                  <th className="w-20 px-3 py-2 font-medium">Source</th>
                  <th className="px-3 py-2 font-medium">Summary</th>
                </tr>
              </thead>
              <tbody>
                {candidates.map((c) => (
                  <tr
                    key={c.providerId}
                    onClick={() => setSelected(c.providerId)}
                    onDoubleClick={() => match.mutate(c)}
                    className={[
                      'cursor-pointer border-b border-hairline/60',
                      selected === c.providerId ? 'bg-active/10' : 'hover:bg-surface-2',
                    ].join(' ')}
                  >
                    <td className="px-4 py-1.5">
                      <div className="h-[54px] w-9 overflow-hidden rounded bg-surface-3">
                        {c.posterUrl ? (
                          <img src={c.posterUrl} alt="" loading="lazy" className="h-full w-full object-cover" />
                        ) : (
                          <div className="grid h-full w-full place-items-center text-fg-dim">
                            <ImageIcon className="size-3.5" aria-hidden />
                          </div>
                        )}
                      </div>
                    </td>
                    <td className="px-3 py-2">
                      <span className="font-medium text-fg">{c.title}</span>
                      {c.originalTitle && c.originalTitle !== c.title ? (
                        <span className="ml-2 text-fg-dim">{c.originalTitle}</span>
                      ) : null}
                    </td>
                    <td className="px-3 py-2 tabular-nums text-fg-muted">{c.year ?? '—'}</td>
                    <td className="px-3 py-2 text-fg-muted">{providerLabel(c.provider || provider)}</td>
                    <td className="px-3 py-2 text-fg-muted">
                      <span className="line-clamp-1">{c.overview || '—'}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {!matches.isFetching && !matches.isError && candidates.length === 0 ? (
            <div className="flex items-center justify-center gap-3 px-4 py-8 text-[12.5px] text-fg-dim">
              No data
              <button type="button" onClick={() => void matches.refetch()} className="text-fg-muted hover:text-fg" aria-label="Search again">
                <RefreshCw className="size-3.5" aria-hidden />
              </button>
            </div>
          ) : null}
        </div>

        {match.isError ? <p className="px-5 py-2 text-[12px] text-active">{(match.error as Error).message}</p> : null}

    </Dialog>
  );
}

function providerLabel(name?: string): string {
  switch (name) {
    case 'tmdb':
      return 'TMDb';
    default:
      return name || 'Provider';
  }
}

function castOf(item: ItemDetail): CastEntry[] {
  return item.people
    .filter((p) => p.role === 'cast')
    .sort((a, b) => a.ordering - b.ordering)
    .map((p) => ({ name: p.name, character: p.character }));
}

function toDateInput(value?: string): string {
  if (!value) return '';
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(value);
  return m ? `${m[1]}-${m[2]}-${m[3]}` : '';
}

const inputClass =
  'w-full rounded-md border border-hairline-strong bg-surface-2 px-3 py-1.5 text-[12.5px] text-fg outline-none placeholder:text-fg-dim focus:border-active/60 focus:bg-surface-3';
