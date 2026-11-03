import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { api } from '../lib/api';

export function MaintenancePage() {
  const [state, setState] = useState<'waiting' | 'offline' | 'pending'>('waiting');
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    let stopped = false;
    let failures = 0;
    const timer = window.setInterval(() => setSeconds((value) => value + 1), 1000);
    const poll = async () => {
      try {
        const status = await api.backupStatus();
        failures = 0;
        if (stopped) return;
        if (!status.maintenance && !status.pendingImport) {
          window.location.assign('/');
          return;
        }
        setState(status.pendingImport ? 'pending' : 'waiting');
      } catch {
        failures++;
        if (!stopped && failures >= 3) setState('offline');
      }
    };
    void poll();
    const interval = window.setInterval(() => void poll(), 1000);
    return () => {
      stopped = true;
      window.clearInterval(interval);
      window.clearInterval(timer);
    };
  }, []);

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg px-4 py-10 text-fg">
      <div className="w-full max-w-md overflow-hidden rounded-[var(--radius-panel)] border border-hairline bg-surface shadow-2xl">
        <div className="h-[3px] bg-active" />
        <div className="px-6 py-7 text-center">
          <div className="mx-auto grid size-12 place-items-center rounded-full bg-active/10 ring-1 ring-active/30">
            <Loader2 className="size-6 animate-spin text-active" aria-hidden />
          </div>
          <h1 className="mt-4 text-[16px] font-semibold text-fg">Import in progress</h1>
          <p className="mt-2 text-[12.5px] text-fg-muted">
            Applying the selected data bundle and restarting the server. Keep this tab open — the UI will return automatically.
          </p>
          <p className="mt-4 text-[11px] text-fg-dim">
            {state === 'offline'
              ? 'The server is restarting. If it does not return, restart the container/server.'
              : state === 'pending'
                ? 'The import is staged and waiting for a server restart.'
                : 'Waiting for the server…'}
          </p>
          {seconds > 60 ? (
            <p className="mt-3 text-[10.5px] text-new">
              If the server does not return automatically, restart the container or the mvs process.
            </p>
          ) : null}
          {seconds > 60 ? (
            <button
              type="button"
              onClick={() => window.location.reload()}
              className="mt-3 rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg"
            >
              Reload
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
