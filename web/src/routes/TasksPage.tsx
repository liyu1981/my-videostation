import { useQuery } from '@tanstack/react-query';
import { Activity } from 'lucide-react';
import { api, type Job } from '../lib/api';
import { EmptyState, Loading } from '../components/State';
import { formatDateTime } from '../lib/format';

export function TasksPage() {
  const jobs = useQuery({
    queryKey: ['jobs', 'page'],
    queryFn: () => api.jobs(50),
    refetchInterval: (query) => {
      const list = query.state.data?.jobs ?? [];
      return list.some((j) => j.status === 'running' || j.status === 'queued') ? 1000 : 4000;
    },
  });

  if (jobs.isLoading) return <Loading />;

  const list = jobs.data?.jobs ?? [];

  return (
    <div className="mx-auto max-w-4xl px-6 py-6 lg:px-8">
      <header className="mb-5">
        <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Tasks</h1>
        <p className="mt-1 text-[12px] text-fg-dim">Scans, metadata matching and other background jobs. Progress updates live.</p>
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
  const active = job.status === 'running' || job.status === 'queued';
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
        <span className="min-w-0 flex-1 truncate text-fg-muted">{job.error || job.message}</span>
        <span className="text-[11px] text-fg-dim">
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
    </div>
  );
}
