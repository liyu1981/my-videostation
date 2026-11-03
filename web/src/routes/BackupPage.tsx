import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { Archive, DatabaseBackup, Download, RefreshCw, Upload } from 'lucide-react';
import { api, type BackupArchive } from '../lib/api';
import { formatDateTime } from '../lib/format';
import { ErrorState, Loading } from '../components/State';

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

export function BackupPage() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const exportsQ = useQuery({ queryKey: ['backup', 'exports'], queryFn: api.backupExports });
  const importsQ = useQuery({ queryKey: ['backup', 'imports'], queryFn: api.backupImports });
  const [selected, setSelected] = useState('');

  useEffect(() => {
    if (!selected && importsQ.data?.imports.length) {
      setSelected(importsQ.data.imports[0]!.name);
    }
  }, [importsQ.data, selected]);

  const createExport = useMutation({
    mutationFn: api.createExport,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['backup', 'exports'] });
    },
  });

  const importBundle = useMutation({
    mutationFn: (name: string) => api.importBackup(name),
    onSuccess: () => {
      void navigate({ to: '/maintenance' });
    },
  });

  if (exportsQ.isLoading || importsQ.isLoading) return <Loading />;
  if (exportsQ.isError) return <ErrorState error={exportsQ.error} />;
  if (importsQ.isError) return <ErrorState error={importsQ.error} />;

  const exports = exportsQ.data!.exports;
  const imports = importsQ.data!.imports;

  const confirmImport = () => {
    if (!selected) return;
    const ok = confirm(
      `Import “${selected}”?\n\n` +
        'A safety export of the current data will be created first.\n' +
        'The database and cache will be replaced, then the server will restart.\n' +
        'The UI will enter maintenance mode. Media files are not touched.\n\n' +
        'Continue?',
    );
    if (ok) importBundle.mutate(selected);
  };

  return (
    <div className="mx-auto max-w-5xl px-6 py-7 lg:px-8">
      <header className="mb-6">
        <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Backup &amp; restore</h1>
        <p className="mt-1 text-[12px] text-fg-dim">
          Export a portable MyVideoStation bundle, or restore the newest bundle placed in the import folder.
        </p>
      </header>

      <section className="rounded-[var(--radius-panel)] border border-hairline bg-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <span className="grid size-9 place-items-center rounded-lg bg-surface-3">
              <DatabaseBackup className="size-4 text-progress" aria-hidden />
            </span>
            <div>
              <h2 className="text-[13.5px] font-semibold">Export current data</h2>
              <p className="mt-0.5 text-[11.5px] text-fg-dim">
                Creates a timestamped <code>.tar.gz</code> in <code>data/export</code> with the SQLite snapshot, artwork/subtitle cache and manifest.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={() => createExport.mutate()}
            disabled={createExport.isPending}
            className="inline-flex items-center gap-2 rounded-lg bg-progress/90 px-3.5 py-2 text-[12.5px] font-medium text-[#062027] hover:bg-progress disabled:opacity-60"
          >
            <Download className="size-4" aria-hidden /> {createExport.isPending ? 'Exporting…' : 'Create export'}
          </button>
        </div>
        {createExport.isError ? <p className="mt-2 text-[12px] text-active">{(createExport.error as Error).message}</p> : null}
        {createExport.isSuccess ? <p className="mt-2 text-[12px] text-progress">Created {createExport.data.name}.</p> : null}

        {exports.length > 0 ? (
          <ul className="mt-4 divide-y divide-hairline rounded-md border border-hairline">
            {exports.map((archive) => (
              <li key={archive.name} className="flex flex-wrap items-center gap-2 px-3 py-2 text-[11.5px]">
                <Archive className="size-3.5 shrink-0 text-fg-dim" aria-hidden />
                <span className="min-w-0 flex-1 truncate font-mono text-fg-muted">{archive.name}</span>
                <span className="text-fg-dim">{formatBytes(archive.size)}</span>
                <span className="text-fg-dim">{formatDateTime(archive.createdAt)}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-4 text-[11.5px] text-fg-dim">No exports yet.</p>
        )}
      </section>

      <section className="mt-5 rounded-[var(--radius-panel)] border border-hairline bg-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <span className="grid size-9 place-items-center rounded-lg bg-surface-3">
              <Upload className="size-4 text-new" aria-hidden />
            </span>
            <div>
              <h2 className="text-[13.5px] font-semibold">Import a bundle</h2>
              <p className="mt-0.5 text-[11.5px] text-fg-dim">
                Copy a <code>.tar.gz</code> export into <code>data/import</code> (Docker: <code>/data/import</code>), then refresh and import the newest bundle.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={() => void importsQ.refetch()}
            className="inline-flex items-center gap-2 rounded-lg border border-hairline px-3 py-2 text-[12px] text-fg-muted hover:border-hairline-strong hover:text-fg"
          >
            <RefreshCw className="size-3.5" aria-hidden /> Refresh
          </button>
        </div>

        {imports.length === 0 ? (
          <div className="mt-4 rounded-md border border-dashed border-hairline px-4 py-6 text-center text-[11.5px] text-fg-dim">
            No bundles in <code>data/import</code>.
          </div>
        ) : (
          <>
            <div className="mt-4 space-y-1">
              {imports.map((archive: BackupArchive) => (
                <label
                  key={archive.name}
                  className={[
                    'flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-[11.5px]',
                    selected === archive.name ? 'border-progress/50 bg-progress/5 text-fg' : 'border-hairline text-fg-muted hover:bg-surface-2',
                  ].join(' ')}
                >
                  <input
                    type="radio"
                    name="import-bundle"
                    checked={selected === archive.name}
                    onChange={() => setSelected(archive.name)}
                    className="size-3.5 accent-[#22d3ee]"
                  />
                  <span className="min-w-0 flex-1 truncate font-mono">{archive.name}</span>
                  <span className="text-fg-dim">{formatBytes(archive.size)}</span>
                  <span className="text-fg-dim">{formatDateTime(archive.createdAt)}</span>
                </label>
              ))}
            </div>
            <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
              <p className="text-[10.5px] text-new">
                A safety export is created automatically before the import. The server restarts and the UI enters maintenance.
              </p>
              <button
                type="button"
                onClick={confirmImport}
                disabled={!selected || importBundle.isPending}
                className="inline-flex items-center gap-2 rounded-lg bg-active px-3.5 py-2 text-[12.5px] font-medium text-white hover:bg-active/90 disabled:opacity-50"
              >
                <Upload className="size-4" aria-hidden /> {importBundle.isPending ? 'Preparing…' : 'Import selected bundle'}
              </button>
            </div>
            {importBundle.isError ? <p className="mt-2 text-[12px] text-active">{(importBundle.error as Error).message}</p> : null}
          </>
        )}
      </section>
    </div>
  );
}
