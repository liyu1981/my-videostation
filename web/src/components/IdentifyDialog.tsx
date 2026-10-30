import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, ExternalLink, Search, X } from 'lucide-react';
import { api, type Candidate, type ItemDetail } from '../lib/api';

/**
 * Identify dialog: search the metadata provider and apply a manual match.
 * Mirrors Video Station's "Search from Video Info Plugin" flow.
 */
export function IdentifyDialog({ item, onClose }: { item: ItemDetail; onClose: () => void }) {
  const qc = useQueryClient();
  const [query, setQuery] = useState(item.title);
  const [year, setYear] = useState(item.year ? String(item.year) : '');
  const [submitted, setSubmitted] = useState(true);

  const matches = useQuery({
    queryKey: ['matches', item.id, query, year],
    queryFn: () => api.matches(item.id, query.trim(), year ? Number(year) : undefined),
    enabled: submitted && query.trim().length > 0,
    retry: false,
  });

  const match = useMutation({
    mutationFn: (c: Candidate) => api.match(item.id, c.providerId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['item', item.id] });
      void qc.invalidateQueries({ queryKey: ['browse'] });
      void qc.invalidateQueries({ queryKey: ['home'] });
      void qc.invalidateQueries({ queryKey: ['libraries'] });
      onClose();
    },
  });

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/70 p-4">
      <div className="w-full max-w-3xl rounded-[var(--radius-panel)] border border-hairline bg-surface shadow-2xl">
        <header className="flex items-start justify-between gap-4 border-b border-hairline px-5 py-3.5">
          <div>
            <h2 className="text-[14px] font-semibold">Identify “{item.title}”</h2>
            <p className="text-[11.5px] text-fg-dim">
              Search TMDB and pick the correct entry. Artwork and cast will be replaced.
            </p>
          </div>
          <button type="button" onClick={onClose} className="rounded-lg p-2 text-fg-dim hover:bg-surface-2 hover:text-fg" aria-label="Close">
            <X className="size-4" aria-hidden />
          </button>
        </header>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            setSubmitted(true);
            void matches.refetch();
          }}
          className="flex flex-wrap items-center gap-2 border-b border-hairline px-5 py-3"
        >
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Title"
            aria-label="Search title"
            className="h-9 min-w-0 flex-1 rounded-lg border border-hairline bg-surface-2 px-3 text-[13px] outline-none placeholder:text-fg-dim focus:border-hairline-strong"
          />
          <input
            value={year}
            onChange={(e) => setYear(e.target.value)}
            placeholder="Year"
            aria-label="Year"
            inputMode="numeric"
            className="h-9 w-20 rounded-lg border border-hairline bg-surface-2 px-3 text-[13px] outline-none placeholder:text-fg-dim focus:border-hairline-strong"
          />
          <button type="submit" className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-surface-3 px-3 text-[12.5px] font-medium hover:bg-surface-3/80">
            <Search className="size-3.5" aria-hidden /> Search
          </button>
        </form>

        <div className="max-h-[55vh] overflow-y-auto p-2">
          {matches.isFetching ? <p className="px-3 py-6 text-center text-[12.5px] text-fg-dim">Searching…</p> : null}

          {matches.isError ? (
            <div className="mx-2 my-2 rounded-lg border border-active/30 bg-active/5 px-3 py-2.5 text-[12px] text-fg-muted">
              {(matches.error as Error).message}
              {(matches.error as Error).message.includes('TMDB API key') ? ' (Settings → Metadata)' : ''}
            </div>
          ) : null}

          {matches.data && matches.data.candidates.length === 0 && !matches.isFetching ? (
            <p className="px-3 py-6 text-center text-[12.5px] text-fg-dim">No candidates found.</p>
          ) : null}

          {matches.data?.candidates.map((c) => (
            <div key={c.providerId} className="flex gap-3 rounded-lg px-3 py-2.5 hover:bg-surface-2">
              <div className="h-[72px] w-12 shrink-0 overflow-hidden rounded-md bg-surface-3">
                {c.posterUrl ? (
                  <img src={c.posterUrl} alt="" className="h-full w-full object-cover" loading="lazy" />
                ) : null}
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-medium">
                  {c.title}
                  {c.year ? <span className="ml-2 font-normal text-fg-dim">{c.year}</span> : null}
                  {c.rating ? <span className="ml-2 text-[11.5px] text-rating">★ {c.rating.toFixed(1)}</span> : null}
                </p>
                {c.overview ? <p className="mt-0.5 line-clamp-2 text-[11.5px] text-fg-muted">{c.overview}</p> : null}
              </div>
              <button
                type="button"
                onClick={() => match.mutate(c)}
                disabled={match.isPending}
                className="inline-flex shrink-0 items-center gap-1.5 self-center rounded-lg border border-hairline px-2.5 py-1.5 text-[11.5px] text-fg-muted hover:border-hairline-strong hover:text-fg disabled:opacity-50"
              >
                <Check className="size-3.5" aria-hidden /> Use
              </button>
            </div>
          ))}
        </div>

        <footer className="flex items-center justify-between gap-3 border-t border-hairline px-5 py-3">
          <a
            href="https://www.themoviedb.org/"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-[11px] text-fg-dim hover:text-fg-muted"
          >
            Metadata by TMDB <ExternalLink className="size-3" aria-hidden />
          </a>
          <button type="button" onClick={onClose} className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg">
            Close
          </button>
        </footer>
      </div>
    </div>
  );
}
