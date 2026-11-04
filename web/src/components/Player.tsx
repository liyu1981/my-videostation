import { useCallback, useEffect, useRef, useState } from 'react';
import { useAtom } from 'jotai';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Airplay,
  Cast,
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
  Settings2,
  Subtitles,
  Volume1,
  Volume2,
  VolumeX,
  X,
} from 'lucide-react';
import { playerAtom } from '../atoms/ui';
import {
  api,
  type DLNADevice,
  type PlaybackSubtitle,
  type QualityOption,
  type TranscodeSession,
} from '../lib/api';

type Menu = 'none' | 'subs' | 'audio' | 'speed' | 'cast' | 'quality';

const HLS_MIME = 'application/vnd.apple.mpegurl';

const HIDE_AFTER_MS = 3000;
const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];
const CUE_SIZES = [
  { label: 'S', value: 0.85 },
  { label: 'M', value: 1 },
  { label: 'L', value: 1.25 },
  { label: 'XL', value: 1.5 },
];
const CUE_COLORS = [
  { label: 'White', value: '#ffffff' },
  { label: 'Yellow', value: '#ffe66d' },
];
const clampDelay = (ms: number) => Math.max(-60000, Math.min(60000, ms));
const cueSizeValue = (v: number) => (CUE_SIZES.some((s) => s.value === v) ? v : 1);
const cueColorValue = (v: string | null) => (CUE_COLORS.some((c) => c.value === v) ? v! : '#ffffff');

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
  const [subtitleDelay, setSubtitleDelay] = useState(0);
  const [cueSize, setCueSize] = useState(() => cueSizeValue(Number(localStorage.getItem('mvs.cueSize')) || 1));
  const [cueColor, setCueColor] = useState(() => cueColorValue(localStorage.getItem('mvs.cueColor')));
  const [autoPlayNext, setAutoPlayNext] = useState(() => localStorage.getItem('mvs.autoPlayNext') === 'true');
  const [countdown, setCountdown] = useState<number | null>(null);
  const [menu, setMenu] = useState<Menu>('none');
  const [controlsVisible, setControlsVisible] = useState(true);
  const [ended, setEnded] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [mediaError, setMediaError] = useState(false);
  const [castMessage, setCastMessage] = useState('');
  const [remoteState, setRemoteState] = useState('disconnected');
  const [dlnaDevices, setDlnaDevices] = useState<DLNADevice[]>([]);
  const [dlnaLoading, setDlnaLoading] = useState(false);
  const [dlnaError, setDlnaError] = useState('');

  // M5: HLS transcode session (only when direct play cannot handle the file).
  const [session, setSession] = useState<TranscodeSession | null>(null);
  const [sessionError, setSessionError] = useState('');
  const [startingSession, setStartingSession] = useState(false);
  const [audioIndex, setAudioIndex] = useState<number | null>(null);
  const [audioChosen, setAudioChosen] = useState(false);
  const [maxHeight, setMaxHeight] = useState(0);
  const sessionRef = useRef<TranscodeSession | null>(null);
  const hlsRef = useRef<{ destroy: () => void } | null>(null);
  const selectionRef = useRef({ audioIndex: -1, maxHeight: 0 });
  const startSeq = useRef(0);
  const seekTimer = useRef<number | null>(null);
  const absMsRef = useRef(0);
  const totalDurationRef = useRef(0);
  const wasPlayingRef = useRef(true);
  const pendingSeekRef = useRef<number | null>(null);

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
  const subtitleURL = track && !track.burnIn
    ? `${track.url}${track.url.includes('?') ? '&' : '?'}delayMs=${subtitleDelay}`
    : '';

  const audioTracks = data?.audio ?? [];
  const defaultAudioIndex = audioTracks.find((a) => a.default)?.index ?? audioTracks[0]?.index ?? -1;
  const chosenAudioIndex = audioIndex ?? defaultAudioIndex;
  const selectedAudio = audioTracks.find((a) => a.index === chosenAudioIndex);
  const burnInIndex = track?.burnIn ? Number(track.id.replace('emb-', '')) : -1;
  const needsSession = Boolean(
    data &&
      data.hls &&
      (!data.directPlay ||
        burnInIndex >= 0 ||
        maxHeight > 0 ||
        (selectedAudio && !selectedAudio.direct) ||
        // A plain <video> cannot switch tracks, so an explicit choice needs HLS.
        (audioChosen && chosenAudioIndex !== defaultAudioIndex)),
  );

  const sessionStartSec = (session?.startMs ?? 0) / 1000;
  const totalDuration = session
    ? session.durationMs > 0
      ? (session.startMs + session.durationMs) / 1000
      : session.startMs / 1000 + duration
    : duration;
  totalDurationRef.current = totalDuration;
  const absCurrent = session ? sessionStartSec + current : current;

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
    if (!v) return;
    const abs = absMsRef.current;
    if (abs > 2000) save(abs, totalDurationRef.current * 1000, false);
  }, [save]);

  const stopSession = useCallback((id?: string) => {
    if (!id) return;
    void api.stopTranscode(id);
  }, []);

  // Start (or restart) an HLS session at an absolute position.
  const startSession = useCallback(
    async (startMs: number, burn: number) => {
      if (!data) return;
      const seq = ++startSeq.current;
      setStartingSession(true);
      setSessionError('');
      try {
        const res = await api.transcode(data.itemId, {
          fileId: data.fileId,
          startMs: Math.max(0, Math.round(startMs)),
          audioIndex: selectionRef.current.audioIndex >= 0 ? selectionRef.current.audioIndex : undefined,
          subtitleIndex: burn >= 0 ? burn : undefined,
          maxHeight: selectionRef.current.maxHeight || undefined,
        });
        if (seq !== startSeq.current) {
          void api.stopTranscode(res.sessionId);
          return;
        }
        const previous = sessionRef.current;
        sessionRef.current = res;
        setSession(res);
        if (previous && previous.sessionId !== res.sessionId) stopSession(previous.sessionId);
      } catch (err) {
        if (seq === startSeq.current) setSessionError((err as Error).message);
      } finally {
        if (seq === startSeq.current) setStartingSession(false);
      }
    },
    [data, stopSession],
  );

  const close = useCallback(() => {
    if (historyPushed.current) {
      // popstate handler closes the player (and saves progress).
      window.history.back();
      return;
    }
    saveNow();
    const current = sessionRef.current;
    sessionRef.current = null;
    stopSession(current?.sessionId);
    setPlayer(null);
  }, [saveNow, setPlayer, stopSession]);

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
      const current = sessionRef.current;
      sessionRef.current = null;
      stopSession(current?.sessionId);
      setPlayer(null);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, [saveNow, setPlayer, stopSession]);

  const poke = useCallback(() => {
    setControlsVisible(true);
    if (hideTimer.current) window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => setControlsVisible(false), HIDE_AFTER_MS);
  }, []);

  // Reset per-item state.
  useEffect(() => {
    startSeq.current += 1; // discard in-flight session requests for the old item
    const previous = sessionRef.current;
    sessionRef.current = null;
    if (previous) void api.stopTranscode(previous.sessionId);
    setSession(null);
    setSessionError('');
    setAudioIndex(null);
    setAudioChosen(false);
    setMaxHeight(0);
    selectionRef.current = { audioIndex: -1, maxHeight: 0 };
    absMsRef.current = 0;
    pendingSeekRef.current = null;
    setSubtitleID('off');
    setSubtitleDelay(0);
    setEnded(false);
    setCountdown(null);
    setMenu('none');
    lastSaved.current = 0;
    resumedFor.current = null;
    setCurrent(0);
    setDuration(0);
    setMediaError(false);
    setCastMessage('');
    setDlnaDevices([]);
    setDlnaError('');
    poke();
  }, [player?.itemId, poke]);

  useEffect(() => {
    localStorage.setItem('mvs.cueSize', String(cueSize));
  }, [cueSize]);
  useEffect(() => {
    localStorage.setItem('mvs.cueColor', cueColor);
  }, [cueColor]);
  useEffect(() => {
    localStorage.setItem('mvs.autoPlayNext', String(autoPlayNext));
  }, [autoPlayNext]);

  // Default subtitle (English preferred, text only: image tracks force burn-in).
  useEffect(() => {
    if (!tracks.length || subtitleID !== 'off') return;
    const preferred = tracks.find((t) => t.default && !t.burnIn) ?? tracks.find((t) => t.default);
    if (preferred && !preferred.burnIn) setSubtitleID(preferred.id);
  }, [tracks, subtitleID]);

  // Direct play vs HLS session.
  useEffect(() => {
    if (!data) return;
    if (!needsSession) {
      const previous = sessionRef.current;
      if (previous) {
        // Returning to direct play: keep the position and drop the job.
        pendingSeekRef.current = absMsRef.current / 1000;
        sessionRef.current = null;
        setSession(null);
        stopSession(previous.sessionId);
      }
      return;
    }
    if (!data.hls) return; // the <video> error path explains that ffmpeg is missing
    const resumeMs = absMsRef.current > 3000 ? absMsRef.current : data.resumeMs > 5000 ? data.resumeMs : 0;
    if (sessionRef.current) wasPlayingRef.current = !videoRef.current?.paused;
    void startSession(resumeMs, burnInIndex);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data?.itemId, data?.fileId, data?.hls, needsSession, burnInIndex, chosenAudioIndex, maxHeight]);

  // Attach the session playlist to the <video> element (hls.js or native HLS).
  useEffect(() => {
    const v = videoRef.current;
    if (!v || !session) return;
    let cancelled = false;
    let hls: import('hls.js').default | null = null;
    const onReady = () => {
      if (cancelled) return;
      v.currentTime = 0;
      if (wasPlayingRef.current) void v.play().catch(() => undefined);
    };
    if (v.canPlayType(HLS_MIME)) {
      v.src = session.playlistUrl;
      v.addEventListener('loadedmetadata', onReady, { once: true });
      void v.play().catch(() => undefined);
      return () => {
        cancelled = true;
        v.removeEventListener('loadedmetadata', onReady);
        v.removeAttribute('src');
        v.load();
      };
    }
    void (async () => {
      const { default: Hls } = await import('hls.js');
      if (cancelled) return;
      if (!Hls.isSupported()) {
        setSessionError('HLS (MSE) playback is not supported in this browser.');
        return;
      }
      hls = new Hls({ enableWorker: true, backBufferLength: 90, maxBufferLength: 30 });
      hlsRef.current = hls;
      hls.loadSource(session.playlistUrl);
      hls.attachMedia(v);
      hls.on(Hls.Events.MANIFEST_PARSED, onReady);
      let networkRetries = 0;
      let mediaRetries = 0;
      hls.on(Hls.Events.ERROR, (_event, info) => {
        if (!info.fatal) return;
        if (info.type === Hls.ErrorTypes.NETWORK_ERROR && networkRetries < 2) {
          networkRetries += 1;
          hls?.startLoad();
          return;
        }
        if (info.type === Hls.ErrorTypes.MEDIA_ERROR && mediaRetries < 1) {
          mediaRetries += 1;
          hls?.recoverMediaError();
          return;
        }
        setSessionError(info.details ? `HLS: ${info.details}` : 'Playback failed');
      });
    })();
    return () => {
      cancelled = true;
      hls?.destroy();
      hlsRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.sessionId]);

  // Stop the session when the player unmounts.
  useEffect(
    () => () => {
      startSeq.current += 1; // discard in-flight session requests
      const current = sessionRef.current;
      sessionRef.current = null;
      if (current) void api.stopTranscode(current.sessionId);
      hlsRef.current?.destroy();
    },
    [],
  );

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

  useEffect(() => {
    const remote = (videoRef.current as any)?.remote;
    if (!remote) {
      setRemoteState('unsupported');
      return;
    }
    const update = () => setRemoteState(remote.state || 'disconnected');
    update();
    remote.addEventListener('connect', update);
    remote.addEventListener('disconnect', update);
    return () => {
      remote.removeEventListener('connect', update);
      remote.removeEventListener('disconnect', update);
    };
  }, [data?.itemId, data?.cast?.url]);

  const togglePlay = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) void v.play();
    else v.pause();
    poke();
  }, [poke]);

  // Seek to an absolute position (seconds from the start of the title).
  const seekTo = useCallback(
    (absolute: number) => {
      const v = videoRef.current;
      if (!v) return;
      const total = totalDurationRef.current || v.duration || 0;
      const target = Math.max(0, Math.min(absolute, total));
      const active = sessionRef.current;
      if (!active) {
        v.currentTime = target;
        poke();
        return;
      }
      const local = target - active.startMs / 1000;
      const produced = v.buffered.length ? v.buffered.end(v.buffered.length - 1) : 0;
      if (local >= 0 && local <= produced + 1) {
        v.currentTime = local;
        poke();
        return;
      }
      // Outside the produced window: restart ffmpeg at the target (debounced,
      // so dragging the scrub bar spawns one job instead of dozens).
      if (seekTimer.current) window.clearTimeout(seekTimer.current);
      wasPlayingRef.current = !v.paused;
      seekTimer.current = window.setTimeout(() => {
        void startSession(target * 1000, burnInIndex);
      }, 220);
      poke();
    },
    [poke, startSession, burnInIndex],
  );

  const seek = useCallback(
    (delta: number) => {
      seekTo(absMsRef.current / 1000 + delta);
    },
    [seekTo],
  );

  // Quality / audio / subtitle switching all restart the session at the
  // current position (or promote a direct-play title to a session).
  const chooseQuality = useCallback(
    (height: number) => {
      selectionRef.current.maxHeight = height;
      setMaxHeight(height);
      setMenu('none');
    },
    [],
  );

  const chooseAudio = useCallback(
    (index: number) => {
      selectionRef.current.audioIndex = index;
      setAudioIndex(index);
      setAudioChosen(true);
      setMenu('none');
    },
    [],
  );

  const chooseSubtitle = useCallback(
    (next?: PlaybackSubtitle) => {
      setMenu('none');
      if (!next) {
        setSubtitleID('off');
            if (sessionRef.current && burnInIndex >= 0) {
          wasPlayingRef.current = !videoRef.current?.paused;
          void startSession(absMsRef.current, -1);
        }
        return;
      }
      setSubtitleID(next.id);
      if (next.burnIn) {
        const index = Number(next.id.replace('emb-', ''));
        wasPlayingRef.current = !videoRef.current?.paused;
        void startSession(absMsRef.current, index);
      } else if (burnInIndex >= 0) {
            wasPlayingRef.current = !videoRef.current?.paused;
        void startSession(absMsRef.current, -1);
      }
    },
    [burnInIndex, startSession],
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

  const queue = player?.queue ?? null;
  const queueIndex = player?.queueIndex ?? -1;
  const queueNext = queue && queueIndex >= 0 && queueIndex + 1 < queue.length ? queue[queueIndex + 1] : undefined;
  const queuePrev = queue && queueIndex > 0 ? queue[queueIndex - 1] : undefined;
  // With a queue, the same-season neighbor may be an episode already played;
  // fall back to the playback endpoint's next episode (including cross-season).
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

  const playPrevious = useCallback(() => {
    if (queuePrev && queue) {
      setPlayer({ ...player!, itemId: queuePrev.itemId, title: queuePrev.title, queue, queueIndex: queueIndex - 1 });
      return;
    }
    const prev = neighbors.data?.prev;
    if (!prev) return;
    setPlayer({ itemId: prev.id, title: prev.title });
  }, [queuePrev, queue, queueIndex, player, neighbors.data, setPlayer]);

  const remoteSupported = typeof HTMLMediaElement !== 'undefined' && 'remote' in HTMLMediaElement.prototype;
  const airplaySupported =
    typeof HTMLMediaElement !== 'undefined' && 'webkitShowPlaybackTargetPicker' in HTMLMediaElement.prototype;
  const castReady = Boolean(data?.cast?.compatible && data?.cast?.reachable && data.cast.url);
  const castReason = data?.cast?.reason || 'This media format needs transcoding before it can be cast.';

  const startRemoteCast = async () => {
    if (!castReady) {
      setCastMessage(castReason);
      return;
    }
    const remote = (videoRef.current as any)?.remote;
    if (!remote?.prompt) {
      setCastMessage('Chromecast/remote playback is unavailable in this browser. Use Chrome/Chromium over HTTPS or localhost.');
      return;
    }
    try {
      await remote.prompt();
      setCastMessage(remote.state === 'connected' ? 'Connected to the cast device.' : 'Choose a cast device in the browser prompt.');
    } catch (err) {
      setCastMessage((err as Error)?.message || 'Could not open the cast device picker.');
    }
  };

  const startAirPlay = () => {
    if (!castReady) {
      setCastMessage(castReason);
      return;
    }
    const video = videoRef.current as any;
    if (!video?.webkitShowPlaybackTargetPicker) {
      setCastMessage('AirPlay needs Safari on a compatible Apple device.');
      return;
    }
    video.webkitShowPlaybackTargetPicker();
    setCastMessage('Choose an AirPlay device.');
  };

  const discoverDLNA = async () => {
    if (!castReady) {
      setCastMessage(castReason);
      return;
    }
    setDlnaLoading(true);
    setDlnaError('');
    try {
      const result = await api.castDLNADevices();
      setDlnaDevices(result.devices);
      if (result.devices.length === 0) setDlnaError('No DLNA renderers found on the LAN.');
    } catch (err) {
      setDlnaError((err as Error).message);
    } finally {
      setDlnaLoading(false);
    }
  };

  const playDLNA = async (device: DLNADevice) => {
    if (!data || !castReady) return;
    try {
      await api.castDLNAPlay(data.fileId, device.controlUrl, player?.title ?? 'MyVideoStation');
      setCastMessage(`Sent to ${device.name}. The renderer controls playback.`);
    } catch (err) {
      setCastMessage((err as Error).message);
    }
  };

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
          chooseSubtitle(subtitleID === 'off' ? tracks.find((t) => t.default) ?? tracks[0] : undefined);
          break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [player, menu, seek, togglePlay, changeVolume, toggleFullscreen, close, tracks, subtitleID, chooseSubtitle]);

  if (!player) return null;

  const progressPct = totalDuration > 0 ? (absCurrent / totalDuration) * 100 : 0;
  const effectiveDuration = totalDuration > 0 ? totalDuration : duration;
  const bufferedPct = effectiveDuration > 0 ? ((sessionStartSec + buffered) / effectiveDuration) * 100 : 0;
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
          {...(session || needsSession ? {} : { src: data.cast?.url || data.streamUrl })}
          autoPlay
          playsInline
          crossOrigin="anonymous"
          {...({ 'x-webkit-airplay': 'allow' } as any)}
          className="absolute inset-0 h-full w-full object-contain"
          style={{ '--mvs-cue-size': `${cueSize}em`, '--mvs-cue-color': cueColor } as React.CSSProperties}
          onClick={togglePlay}
          onLoadedMetadata={(e) => {
            const v = e.currentTarget;
            setDuration(v.duration || 0);
            setMediaError(false);
            if (sessionRef.current) return; // HLS timeline starts at the session offset
            const pending = pendingSeekRef.current;
            if (pending !== null) {
              pendingSeekRef.current = null;
              v.currentTime = pending;
              resumedFor.current = data.itemId;
              return;
            }
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
            const absMs = (sessionRef.current?.startMs ?? 0) + v.currentTime * 1000;
            absMsRef.current = absMs;
            // A backward seek/restart should not suppress the next progress save.
            if (absMs < lastSaved.current - 5000) lastSaved.current = absMs;
            if (absMs - lastSaved.current >= 10000) {
              lastSaved.current = absMs;
              save(absMs, totalDurationRef.current * 1000, false);
            }
          }}
          onPlay={() => setPlaying(true)}
          onPause={(e) => {
            setPlaying(false);
            const v = e.currentTarget;
            const absMs = (sessionRef.current?.startMs ?? 0) + v.currentTime * 1000;
            if (v.currentTime > 2 && !v.ended) save(absMs, totalDurationRef.current * 1000, false);
          }}
          onEnded={(e) => {
            const v = e.currentTarget;
            setPlaying(false);
            const total = totalDurationRef.current || v.duration || 0;
            save(total * 1000, total * 1000, true);
            if (nextItemID) setEnded(true);
          }}
          onError={() => {
            if (sessionRef.current || needsSession) return;
            setMediaError(true);
          }}
          onVolumeChange={(e) => {
            setVolume(e.currentTarget.volume);
            setMuted(e.currentTarget.muted);
          }}
        >
          {track ? (
            <track
              key={`${track.id}:${subtitleDelay}`}
              kind="subtitles"
              src={subtitleURL}
              label={track.label}
              srcLang={track.language || 'und'}
              default
            />
          ) : null}
        </video>
      ) : null}

      {track ? <SubtitleEnabler video={videoRef.current} active={`${track.id}:${subtitleDelay}`} /> : null}

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

      {startingSession && !session ? (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-center text-fg-muted">
          <Loader2 className="size-5 animate-spin" aria-hidden />
          <p className="text-[13px]">Preparing transcode…</p>
          <p className="max-w-md text-[11.5px] text-fg-dim">
            {data?.reasons?.length ? `This file needs ${data.reasons.join(', ')} conversion.` : 'Encoding the first segments.'}
          </p>
        </div>
      ) : null}
      {sessionError ? (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 px-8 text-center">
          <p className="max-w-lg text-[14px] text-fg">Playback failed</p>
          <p className="max-w-lg text-[12.5px] text-fg-muted">{sessionError}</p>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => void startSession(absMsRef.current, burnInIndex)}
              className="rounded-lg bg-progress/90 px-3.5 py-1.5 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
            >
              Retry
            </button>
            <button type="button" onClick={close} className="rounded-lg border border-hairline px-3.5 py-1.5 text-[12.5px] text-fg-muted hover:text-fg">
              Close
            </button>
          </div>
        </div>
      ) : null}

      {mediaError ? (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 px-8 text-center">
          <p className="max-w-lg text-[14px] text-fg">This file can’t be played in your browser.</p>
          <p className="max-w-lg text-[12.5px] text-fg-muted">
            {data?.hls
              ? 'The server could not transcode this file. Check the server logs for the ffmpeg error.'
              : 'Matroska/HEVC/DTS support varies by browser, and transcoding is disabled because ffmpeg was not found on the server.'}
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
            {queue && queue.length > 1 ? (
              <p className="mt-0.5 text-[10.5px] text-fg-dim">
                Queue {Math.max(1, queueIndex + 1)} / {queue.length}
              </p>
            ) : null}
            {data ? (
              <p className="mt-0.5 truncate text-[11.5px] text-fg-dim">
                {data.videoCodec ? data.videoCodec.toUpperCase() : 'Video'}
                {selectedAudio ? ` · ${selectedAudio.codec.toUpperCase()} ${selectedAudio.channels}ch` : ''}
                {' · '}
                {session ? (session.mode === 'remux' ? 'Remux' : `Transcode${session.height ? ` ${session.height}p` : ''}`) : 'Direct play'}
                {startingSession && session ? ' (restarting…)' : ''}
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
            max={Math.max(effectiveDuration, 1)}
            step={0.1}
            value={Math.min(absCurrent, effectiveDuration || 0)}
            onChange={(e) => seekTo(Number(e.target.value))}
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
            {fmt(absCurrent)} <span className="text-fg-dim">/ {fmt(effectiveDuration)}</span>
          </span>

          <div className="ml-auto flex items-center gap-1.5">
            <MenuButton label="Subtitles" active={subtitleID !== 'off'} onClick={() => setMenu(menu === 'subs' ? 'none' : 'subs')} icon={<Subtitles className="size-[18px]" aria-hidden />} />
            <MenuButton label="Cast" active={remoteState === 'connected'} onClick={() => setMenu(menu === 'cast' ? 'none' : 'cast')} icon={<Cast className="size-[18px]" aria-hidden />} />
            <MenuButton label="Audio" onClick={() => setMenu(menu === 'audio' ? 'none' : 'audio')} icon={<Volume2 className="size-[18px]" aria-hidden />} />
            <MenuButton label="Speed" onClick={() => setMenu(menu === 'speed' ? 'none' : 'speed')} icon={<Gauge className="size-[18px]" aria-hidden />} text={speed !== 1 ? `${speed}×` : undefined} />
            <MenuButton
              label="Quality"
              active={maxHeight > 0}
              onClick={() => setMenu(menu === 'quality' ? 'none' : 'quality')}
              icon={<Settings2 className="size-[18px]" aria-hidden />}
              text={maxHeight > 0 ? `${maxHeight}p` : undefined}
            />
            <button type="button" onClick={toggleFullscreen} className={ctrlBtn} aria-label={fullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
              {fullscreen ? <Minimize className="size-[18px]" aria-hidden /> : <Maximize className="size-[18px]" aria-hidden />}
            </button>
          </div>
        </div>
      </div>

      {/* menus */}
      {menu !== 'none' ? (
        <div className="absolute bottom-24 right-5 z-30 w-72 overflow-hidden rounded-lg border border-hairline bg-surface/95 shadow-2xl backdrop-blur">
          <div className="border-b border-hairline px-3 py-2 text-[10.5px] uppercase tracking-wide text-fg-dim">
            {menu === 'subs'
              ? 'Subtitles'
              : menu === 'audio'
                ? 'Audio tracks'
                : menu === 'cast'
                  ? 'Cast'
                  : menu === 'quality'
                    ? 'Quality'
                    : 'Playback speed'}
          </div>
          <div className="max-h-[70vh] overflow-y-auto p-1">
            {menu === 'subs' ? (
              <>
                <MenuRow selected={subtitleID === 'off'} onClick={() => chooseSubtitle(undefined)}>Off</MenuRow>
                {tracks.map((t: PlaybackSubtitle) => (
                  <MenuRow key={t.id} selected={subtitleID === t.id} onClick={() => chooseSubtitle(t)}>
                    {t.label}
                    <span className="ml-2 text-[10.5px] uppercase text-fg-dim">
                      {t.language ? `${t.language} · ` : ''}
                      {t.burnIn ? 'image · burn-in' : t.source}
                    </span>
                  </MenuRow>
                ))}
                {tracks.length === 0 ? <p className="px-3 py-2 text-[11.5px] text-fg-dim">No subtitle tracks</p> : null}
                {track?.burnIn ? (
                  <p className="px-3 py-2 text-[10.5px] text-fg-dim">
                    Image subtitles are rendered into the video by the transcoder; delay and styling do not apply.
                  </p>
                ) : (
                <div className="border-t border-hairline px-2.5 pb-2 pt-2">
                  <div className="mb-1 flex items-center justify-between text-[10.5px] uppercase tracking-wide text-fg-dim">
                    <span>Delay</span>
                    <span className="tabular-nums">
                      {subtitleDelay === 0 ? '0.0s' : `${subtitleDelay > 0 ? '+' : ''}${(subtitleDelay / 1000).toFixed(1)}s`}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {[-500, -100, 100, 500].map((delta) => (
                      <button
                        key={delta}
                        type="button"
                        onClick={() => setSubtitleDelay((value) => clampDelay(value + delta))}
                        className="rounded border border-hairline px-1.5 py-1 text-[10.5px] text-fg-muted hover:border-hairline-strong hover:text-fg"
                      >
                        {delta < 0 ? '−' : '+'}
                        {Math.abs(delta) / 1000}s
                      </button>
                    ))}
                    <button
                      type="button"
                      onClick={() => setSubtitleDelay(0)}
                      className="rounded border border-hairline px-1.5 py-1 text-[10.5px] text-fg-muted hover:border-hairline-strong hover:text-fg"
                    >
                      Reset
                    </button>
                  </div>
                  <div className="mb-1 mt-2 text-[10.5px] uppercase tracking-wide text-fg-dim">Size</div>
                  <div className="flex gap-1">
                    {CUE_SIZES.map((size) => (
                      <button
                        key={size.value}
                        type="button"
                        onClick={() => setCueSize(size.value)}
                        className={[
                          'rounded border px-2 py-1 text-[10.5px]',
                          cueSize === size.value
                            ? 'border-progress text-progress'
                            : 'border-hairline text-fg-muted hover:border-hairline-strong hover:text-fg',
                        ].join(' ')}
                      >
                        {size.label}
                      </button>
                    ))}
                  </div>
                  <div className="mb-1 mt-2 text-[10.5px] uppercase tracking-wide text-fg-dim">Colour</div>
                  <div className="flex gap-1">
                    {CUE_COLORS.map((color) => (
                      <button
                        key={color.value}
                        type="button"
                        onClick={() => setCueColor(color.value)}
                        className={[
                          'rounded border px-2 py-1 text-[10.5px]',
                          cueColor === color.value
                            ? 'border-progress text-progress'
                            : 'border-hairline text-fg-muted hover:border-hairline-strong hover:text-fg',
                        ].join(' ')}
                      >
                        {color.label}
                      </button>
                    ))}
                  </div>
                </div>
                )}
              </>
            ) : null}
            {menu === 'audio' ? (
              <>
                {audioTracks.map((a) => (
                  <MenuRow key={a.index} selected={a.index === chosenAudioIndex} onClick={() => chooseAudio(a.index)}>
                    {a.codec.toUpperCase()} · {a.channels}ch
                    {a.language ? <span className="ml-2 text-[10.5px] uppercase text-fg-dim">{a.language}</span> : null}
                    {a.title ? <span className="ml-2 text-[10.5px] text-fg-dim">{a.title}</span> : null}
                  </MenuRow>
                ))}
                {audioTracks.length === 0 ? <p className="px-3 py-2 text-[11.5px] text-fg-dim">No audio tracks</p> : null}
                {data && !data.hls ? (
                  <p className="px-3 py-2 text-[10.5px] text-fg-dim">Track switching needs ffmpeg on the server.</p>
                ) : (
                  <p className="px-3 py-2 text-[10.5px] text-fg-dim">
                    Switching tracks restarts the transcode at the current position.
                  </p>
                )}
              </>
            ) : null}
            {menu === 'cast' ? (
              <div className="space-y-2 p-1">
                {!data ? (
                  <p className="px-2 py-1 text-[11.5px] text-fg-dim">Preparing playback…</p>
                ) : !castReady ? (
                  <div className="rounded-md border border-new/30 bg-new/5 px-2.5 py-2">
                    <p className="text-[11.5px] font-medium text-new">Casting is not ready for this file.</p>
                    <p className="mt-1 text-[10.5px] text-fg-muted">{castReason}</p>
                    {!data.cast?.reachable ? (
                      <p className="mt-1 text-[10.5px] text-fg-muted">Open MyVideoStation via the NAS LAN IP, not localhost.</p>
                    ) : null}
                  </div>
                ) : (
                  <>
                    <div>
                      <p className="px-1 pb-1 text-[10.5px] uppercase tracking-wide text-fg-dim">Chromecast</p>
                      <button
                        type="button"
                        onClick={() => void startRemoteCast()}
                        disabled={!remoteSupported}
                        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[11.5px] text-fg-muted hover:bg-surface-3 hover:text-fg disabled:opacity-40"
                      >
                        <Cast className="size-3.5" aria-hidden />
                        Cast via Chrome remote playback
                      </button>
                      {!remoteSupported ? (
                        <p className="px-1 text-[10px] text-fg-dim">Requires Chrome/Chromium over HTTPS or localhost.</p>
                      ) : null}
                    </div>
                    <div>
                      <p className="px-1 pb-1 text-[10.5px] uppercase tracking-wide text-fg-dim">AirPlay</p>
                      <button
                        type="button"
                        onClick={startAirPlay}
                        disabled={!airplaySupported}
                        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[11.5px] text-fg-muted hover:bg-surface-3 hover:text-fg disabled:opacity-40"
                      >
                        <Airplay className="size-3.5" aria-hidden />
                        Open AirPlay picker
                      </button>
                      {!airplaySupported ? <p className="px-1 text-[10px] text-fg-dim">Requires Safari.</p> : null}
                    </div>
                    <div>
                      <p className="px-1 pb-1 text-[10.5px] uppercase tracking-wide text-fg-dim">DLNA</p>
                      <button
                        type="button"
                        onClick={() => void discoverDLNA()}
                        disabled={dlnaLoading}
                        className="w-full rounded-md border border-hairline px-2 py-1.5 text-left text-[11.5px] text-fg-muted hover:border-hairline-strong hover:text-fg disabled:opacity-40"
                      >
                        {dlnaLoading ? 'Searching for renderers…' : 'Discover DLNA renderers'}
                      </button>
                      {dlnaDevices.map((device) => (
                        <button
                          key={device.id}
                          type="button"
                          onClick={() => void playDLNA(device)}
                          className="mt-1 block w-full truncate rounded-md px-2 py-1 text-left text-[11.5px] text-fg-muted hover:bg-surface-3 hover:text-fg"
                        >
                          {device.name}
                        </button>
                      ))}
                      {dlnaError ? <p className="px-1 text-[10.5px] text-active">{dlnaError}</p> : null}
                    </div>
                  </>
                )}
                {castMessage ? <p className="px-1 text-[10.5px] text-progress">{castMessage}</p> : null}
              </div>
            ) : null}
            {menu === 'quality' ? (
              <>
                {(data?.qualities ?? []).map((q: QualityOption) => (
                  <MenuRow key={q.height} selected={maxHeight === q.height} onClick={() => chooseQuality(q.height)}>
                    {q.label}
                    <span className="ml-2 text-[10.5px] text-fg-dim">
                      {q.height === 0 ? (data?.directPlay ? 'original' : `source${data?.height ? ` · ${data.height}p` : ''}`) : `${q.height}p`}
                    </span>
                  </MenuRow>
                ))}
                {!data?.hls ? (
                  <p className="px-3 py-2 text-[10.5px] text-fg-dim">Quality selection needs ffmpeg on the server.</p>
                ) : (
                  <p className="px-3 py-2 text-[10.5px] text-fg-dim">Lower resolutions reduce bandwidth and CPU.</p>
                )}
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
          <div className="flex items-center justify-between gap-2">
            <p className="text-[10.5px] uppercase tracking-wide text-fg-dim">Up next</p>
            {autoPlayNext && countdown !== null ? <span className="text-[10.5px] text-progress">Playing in {countdown}s</span> : null}
          </div>
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
