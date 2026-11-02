import type { ReactNode } from 'react';
import { Film } from 'lucide-react';

export function AuthCard({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-bg px-4 py-10 text-fg">
      <div className="w-full max-w-[380px]">
        <div className="mb-5 flex items-center justify-center gap-2.5">
          <span className="grid size-9 place-items-center rounded-lg bg-active/15 ring-1 ring-active/30">
            <Film className="size-[18px] text-active" aria-hidden />
          </span>
          <span className="text-[16px] font-semibold tracking-tight">
            My<span className="text-fg-muted">VideoStation</span>
          </span>
        </div>
        <div className="overflow-hidden rounded-[var(--radius-panel)] border border-hairline bg-surface shadow-2xl">
          <div className="h-[3px] bg-active" />
          <div className="px-6 py-6">
            <h1 className="text-center text-[14px] font-semibold text-active">{title}</h1>
            {subtitle ? <p className="mt-1.5 text-center text-[12px] text-fg-dim">{subtitle}</p> : null}
            <div className="mt-5">{children}</div>
          </div>
        </div>
        <p className="mt-4 text-center text-[10.5px] text-fg-dim">Private, self-hosted media library</p>
      </div>
    </div>
  );
}

export const authInputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';

export const authButtonClass =
  'w-full rounded-lg bg-[#2f7de1] px-4 py-2 text-[13px] font-medium text-white transition-colors hover:bg-[#3b8bea] disabled:opacity-60';
