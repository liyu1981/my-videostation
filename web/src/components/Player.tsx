import { useEffect, useRef, useState } from 'react';
import { useAtom } from 'jotai';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Captions, ChevronRight, Loader2, X } from 'lucide-react';
import { playerAtom } from '../atoms/ui';
import { api } from '../lib/api';

/**
 * Direct-play modal (M4): byte-range streaming, resume, subtitle tracks and next-up.
 * Codec support is whatever the browser can decode; transcoding arrives in M5.
 */
export function PlayerModal() {
  const [player, setPlayer] = useAtom(playerAtom);
  const qc = useQueryClient();
  const videoRef = useRef<HTMLVideoElement | null>(null);

  const [subtitleID, setSubtitleID] = useState<string>('off');
  const [menuOpen, setMenuOpen] = useState(false);
  const [ended, setEnded] = useState(false);
  const lastSaved = useRef(0);
  const resumedFor = useRef<number | null>(null);

  const playback = useQuery({
    queryKey: ['playback', player?.itemId ?? 0],
    queryFn: () => api.playback(player!.itemId),
    enabled: Boolean(player),
    staleTime: 0,
  });

  // Reset per-item state when switching items (e.g. next episode).
  useEffect(() => {
    setSubtitleID('off');
    setEnded(false);
    setMenuOpen(false);
    lastSaved.current = 0;
    resumedFor.current = null;
  }, [player?.itemId]);

  // Pick the default subtitle once per item.
  useEffect(() => {
    const tracks = playback.data?.subtitles ?? [];
    if (!tracks.length || subtitleID !== 'off') return;
    const preferred = tracks.find((t) => t.default) ?? null;
    if (preferred) setSubtitleID(preferred.id);
  }, [playback.data, subtitleID]);

  const close = () => {
    const v = videoRef.current;
    if (v && v.currentTime > 2) save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
    setPlayer(null);
  };

  const save = (positionMs: number, durationMs: number, played: boolean) => {
    if (!player) return;
    void api
      .progress(player.itemId, { positionMs: Math.round(positionMs), durationMs: Math.round(durationMs), played })
      .then(() => {
        void qc.invalidateQueries({ queryKey: ['item', player.itemId] });
        void qc.invalidateQueries({ queryKey: ['home'] });
      })
      .catch(() => undefined);
  };

  // Keyboard shortcuts.
  useEffect(() => {
    if (!player) return;
    const onKey = (e: KeyboardEvent) => {
      const v = videoRef.current;
      if (!v) return;
      switch (e.key) {
        case 'Escape':
          if (menuOpen) setMenuOpen(false);
          else close();
          break;
        case ' ':
          e.preventDefault();
          if (v.paused) void v.play();
          else v.pause();
          break;
        case 'ArrowLeft':
          v.currentTime = Math.max(0, v.currentTime - 10);
          break;
        case 'ArrowRight':
          v.currentTime = Math.min(v.duration || 0, v.currentTime + 10);
          break;
        case 'f':
          if (document.fullscreenElement) void document.exitFullscreen();
          else void v.requestFullscreen();
          break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [player, menuOpen]);

  if (!player) return null;

  const data = playback.data;
  const tracks = data?.subtitles ?? [];
  const track = tracks.find((t) => t.id === subtitleID);
  const next = data?.next;

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black">
      <div className="flex h-14 shrink-0 items-center justify-between gap-4 px-4">
        <p className="truncate text-[13.5px] font-medium text-fg">{player.title}</p>
        <div className="flex items-center gap-1">
          <div className="relative">
            <button
              type="button"
              onClick={() => setMenuOpen((v) => !v)}
              aria-label="Subtitles"
              aria-expanded={menuOpen}
              className="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-2 text-[12px] text-fg-muted hover:bg-white/10 hover:text-fg"
            >
              <Captions className="size-4" aria-hidden />
              {track ? track.label : 'Off'}
            </button>
            {menuOpen ? (
              <div className="absolute right-0 top-full z-10 mt-1 w-56 rounded-lg border border-hairline bg-surface-2 p-1 shadow-2xl">
                <button
                  type="button"
                  onClick={() => {
                    setSubtitleID('off');
                    setMenuOpen(false);
                  }}
                  className={[
                    'flex w-full items-center justify-between rounded-md px-2.5 py-1.5 text-left text-[12.5px] hover:bg-surface-3',
                    subtitleID === 'off' ? 'text-fg' : 'text-fg-muted',
                  ].join(' ')}
                >
                  Off
                </button>
                {tracks.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => {
                      setSubtitleID(t.id);
                      setMenuOpen(false);
                    }}
                    className={[
                      'flex w-full items-center justify-between rounded-md px-2.5 py-1.5 text-left text-[12.5px] hover:bg-surface-3',
                      subtitleID === t.id ? 'text-fg' : 'text-fg-muted',
                    ].join(' ')}
                  >
                    <span>{t.label}{t.language ? ` · ${t.language}` : ''}</span>
                    <span className="text-[10px] uppercase text-fg-dim">{t.source}</span>
                  </button>
                ))}
                {tracks.length === 0 ? <p className="px-2.5 py-1.5 text-[11.5px] text-fg-dim">No subtitle tracks</p> : null}
              </div>
            ) : null}
          </div>
          <button
            type="button"
            onClick={close}
            className="rounded-lg p-2 text-fg-muted hover:bg-white/10 hover:text-fg"
            aria-label="Close player"
          >
            <X className="size-5" aria-hidden />
          </button>
        </div>
      </div>

      <div className="relative flex min-h-0 flex-1 items-center justify-center bg-black">
        {playback.isLoading ? (
          <div className="absolute inset-0 z-10 flex items-center justify-center gap-2 bg-black text-[13px] text-fg-dim">
            <Loader2 className="size-4 animate-spin" aria-hidden /> Preparing playback…
          </div>
        ) : null}
        {playback.isError ? (
          <p className="absolute inset-0 z-10 flex max-w-none items-center justify-center bg-black px-8 text-center text-[13px] text-fg-muted">
            {(playback.error as Error).message}
          </p>
        ) : null}

        {data ? (
          <video
            ref={videoRef}
            src={data.streamUrl}
            controls
            autoPlay
            crossOrigin="anonymous"
            className="h-full w-full object-contain"
            onLoadedMetadata={(e) => {
              const v = e.currentTarget;
              if (resumedFor.current === data.itemId) return;
              resumedFor.current = data.itemId;
              if (data.resumeMs > 5000 && (!v.duration || data.resumeMs < v.duration * 0.9 * 1000)) {
                v.currentTime = data.resumeMs / 1000;
              }
            }}
            onTimeUpdate={(e) => {
              const v = e.currentTarget;
              if (v.currentTime - lastSaved.current >= 10) {
                lastSaved.current = v.currentTime;
                save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
              }
            }}
            onPause={(e) => {
              const v = e.currentTarget;
              if (v.currentTime > 2 && !v.ended) save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
            }}
            onEnded={(e) => {
              const v = e.currentTarget;
              save(v.duration * 1000, v.duration * 1000, true);
              if (next) setEnded(true);
            }}
          >
            {track ? <track key={track.id} kind="subtitles" src={track.url} label={track.label} srcLang={track.language || 'und'} default /> : null}
          </video>
        ) : null}

        {track && videoRef.current ? (
          <SubtitleEnabler video={videoRef.current} active={track.id} />
        ) : null}

        {ended && next ? (
          <div className="absolute bottom-10 right-10 w-80 rounded-[var(--radius-panel)] border border-hairline bg-surface/95 p-4 shadow-2xl backdrop-blur">
            <p className="text-[11px] uppercase tracking-wide text-fg-dim">Up next</p>
            <p className="mt-1 text-[14px] font-medium text-fg">
              {next.seasonNumber ? `S${String(next.seasonNumber).padStart(2, '0')}` : ''}
              {next.episodeNumber ? `E${String(next.episodeNumber).padStart(2, '0')} · ` : ''}
              {next.title}
            </p>
            <div className="mt-3 flex items-center gap-2">
              <button
                type="button"
                onClick={() => setPlayer({ itemId: next.itemId, title: next.title })}
                className="inline-flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-progress/90 px-3 py-1.5 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
              >
                Play next <ChevronRight className="size-3.5" aria-hidden />
              </button>
              <button
                type="button"
                onClick={() => setEnded(false)}
                className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg"
              >
                Dismiss
              </button>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** Forces the selected subtitle track on after the <track> element is (re)mounted. */
function SubtitleEnabler({ video, active }: { video: HTMLVideoElement; active: string }) {
  useEffect(() => {
    const tracks = video.textTracks;
    for (let i = 0; i < tracks.length; i++) {
      tracks[i].mode = i === 0 ? 'showing' : 'disabled';
    }
  }, [video, active]);
  return null;
}
