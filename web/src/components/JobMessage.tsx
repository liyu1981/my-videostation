import { useState } from 'react';
import { Check, ChevronDown, ChevronUp, Copy } from 'lucide-react';

/**
 * Truncated job message with an expandable full-text view and copy button.
 * Long backend errors (SQL constraints, ffprobe output) are otherwise unreadable.
 */
export function CopyButton({ text, label }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try {
        document.execCommand('copy');
      } finally {
        ta.remove();
      }
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <button
      type="button"
      onClick={copy}
      aria-label="Copy"
      className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10.5px] text-fg-dim hover:bg-surface-3 hover:text-fg-muted"
    >
      {copied ? <Check className="size-3 text-progress" aria-hidden /> : <Copy className="size-3" aria-hidden />}
      {label ?? (copied ? 'Copied' : 'Copy')}
    </button>
  );
}

export function JobMessage({ text, className = '' }: { text: string; className?: string }) {
  const [open, setOpen] = useState(false);

  if (!text) return null;

  return (
    <div className={['min-w-0 flex-1', className].join(' ')}>
      <div className="flex items-center gap-1.5">
        <span className={open ? 'hidden' : 'min-w-0 flex-1 truncate text-fg-muted'}>{text}</span>
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className="inline-flex shrink-0 items-center gap-1 rounded px-1 py-0.5 text-[10.5px] text-fg-dim hover:bg-surface-3 hover:text-fg-muted"
        >
          {open ? <ChevronUp className="size-3" aria-hidden /> : <ChevronDown className="size-3" aria-hidden />}
          {open ? 'Hide' : 'Details'}
        </button>
        <span className="shrink-0">
          <CopyButton text={text} label="" />
        </span>
      </div>
      {open ? (
        <pre className="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border border-hairline bg-surface-2 p-2.5 font-mono text-[11.5px] leading-relaxed text-fg-muted">
          {text}
        </pre>
      ) : null}
    </div>
  );
}
