import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { FolderPlus, HardDrive, Pencil, Plus, RefreshCw, RotateCcw, Sparkles, Trash2, X } from 'lucide-react';
import { api, type Job, type Library, type LibraryType } from '../lib/api';
import { formatDateTime } from '../lib/format';
import { JobMessage } from '../components/JobMessage';
import { ErrorState, Loading } from '../components/State';

const TYPE_LABEL: Record<LibraryType, string> = {
  movie: 'Movies',
  tv: 'TV Shows',
  home_video: 'Home Videos',
  tv_recording: 'TV Recordings',
};

export function LibrariesPage() {
  const qc = useQueryClient();
  const libs = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const jobs = useQuery({
    queryKey: ['jobs'],
    queryFn: () => api.jobs(100),
    refetchInterval: (query) => {
      const list = query.state.data?.jobs ?? [];
      return list.some((j) => j.status === 'running' || j.status === 'queued') ? 1000 : false;
    },
  });

  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const [showAdd, setShowAdd] = useState(false);
  const [name, setName] = useState('');
  const [type, setType] = useState<LibraryType>('movie');
  const [paths, setPaths] = useState('');
  const [editing, setEditing] = useState<Library | null>(null);

  const create = useMutation({
    mutationFn: () =>
      api.createLibrary({
        name: name.trim(),
        type,
        paths: paths
          .split('\n')
          .map((p) => p.trim())
          .filter(Boolean),
      }),
    onSuccess: async (lib) => {
      setShowAdd(false);
      setName('');
      setPaths('');
      await qc.invalidateQueries({ queryKey: ['libraries'] });
      await api.scanLibrary(lib.id);
      await qc.invalidateQueries({ queryKey: ['jobs'] });
    },
  });

  const enrich = useMutation({
    mutationFn: (id: number) => api.enrichLibrary(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['jobs'] });
    },
  });

  const rescan = useMutation({
    mutationFn: (id: number) => api.scanLibrary(id, true),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['jobs'] });
    },
  });

  const scan = useMutation({
    mutationFn: (id: number) => api.scanLibrary(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['jobs'] });
    },
  });

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteLibrary(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['libraries'] });
      void qc.invalidateQueries({ queryKey: ['home'] });
      void qc.invalidateQueries({ queryKey: ['browse'] });
    },
  });

  const update = useMutation({
    mutationFn: (input: { id: number; name: string; paths: string[]; autoScanMinutes: number }) =>
      api.updateLibrary(input.id, { name: input.name, paths: input.paths, autoScanMinutes: input.autoScanMinutes }),
    onSuccess: () => {
      setEditing(null);
      void qc.invalidateQueries({ queryKey: ['libraries'] });
    },
  });

  if (libs.isLoading) return <Loading />;
  if (libs.isError) return <ErrorState error={libs.error} />;

  return (
    <div className="mx-auto max-w-5xl px-6 py-7 lg:px-8">
      <header className="mb-6 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Libraries</h1>
          <p className="mt-1 text-[12px] text-fg-dim">
            Read-only sources. Files are indexed in place and never modified.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link to="/tasks" className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:border-hairline-strong hover:text-fg">
            All tasks
          </Link>
          <button
            type="button"
            onClick={() => setShowAdd((v) => !v)}
            className="inline-flex items-center gap-2 rounded-lg bg-progress/90 px-3.5 py-2 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
          >
            <Plus className="size-4" aria-hidden /> Add library
          </button>
        </div>
      </header>

      {showAdd ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
          className="mb-6 rounded-[var(--radius-panel)] border border-hairline bg-surface p-4"
        >
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-[13.5px] font-semibold">New library</h2>
            <button type="button" onClick={() => setShowAdd(false)} className="rounded p-1 text-fg-dim hover:text-fg" aria-label="Close">
              <X className="size-4" aria-hidden />
            </button>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="block">
              <span className="mb-1 block text-[12px] text-fg-muted">Name</span>
              <input value={name} onChange={(e) => setName(e.target.value)} required className={inputClass} placeholder="Movies" />
            </label>
            <label className="block">
              <span className="mb-1 block text-[12px] text-fg-muted">Type</span>
              <select value={type} onChange={(e) => setType(e.target.value as LibraryType)} className={inputClass}>
                {(Object.keys(TYPE_LABEL) as LibraryType[]).map((t) => (
                  <option key={t} value={t}>
                    {TYPE_LABEL[t]}
                  </option>
                ))}
              </select>
            </label>
            <label className="block sm:col-span-2">
              <span className="mb-1 block text-[12px] text-fg-muted">Folders (one absolute path per line)</span>
              <textarea
                value={paths}
                onChange={(e) => setPaths(e.target.value)}
                rows={3}
                required
                className={`${inputClass} font-mono text-[12px]`}
                placeholder="/mnt/media/movies"
              />
            </label>
          </div>
          {create.isError ? <p className="mt-2 text-[12px] text-active">{(create.error as Error).message}</p> : null}
          <div className="mt-3 flex justify-end gap-2">
            <button type="button" onClick={() => setShowAdd(false)} className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg">
              Cancel
            </button>
            <button type="submit" disabled={create.isPending} className="rounded-lg bg-surface-3 px-3 py-1.5 text-[12px] font-medium hover:bg-surface-3/80 disabled:opacity-60">
              {create.isPending ? 'Creating…' : 'Create and scan'}
            </button>
          </div>
        </form>
      ) : null}

      <div className="space-y-3">
        {libs.data!.libraries.map((lib) => (
          <div key={lib.id} className="rounded-[var(--radius-panel)] border border-hairline bg-surface p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <HardDrive className="size-4 text-fg-dim" aria-hidden />
                  <h2 className="text-[14px] font-semibold">{lib.name}</h2>
                  <span className="rounded-full bg-surface-3 px-2 py-0.5 text-[10.5px] text-fg-muted">{TYPE_LABEL[lib.type]}</span>
                  <span className="text-[11.5px] text-fg-dim">{lib.itemCount} items</span>
                  {lib.autoScanMinutes > 0 ? (
                    <span className="rounded-full border border-hairline px-2 py-0.5 text-[10.5px] text-fg-dim">
                      auto every {lib.autoScanMinutes >= 1440 ? `${lib.autoScanMinutes / 1440}d` : `${lib.autoScanMinutes / 60}h`}
                    </span>
                  ) : null}
                  {lib.unmatchedCount > 0 ? (
                    <Link
                      to="/browse"
                      search={{
                        kind: lib.type === 'tv' ? 'show' : 'movie',
                        libraryId: lib.id,
                        sort: '',
                        page: 1,
                        q: '',
                        status: 'unmatched',
                        category: 'recent_added',
                        genre: '',
                        year: undefined,
                        watched: undefined,
                        folder: '',
                      }}
                      className="rounded-full border border-new/40 bg-new/10 px-2 py-0.5 text-[10.5px] text-new hover:bg-new/15"
                    >
                      {lib.unmatchedCount} unmatched
                    </Link>
                  ) : null}
                </div>
                <ul className="mt-1.5 space-y-0.5 font-mono text-[11.5px] text-fg-dim">
                  {lib.paths.map((p) => (
                    <li key={p.id} className="truncate" title={p.path}>
                      {p.path}
                      {p.lastScanAt ? <span className="ml-2 font-sans text-[10.5px] text-fg-dim">· scanned {formatDateTime(p.lastScanAt)}</span> : null}
                    </li>
                  ))}
                </ul>
              </div>
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => enrich.mutate(lib.id)}
                  disabled={!settings.data?.metadata.apiKeySet}
                  title={settings.data?.metadata.apiKeySet ? 'Match unmatched items with TMDB' : 'Add a TMDB API key in Settings first'}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-hairline px-2.5 py-1.5 text-[11.5px] text-fg-muted hover:border-hairline-strong hover:text-fg disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <Sparkles className="size-3.5" aria-hidden /> Metadata
                </button>
                <button
                  type="button"
                  onClick={() => scan.mutate(lib.id)}
                  title="Incremental scan (only new or changed files)"
                  className="inline-flex items-center gap-1.5 rounded-lg border border-hairline px-2.5 py-1.5 text-[11.5px] text-fg-muted hover:border-hairline-strong hover:text-fg"
                >
                  <RefreshCw className="size-3.5" aria-hidden /> Scan
                </button>
                <button
                  type="button"
                  onClick={() => {
                    if (confirm(`Re-probe every file in “${lib.name}”?\n\nThis ignores the size/mtime cache and is slower than a normal scan.`)) {
                      rescan.mutate(lib.id);
                    }
                  }}
                  title="Full rescan: re-probe every file and re-apply sidecar metadata"
                  className="inline-flex items-center gap-1.5 rounded-lg border border-hairline px-2.5 py-1.5 text-[11.5px] text-fg-muted hover:border-hairline-strong hover:text-fg"
                >
                  <RotateCcw className="size-3.5" aria-hidden /> Rescan
                </button>
                <button
                  type="button"
                  onClick={() => setEditing(editing?.id === lib.id ? null : lib)}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-hairline px-2.5 py-1.5 text-[11.5px] text-fg-muted hover:border-hairline-strong hover:text-fg"
                >
                  <Pencil className="size-3.5" aria-hidden /> Edit
                </button>
                <button
                  type="button"
                  onClick={() => {
                    if (confirm(`Remove library “${lib.name}”? Media files are not deleted.`)) remove.mutate(lib.id);
                  }}
                  className="rounded-lg border border-hairline p-1.5 text-fg-dim hover:border-active/50 hover:text-active"
                  aria-label={`Delete library ${lib.name}`}
                >
                  <Trash2 className="size-3.5" aria-hidden />
                </button>
              </div>
            </div>

            {editing?.id === lib.id ? (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  const form = e.currentTarget;
                  const nameInput = form.elements.namedItem('name') as HTMLInputElement;
                  const pathsInput = form.elements.namedItem('paths') as HTMLTextAreaElement;
                  const scanSelect = form.elements.namedItem('autoScanMinutes') as HTMLSelectElement;
                  update.mutate({
                    id: lib.id,
                    name: nameInput.value.trim(),
                    paths: pathsInput.value
                      .split('\n')
                      .map((p) => p.trim())
                      .filter(Boolean),
                    autoScanMinutes: Number(scanSelect.value),
                  });
                }}
                className="mt-3 border-t border-hairline pt-3"
              >
                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="block">
                    <span className="mb-1 block text-[11.5px] text-fg-muted">Name</span>
                    <input name="name" defaultValue={lib.name} className={inputClass} />
                  </label>
                  <label className="block">
                    <span className="mb-1 block text-[11.5px] text-fg-muted">Auto-scan</span>
                    <select name="autoScanMinutes" defaultValue={String(lib.autoScanMinutes || 0)} className={inputClass}>
                      <option value="0">Off (manual only)</option>
                      <option value="60">Every hour</option>
                      <option value="360">Every 6 hours</option>
                      <option value="1440">Daily</option>
                      <option value="10080">Weekly</option>
                    </select>
                  </label>
                  <label className="block sm:col-span-2">
                    <span className="mb-1 block text-[11.5px] text-fg-muted">Folders</span>
                    <textarea name="paths" defaultValue={lib.paths.map((p) => p.path).join('\n')} rows={3} className={`${inputClass} font-mono text-[12px]`} />
                  </label>
                </div>
                {update.isError ? <p className="mt-2 text-[12px] text-active">{(update.error as Error).message}</p> : null}
                <div className="mt-2 flex justify-end gap-2">
                  <button type="button" onClick={() => setEditing(null)} className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg">
                    Cancel
                  </button>
                  <button type="submit" className="rounded-lg bg-surface-3 px-3 py-1.5 text-[12px] font-medium hover:bg-surface-3/80">
                    Save
                  </button>
                </div>
              </form>
            ) : null}

            {(() => {
              const libJobs = (jobs.data?.jobs ?? []).filter(
                (j) => j.libraryId === lib.id && (j.status === 'queued' || j.status === 'running' || j.status === 'failed'),
              );
              if (libJobs.length === 0) return null;
              return (
                <div className="mt-3 overflow-hidden rounded-lg border border-hairline bg-surface-2/40">
                  <p className="border-b border-hairline px-3 py-1.5 text-[10.5px] uppercase tracking-wide text-fg-dim">
                    Outstanding tasks
                  </p>
                  <div className="divide-y divide-hairline">
                    {libJobs.map((job) => (
                      <JobRow key={job.id} job={job} active={job.status === 'running' || job.status === 'queued'} />
                    ))}
                  </div>
                </div>
              );
            })()}
          </div>
        ))}

        {libs.data!.libraries.length === 0 && !showAdd ? (
          <div className="flex flex-col items-center gap-2 rounded-[var(--radius-panel)] border border-dashed border-hairline px-6 py-12 text-center">
            <FolderPlus className="size-7 text-fg-dim" aria-hidden />
            <p className="text-[13.5px] font-medium">No libraries yet</p>
            <p className="text-[12px] text-fg-dim">Add a folder containing movies, TV shows or home videos.</p>
          </div>
        ) : null}
      </div>

    </div>
  );
}

function JobRow({ job, active = false }: { job: Job; active?: boolean }) {
  const statusColor =
    job.status === 'succeeded'
      ? 'text-fg-dim'
      : job.status === 'failed'
        ? 'text-active'
        : 'text-progress';
  return (
    <div className="px-3 py-2">
      <div className="flex items-start justify-between gap-3 text-[12px]">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <span className={`shrink-0 font-medium ${statusColor}`}>{job.status}</span>
          <span className="shrink-0 text-fg-muted">{job.type}</span>
          <JobMessage text={job.error || job.message || ''} />
        </div>
        <span className="shrink-0 pt-0.5 text-[11px] text-fg-dim">
          {active ? `${Math.round(job.progress)}%` : formatDateTime(job.finishedAt ?? job.createdAt)}
        </span>
      </div>
      {active ? (
        <div className="mt-2 h-1 overflow-hidden rounded-full bg-surface-3">
          <div className="h-full bg-progress transition-[width]" style={{ width: `${Math.max(2, job.progress)}%` }} />
        </div>
      ) : null}
    </div>
  );
}

const inputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';
