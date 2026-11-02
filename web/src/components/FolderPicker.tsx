import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ChevronRight, Folder, FolderOpen, Loader2 } from 'lucide-react';
import { api } from '../lib/api';
import { Dialog, DialogButton } from './Dialog';

/**
 * Server-side folder browser used by the library Add/Edit forms. It only lists
 * directories and never writes anything; the selected path is fed back into the
 * form's existing text field.
 */
export function FolderPickerDialog({
  initialPath,
  onClose,
  onSelect,
}: {
  initialPath?: string;
  onClose: () => void;
  onSelect: (path: string) => void;
}) {
  const start = initialPath?.trim() || '/';
  const [path, setPath] = useState(start);
  const [input, setInput] = useState(start);
  const listing = useQuery({
    queryKey: ['fs', path],
    queryFn: () => api.listFS(path),
    retry: false,
    staleTime: 5000,
  });

  useEffect(() => {
    if (listing.data) setInput(listing.data.path);
  }, [listing.data]);

  const current = listing.data?.path ?? path;
  const segments = current.split('/').filter(Boolean);
  const crumbs = [
    { name: '/', path: '/' },
    ...segments.map((name, i) => ({ name, path: '/' + segments.slice(0, i + 1).join('/') })),
  ];

  const go = (next: string) => {
    const cleaned = next.trim();
    if (!cleaned) return;
    setPath(cleaned);
    setInput(cleaned);
  };

  return (
    <Dialog
      title="Select folder"
      size="max-w-2xl"
      onClose={onClose}
      bodyClassName="p-0"
      footer={
        <>
          <span className="min-w-0 truncate font-mono text-[11px] text-fg-dim" title={current}>
            {current}
          </span>
          <span className="flex shrink-0 gap-2">
            <DialogButton onClick={onClose}>Cancel</DialogButton>
            <DialogButton
              variant="primary"
              onClick={() => onSelect(current)}
              disabled={listing.isLoading || listing.isError}
            >
              Use this folder
            </DialogButton>
          </span>
        </>
      }
    >
      <div className="space-y-3 p-4">
        <div className="flex gap-2">
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                go(input);
              }
            }}
            aria-label="Folder path"
            placeholder="/volume1/video"
            className="w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 font-mono text-[12px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3"
          />
          <DialogButton onClick={() => go(input)}>Go</DialogButton>
        </div>

        <div className="flex flex-wrap items-center gap-1.5">
          {(listing.data?.roots ?? []).map((root) => (
            <button
              key={root.path}
              type="button"
              onClick={() => go(root.path)}
              className="inline-flex items-center gap-1 rounded-md border border-hairline bg-surface-2 px-2 py-1 text-[11px] text-fg-muted hover:border-hairline-strong hover:text-fg"
            >
              <FolderOpen className="size-3 text-progress" aria-hidden />
              {root.path === '/' ? 'Root' : root.name}
            </button>
          ))}
        </div>

        <div className="flex flex-wrap items-center gap-1 text-[11.5px] text-fg-dim">
          {crumbs.map((crumb, i) => (
            <span key={crumb.path} className="flex items-center gap-1">
              {i > 0 ? <ChevronRight className="size-3 text-fg-dim" aria-hidden /> : null}
              <button
                type="button"
                onClick={() => go(crumb.path)}
                className="max-w-[14rem] truncate rounded px-1 py-0.5 hover:bg-surface-2 hover:text-fg"
              >
                {crumb.name}
              </button>
            </span>
          ))}
        </div>

        <div className="overflow-hidden rounded-lg border border-hairline">
          <div className="flex items-center justify-between border-b border-hairline bg-surface-2 px-3 py-2 text-[11px] text-fg-dim">
            <span>
              {listing.data ? `${listing.data.entries.length} folder${listing.data.entries.length === 1 ? '' : 's'}` : 'Folders'}
            </span>
            <button
              type="button"
              onClick={() => listing.data?.parent && go(listing.data.parent)}
              disabled={!listing.data?.parent}
              className="rounded border border-hairline px-2 py-0.5 text-[10.5px] text-fg-muted hover:text-fg disabled:opacity-40"
            >
              Up
            </button>
          </div>

          {listing.isLoading ? (
            <div className="flex items-center justify-center gap-2 p-8 text-[12px] text-fg-dim">
              <Loader2 className="size-4 animate-spin" aria-hidden /> Loading…
            </div>
          ) : listing.isError ? (
            <div className="p-6 text-center text-[12px] text-active">{(listing.error as Error).message}</div>
          ) : listing.data!.entries.length === 0 ? (
            <div className="p-6 text-center text-[12px] text-fg-dim">No subfolders here.</div>
          ) : (
            <ul className="max-h-[320px] divide-y divide-hairline overflow-y-auto">
              {listing.data!.entries.map((entry) => (
                <li key={entry.path}>
                  <button
                    type="button"
                    onClick={() => go(entry.path)}
                    className="flex w-full items-center gap-2 px-3 py-2 text-left text-[12.5px] text-fg hover:bg-surface-2"
                  >
                    <Folder className="size-4 shrink-0 text-progress" aria-hidden />
                    <span className="min-w-0 flex-1 truncate">{entry.name}</span>
                    <ChevronRight className="size-3.5 shrink-0 text-fg-dim" aria-hidden />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <p className="text-[11px] text-fg-dim">
          Only folders are shown. Open a folder to browse deeper, then choose “Use this folder”. The path is also
          editable above.
        </p>
      </div>
    </Dialog>
  );
}
