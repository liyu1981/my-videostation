import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, KeyRound, RefreshCw } from 'lucide-react';
import { api } from '../lib/api';
import { Loading } from '../components/State';

export function SettingsPage() {
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const [key, setKey] = useState('');
  const [language, setLanguage] = useState('');
  const [region, setRegion] = useState('');
  const [saved, setSaved] = useState(false);

  const save = useMutation({
    mutationFn: () =>
      api.saveSettings({
        ...(key.trim() ? { tmdbApiKey: key.trim() } : {}),
        ...(language.trim() ? { language: language.trim() } : {}),
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
            <span className="mb-1 block text-[12px] text-fg-muted">Metadata language</span>
            <input
              value={language}
              onChange={(e) => setLanguage(e.target.value)}
              placeholder={settings.data?.metadata.language ?? 'en-US'}
              className={inputClass}
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
              Remove key
            </button>
          ) : null}
          {saved ? <span className="text-[11.5px] text-progress">Saved</span> : null}
        </div>
      </form>

      <section className="mt-6 rounded-[var(--radius-panel)] border border-hairline bg-surface/60 p-5">
        <h2 className="text-[13.5px] font-semibold">How matching works</h2>
        <ol className="mt-2 list-decimal space-y-1 pl-5 text-[12px] text-fg-muted">
          <li>Title and year from the filename are compared against TMDB search results.</li>
          <li>Candidates below 55% confidence are rejected and the item is left unmatched.</li>
          <li>Locked fields (metadata locks) are never overwritten by a refresh.</li>
          <li>Use “Identify” on an item to pick the right entry manually — that always wins.</li>
        </ol>
      </section>
    </div>
  );
}

const inputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';
