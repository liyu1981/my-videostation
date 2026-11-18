import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { HardDrive, Trash2 } from 'lucide-react';
import { api } from '../lib/api';
import { formatBytes, formatDateTime } from '../lib/format';
import { EmptyState, ErrorState, Loading } from '../components/State';

export function TranscodesPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ['transcodes'], queryFn: api.transcodes, staleTime: 5_000 });
  const remove = useMutation({
    mutationFn: (id: number) => api.deleteTranscode(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['transcodes'] });
      void qc.invalidateQueries({ queryKey: ['item-variants'] });
    },
  });

  const variants = list.data?.variants ?? [];
  const totalBytes = list.data?.totalBytes ?? 0;

  return (
    <div className="px-6 py-6 lg:px-8">
      <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-[20px] font-semibold tracking-tight text-fg-heading">
            <HardDrive className="size-5 text-fg-dim" aria-hidden />
            Transcode cache
          </h1>
          <p className="mt-1 text-[12px] text-fg-dim">
            {variants.length} cached version{variants.length === 1 ? '' : 's'} · {formatBytes(totalBytes)}
          </p>
        </div>
      </header>

      {list.isLoading ? <Loading /> : null}
      {list.isError ? <ErrorState error={list.error} /> : null}
      {remove.isError ? <p className="mb-3 text-[12px] text-active">{(remove.error as Error).message}</p> : null}

      {!list.isLoading && !list.isError && variants.length === 0 ? (
        <EmptyState
          title="No transcoded versions"
          description="Variants are created on demand when a browser cannot play a source directly."
        />
      ) : null}

      {variants.length > 0 ? (
        <div className="overflow-hidden rounded-[var(--radius-panel)] border border-hairline">
          <table className="w-full text-left text-[12px]">
            <thead className="bg-surface-2 text-[10.5px] uppercase tracking-wide text-fg-dim">
              <tr>
                <th className="px-3 py-2 font-medium">Original</th>
                <th className="px-3 py-2 font-medium">Variant file</th>
                <th className="px-3 py-2 font-medium">State</th>
                <th className="px-3 py-2 font-medium">Size</th>
                <th className="px-3 py-2 font-medium">Created</th>
                <th className="px-3 py-2 font-medium">Last used</th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody>
              {variants.map((v) => {
                const busy = v.state === 'queued' || v.state === 'running';
                return (
                  <tr key={v.id} className="border-t border-hairline align-top">
                    <td className="max-w-[320px] px-3 py-2">
                      <p className="truncate font-medium text-fg" title={v.originalPath}>{v.title || v.originalPath}</p>
                      <p className="truncate text-[10.5px] text-fg-dim" title={v.originalPath}>{v.originalPath}</p>
                    </td>
                    <td className="max-w-[320px] px-3 py-2">
                      <p className="truncate text-fg-muted" title={v.path}>{v.path || '—'}</p>
                      {v.error ? <p className="mt-0.5 line-clamp-2 text-[10.5px] text-active">{v.error}</p> : null}
                    </td>
                    <td className="px-3 py-2">
                      <span className={[
                        'rounded px-1.5 py-0.5 text-[10px]',
                        v.state === 'ready' ? 'bg-progress/15 text-progress'
                          : v.state === 'failed' ? 'bg-active/15 text-active'
                            : 'bg-surface-3 text-fg-muted',
                      ].join(' ')}>
                        {v.state}
                      </span>
                    </td>
                    <td className="whitespace-nowrap px-3 py-2 tabular-nums text-fg-muted">{formatBytes(v.bytes)}</td>
                    <td className="whitespace-nowrap px-3 py-2 text-fg-dim">{formatDateTime(v.createdAt)}</td>
                    <td className="whitespace-nowrap px-3 py-2 text-fg-dim">{v.lastUsedAt ? formatDateTime(v.lastUsedAt) : '—'}</td>
                    <td className="px-3 py-2 text-right">
                      <button
                        type="button"
                        disabled={busy || remove.isPending}
                        onClick={() => {
                          if (!window.confirm(`Delete this transcoded version?\n\n${v.path}`)) return;
                          remove.mutate(v.id);
                        }}
                        className="inline-flex h-7 items-center gap-1 rounded-md border border-hairline px-2 text-[11px] text-fg-muted hover:border-active/50 hover:text-active disabled:cursor-not-allowed disabled:opacity-40"
                        title={busy ? 'Wait for the transcode job to finish' : 'Delete variant'}
                      >
                        <Trash2 className="size-3" aria-hidden />
                        Delete
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
