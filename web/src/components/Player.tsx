import { useCallback, useEffect, useRef, useState } from 'react';
import { useAtom } from 'jotai';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ChevronRight,
  Gauge,
  Loader2,
  Maximize,
  Minimize,
  Pause,
  Play,
  RotateCcw,
  RotateCw,
  SkipBack,
  SkipForward,
  Subtitles,
  Volume1,
  Volume2,
  VolumeX,
  X,
} from 'lucide-react';
import { playerAtom } from '../atoms/ui';
import { api, type PlaybackSubtitle } from '../lib/api';

type Menu = 'none' | 'subs' | 'audio' | 'speed';

const HIDE_AFTER_MS = 3000;
const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];

/**
 * Video-Station-style player: custom control bar over a full-bleed video,
 * auto-hiding UI (cursor hides too), progress + buffered bar, volume, subtitle
 * menu, audio-track list, playback speed, fullscreen, keyboard shortcuts and
 * next-episode prompt.
 */
export function PlayerModal() {
  const [player, setPlayer] = useAtom(playerAtom);
  const qc = useQueryClient();
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  const [playing, setPlaying] = useState(false);
  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(0);
  const [buffered, setBuffered] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [subtitleID, setSubtitleID] = useState('off');
  const [menu, setMenu] = useState<Menu>('none');
  const [controlsVisible, setControlsVisible] = useState(true);
  const [ended, setEnded] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [mediaError, setMediaError] = useState(false);

  const hideTimer = useRef<number | null>(null);
  const lastSaved = useRef(0);
  const resumedFor = useRef<number | null>(null);
  // Opening the player pushes one history entry, so the browser Back button closes it.
  const historyPushed = useRef(false);

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
  const tracks = data?.subtitles ?? [];
  const track = tracks.find((t) => t.id === subtitleID);

  const save = useCallback(
    (positionMs: number, durationMs: number, played: boolean) => {
      if (!player) return;
      void api
        .progress(player.itemId, { positionMs: Math.round(positionMs), durationMs: Math.round(durationMs), played })
        .then(() => {
          void qc.invalidateQueries({ queryKey: ['item', player.itemId] });
          void qc.invalidateQueries({ queryKey: ['home'] });
        })
        .catch(() => undefined);
    },
    [player, qc],
  );

  const saveNow = useCallback(() => {
    const v = videoRef.current;
    if (v && v.currentTime > 2) save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
  }, [save]);

  const close = useCallback(() => {
    if (historyPushed.current) {
      // popstate handler closes the player (and saves progress).
      window.history.back();
      return;
    }
    saveNow();
    setPlayer(null);
  }, [saveNow, setPlayer]);

  // Push a history entry when the player opens; Back then closes the player.
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
      setPlayer(null);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, [saveNow, setPlayer]);

  const poke = useCallback(() => {
    setControlsVisible(true);
    if (hideTimer.current) window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => setControlsVisible(false), HIDE_AFTER_MS);
  }, []);

  // Reset per-item state.
  useEffect(() => {
    setSubtitleID('off');
    setEnded(false);
    setMenu('none');
    lastSaved.current = 0;
    resumedFor.current = null;
    setCurrent(0);
    setDuration(0);
    setMediaError(false);
    poke();
  }, [player?.itemId, poke]);

  // Default subtitle (English preferred).
  useEffect(() => {
    if (!tracks.length || subtitleID !== 'off') return;
    const preferred = tracks.find((t) => t.default);
    if (preferred) setSubtitleID(preferred.id);
  }, [tracks, subtitleID]);

  // Auto-hide while playing.
  useEffect(() => {
    if (!controlsVisible) return;
    if (hideTimer.current) window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => setControlsVisible(false), HIDE_AFTER_MS);
    return () => {
      if (hideTimer.current) window.clearTimeout(hideTimer.current);
    };
  }, [controlsVisible, playing]);

  useEffect(() => {
    const onFs = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', onFs);
    return () => document.removeEventListener('fullscreenchange', onFs);
  }, []);

  const togglePlay = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) void v.play();
    else v.pause();
    poke();
  }, [poke]);

  const seek = useCallback(
    (delta: number) => {
      const v = videoRef.current;
      if (!v) return;
      v.currentTime = Math.min(Math.max(0, v.currentTime + delta), v.duration || 0);
      poke();
    },
    [poke],
  );

  const changeVolume = useCallback((value: number) => {
    const v = videoRef.current;
    if (!v) return;
    v.volume = value;
    v.muted = value === 0;
  }, []);

  const toggleFullscreen = useCallback(() => {
    const el = rootRef.current;
    if (!el) return;
    if (document.fullscreenElement) void document.exitFullscreen();
    else void el.requestFullscreen();
  }, []);

  const nextTitle = neighbors.data?.next?.title ?? data?.next?.title;
  const nextItemID = neighbors.data?.next?.id ?? data?.next?.itemId;

  const playNext = useCallback(() => {
    if (!nextItemID || !nextTitle) return;
    setPlayer({ itemId: nextItemID, title: nextTitle });
  }, [nextItemID, nextTitle, setPlayer]);

  const playPrevious = useCallback(() => {
    const prev = neighbors.data?.prev;
    if (!prev) return;
    setPlayer({ itemId: prev.id, title: prev.title });
  }, [neighbors.data, setPlayer]);

  // Keyboard shortcuts.
  useEffect(() => {
    if (!player) return;
    const onKey = (e: KeyboardEvent) => {
      const v = videoRef.current;
      if (!v) return;
      switch (e.key) {
        case 'Escape':
          if (menu !== 'none') setMenu('none');
          else if (document.fullscreenElement) void document.exitFullscreen();
          else close();
          break;
        case ' ':
        case 'k':
          e.preventDefault();
          togglePlay();
          break;
        case 'ArrowLeft':
          seek(-10);
          break;
        case 'ArrowRight':
          seek(10);
          break;
        case 'ArrowUp':
          e.preventDefault();
          changeVolume(Math.min(1, v.volume + 0.1));
          break;
        case 'ArrowDown':
          e.preventDefault();
          changeVolume(Math.max(0, v.volume - 0.1));
          break;
        case 'm':
          v.muted = !v.muted;
          break;
        case 'f':
          toggleFullscreen();
          break;
        case 'c':
          setSubtitleID((id) => (id === 'off' ? (tracks.find((t) => t.default)?.id ?? tracks[0]?.id ?? 'off') : 'off'));
          break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [player, menu, seek, togglePlay, changeVolume, toggleFullscreen, close, tracks]);

  if (!player) return null;

  const progressPct = duration > 0 ? (current / duration) * 100 : 0;
  const bufferedPct = duration > 0 ? (buffered / duration) * 100 : 0;
  const chrome = controlsVisible ? 'opacity-100' : 'pointer-events-none opacity-0';

  return (
    <div
      ref={rootRef}
      className={['fixed inset-0 z-50 select-none bg-black', controlsVisible ? '' : 'cursor-none'].join(' ')}
      onMouseMove={poke}
      onMouseLeave={() => playing && setControlsVisible(false)}
      onDoubleClick={toggleFullscreen}
    >
      {data ? (
        <video
          ref={videoRef}
          src={data.streamUrl}
          autoPlay
          crossOrigin="anonymous"
          className="absolute inset-0 h-full w-full object-contain"
          onClick={togglePlay}
          onLoadedMetadata={(e) => {
            const v = e.currentTarget;
            setDuration(v.duration || 0);
            setMediaError(false);
            if (resumedFor.current !== data.itemId) {
              resumedFor.current = data.itemId;
              if (data.resumeMs > 5000 && (!v.duration || data.resumeMs < v.duration * 0.9 * 1000)) {
                v.currentTime = data.resumeMs / 1000;
              }
            }
          }}
          onTimeUpdate={(e) => {
            const v = e.currentTarget;
            setCurrent(v.currentTime);
            if (v.buffered.length) setBuffered(v.buffered.end(v.buffered.length - 1));
            if (v.currentTime - lastSaved.current >= 10) {
              lastSaved.current = v.currentTime;
              save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
            }
          }}
          onPlay={() => setPlaying(true)}
          onPause={(e) => {
            setPlaying(false);
            const v = e.currentTarget;
            if (v.currentTime > 2 && !v.ended) save(v.currentTime * 1000, (v.duration || 0) * 1000, false);
          }}
          onEnded={(e) => {
            const v = e.currentTarget;
            setPlaying(false);
            save(v.duration * 1000, v.duration * 1000, true);
            if (nextItemID) setEnded(true);
          }}
          onError={() => setMediaError(true)}
          onVolumeChange={(e) => {
            setVolume(e.currentTarget.volume);
            setMuted(e.currentTarget.muted);
          }}
        >
          {track ? (
            <track key={track.id} kind="subtitles" src={track.url} label={track.label} srcLang={track.language || 'und'} default />
          ) : null}
        </video>
      ) : null}

      {track ? <SubtitleEnabler video={videoRef.current} active={track.id} /> : null}

      {playback.isLoading ? (
        <div className="absolute inset-0 flex items-center justify-center gap-2 text-[13px] text-fg-dim">
          <Loader2 className="size-4 animate-spin" aria-hidden /> Preparing playback…
        </div>
      ) : null}
      {playback.isError ? (
        <p className="absolute inset-0 flex items-center justify-center px-8 text-center text-[13px] text-fg-muted">
          {(playback.error as Error).message}
        </p>
      ) : null}

      {mediaError ? (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 px-8 text-center">
          <p className="max-w-lg text-[14px] text-fg">This file can’t be played in your browser.</p>
          <p className="max-w-lg text-[12.5px] text-fg-muted">
            Matroska/HEVC/DTS support varies by browser. Transcoding arrives in M5 — until then, download the file or play it with a
            desktop player.
          </p>
          <div className="flex items-center gap-2">
            {data ? (
              <a
                href={data.streamUrl}
                download
                className="rounded-lg bg-progress/90 px-3.5 py-1.5 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
              >
                Download file
              </a>
            ) : null}
            <button type="button" onClick={close} className="rounded-lg border border-hairline px-3.5 py-1.5 text-[12.5px] text-fg-muted hover:text-fg">
              Close
            </button>
          </div>
        </div>
      ) : null}

      {/* paused big play */}
      {data && !mediaError && !playing && !ended ? (
        <button
          type="button"
          onClick={togglePlay}
          aria-label="Play"
          className="absolute left-1/2 top-1/2 z-20 grid size-20 -translate-x-1/2 -translate-y-1/2 place-items-center rounded-full bg-black/45 ring-1 ring-white/25 backdrop-blur transition-transform hover:scale-105"
        >
          <Play className="ml-1 size-9 text-white" aria-hidden />
        </button>
      ) : null}

      {/* top bar */}
      <div className={`absolute inset-x-0 top-0 z-20 bg-gradient-to-b from-black/85 to-transparent px-5 pb-10 pt-4 transition-opacity duration-300 ${chrome}`}>
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <p className="truncate text-[14px] font-medium text-fg">{player.title}</p>
            {data && data.audio.length > 0 ? (
              <p className="mt-0.5 truncate text-[11.5px] text-fg-dim">
                {data.audio[0]!.codec.toUpperCase()} · {data.audio[0]!.channels}ch
              </p>
            ) : null}
          </div>
          <button
            type="button"
            onClick={close}
            className="rounded-lg p-2 text-fg-muted transition-colors hover:bg-white/10 hover:text-fg"
            aria-label="Close player"
          >
            <X className="size-5" aria-hidden />
          </button>
        </div>
      </div>

      {/* bottom controls */}
      <div className={`absolute inset-x-0 bottom-0 z-20 bg-gradient-to-t from-black/90 via-black/60 to-transparent px-5 pb-4 pt-14 transition-opacity duration-300 ${chrome}`}>
        <div className="group/bar relative">
          <input
            type="range"
            min={0}
            max={Math.max(duration, 1)}
            step={0.1}
            value={current}
            onChange={(e) => {
              const v = videoRef.current;
              if (v) v.currentTime = Number(e.target.value);
            }}
            aria-label="Seek"
            className="player-range"
            style={{ ['--player-progress' as never]: `${progressPct}%` }}
          />
          <div
            className="pointer-events-none absolute left-0 top-1/2 h-[4px] -translate-y-1/2 rounded-full bg-white/25"
            style={{ width: `${bufferedPct}%` }}
          />
        </div>

        <div className="mt-2.5 flex items-center gap-1.5 text-fg-muted">
          <button type="button" onClick={playPrevious} disabled={!neighbors.data?.prev} className={ctrlBtn} aria-label="Previous item">
            <SkipBack className="size-[18px]" aria-hidden />
          </button>
          <button type="button" onClick={() => seek(-10)} className={ctrlBtn} aria-label="Back 10 seconds">
            <RotateCcw className="size-[18px]" aria-hidden />
          </button>
          <button type="button" onClick={togglePlay} className={ctrlBtn} aria-label={playing ? 'Pause' : 'Play'}>
            {playing ? <Pause className="size-5" aria-hidden /> : <Play className="size-5" aria-hidden />}
          </button>
          <button type="button" onClick={() => seek(10)} className={ctrlBtn} aria-label="Forward 10 seconds">
            <RotateCw className="size-[18px]" aria-hidden />
          </button>
          <button type="button" onClick={playNext} disabled={!nextItemID} className={ctrlBtn} aria-label="Next item">
            <SkipForward className="size-[18px]" aria-hidden />
          </button>

          <div className="group/vol ml-1 flex items-center">
            <button
              type="button"
              onClick={() => {
                const v = videoRef.current;
                if (v) v.muted = !v.muted;
              }}
              className={ctrlBtn}
              aria-label={muted ? 'Unmute' : 'Mute'}
            >
              {muted || volume === 0 ? <VolumeX className="size-[18px]" aria-hidden /> : volume < 0.5 ? <Volume1 className="size-[18px]" aria-hidden /> : <Volume2 className="size-[18px]" aria-hidden />}
            </button>
            <input
              type="range"
              min={0}
              max={1}
              step={0.01}
              value={muted ? 0 : volume}
              onChange={(e) => changeVolume(Number(e.target.value))}
              aria-label="Volume"
              className="player-range player-range-sm w-0 opacity-0 transition-all duration-200 group-hover/vol:w-20 group-hover/vol:opacity-100"
            />
          </div>

          <span className="ml-2 tabular-nums text-[12px] text-fg">
            {fmt(current)} <span className="text-fg-dim">/ {fmt(duration)}</span>
          </span>

          <div className="ml-auto flex items-center gap-1.5">
            <MenuButton label="Subtitles" active={subtitleID !== 'off'} onClick={() => setMenu(menu === 'subs' ? 'none' : 'subs')} icon={<Subtitles className="size-[18px]" aria-hidden />} />
            <MenuButton label="Audio" onClick={() => setMenu(menu === 'audio' ? 'none' : 'audio')} icon={<Volume2 className="size-[18px]" aria-hidden />} />
            <MenuButton label="Speed" onClick={() => setMenu(menu === 'speed' ? 'none' : 'speed')} icon={<Gauge className="size-[18px]" aria-hidden />} text={speed !== 1 ? `${speed}×` : undefined} />
            <button type="button" onClick={toggleFullscreen} className={ctrlBtn} aria-label={fullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
              {fullscreen ? <Minimize className="size-[18px]" aria-hidden /> : <Maximize className="size-[18px]" aria-hidden />}
            </button>
          </div>
        </div>
      </div>

      {/* menus */}
      {menu !== 'none' ? (
        <div className="absolute bottom-24 right-5 z-30 w-64 overflow-hidden rounded-lg border border-hairline bg-surface/95 shadow-2xl backdrop-blur">
          <div className="border-b border-hairline px-3 py-2 text-[10.5px] uppercase tracking-wide text-fg-dim">
            {menu === 'subs' ? 'Subtitles' : menu === 'audio' ? 'Audio tracks' : 'Playback speed'}
          </div>
          <div className="max-h-64 overflow-y-auto p-1">
            {menu === 'subs' ? (
              <>
                <MenuRow selected={subtitleID === 'off'} onClick={() => { setSubtitleID('off'); setMenu('none'); }}>Off</MenuRow>
                {tracks.map((t: PlaybackSubtitle) => (
                  <MenuRow key={t.id} selected={subtitleID === t.id} onClick={() => { setSubtitleID(t.id); setMenu('none'); }}>
                    {t.label}
                    {t.language ? <span className="ml-2 text-[10.5px] uppercase text-fg-dim">{t.language} · {t.source}</span> : null}
                  </MenuRow>
                ))}
                {tracks.length === 0 ? <p className="px-3 py-2 text-[11.5px] text-fg-dim">No subtitle tracks</p> : null}
              </>
            ) : null}
            {menu === 'audio' ? (
              <>
                {(data?.audio ?? []).map((a) => (
                  <MenuRow key={a.index} selected={Boolean(a.default)} onClick={() => undefined}>
                    {a.codec.toUpperCase()} · {a.channels}ch
                    {a.language ? <span className="ml-2 text-[10.5px] uppercase text-fg-dim">{a.language}</span> : null}
                    {a.title ? <span className="ml-2 text-[10.5px] text-fg-dim">{a.title}</span> : null}
                  </MenuRow>
                ))}
                {(data?.audio ?? []).length === 0 ? <p className="px-3 py-2 text-[11.5px] text-fg-dim">No audio tracks</p> : null}
                <p className="px-3 py-2 text-[10.5px] text-fg-dim">Track switching needs transcoding (M5); the browser plays the default track.</p>
              </>
            ) : null}
            {menu === 'speed' ? (
              SPEEDS.map((s) => (
                <MenuRow key={s} selected={speed === s} onClick={() => {
                  const v = videoRef.current;
                  if (v) v.playbackRate = s;
                  setSpeed(s);
                  setMenu('none');
                }}>
                  {s === 1 ? 'Normal' : `${s}×`}
                </MenuRow>
              ))
            ) : null}
          </div>
        </div>
      ) : null}

      {/* next up */}
      {ended && nextItemID ? (
        <div className="absolute bottom-28 right-6 z-30 w-72 rounded-lg border border-hairline bg-surface/95 p-4 shadow-2xl backdrop-blur">
          <p className="text-[10.5px] uppercase tracking-wide text-fg-dim">Up next</p>
          <p className="mt-1 line-clamp-2 text-[13.5px] font-medium text-fg">
            {nextTitle}
          </p>
          <div className="mt-3 flex items-center gap-2">
            <button type="button" onClick={playNext} className="inline-flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-progress/90 px-3 py-1.5 text-[12px] font-medium text-[#062027] hover:bg-progress">
              Play now <ChevronRight className="size-3.5" aria-hidden />
            </button>
            <button type="button" onClick={() => setEnded(false)} className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg">
              Dismiss
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

const ctrlBtn =
  'grid size-9 place-items-center rounded-md text-fg-muted transition-colors hover:bg-white/10 hover:text-fg disabled:opacity-30 disabled:hover:bg-transparent';

function MenuButton({ label, icon, onClick, active, text }: { label: string; icon: React.ReactNode; onClick: () => void; active?: boolean; text?: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      className={['inline-flex items-center gap-1 rounded-md px-2 py-2 text-[11.5px] transition-colors hover:bg-white/10', active ? 'text-progress' : 'text-fg-muted hover:text-fg'].join(' ')}
    >
      {icon}
      {text ? <span className="tabular-nums">{text}</span> : null}
    </button>
  );
}

function MenuRow({ children, selected, onClick }: { children: React.ReactNode; selected?: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        'flex w-full items-center justify-between rounded-md px-2.5 py-1.5 text-left text-[12px] hover:bg-surface-3',
        selected ? 'text-fg' : 'text-fg-muted',
      ].join(' ')}
    >
      <span className="flex min-w-0 items-center">{children}</span>
      {selected ? <span className="ml-2 size-1.5 rounded-full bg-progress" aria-hidden /> : null}
    </button>
  );
}

/** Keeps the selected subtitle track enabled after the <track> element remounts. */
function SubtitleEnabler({ video, active }: { video: HTMLVideoElement | null; active: string }) {
  useEffect(() => {
    if (!video) return;
    const tracks = video.textTracks;
    for (let i = 0; i < tracks.length; i++) tracks[i].mode = i === 0 ? 'showing' : 'disabled';
  }, [video, active]);
  return null;
}

function fmt(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) seconds = 0;
  const s = Math.floor(seconds % 60);
  const m = Math.floor((seconds / 60) % 60);
  const h = Math.floor(seconds / 3600);
  const mm = String(m).padStart(2, '0');
  const ss = String(s).padStart(2, '0');
  return h > 0 ? `${h}:${mm}:${ss}` : `${m}:${ss}`;
}
