import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, Image, KeyRound, RefreshCw, Trash2 } from 'lucide-react';
import { api, type ArtworkSettings } from '../lib/api';
import { formatBytes } from '../lib/format';
import { Loading } from '../components/State';

export function SettingsPage() {
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const [key, setKey] = useState('');
  const [region, setRegion] = useState('');
  const [saved, setSaved] = useState(false);
  const [art, setArt] = useState<ArtworkSettings>({
    thumbnails: true,
    posterWidth: 320,
    backdropWidth: 1280,
    quality: 82,
    coverPreference: 'provider',
  });
  const [artSaved, setArtSaved] = useState('');
  const [artError, setArtError] = useState('');

  useEffect(() => {
    if (settings.data?.artwork) setArt(settings.data.artwork);
  }, [settings.data?.artwork]);

  const thumbs = useQuery({ queryKey: ['thumbnail-stats'], queryFn: api.thumbnailStats });
  const jobs = useQuery({
    queryKey: ['jobs'],
    queryFn: () => api.jobs(20),
    refetchInterval: (query) => {
      const list = query.state.data?.jobs ?? [];
      return list.some((j) => j.type === 'thumbnails' && (j.status === 'running' || j.status === 'queued')) ? 1000 : false;
    },
  });
  const thumbnailJob = (jobs.data?.jobs ?? []).find((j) => j.type === 'thumbnails');

  const saveArtwork = useMutation({
    mutationFn: () => api.saveSettings({ artwork: art }),
    onSuccess: () => {
      setArtSaved('Saved');
      setArtError('');
      setTimeout(() => setArtSaved(''), 2500);
      void qc.invalidateQueries({ queryKey: ['settings'] });
      void qc.invalidateQueries({ queryKey: ['thumbnail-stats'] });
    },
    onError: (err) => setArtError((err as Error).message),
  });

  const rebuildThumbs = useMutation({
    mutationFn: () => api.rebuildThumbnails(),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['jobs'] });
      void qc.invalidateQueries({ queryKey: ['thumbnail-stats'] });
    },
    onError: (err) => setArtError((err as Error).message),
  });

  const clearThumbs = useMutation({
    mutationFn: () => api.clearThumbnails(),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['thumbnail-stats'] });
    },
    onError: (err) => setArtError((err as Error).message),
  });

  const save = useMutation({
    mutationFn: () =>
      api.saveSettings({
        ...(key.trim() ? { tmdbApiKey: key.trim() } : {}),
        ...(region.trim() ? { region: region.trim() } : {}),
      }),
    onSuccess: () => {
      setKey('');
      setSaved(true);
      setTimeout(() => setSaved(false), 2500);
      void qc.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  const clearKey = useMutation({
    mutationFn: () => api.saveSettings({ tmdbApiKey: '' }),
    onSuccess: () => {
      setSaved(true);
      setTimeout(() => setSaved(false), 2500);
      void qc.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  if (settings.isLoading) return <Loading />;

  return (
    <div className="mx-auto max-w-3xl px-6 py-7 lg:px-8">
      <header className="mb-6">
        <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Settings</h1>
        <p className="mt-1 text-[12px] text-fg-dim">Metadata provider and matching preferences.</p>
      </header>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
        className="rounded-[var(--radius-panel)] border border-hairline bg-surface p-5"
      >
        <div className="mb-4 flex items-start gap-3">
          <span className="grid size-9 place-items-center rounded-lg bg-surface-3">
            <KeyRound className="size-4 text-fg-muted" aria-hidden />
          </span>
          <div>
            <h2 className="text-[13.5px] font-semibold">The Movie Database (TMDB)</h2>
            <p className="text-[11.5px] text-fg-dim">
              Used for titles, overviews, cast, genres and artwork.{' '}
              {settings.data?.metadata.apiKeySet ? (
                <span className="text-progress">A key is configured.</span>
              ) : (
                <span className="text-new">No key configured — matching is disabled.</span>
              )}
            </p>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block sm:col-span-2">
            <span className="mb-1 block text-[12px] text-fg-muted">API key (v3) or read access token (v4)</span>
            <input
              type="password"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder={settings.data?.metadata.apiKeySet ? '•••••••• (leave blank to keep)' : 'paste your TMDB key'}
              className={inputClass}
              autoComplete="off"
            />
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Certification region</span>
            <input
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              placeholder={settings.data?.metadata.region ?? 'US'}
              className={inputClass}
            />
          </label>
        </div>

        {save.isError ? <p className="mt-3 text-[12px] text-active">{(save.error as Error).message}</p> : null}

        <div className="mt-4 flex flex-wrap items-center gap-2">
          <button
            type="submit"
            disabled={save.isPending}
            className="inline-flex items-center gap-1.5 rounded-lg bg-progress/90 px-3.5 py-1.5 text-[12.5px] font-medium text-[#062027] hover:bg-progress disabled:opacity-60"
          >
            {save.isPending ? <RefreshCw className="size-3.5 animate-spin" aria-hidden /> : <Check className="size-3.5" aria-hidden />}
            Save
          </button>
          {settings.data?.metadata.apiKeySet ? (
            <button
              type="button"
              onClick={() => {
                if (confirm('Remove the stored TMDB API key? Matching will stop working.')) clearKey.mutate();
              }}
              className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:border-active/50 hover:text-active"
            >
              Remove TMDb key
            </button>
          ) : null}
          {saved ? <span className="text-[11.5px] text-progress">Saved</span> : null}
        </div>
      </form>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          saveArtwork.mutate();
        }}
        className="mt-6 rounded-[var(--radius-panel)] border border-hairline bg-surface p-5"
      >
        <div className="mb-4 flex items-start gap-3">
          <span className="grid size-9 place-items-center rounded-lg bg-surface-3">
            <Image className="size-4 text-fg-muted" aria-hidden />
          </span>
          <div>
            <h2 className="text-[13.5px] font-semibold">Artwork &amp; thumbnails</h2>
            <p className="text-[11.5px] text-fg-dim">
              Browse pages request smaller copies of posters and backdrops. Originals are never modified or
              deleted; thumbnails live in <code className="rounded bg-surface-3 px-1">/data/cache/thumbs</code> and
              can be rebuilt at any time.
            </p>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <label className="flex items-center gap-2 text-[12px] text-fg-muted">
            <input
              type="checkbox"
              checked={art.thumbnails}
              onChange={(e) => setArt({ ...art, thumbnails: e.target.checked })}
              className="size-4 rounded border-hairline bg-surface-2"
            />
            Generate thumbnails
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Poster size</span>
            <select
              value={art.posterWidth}
              onChange={(e) => setArt({ ...art, posterWidth: Number(e.target.value) })}
              className={inputClass}
            >
              <option value={200}>Small — 200 px</option>
              <option value={320}>Medium — 320 px</option>
              <option value={480}>Large — 480 px</option>
            </select>
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Backdrop size</span>
            <select
              value={art.backdropWidth}
              onChange={(e) => setArt({ ...art, backdropWidth: Number(e.target.value) })}
              className={inputClass}
            >
              <option value={0}>Original — no scaling</option>
              <option value={960}>960 px</option>
              <option value={1280}>1280 px</option>
              <option value={1920}>1920 px</option>
            </select>
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">JPEG quality</span>
            <select
              value={art.quality}
              onChange={(e) => setArt({ ...art, quality: Number(e.target.value) })}
              className={inputClass}
            >
              <option value={70}>Smaller files — 70</option>
              <option value={82}>Balanced — 82</option>
              <option value={92}>High — 92</option>
            </select>
          </label>
          <label className="block sm:col-span-2">
            <span className="mb-1 block text-[12px] text-fg-muted">Video cover setting</span>
            <select
              value={art.coverPreference}
              onChange={(e) => setArt({ ...art, coverPreference: e.target.value as ArtworkSettings['coverPreference'] })}
              className={inputClass}
            >
              <option value="provider">Provider artwork replaces local files</option>
              <option value="local">Keep poster/backdrop files found next to the media</option>
            </select>
            <span className="mt-1 block text-[10.5px] text-fg-dim">
              Locked poster/backdrop fields are always kept. Local files are discovered on scan (sidecar names
              or .nfo thumb/fanart references).
            </span>
          </label>
        </div>

        {artError ? <p className="mt-3 text-[12px] text-active">{artError}</p> : null}

        <div className="mt-4 flex flex-wrap items-center gap-2">
          <button
            type="submit"
            disabled={saveArtwork.isPending}
            className="inline-flex items-center gap-1.5 rounded-lg bg-progress/90 px-3.5 py-1.5 text-[12.5px] font-medium text-[#062027] hover:bg-progress disabled:opacity-60"
          >
            {saveArtwork.isPending ? <RefreshCw className="size-3.5 animate-spin" aria-hidden /> : <Check className="size-3.5" aria-hidden />}
            Save
          </button>
          <button
            type="button"
            onClick={() => rebuildThumbs.mutate()}
            disabled={rebuildThumbs.isPending}
            className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:border-hairline-strong hover:text-fg disabled:opacity-50"
          >
            Rebuild thumbnails
          </button>
          <button
            type="button"
            onClick={() => clearThumbs.mutate()}
            disabled={clearThumbs.isPending || (thumbs.data?.files ?? 0) === 0}
            className="inline-flex items-center gap-1.5 rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:border-active/50 hover:text-active disabled:opacity-40"
          >
            <Trash2 className="size-3.5" aria-hidden /> Clear cache
          </button>
          {thumbs.data ? (
            <span className="text-[11.5px] text-fg-dim">
              {thumbs.data.files} file{thumbs.data.files === 1 ? '' : 's'}
              {thumbs.data.bytes > 0 ? ` · ${formatBytes(thumbs.data.bytes)}` : ''}
            </span>
          ) : null}
          {artSaved ? <span className="text-[11.5px] text-progress">{artSaved}</span> : null}
        </div>
        {thumbnailJob && (thumbnailJob.status === 'running' || thumbnailJob.status === 'queued') ? (
          <p className="mt-2 text-[11px] text-progress">
            Rebuilding thumbnails… {Math.round(thumbnailJob.progress)}%
          </p>
        ) : null}
        {thumbnailJob?.status === 'succeeded' ? (
          <p className="mt-2 text-[11px] text-fg-dim">{thumbnailJob.message}</p>
        ) : null}
      </form>

      <section className="mt-6 rounded-[var(--radius-panel)] border border-hairline bg-surface/60 p-5">
        <h2 className="text-[13.5px] font-semibold">How matching works</h2>
        <ol className="mt-2 list-decimal space-y-1 pl-5 text-[12px] text-fg-muted">
          <li>Automatic enrichment matches title/year against TMDb.</li>
          <li>Automatic matching uses a best-effort 35% threshold; low-confidence matches are flagged for review.</li>
          <li>Locked fields are never overwritten by a refresh; manual matches and edits always win.</li>
          <li>Use Edit video info to search the provider and pick the right entry manually.</li>
          <li>Additional metadata providers can be plugged in; the search dialog shows a provider selector when more than one is installed.</li>
        </ol>
      </section>
    </div>
  );
}

const inputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';
