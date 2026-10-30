import type { ReactNode } from 'react';
import { Loader2, TriangleAlert } from 'lucide-react';

export function Loading({ label = 'Loading…' }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-3 py-16 text-[13px] text-fg-dim">
      <Loader2 className="size-4 animate-spin" aria-hidden />
      {label}
    </div>
  );
}

export function ErrorState({ error }: { error: unknown }) {
  const message = error instanceof Error ? error.message : 'Something went wrong';
  return (
    <div className="mx-6 my-10 flex items-start gap-3 rounded-[var(--radius-panel)] border border-active/30 bg-active/5 p-4">
      <TriangleAlert className="mt-0.5 size-4 shrink-0 text-active" aria-hidden />
      <div>
        <p className="text-[13.5px] font-medium">Could not load this view</p>
        <p className="mt-1 text-[12.5px] text-fg-muted">{message}</p>
      </div>
    </div>
  );
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="mx-6 my-10 flex flex-col items-center justify-center gap-3 rounded-[var(--radius-panel)] border border-dashed border-hairline bg-surface/40 px-6 py-14 text-center">
      {icon ? <div className="text-fg-dim">{icon}</div> : null}
      <p className="text-[14px] font-medium text-fg">{title}</p>
      {description ? <p className="max-w-md text-[12.5px] text-fg-muted">{description}</p> : null}
      {action}
    </div>
  );
}

export function SectionHeader({ title, aside }: { title: string; aside?: ReactNode }) {
  return (
    <div className="mb-3 flex items-end justify-between gap-4">
      <h2 className="text-[15px] font-semibold tracking-tight text-fg-heading">{title}</h2>
      {aside}
    </div>
  );
}
