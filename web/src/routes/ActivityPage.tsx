import { useQuery } from '@tanstack/react-query';
import { RefreshCw, ScrollText } from 'lucide-react';
import { api } from '../lib/api';
import { formatDateTime } from '../lib/format';
import { useI18n } from '../lib/i18n';
import { ErrorState, Loading } from '../components/State';

export function ActivityPage() {
  const { t } = useI18n();
  const events = useQuery({ queryKey: ['audit'], queryFn: () => api.audit(200) });
  if (events.isLoading) return <Loading />;
  if (events.isError) return <ErrorState error={events.error} />;

  const list = events.data!.events;
  return (
    <div className="mx-auto max-w-6xl px-6 py-7 lg:px-8">
      <header className="mb-6 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">{t('activity.title')}</h1>
          <p className="mt-1 text-[12px] text-fg-dim">{t('activity.subtitle')}</p>
        </div>
        <button
          type="button"
          onClick={() => void events.refetch()}
          className="inline-flex items-center gap-2 rounded-lg border border-hairline px-3 py-2 text-[12px] text-fg-muted hover:border-hairline-strong hover:text-fg"
        >
          <RefreshCw className="size-3.5" aria-hidden /> {t('activity.refresh')}
        </button>
      </header>

      {list.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-[var(--radius-panel)] border border-dashed border-hairline px-6 py-14 text-center">
          <ScrollText className="size-7 text-fg-dim" aria-hidden />
          <p className="text-[13.5px] font-medium">No activity yet</p>
          <p className="text-[12px] text-fg-dim">Admin and account actions will appear here.</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius-panel)] border border-hairline bg-surface">
          <div className="hidden grid-cols-[10rem_9rem_1fr_1fr] gap-3 border-b border-hairline bg-surface-2 px-4 py-2 text-[10.5px] uppercase tracking-wide text-fg-dim md:grid">
            <span>When</span>
            <span>Actor</span>
            <span>Action</span>
            <span>Details</span>
          </div>
          {list.map((event) => (
            <div key={event.id} className="grid gap-1 border-b border-hairline px-4 py-2.5 last:border-b-0 md:grid-cols-[10rem_9rem_1fr_1fr] md:items-center md:gap-3">
              <span className="font-mono text-[10.5px] text-fg-dim">{formatDateTime(event.createdAt)}</span>
              <span className="truncate text-[12px] text-fg">{event.username || '—'}</span>
              <span className="min-w-0">
                <span className="rounded border border-hairline px-1.5 py-0.5 font-mono text-[10.5px] text-fg-muted">{event.action}</span>
                {event.target ? <span className="ml-2 text-[11px] text-fg-dim">{event.target}</span> : null}
              </span>
              <span className="min-w-0 truncate text-[11.5px] text-fg-muted" title={`${event.detail ?? ''}${event.ip ? ` · ${event.ip}` : ''}`}>
                {event.detail || '—'}
                {event.ip ? <span className="ml-2 font-mono text-[10px] text-fg-dim">{event.ip}</span> : null}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
