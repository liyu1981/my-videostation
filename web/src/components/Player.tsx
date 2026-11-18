import { useCallback, useEffect, useRef, useState } from 'react';
import { useAtom } from 'jotai';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ChevronRight, Loader2, X } from 'lucide-react';
import { MoviPlayer, MoviTrack, type MoviElement } from 'movi-player/react';
import { playerAtom } from '../atoms/ui';
import { api } from '../lib/api';

/**
 * Full-screen playback modal.
 *
 * The actual media engine is Movi Player: it demuxes MKV/HEVC/AV1 in the
 * browser (FFmpeg WASM + WebCodecs), so the web player no longer needs the
 * server HLS/transcode path for formats a native <video> cannot open.
 */
export function PlayerModal() {
  const [player, setPlayer] = useAtom(playerAtom);
  const qc = useQueryClient();
  const elRef = useRef<MoviElement>(null);
  const [ready, setReady] = useState(false);
  const [src, setSrc] = useState<{ itemId: number; url: string } | null>(null);
  const [ended, setEnded] = useState(false);
  const [error, setError] = useState('');
  const [duration, setDuration] = useState(0);
  const [countdown, setCountdown] = useState<number | null>(null);
  const [autoPlayNext, setAutoPlayNext] = useState(() => localStorage.getItem('mvs.autoPlayNext') === 'true');
  const lastSaved = useRef(0);
  const resumedFor = useRef<number | null>(null);
  const historyPushed = useRef(false);
  const fallbackTried = useRef(false);

  const playback = useQuery({
    queryKey: ['playback', player?.itemId ?? 0],
    queryFn: () => api.playback(player!.itemId),
    enabled: Boolean(player),
    staleTime: 0,
  });
  const neighbors = useQuery({
    queryKey: ['neighbors', player?.itemId ?? 0],
    queryFn: () => api.neighbors(player!.itemId),
    enabled: Boolean(player),
  });
  const data = playback.data;

  const save = useCallback(
    (positionMs: number, durationMs: number, played: boolean) => {
      if (!player || !data || data.itemId !== player.itemId) return;
      void api
        .progress(player.itemId, { positionMs: Math.round(positionMs), durationMs: Math.round(durationMs), played })
        .then(() => {
          void qc.invalidateQueries({ queryKey: ['item', player.itemId] });
          void qc.invalidateQueries({ queryKey: ['home'] });
        })
        .catch(() => undefined);
    },
    [player, data, qc],
  );

  const saveNow = useCallback(() => {
    const el = elRef.current;
    if (!el) return;
    const position = (el.currentTime || 0) * 1000;
    if (position > 2000) save(position, (el.duration || 0) * 1000, false);
  }, [save]);

  const close = useCallback(() => {
    elRef.current?.pause();
    if (historyPushed.current) {
      window.history.back();
      return;
    }
    saveNow();
    setPlayer(null);
  }, [saveNow, setPlayer]);

  useEffect(() => {
    if (!player) return;
    if (!historyPushed.current) {
      window.history.pushState({ ...(window.history.state ?? {}), mvsPlayer: true }, '');
      historyPushed.current = true;
    }
  }, [player]);

  useEffect(() => {
    const onPopState = () => {
      if (!historyPushed.current) return;
      historyPushed.current = false;
      saveNow();
      elRef.current?.pause();
      setPlayer(null);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, [saveNow, setPlayer]);

  // Reset per-item state when the player switches to another item.
  useEffect(() => {
    setEnded(false);
    setError('');
    setDuration(0);
    setCountdown(null);
    lastSaved.current = 0;
    resumedFor.current = null;
    fallbackTried.current = false;
    setSrc(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [player?.itemId, data?.streamUrl]);

  // Apply the server resume position once the element knows its duration.
  useEffect(() => {
    const el = elRef.current;
    if (!el || !data || !ready) return;
    const apply = () => {
      if (resumedFor.current === data.itemId) return;
      resumedFor.current = data.itemId;
      const target = data.resumeMs;
      if (target > 5000 && (!el.duration || target < el.duration * 900)) {
        el.currentTime = target / 1000;
      }
    };
    if (el.duration) {
      apply();
      return;
    }
    el.addEventListener('loadedmetadata', apply, { once: true });
    return () => el.removeEventListener('loadedmetadata', apply);
  }, [ready, data]);

  useEffect(() => {
    localStorage.setItem('mvs.autoPlayNext', String(autoPlayNext));
  }, [autoPlayNext]);

  const onTimeUpdate = useCallback(
    (time: number) => {
      const el = elRef.current;
      const total = el?.duration || 0;
      if (total) setDuration(total);
      if (time > 1 && Math.abs(time * 1000 - lastSaved.current) >= 5000) {
        lastSaved.current = time * 1000;
        save(time * 1000, total * 1000, false);
      }
    },
    [save],
  );

  const onEnded = useCallback(() => {
    const total = (elRef.current?.duration || duration) * 1000;
    save(total, total, true);
    setEnded(true);
  }, [duration, save]);

  const queue = player?.queue ?? null;
  const queueIndex = player?.queueIndex ?? -1;
  const queueNext = queue && queueIndex >= 0 && queueIndex + 1 < queue.length ? queue[queueIndex + 1] : undefined;
  const queuePrev = queue && queueIndex > 0 ? queue[queueIndex - 1] : undefined;
  const normalNextTitle = queue ? data?.next?.title : neighbors.data?.next?.title ?? data?.next?.title;
  const normalNextID = queue ? data?.next?.itemId : neighbors.data?.next?.id ?? data?.next?.itemId;
  const nextTitle = queueNext?.title ?? normalNextTitle;
  const nextItemID = queueNext?.itemId ?? normalNextID;

  const playNext = useCallback(() => {
    if (!nextItemID || !nextTitle) return;
    if (queueNext && queue) {
      setPlayer({ ...player!, itemId: queueNext.itemId, title: queueNext.title, queue, queueIndex: queueIndex + 1 });
      return;
    }
    setPlayer({ itemId: nextItemID, title: nextTitle });
  }, [nextItemID, nextTitle, queueNext, queue, queueIndex, player, setPlayer]);

  const playPrevious = useCallback(() => {
    if (queuePrev && queue) {
      setPlayer({ ...player!, itemId: queuePrev.itemId, title: queuePrev.title, queue, queueIndex: queueIndex - 1 });
      return;
    }
    const prev = neighbors.data?.prev;
    if (!prev) return;
    setPlayer({ itemId: prev.id, title: prev.title });
  }, [queuePrev, queue, queueIndex, player, neighbors.data, setPlayer]);

  useEffect(() => {
    if (!ended || !autoPlayNext || !nextItemID) {
      setCountdown(null);
      return;
    }
    setCountdown(5);
    const timer = window.setInterval(() => {
      setCountdown((value) => (value === null ? null : value - 1));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [ended, autoPlayNext, nextItemID]);

  useEffect(() => {
    if (countdown === 0 && ended && autoPlayNext) playNext();
  }, [countdown, ended, autoPlayNext, playNext]);

  // If Movi cannot decode a source, fall back to the server's cached MP4
  // variant (the same prepare/poll path used by the previous player).
  const tryFallback = useCallback(async () => {
    if (!data || fallbackTried.current || !data.prepare) return;
    fallbackTried.current = true;
    const itemId = data.itemId;
    setError('Preparing a compatible stream…');
    try {
      let res = await api.prepare(itemId, { fileId: data.fileId });
      while (res.state === 'queued' || res.state === 'running') {
        if (player?.itemId !== itemId) return;
        setError(`Preparing a compatible stream… ${Math.round(res.progress ?? 0)}%`);
        await new Promise((resolve) => window.setTimeout(resolve, 1000));
        res = await api.prepare(itemId, { fileId: data.fileId });
      }
      if (player?.itemId !== itemId) return;
      if (res.state === 'ready' && res.url) {
        setSrc({ itemId: data.itemId, url: res.url });
        setError('');
      } else {
        setError(res.error || 'This file could not be prepared for playback.');
      }
    } catch (err) {
      setError((err as Error).message);
    }
  }, [data, player?.itemId]);

  const handleReady = useCallback((el: MoviElement) => {
    setReady(true);
    if (el.duration) setDuration(el.duration);
  }, []);

  const handlePlay = useCallback(() => setEnded(false), []);

  const handleError = useCallback((err: unknown) => {
    setError(err instanceof Error ? err.message : String(err));
    void tryFallback();
  }, [tryFallback]);

  const handleErrorDisplay = useCallback((info: { title: string | null; message: string | null }) => {
    setError(info.message || info.title || 'Playback failed');
    void tryFallback();
  }, [tryFallback]);

  const tracks = (data?.subtitles ?? []).filter((t) => !t.burnIn);

  return (
    <div className="fixed inset-0 z-50 select-none bg-black">
      {!player ? null : playback.isLoading || !data ? (
        <div className="absolute inset-0 flex items-center justify-center gap-2 text-[13px] text-fg-dim">
          <Loader2 className="size-4 animate-spin" aria-hidden /> Preparing playback…
        </div>
      ) : (
        <MoviPlayer
          ref={elRef}
          src={src?.itemId === data.itemId ? src.url : data.streamUrl}
          poster={data.posterUrl}
          title={player.title}
          showtitle
          controls
          autoplay
          playsinline
          theme="dark"
          className="h-full w-full"
          style={{ width: '100%', height: '100%' }}
          onReady={handleReady}
          onTimeUpdate={onTimeUpdate}
          onPlay={handlePlay}
          onPause={saveNow}
          onEnded={onEnded}
          onError={handleError}
          onErrorDisplay={handleErrorDisplay}
        >
          {tracks.map((track) => (
            <MoviTrack
              key={track.id}
              src={track.url}
              kind="subtitles"
              srcLang={track.language}
              label={track.label}
              format="vtt"
              default={track.default}
            />
          ))}
        </MoviPlayer>
      )}

      <div className="absolute right-4 top-4 z-40 flex items-center gap-2">
        {queuePrev || neighbors.data?.prev ? (
          <button
            type="button"
            onClick={playPrevious}
            className="grid size-9 place-items-center rounded-full bg-black/50 text-fg-muted ring-1 ring-white/15 backdrop-blur hover:text-fg"
            aria-label="Previous item"
            title="Previous item"
          >
            <ChevronRight className="size-4 rotate-180" aria-hidden />
          </button>
        ) : null}
        <button
          type="button"
          onClick={close}
          className="grid size-9 place-items-center rounded-full bg-black/50 text-fg-muted ring-1 ring-white/15 backdrop-blur hover:text-fg"
          aria-label="Close player"
          title="Close player"
        >
          <X className="size-4" aria-hidden />
        </button>
      </div>

      {error ? (
        <div className="absolute inset-x-0 bottom-24 z-30 mx-auto max-w-lg rounded-lg border border-active/40 bg-surface/95 px-4 py-3 text-center text-[12.5px] text-fg-muted shadow-2xl backdrop-blur">
          {error}
        </div>
      ) : null}

      {ended && nextItemID ? (
        <div className="absolute bottom-28 right-6 z-30 w-72 rounded-lg border border-hairline bg-surface/95 p-4 shadow-2xl backdrop-blur">
          <div className="flex items-center justify-between gap-2">
            <p className="text-[10.5px] uppercase tracking-wide text-fg-dim">Up next</p>
            {autoPlayNext && countdown !== null ? <span className="text-[10.5px] text-progress">Playing in {countdown}s</span> : null}
          </div>
          <p className="mt-1 line-clamp-2 text-[13.5px] font-medium text-fg">{nextTitle}</p>
          <div className="mt-3 flex items-center gap-2">
            <button type="button" onClick={playNext} className="inline-flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-progress/90 px-3 py-1.5 text-[12px] font-medium text-[#062027] hover:bg-progress">
              Play now <ChevronRight className="size-3.5" aria-hidden />
            </button>
            <button type="button" onClick={() => setEnded(false)} className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg">
              Dismiss
            </button>
          </div>
          <label className="mt-3 flex items-center gap-2 text-[10.5px] text-fg-dim">
            <input
              type="checkbox"
              checked={autoPlayNext}
              onChange={(e) => setAutoPlayNext(e.target.checked)}
              className="size-3.5 rounded border-hairline bg-surface-2"
            />
            Autoplay next episode
          </label>
        </div>
      ) : null}
    </div>
  );
}
