import { HardDrive, Subtitles, Volume2 } from 'lucide-react';
import type { MediaFile } from '../lib/api';
import { audioSummary, formatBytes, formatRuntime, resolutionLabel } from '../lib/format';

export function MediaInfoPanel({ files }: { files: MediaFile[] }) {
  return (
    <div className="space-y-3">
      {files.map((file) => (
        <div key={file.id} className="rounded-[var(--radius-panel)] border border-hairline bg-surface">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-hairline px-4 py-3">
            <HardDrive className="size-4 text-fg-dim" aria-hidden />
            <p className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-fg" title={file.path}>
              {file.relPath || file.path}
            </p>
            <span className="text-[11.5px] text-fg-dim">{formatBytes(file.size)}</span>
            {file.missing ? (
              <span className="rounded-full bg-active/15 px-2 py-0.5 text-[10.5px] text-active">Missing</span>
            ) : null}
          </div>

          <dl className="grid grid-cols-2 gap-x-6 gap-y-2 px-4 py-3 text-[12px] sm:grid-cols-4">
            <Field label="Container" value={file.container?.toUpperCase()} />
            <Field label="Duration" value={formatRuntime(file.durationMs)} />
            <Field label="Resolution" value={file.width ? `${file.width}×${file.height}` : ''} />
            <Field label="Bitrate" value={file.bitrate ? `${(file.bitrate / 1_000_000).toFixed(1)} Mbps` : ''} />
            <Field label="Video" value={[file.videoCodec?.toUpperCase(), file.hdr].filter(Boolean).join(' · ')} />
            <Field label="Audio" value={audioSummary(file)} />
            <Field label="Size" value={formatBytes(file.size)} />
            <Field label="Badge" value={resolutionLabel(file.width, file.height)} />
          </dl>

          {file.audioStreams.length > 0 || file.subtitleStreams.length > 0 ? (
            <div className="grid gap-3 border-t border-hairline px-4 py-3 sm:grid-cols-2">
              <div>
                <p className="mb-1.5 flex items-center gap-1.5 text-[11px] uppercase tracking-wide text-fg-dim">
                  <Volume2 className="size-3.5" aria-hidden /> Audio tracks
                </p>
                <ul className="space-y-1 text-[12px] text-fg-muted">
                  {file.audioStreams.map((a) => (
                    <li key={a.index} className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-fg">{a.codec.toUpperCase()}</span>
                      <span>{a.channels}ch</span>
                      {a.layout ? <span className="text-fg-dim">{a.layout}</span> : null}
                      {a.language ? <span className="text-fg-dim">{a.language}</span> : null}
                      {a.title ? <span className="text-fg-dim">{a.title}</span> : null}
                      {a.default ? <span className="rounded bg-surface-3 px-1.5 text-[10px] text-fg-muted">default</span> : null}
                    </li>
                  ))}
                </ul>
              </div>
              <div>
                <p className="mb-1.5 flex items-center gap-1.5 text-[11px] uppercase tracking-wide text-fg-dim">
                  <Subtitles className="size-3.5" aria-hidden /> Subtitles
                </p>
                {file.subtitleStreams.length === 0 ? (
                  <p className="text-[12px] text-fg-dim">None embedded</p>
                ) : (
                  <ul className="space-y-1 text-[12px] text-fg-muted">
                    {file.subtitleStreams.map((s) => (
                      <li key={s.index} className="flex flex-wrap items-center gap-2">
                        <span className="font-medium text-fg">{s.codec.toUpperCase()}</span>
                        {s.language ? <span className="text-fg-dim">{s.language}</span> : null}
                        {s.title ? <span className="text-fg-dim">{s.title}</span> : null}
                        {s.forced ? <span className="rounded bg-surface-3 px-1.5 text-[10px] text-fg-muted">forced</span> : null}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function Field({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex min-w-0 flex-col">
      <dt className="text-[10.5px] uppercase tracking-wide text-fg-dim">{label}</dt>
      <dd className="truncate text-fg-muted">{value || '—'}</dd>
    </div>
  );
}
