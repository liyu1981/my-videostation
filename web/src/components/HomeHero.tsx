import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useAtom } from 'jotai';
import { ChevronLeft, ChevronRight, Play, Star } from 'lucide-react';
import type { Item } from '../lib/api';
import { formatRuntime, posterGradient } from '../lib/format';
import { playerAtom } from '../atoms/ui';

const AUTOPLAY_MS = 8000;
const TRANSITION_MS = 700;

/**
 * Video-Station-style hero with a ReactBits-inspired carousel feel:
 * slide + crossfade between backdrops, ken-burns drift, staggered blur/rise text,
 * autoplay progress bars, drag/swipe, hover pause and reduced-motion support.
 */
export function HomeHero({ items }: { items: Item[] }) {
  const slides = useMemo(() => dedupe(items).slice(0, 10), [items]);
  const [index, setIndex] = useState(0);
  const [prev, setPrev] = useState<number | null>(null);
  const [paused, setPaused] = useState(false);
  const [dragging, setDragging] = useState(false);
  const dragX = useRef<number | null>(null);
  const dragged = useRef(false);
  const [, setPlayer] = useAtom(playerAtom);
  const navigate = useNavigate();
  const reduced = usePrefersReducedMotion();

  useEffect(() => {
    if (index >= slides.length) setIndex(0);
  }, [index, slides.length]);

  // Autoplay restarts on every slide change (and when hover/drag pauses).
  useEffect(() => {
    if (paused || dragging || reduced || slides.length < 2) return;
    const id = window.setTimeout(() => {
      setPrev(index);
      setIndex((index + 1) % slides.length);
    }, AUTOPLAY_MS);
    return () => window.clearTimeout(id);
  }, [index, paused, dragging, reduced, slides.length]);

  // Drop the outgoing layer once its fade-out finishes.
  useEffect(() => {
    if (prev === null) return;
    const id = window.setTimeout(() => setPrev(null), TRANSITION_MS + 50);
    return () => window.clearTimeout(id);
  }, [prev]);

  if (slides.length === 0) return null;

  const item = slides[index]!;
  const previous = prev !== null ? slides[prev] : undefined;
  const upcoming = [...slides.slice(index + 1), ...slides.slice(0, index)].slice(0, 2);
  const stars = item.rating ? Math.round(item.rating / 2) : 0;

  const go = (delta: number) => {
    const next = (index + delta + slides.length) % slides.length;
    if (next === index) return;
    setPrev(index);
    setIndex(next);
  };

  const rise = (delay: number) =>
    reduced ? undefined : { animation: `hero-rise 620ms cubic-bezier(0.22, 1, 0.36, 1) ${delay}ms both` };

  return (
    <section
      className="hero-height group relative mx-6 mt-6 touch-pan-y overflow-hidden rounded-xl border border-hairline bg-black lg:mx-8"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onPointerDown={(e) => {
        if (e.pointerType === 'mouse' && e.button !== 0) return;
        dragX.current = e.clientX;
        dragged.current = false;
      }}
      onPointerMove={(e) => {
        if (dragX.current === null || dragged.current) return;
        // Only start a drag after real movement, so clicks on arrows/buttons work.
        if (Math.abs(e.clientX - dragX.current) < 14) return;
        dragged.current = true;
        setDragging(true);
        try {
          (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
        } catch {
          /* ignore */
        }
      }}
      onPointerUp={(e) => {
        if (dragX.current === null) return;
        const dx = e.clientX - dragX.current;
        const wasDragged = dragged.current;
        dragX.current = null;
        dragged.current = false;
        setDragging(false);
        if (wasDragged && Math.abs(dx) > 60) go(dx < 0 ? 1 : -1);
      }}
      onPointerCancel={() => {
        dragX.current = null;
        dragged.current = false;
        setDragging(false);
      }}
      aria-roledescription="carousel"
      aria-label="Featured"
    >
      <div className="flex h-full">
        {/* poster (full hero height, like VS) */}
        <Link
          key={`poster-${item.id}`}
          to="/items/$itemId"
          params={{ itemId: String(item.id) }}
          className="hidden h-full shrink-0 border-r border-white/10 sm:block"
          style={{
            aspectRatio: '2 / 3',
            ...(reduced ? {} : { animation: `hero-poster-in 640ms cubic-bezier(0.22, 1, 0.36, 1) both` }),
          }}
          aria-label={item.title}
        >
          <PosterArt item={item} />
        </Link>

        {/* active backdrop + text */}
        <div className="relative min-w-0 flex-1 overflow-hidden">
          {previous ? (
            <div
              key={`out-${previous.id}`}
              className="absolute inset-0"
              style={reduced ? { opacity: 0 } : { animation: `hero-slide-out ${TRANSITION_MS}ms cubic-bezier(0.4, 0, 1, 1) both` }}
              aria-hidden
            >
              <Backdrop item={previous} zoom={false} reduced={reduced} />
            </div>
          ) : null}

          <div
            key={`in-${item.id}`}
            className="absolute inset-0"
            style={reduced ? undefined : { animation: `hero-slide-in ${TRANSITION_MS}ms cubic-bezier(0.22, 1, 0.36, 1) both` }}
          >
            <Backdrop item={item} zoom reduced={reduced} />
            <div className="absolute inset-0 bg-gradient-to-r from-bg/90 via-bg/45 to-bg/10" />
            <div className="absolute inset-0 bg-gradient-to-t from-bg/95 via-bg/15 to-bg/25" />
          </div>

          <div
            key={`text-${item.id}`}
            className="relative flex h-full flex-col justify-end px-5 pb-6 sm:px-6 lg:pb-8"
          >
            <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.16em] text-progress" style={rise(40)}>
              {item.kind === 'show' ? 'TV Show' : item.kind === 'home_video' ? 'Home Video' : 'Movie'}
              {item.year ? ` · ${item.year}` : ''}
            </p>
            <h1
              className="line-clamp-2 text-[26px] font-semibold leading-[1.12] tracking-tight text-fg-heading sm:text-[30px] lg:text-[36px] xl:text-[40px]"
              style={rise(110)}
            >
              {item.title}
            </h1>
            {item.tagline ? (
              <p className="mt-1 line-clamp-1 text-[13px] italic text-fg-muted" style={rise(180)}>
                {item.tagline}
              </p>
            ) : null}

            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[12.5px] text-fg-muted" style={rise(240)}>
              {item.rating ? (
                <span className="flex items-center gap-0.5" aria-label={`${item.rating.toFixed(1)} out of 10`}>
                  {[0, 1, 2, 3, 4].map((i) => (
                    <Star
                      key={i}
                      className={['size-3.5', i < stars ? 'fill-rating text-rating' : 'text-white/25'].join(' ')}
                      aria-hidden
                    />
                  ))}
                </span>
              ) : null}
              {item.runtimeMs ? <span>{formatRuntime(item.runtimeMs)}</span> : null}
              {item.contentRating ? (
                <span className="rounded-sm border border-white/40 px-1.5 py-px text-[11px]">{item.contentRating}</span>
              ) : null}
              {item.played ? <span className="rounded bg-white/10 px-2 py-0.5 text-[11px]">Watched</span> : null}
            </div>

            {item.overview ? (
              <p className="mt-2 line-clamp-2 max-w-xl text-[12.5px] leading-relaxed text-fg-muted" style={rise(300)}>
                {item.overview}
              </p>
            ) : null}

            <div className="mt-3.5 flex items-center gap-2" style={rise(360)}>
              {item.fileCount > 0 ? (
                <button
                  type="button"
                  onClick={() => setPlayer({ itemId: item.id, title: item.title })}
                  className="inline-flex items-center gap-2 rounded-lg bg-progress/90 px-3.5 py-1.5 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
                >
                  <Play className="size-3.5" aria-hidden /> Play
                </button>
              ) : null}
              <button
                type="button"
                onClick={() => void navigate({ to: '/items/$itemId', params: { itemId: String(item.id) } })}
                className="rounded-lg border border-hairline-strong bg-black/25 px-3.5 py-1.5 text-[12.5px] text-fg backdrop-blur hover:border-white/30 hover:bg-black/45"
              >
                More info
              </button>
            </div>
          </div>
        </div>

        {/* upcoming posters, clipped by the strip (VS-style) */}
        {upcoming.length > 0 ? (
          <div
            className="relative hidden h-full shrink-0 overflow-hidden md:block"
            style={{ width: 'clamp(140px, 24%, 340px)' }}
            aria-hidden
          >
            <div className="absolute inset-y-0 left-0 flex h-full">
              {upcoming.map((s2) => (
                <button
                  key={s2.id}
                  type="button"
                  onClick={() => {
                    setPrev(index);
                    setIndex(slides.findIndex((x) => x.id === s2.id));
                  }}
                  tabIndex={-1}
                  className="relative h-full shrink-0 opacity-70 transition-opacity duration-300 hover:opacity-100"
                  style={{ aspectRatio: '2 / 3' }}
                >
                  <PosterArt item={s2} />
                  <span className="absolute inset-0 bg-gradient-to-r from-black/50 to-transparent" />
                </button>
              ))}
            </div>
            <span className="pointer-events-none absolute inset-y-0 left-0 z-10 w-20 bg-gradient-to-r from-bg/80 to-transparent" />
          </div>
        ) : null}
      </div>

      {/* arrows + autoplay bars */}
      {slides.length > 1 ? (
        <>
          <button
            type="button"
            onClick={() => go(-1)}
            aria-label="Previous"
            className="absolute left-2 top-1/2 z-20 -translate-y-1/2 rounded-full bg-black/40 p-2 text-fg opacity-0 backdrop-blur transition-opacity hover:bg-black/60 focus-visible:opacity-100 group-hover:opacity-100"
          >
            <ChevronLeft className="size-5" aria-hidden />
          </button>
          <button
            type="button"
            onClick={() => go(1)}
            aria-label="Next"
            className="absolute right-2 top-1/2 z-20 -translate-y-1/2 rounded-full bg-black/40 p-2 text-fg opacity-0 backdrop-blur transition-opacity hover:bg-black/60 focus-visible:opacity-100 group-hover:opacity-100"
          >
            <ChevronRight className="size-5" aria-hidden />
          </button>
          <div className="absolute bottom-4 right-4 z-20 flex items-center gap-1.5 rounded-full bg-black/35 px-2 py-1.5 backdrop-blur">
            {slides.map((s, i) => (
              <button
                key={s.id}
                type="button"
                onClick={() => {
                  if (i === index) return;
                  setPrev(index);
                  setIndex(i);
                }}
                aria-label={`Slide ${i + 1}`}
                aria-current={i === index}
                className="h-[3px] w-6 overflow-hidden rounded-full bg-white/25"
              >
                {i === index ? (
                  <span
                    key={`bar-${index}-${paused || dragging}`}
                    className="block h-full origin-left rounded-full bg-progress"
                    style={
                      reduced
                        ? { transform: 'scaleX(1)' }
                        : {
                            animation: `hero-bar ${AUTOPLAY_MS}ms linear both`,
                            animationPlayState: paused || dragging ? 'paused' : 'running',
                          }
                    }
                  />
                ) : null}
              </button>
            ))}
          </div>
        </>
      ) : null}
    </section>
  );
}

