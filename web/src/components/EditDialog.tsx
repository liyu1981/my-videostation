import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Lock, LockOpen, X } from 'lucide-react';
import { api, type ItemDetail, type ItemPatch } from '../lib/api';

const LOCK_FIELDS = [
  ['title', 'Title'],
  ['original_title', 'Original title'],
  ['year', 'Year'],
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

export function EditDialog({ item, onClose }: { item: ItemDetail; onClose: () => void }) {
  const qc = useQueryClient();
  const [title, setTitle] = useState(item.title);
  const [year, setYear] = useState(item.year ? String(item.year) : '');
  const [tagline, setTagline] = useState(item.tagline ?? '');
  const [overview, setOverview] = useState(item.overview ?? '');
  const [rating, setRating] = useState(item.rating ? String(item.rating) : '');
  const [contentRating, setContentRating] = useState(item.contentRating ?? '');
  const [genres, setGenres] = useState(item.genres.join(', '));
  const [posterUrl, setPosterUrl] = useState('');
  const [backdropUrl, setBackdropUrl] = useState('');
  const [locks, setLocks] = useState<string[]>(item.locks);
  const [locked, setLocked] = useState(item.locked);

  const mutation = useMutation({
    mutationFn: async (patch: ItemPatch) => {
      const res = await api.patchItem(item.id, patch);
      if (posterUrl.trim()) await api.setArt(item.id, 'poster', posterUrl.trim());
      if (backdropUrl.trim()) await api.setArt(item.id, 'backdrop', backdropUrl.trim());
      return res;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['item', item.id] });
      void qc.invalidateQueries({ queryKey: ['browse'] });
      void qc.invalidateQueries({ queryKey: ['home'] });
      void qc.invalidateQueries({ queryKey: ['search'] });
      onClose();
    },
  });

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const patch: ItemPatch = {
      title: title.trim(),
      tagline: tagline.trim(),
      overview: overview.trim(),
      contentRating: contentRating.trim(),
      genres: genres
        .split(',')
        .map((g) => g.trim())
        .filter(Boolean),
      locks,
      locked,
    };
    const y = Number(year);
    if (year.trim() !== '' && Number.isFinite(y)) patch.year = y;
    const r = Number(rating);
    if (rating.trim() !== '' && Number.isFinite(r)) patch.rating = r;
    mutation.mutate(patch);
  };

  const toggleLock = (field: string) => {
    setLocks((prev) => (prev.includes(field) ? prev.filter((f) => f !== field) : [...prev, field]));
  };

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/70 p-4 sm:items-center">
      <form
        onSubmit={submit}
        className="w-full max-w-3xl rounded-[var(--radius-panel)] border border-hairline bg-surface shadow-2xl"
      >
        <header className="flex items-center justify-between border-b border-hairline px-5 py-3.5">
          <div>
            <h2 className="text-[14px] font-semibold">Edit metadata</h2>
            <p className="text-[11.5px] text-fg-dim">Manual edits are preserved; locked fields survive future refreshes.</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-lg p-2 text-fg-dim hover:bg-surface-2 hover:text-fg" aria-label="Close">
            <X className="size-4" aria-hidden />
          </button>
        </header>

        <div className="grid max-h-[70vh] gap-4 overflow-y-auto p-5 sm:grid-cols-2">
          <label className="block sm:col-span-2">
            <span className="mb-1 block text-[12px] text-fg-muted">Title</span>
            <input value={title} onChange={(e) => setTitle(e.target.value)} className={inputClass} />
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Year</span>
            <input value={year} onChange={(e) => setYear(e.target.value)} inputMode="numeric" className={inputClass} />
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Rating (0–10)</span>
            <input value={rating} onChange={(e) => setRating(e.target.value)} inputMode="decimal" className={inputClass} />
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Certification</span>
            <input value={contentRating} onChange={(e) => setContentRating(e.target.value)} placeholder="PG-13" className={inputClass} />
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Genres (comma separated)</span>
            <input value={genres} onChange={(e) => setGenres(e.target.value)} placeholder="Drama, History" className={inputClass} />
          </label>
          <label className="block sm:col-span-2">
            <span className="mb-1 block text-[12px] text-fg-muted">Tagline</span>
            <input value={tagline} onChange={(e) => setTagline(e.target.value)} className={inputClass} />
          </label>
          <label className="block sm:col-span-2">
            <span className="mb-1 block text-[12px] text-fg-muted">Overview</span>
            <textarea value={overview} onChange={(e) => setOverview(e.target.value)} rows={5} className={`${inputClass} resize-y`} />
          </label>

          <label className="block sm:col-span-2">
            <span className="mb-1 block text-[12px] text-fg-muted">Poster image URL (optional)</span>
            <input
              value={posterUrl}
              onChange={(e) => setPosterUrl(e.target.value)}
              placeholder="https://…/poster.jpg"
              className={inputClass}
            />
          </label>
          <label className="block sm:col-span-2">
            <span className="mb-1 block text-[12px] text-fg-muted">Backdrop image URL (optional)</span>
            <input
              value={backdropUrl}
              onChange={(e) => setBackdropUrl(e.target.value)}
              placeholder="https://…/backdrop.jpg"
              className={inputClass}
            />
          </label>

          <fieldset className="sm:col-span-2">
            <legend className="mb-2 text-[12px] text-fg-muted">Locked fields</legend>
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
                      'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11.5px] transition-colors',
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
          </fieldset>

          <label className="flex items-center gap-2 text-[12.5px] text-fg-muted sm:col-span-2">
            <input
              type="checkbox"
              checked={locked}
              onChange={(e) => setLocked(e.target.checked)}
              className="size-4 rounded border-hairline bg-surface-2"
            />
            Lock all metadata for this item
          </label>

          {mutation.isError ? (
            <p className="text-[12px] text-active sm:col-span-2">{(mutation.error as Error).message}</p>
          ) : null}
        </div>

        <footer className="flex items-center justify-end gap-2 border-t border-hairline px-5 py-3">
          <button type="button" onClick={onClose} className="rounded-lg border border-hairline px-3.5 py-1.5 text-[12.5px] text-fg-muted hover:text-fg">
            Cancel
          </button>
          <button
            type="submit"
            disabled={mutation.isPending}
            className="rounded-lg bg-progress/90 px-3.5 py-1.5 text-[12.5px] font-medium text-[#062027] hover:bg-progress disabled:opacity-60"
          >
            {mutation.isPending ? 'Saving…' : 'Save changes'}
          </button>
        </footer>
      </form>
    </div>
  );
}

const inputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';
