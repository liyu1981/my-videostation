import { useEffect, useRef } from 'react';
import { useAtom } from 'jotai';
import { useQueryClient } from '@tanstack/react-query';
import { X } from 'lucide-react';
import { playerAtom } from '../atoms/ui';
import { api } from '../lib/api';

/**
 * Minimal direct-play modal (M1 slice of playback): byte-range streaming, resume,
 * and progress reporting. Codec support is whatever the browser can decode.
 */
export function PlayerModal() {
  const [player, setPlayer] = useAtom(playerAtom);
  const qc = useQueryClient();
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const lastSaved = useRef(0);

  useEffect(() => {
    if (!player) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [player]);

  if (!player) return null;

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

  const close = () => {
    const v = videoRef.current;
    if (v && v.currentTime > 2) {
      save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
    }
    setPlayer(null);
  };

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black/95">
      <div className="flex h-14 shrink-0 items-center justify-between gap-4 px-4">
        <p className="truncate text-[13.5px] font-medium text-fg">{player.title}</p>
        <button
          type="button"
          onClick={close}
          className="rounded-lg p-2 text-fg-muted hover:bg-white/10 hover:text-fg"
          aria-label="Close player"
        >
          <X className="size-5" aria-hidden />
        </button>
      </div>
      <div className="flex min-h-0 flex-1 items-center justify-center px-4 pb-4">
        <video
          ref={videoRef}
          src={player.streamUrl}
          controls
          autoPlay
          className="max-h-full w-full max-w-6xl rounded-lg bg-black shadow-2xl"
          onTimeUpdate={(e) => {
            const v = e.currentTarget;
            if (v.currentTime - lastSaved.current >= 10) {
              lastSaved.current = v.currentTime;
              save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
            }
          }}
          onEnded={(e) => {
            const v = e.currentTarget;
            save(v.duration * 1000, v.duration * 1000, true);
          }}
        />
      </div>
      <p className="px-4 pb-4 text-center text-[11.5px] text-fg-dim">
        Direct play — the browser must support the file's codecs. Transcoding arrives in M5.
      </p>
    </div>
  );
}