function Backdrop({ item, zoom, reduced }: { item: Item; zoom: boolean; reduced: boolean }) {
  if (item.backdropUrl) {
    return (
      <div className="relative h-full w-full">
        {/* instant placeholder so the hero never looks empty while the backdrop loads */}
        {item.posterUrl ? (
          <img src={item.posterUrl} alt="" className="absolute inset-0 h-full w-full scale-110 object-cover blur-2xl brightness-[0.45]" />
        ) : (
          <div className="absolute inset-0" style={{ background: posterGradient(item.title) }} />
        )}
        <img
          src={item.backdropUrl}
          alt=""
          decoding="async"
          className="relative h-full w-full object-cover"
          style={zoom && !reduced ? { animation: 'hero-kenburns 9s ease-out both' } : undefined}
        />
      </div>
    );
  }
  if (item.posterUrl) {
    return (
      <img
        src={item.posterUrl}
        alt=""
        className="h-full w-full scale-110 object-cover blur-2xl brightness-[0.55]"
        style={zoom && !reduced ? { animation: 'hero-kenburns 9s ease-out both' } : undefined}
      />
    );
  }
  return <div className="h-full w-full" style={{ background: posterGradient(item.title) }} />;
}

function PosterArt({ item }: { item: Item }) {
  if (item.posterUrl) {
    return <img src={item.posterUrl} alt="" loading="lazy" className="h-full w-full object-cover" />;
  }
  return (
    <div className="h-full w-full" style={{ background: posterGradient(item.title) }}>
      <span className="flex h-full items-center justify-center p-2 text-center text-[11px] text-white/40">
        {item.title}
      </span>
    </div>
  );
}

function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() =>
    typeof window !== 'undefined' ? window.matchMedia('(prefers-reduced-motion: reduce)').matches : false,
  );
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = () => setReduced(mq.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);
  return reduced;
}

function dedupe(items: Item[]): Item[] {
  const seen = new Set<number>();
  const out: Item[] = [];
  for (const item of items) {
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    out.push(item);
  }
  // Prefer items that have a real backdrop for the hero.
  return out.sort((a, b) => Number(Boolean(b.backdropUrl)) - Number(Boolean(a.backdropUrl)));
}
