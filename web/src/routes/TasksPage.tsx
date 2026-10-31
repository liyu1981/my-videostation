import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Activity, ChevronDown, ChevronUp, Trash2 } from 'lucide-react';
import { api, type Job, type JobLog } from '../lib/api';
import { EmptyState, Loading } from '../components/State';
import { formatDateTime } from '../lib/format';
import { CopyButton, JobMessage } from '../components/JobMessage';

export function TasksPage() {
  const qc = useQueryClient();
  const jobs = useQuery({
    queryKey: ['jobs', 'page'],
    queryFn: () => api.jobs(100),
    refetchInterval: (query) => {
      const list = query.state.data?.jobs ?? [];
      return list.some((j) => j.status === 'running' || j.status === 'queued') ? 1000 : 5000;
    },
  });
  const clearFinished = useMutation({
    mutationFn: api.clearFinishedJobs,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['jobs'] }),
  });

  if (jobs.isLoading) return <Loading />;

  const list = jobs.data?.jobs ?? [];

  return (
    <div className="mx-auto max-w-5xl px-6 py-6 lg:px-8">
      <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Tasks</h1>
          <p className="mt-1 text-[12px] text-fg-dim">
            Scans and metadata jobs with their full log. Logs stay until you delete the task.
          </p>
        </div>
        <button
          type="button"
          onClick={() => {
            if (confirm('Delete all finished tasks and their logs?')) clearFinished.mutate();
          }}
          disabled={clearFinished.isPending || list.every((j) => j.status === 'running' || j.status === 'queued')}
          className="inline-flex items-center gap-1.5 rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:border-hairline-strong hover:text-fg disabled:opacity-40"
        >
          <Trash2 className="size-3.5" aria-hidden /> Clear finished
        </button>
      </header>

      {list.length === 0 ? (
        <EmptyState icon={<Activity className="size-7" />} title="No tasks yet" description="Scans and metadata jobs appear here." />
      ) : (
        <div className="space-y-2">
          {list.map((job) => (
            <JobCard key={job.id} job={job} />
          ))}
        </div>
      )}
    </div>
  );
}

function JobCard({ job }: { job: Job }) {
  const qc = useQueryClient();
  const [showLogs, setShowLogs] = useState(false);
  const active = job.status === 'running' || job.status === 'queued';

  const logs = useQuery({
    queryKey: ['job-logs', job.id],
    queryFn: () => api.jobLogs(job.id),
    enabled: showLogs,
    refetchInterval: showLogs && active ? 1200 : false,
  });
  const remove = useMutation({
    mutationFn: () => api.deleteJob(job.id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['jobs'] }),
  });

  const entries = logs.data?.logs ?? [];
  const logText = entries
    .map((l: JobLog) => `${new Date(l.createdAt).toLocaleTimeString()} ${l.level.toUpperCase().padEnd(5)} ${l.message}`)
    .join('\n');

  const badge =
    job.status === 'succeeded'
      ? 'border-hairline text-fg-dim'
      : job.status === 'failed'
        ? 'border-active/40 bg-active/10 text-active'
        : 'border-progress/40 bg-progress/10 text-progress';

  return (
    <div className="rounded-[var(--radius-panel)] border border-hairline bg-surface px-4 py-3">
      <div className="flex flex-wrap items-center gap-2 text-[12.5px]">
        <span className={`rounded-full border px-2 py-0.5 text-[10.5px] uppercase tracking-wide ${badge}`}>{job.status}</span>
        <span className="font-medium text-fg">{job.type}</span>
        <span className="text-[11px] text-fg-dim">#{job.id}</span>
        <JobMessage text={job.error || job.message || ''} />

        <button
          type="button"
          onClick={() => setShowLogs((v) => !v)}
          aria-expanded={showLogs}
          className="inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-fg-dim hover:bg-surface-3 hover:text-fg-muted"
        >
          {showLogs ? <ChevronUp className="size-3" aria-hidden /> : <ChevronDown className="size-3" aria-hidden />}
          Logs
        </button>
        <button
          type="button"
          onClick={() => {
            if (confirm(`Delete task #${job.id} and its logs?`)) remove.mutate();
          }}
          aria-label="Delete task"
          title="Delete task"
          className="shrink-0 rounded p-1 text-fg-dim hover:bg-surface-3 hover:text-active"
        >
          <Trash2 className="size-3.5" aria-hidden />
        </button>
        <span className="ml-auto shrink-0 text-[11px] text-fg-dim">
          {job.finishedAt ? formatDateTime(job.finishedAt) : formatDateTime(job.startedAt ?? job.createdAt)}
        </span>
      </div>

      {active ? (
        <div className="mt-2 flex items-center gap-3">
          <div className="h-1 flex-1 overflow-hidden rounded-full bg-surface-3">
            <div className="h-full bg-progress transition-[width]" style={{ width: `${Math.max(2, job.progress)}%` }} />
          </div>
          <span className="w-10 text-right text-[11px] text-fg-dim">{Math.round(job.progress)}%</span>
        </div>
      ) : null}

      {showLogs ? (
        <div className="mt-2.5 overflow-hidden rounded-md border border-hairline bg-surface-2">
          <div className="flex items-center justify-between border-b border-hairline px-2.5 py-1.5">
            <span className="text-[10.5px] uppercase tracking-wide text-fg-dim">
              {entries.length > 0 ? `${entries.length} log lines` : 'no logs'}
            </span>
            <CopyButton text={logText} label="Copy log" />
          </div>
          <div className="max-h-72 overflow-auto py-1 font-mono text-[11.5px] leading-relaxed">
            {logs.isLoading ? <p className="px-2.5 py-2 text-fg-dim">Loading logs…</p> : null}
            {!logs.isLoading && entries.length === 0 ? <p className="px-2.5 py-2 text-fg-dim">No log lines recorded.</p> : null}
            {entries.map((l) => (
              <div key={l.id} className="flex gap-2 px-2.5 py-[1px]">
                <span className="shrink-0 text-fg-dim">{new Date(l.createdAt).toLocaleTimeString()}</span>
                <span
                  className={[
                    'w-11 shrink-0 uppercase',
                    l.level === 'error' ? 'text-active' : l.level === 'warn' ? 'text-new' : 'text-fg-dim',
                  ].join(' ')}
                >
                  {l.level}
                </span>
                <span className="min-w-0 whitespace-pre-wrap break-words text-fg-muted">{l.message}</span>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
