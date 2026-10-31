export function formatRuntime(ms?: number): string {
  if (!ms || ms <= 0) return '';
  const totalMinutes = Math.round(ms / 60000);
  const h = Math.floor(totalMinutes / 60);
  const m = totalMinutes % 60;
  return h > 0 ? `${h}h ${m.toString().padStart(2, '0')}m` : `${m}m`;
}

export function formatClock(ms?: number): string {
  if (!ms || ms <= 0) return '';
  const total = Math.round(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const mm = String(m).padStart(2, '0');
  const ss = String(s).padStart(2, '0');
  return h > 0 ? `${h}:${mm}:${ss}` : `${m}:${ss}`;
}

export function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatReleaseDate(value?: string): string {
  if (!value) return '';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

export function formatDate(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

export function formatDateTime(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

export function episodeLabel(season?: number, episode?: number, end?: number): string {
  if (!episode) return '';
  const s = season && season > 0 ? `S${String(season).padStart(2, '0')}` : '';
  const first = `E${String(episode).padStart(2, '0')}`;
  const last = end && end > episode ? `-E${String(end).padStart(2, '0')}` : '';
  return `${s}${first}${last}`;
}

export function resolutionLabel(_width?: number, height?: number): string {
  if (!height) return '';
  if (height >= 2000) return '4K';
  if (height >= 1000) return '1080p';
  if (height >= 700) return '720p';
  return `${height}p`;
}

export function audioSummary(file?: { audioStreams: { codec: string; channels: number; layout?: string }[] }): string {
  const track = file?.audioStreams?.[0];
  if (!track) return '';
  const channels = track.channels >= 8 ? '7.1' : track.channels === 6 ? '5.1' : track.channels === 2 ? 'stereo' : `${track.channels}ch`;
  return `${track.codec.toUpperCase()} ${channels}`;
}

export function titleFromPath(path: string): string {
  const base = path.split('/').pop() ?? path;
  return base.replace(/\.[^.]+$/, '');
}

/** Deterministic accent gradient for items without artwork. */
export function posterGradient(seed: string): string {
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) % 360;
  const h2 = (h + 38) % 360;
  return `linear-gradient(145deg, hsl(${h} 26% 24%), hsl(${h2} 30% 12%))`;
}

export function kindLabel(kind: string): string {
  switch (kind) {
    case 'movie':
      return 'Movies';
    case 'show':
      return 'TV Shows';
    case 'home_video':
      return 'Home Videos';
    case 'episode':
      return 'Episodes';
    default:
      return 'Library';
  }
}
