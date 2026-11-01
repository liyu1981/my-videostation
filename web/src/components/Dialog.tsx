import { X } from 'lucide-react';
import type { ReactNode } from 'react';

/**
 * Video-Station-style dialog chrome: red top rule, centred red title, optional
 * tab bar, scrolling body and a right-aligned footer. Every dialog in the app
 * uses this so they stay consistent.
 */
export function Dialog({
  title,
  onClose,
  children,
  footer,
  tabs,
  size = 'max-w-3xl',
  z = 40,
  overlay = 'bg-black/70',
  bodyClassName,
  closeLabel = 'Close',
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  tabs?: ReactNode;
  size?: string;
  z?: number;
  overlay?: string;
  bodyClassName?: string;
  closeLabel?: string;
}) {
  return (
    <div
      className={[
        'fixed inset-0 flex items-start justify-center overflow-y-auto p-4 sm:items-center',
        z >= 60 ? 'z-[60]' : z >= 50 ? 'z-50' : 'z-40',
        overlay,
      ].join(' ')}
    >
      <div
        role="dialog"
        aria-modal="true"
        className={`w-full ${size} overflow-hidden rounded-[var(--radius-panel)] border border-hairline bg-surface shadow-2xl`}
      >
        <div className="h-[3px] bg-active" />
        <header className="relative flex items-center justify-center border-b border-hairline px-5 py-3">
          <h2 className="text-[13.5px] font-semibold text-active">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label={closeLabel}
            className="absolute right-3 rounded-lg p-1.5 text-fg-dim hover:bg-surface-2 hover:text-fg"
          >
            <X className="size-4" aria-hidden />
          </button>
        </header>
        {tabs ? <div className="flex items-center gap-6 border-b border-hairline px-5">{tabs}</div> : null}
        <div className={`max-h-[66vh] overflow-y-auto ${bodyClassName ?? ''}`}>{children}</div>
        {footer ? <footer className="flex items-center justify-between gap-3 border-t border-hairline px-5 py-3">{footer}</footer> : null}
      </div>
    </div>
  );
}

export function DialogTab({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        '-mb-px border-b-2 px-0.5 py-2.5 text-[12.5px] transition-colors',
        active ? 'border-active font-medium text-fg' : 'border-transparent text-fg-muted hover:text-fg',
      ].join(' ')}
    >
      {children}
    </button>
  );
}

export function DialogButton({
  variant = 'secondary',
  children,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' }) {
  const cls =
    variant === 'primary'
      ? 'inline-flex items-center gap-1.5 rounded-md bg-[#2f7de1] px-4 py-1.5 text-[12.5px] font-medium text-white hover:bg-[#3b8bea] disabled:opacity-60'
      : 'rounded-md border border-hairline-strong bg-surface-2 px-4 py-1.5 text-[12.5px] text-fg-muted hover:text-fg disabled:opacity-60';
  return (
    <button type="button" {...props} className={cls}>
      {children}
    </button>
  );
}
