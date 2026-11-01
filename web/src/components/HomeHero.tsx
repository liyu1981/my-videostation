import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react';
import { Link } from '@tanstack/react-router';
import { useAtom } from 'jotai';
import { Play, Star } from 'lucide-react';
import type { Item } from '../lib/api';
import { certLabel, formatClock, posterGradient } from '../lib/format';
import { playerAtom } from '../atoms/ui';

const AUTOPLAY_MS = 8000;
const TRANSITION_MS = 650;
const EASE = 'cubic-bezier(0.45, 0, 0.25, 1)';

type Rect = { x: number; y: number; w: number; h: number };
type Promote = {
  item: Item; // clicked title: its block is already expanded, it only moves
  oldItem: Item; // previously expanded title, slides out left
  from: Rect; // expanded block at the clicked spine's position (relative to the hero)
  to: Rect; // expanded poster slot (relative to the hero)
  heroW: number; // hero width, so the moving backdrop aligns exactly at the end
  phase: 'start' | 'move';
};

/**
 * Video-Station-style hero: full-bleed backdrop, a full-height expanded poster on
 * the left, and a "bookshelf" of tightly packed full-height poster spines on the
 * right. The shelf is a ring (three copies) and glides one spine per change.
 *
 * Clicking a spine runs the two-phase VS motion: first the clicked spine expands
 * in place while the old poster collapses in place, then the expanded poster flies
 * left into the poster slot while the shelf glides and the ring rotates.
 */
export function HomeHero({ items }: { items: Item[] }) {
  const slides = useMemo(() => dedupe(items).slice(0, 10), [items]);
  const count = slides.length;
  const [index, setIndex] = useState(0);
  const [prev, setPrev] = useState<number | null>(null);
  const [paused, setPaused] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [shelfOffset, setShelfOffset] = useState(1);
  const [shelfAnimate, setShelfAnimate] = useState(true);
  const [promote, setPromote] = useState<Promote | null>(null);
  const heroRef = useRef<HTMLElement>(null);
  const posterRef = useRef<HTMLDivElement>(null);
  const dragX = useRef<number | null>(null);
  const dragged = useRef(false);
  const suppressClick = useRef(false);
  const snapTimer = useRef<number | null>(null);
  const promoteTimers = useRef<number[]>([]);
  const [, setPlayer] = useAtom(playerAtom);
  const reduced = usePrefersReducedMotion();

  useEffect(() => {
    if (index >= count) setIndex(0);
  }, [index, count]);

  // Autoplay restarts on every slide change (and when hover/drag/promote pauses).
  useEffect(() => {
    if (paused || dragging || reduced || count < 2 || promote) return;
    const id = window.setTimeout(() => goTo((index + 1) % count), AUTOPLAY_MS);
    return () => window.clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [index, paused, dragging, reduced, count, promote]);

  // Drop the outgoing backdrop layer once its fade-out finishes.
  useEffect(() => {
    if (prev === null) return;
    const id = window.setTimeout(() => setPrev(null), TRANSITION_MS + 50);
    return () => window.clearTimeout(id);
  }, [prev]);

  useEffect(() => () => {
    if (snapTimer.current) window.clearTimeout(snapTimer.current);
    for (const t of promoteTimers.current) window.clearTimeout(t);
  }, []);

  if (count === 0) return null;

  const item = slides[index]!;
  const previous = prev !== null ? slides[prev] : undefined;
  const sliver = count > 1 ? slides[(index - 1 + count) % count]! : undefined;
  const stars = item.rating ? Math.round(item.rating / 2) : 0;
  const cert = certLabel(item.contentRating);
  // Three copies keep the ring populated even at the wrap-around offset.
  const shelf = count > 1 ? [...slides, ...slides, ...slides] : slides;

  /** Change slide; the shelf glides one spine left (with a snap on wrap-around). */
  function goTo(next: number, force = false) {
    if (count < 2 || next === index) return;
    if (promote && !force) return;
    setPrev(index);

    // Forward wrap: glide one extra spine, then snap back a full cycle.
    if (next === 0 && index === count - 1) {
      if (reduced) {
        setShelfAnimate(false);
        setShelfOffset(1);
        setIndex(0);
        return;
      }
      setShelfAnimate(true);
      setShelfOffset(count + 1);
      setIndex(0);
      if (snapTimer.current) window.clearTimeout(snapTimer.current);
      snapTimer.current = window.setTimeout(() => {
        setShelfAnimate(false);
        setShelfOffset(1);
        requestAnimationFrame(() => requestAnimationFrame(() => setShelfAnimate(true)));
        snapTimer.current = null;
      }, TRANSITION_MS + 40);
      return;
    }

    // Backward wrap: jump to the far end without a long right-to-left sweep.
    if (next === count - 1 && index === 0) {
      setShelfAnimate(false);
      setShelfOffset(count);
      setIndex(count - 1);
      requestAnimationFrame(() => requestAnimationFrame(() => setShelfAnimate(true)));
      return;
    }

    setShelfAnimate(!reduced);
    setShelfOffset(next + 1);
    setIndex(next);
  }

  /** The clicked block is already expanded; we only move it to the poster slot. */
  function promoteSpine(target: Item, el: HTMLElement) {
    if (count < 2 || promote) return;
    const targetIndex = slides.findIndex((s) => s.id === target.id);
    if (targetIndex < 0 || targetIndex === index) return;
    const hero = heroRef.current;
    const slot = posterRef.current;
    if (!hero || !slot || reduced) {
      goTo(targetIndex);
      return;
    }
    const hr = hero.getBoundingClientRect();
    const fr = el.getBoundingClientRect();
    const sr = slot.getBoundingClientRect();
    // The clicked poster is already expanded (poster width), centred on its spine.
    const from: Rect = {
      x: fr.left - hr.left + fr.width / 2 - sr.width / 2,
      y: fr.top - hr.top,
      w: sr.width,
      h: sr.height,
    };
    const to: Rect = { x: sr.left - hr.left, y: sr.top - hr.top, w: sr.width, h: sr.height };

    setPromote({ item: target, oldItem: item, from, to, heroW: hr.width, phase: 'start' });
    setPrev(index);
    // Backdrop/title/shelf change now; the moving block lands on the final backdrop.
    goTo(targetIndex, true);

    const raf = requestAnimationFrame(() => {
      setPromote((p) => (p ? { ...p, phase: 'move' } : p));
    });
    const t = window.setTimeout(() => setPromote(null), TRANSITION_MS + 120);
    promoteTimers.current.push(raf, t);
  }

  const rise = (delay: number) =>
    reduced ? undefined : { animation: `hero-rise 620ms cubic-bezier(0.22, 1, 0.36, 1) ${delay}ms both` };

  return (
    <section
      ref={heroRef}
      className="hero-height hero-carousel group relative mx-6 mt-6 touch-pan-y overflow-hidden rounded-xl bg-black lg:mx-8"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onPointerDown={(e) => {
        if (e.pointerType === 'mouse' && e.button !== 0) return;
        dragX.current = e.clientX;
        dragged.current = false;
      }}
      onPointerMove={(e) => {
        if (dragX.current === null || dragged.current) return;
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
        if (wasDragged) {
          suppressClick.current = true;
          window.setTimeout(() => {
            suppressClick.current = false;
          }, 0);
          if (Math.abs(dx) > 60) goTo((index + (dx < 0 ? 1 : -1) + count) % count);
        }
      }}
      onPointerCancel={() => {
        dragX.current = null;
        dragged.current = false;
        setDragging(false);
      }}
      onClickCapture={(e) => {
        if (!suppressClick.current) return;
        suppressClick.current = false;
        e.preventDefault();
        e.stopPropagation();
      }}
      aria-roledescription="carousel"
      aria-label="Featured"
    >
      {/* backdrop block: fixed 16:9, to the right of the poster */}
      <div
        className="absolute top-0 bottom-0 z-0 overflow-hidden"
        style={{ left: 'calc(3rem + var(--poster-w))', aspectRatio: '16 / 9', height: '100%' }}
      >
        {previous ? (
          <div
            key={`out-${previous.id}`}
            className="absolute inset-0"
            style={reduced ? { opacity: 0 } : { animation: `hero-fade-out ${TRANSITION_MS}ms ease-out both` }}
            aria-hidden
          >
            <Backdrop item={previous} zoom={false} reduced={reduced} />
          </div>
        ) : null}
        <div
          key={`in-${item.id}`}
          className="absolute inset-0"
          style={reduced ? undefined : { animation: `hero-fade-in ${TRANSITION_MS}ms ease-out both` }}
        >
          <Backdrop item={item} zoom reduced={reduced} />
        </div>
        <div className="absolute inset-0 bg-black/15" />
        <div className="absolute inset-0 bg-gradient-to-r from-black/75 via-black/25 to-transparent" />
        <div className="absolute inset-x-0 bottom-0 h-3/5 bg-gradient-to-t from-black/90 via-black/25 to-transparent" />
      </div>

      {/* hint of the previous poster at the far left, like a ring */}
      {sliver?.posterUrl ? (
        <div className="pointer-events-none absolute inset-y-0 left-0 z-10 hidden w-12 overflow-hidden opacity-90 sm:block" aria-hidden>
          <img src={sliver.posterUrl} alt="" className="h-full w-full object-cover object-left" />
          <span className="absolute inset-0 bg-gradient-to-r from-black/35 via-transparent to-black/10" />
        </div>
      ) : null}

      {/* expanded poster: full hero height, no top/bottom gap */}
      <div
        ref={posterRef}
        className="group/poster absolute left-12 top-0 z-10 hidden h-full sm:block"
        style={{ width: 'var(--poster-w)', opacity: promote ? 0 : 1 }}
      >
        <div className="relative h-full overflow-hidden rounded-[5px] shadow-[0_18px_50px_rgba(0,0,0,0.55)] ring-1 ring-white/10">
          {previous ? (
            <div
              key={`poster-out-${previous.id}`}
              className="absolute inset-0"
              style={reduced ? { opacity: 0 } : { animation: `hero-fade-out 420ms ease-out both` }}
              aria-hidden
            >
              <PosterArt item={previous} />
            </div>
          ) : null}
          <div
            key={`poster-in-${item.id}`}
            className="absolute inset-0"
            style={reduced ? undefined : { animation: `hero-fade-in 420ms ease-out both` }}
          >
            <PosterArt item={item} />
          </div>
          <Link
            to="/items/$itemId"
            params={{ itemId: String(item.id) }}
            className="absolute inset-0"
            aria-label={item.title}
          />
          <span className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/25 via-transparent to-transparent" />
          {item.fileCount > 0 ? (
            <button
              type="button"
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                setPlayer({ itemId: item.id, title: item.title });
              }}
              className="absolute bottom-3 left-3 z-10 inline-flex items-center gap-1.5 rounded-md bg-black/65 px-2.5 py-1.5 text-[11.5px] font-medium text-fg opacity-0 backdrop-blur transition-opacity hover:bg-black/85 focus-visible:opacity-100 group-hover/poster:opacity-100"
            >
              <Play className="size-3 fill-current" aria-hidden /> Play
            </button>
          ) : null}
        </div>
      </div>

      {/* title / rating, bottom-aligned in the backdrop area */}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 z-[5] flex">
        <div className="hidden shrink-0 sm:block" style={{ width: 'calc(3rem + var(--poster-w) + 1.5rem)' }} />
        <Link
          to="/items/$itemId"
          params={{ itemId: String(item.id) }}
          className="pointer-events-auto min-w-0 flex-1 pb-7 pl-4 pr-4 sm:pl-0 sm:pr-6"
          aria-label={`${item.title} details`}
        >
          <h1
            key={`title-${item.id}`}
            className="line-clamp-2 text-[24px] font-semibold leading-tight tracking-tight text-fg-heading drop-shadow-[0_2px_12px_rgba(0,0,0,0.65)] sm:text-[27px] xl:text-[30px]"
            style={rise(60)}
          >
            {item.title}
            {item.year ? <span className="ml-2 font-normal text-fg-muted">({item.year})</span> : null}
          </h1>
          <div className="mt-2 flex items-end justify-between gap-4" style={rise(140)}>
            <span className="flex items-center gap-0.5" aria-label={item.rating ? `${item.rating.toFixed(1)} out of 10` : 'Not rated'}>
              {[0, 1, 2, 3, 4].map((i) => (
                <Star
                  key={i}
                  className={['size-4', i < stars ? 'fill-rating text-rating' : 'text-white/30'].join(' ')}
                  aria-hidden
                />
              ))}
            </span>
            <span className="flex shrink-0 items-center gap-3 text-[13px] text-fg-muted">
              {item.runtimeMs ? <span className="tabular-nums">{formatClock(item.runtimeMs)}</span> : null}
              {cert ? (
                <span className="border border-white/60 px-2 py-0.5 text-[11px] leading-none tracking-wide text-fg">{cert}</span>
              ) : null}
            </span>
          </div>
        </Link>
        <div
          className="hidden shrink-0 lg:block"
          style={{
            width: 'max(calc(100% - 3rem - var(--poster-w) - var(--bg-w)), calc(2 * var(--spine-w)))',
          }}
        />
      </div>

      {/* bookshelf of upcoming posters (ring) */}
      {count > 1 ? (
        <div
          className="hero-shelf absolute inset-y-0 right-0 z-10 hidden overflow-hidden lg:block"
          style={{ left: 'min(calc(3rem + var(--poster-w) + var(--bg-w)), calc(100% - 2 * var(--spine-w)))' }}
          aria-label="More featured titles"
        >
          <div
            className="absolute inset-y-0 left-0 flex h-full will-change-transform"
            style={{
              transform: `translateX(calc(var(--spine-w) * -${shelfOffset}))`,
              transition: shelfAnimate && !reduced ? `transform ${TRANSITION_MS}ms ${EASE}` : 'none',
            }}
          >
            {shelf.map((s, i) => (
              <button
                key={`${s.id}-${i}`}
                type="button"
                onClick={(e) => promoteSpine(s, e.currentTarget)}
                title={s.title}
                aria-label={`Featured: ${s.title}`}
                className="group/spine relative h-full shrink-0 overflow-hidden border-r border-black/60"
                style={{ width: 'var(--spine-w)' }}
              >
                <PosterArt item={s} />
                <span className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/80 via-black/35 to-black/10 transition-opacity duration-300 group-hover/spine:opacity-0" />
              </button>
            ))}
          </div>
          <span className="pointer-events-none absolute inset-y-0 left-0 w-6 bg-gradient-to-r from-black/60 to-transparent" />
        </div>
      ) : null}

      {/* two-phase promotion clones */}
      {promote ? (
        <>
          {/* old block: below the shelf/sliver so the sliver (previous item) stays correct */}
          <div className="pointer-events-none absolute inset-0 z-[4]" aria-hidden>
            <div className="absolute overflow-hidden" style={bgCloneStyle(promote, 'old')}>
              <Backdrop item={promote.oldItem} zoom={false} reduced={reduced} />
              <div className="absolute inset-0 bg-black/15" />
              <div className="absolute inset-0 bg-gradient-to-r from-black/75 via-black/25 to-transparent" />
              <div className="absolute inset-x-0 bottom-0 h-3/5 bg-gradient-to-t from-black/90 via-black/25 to-transparent" />
            </div>
          </div>
          <div className="pointer-events-none absolute inset-0 z-[5]" aria-hidden>
            <CloneArt item={promote.oldItem} style={posterCloneStyle(promote, 'old')} />
          </div>

          {/* new block: bg under the shelf, poster above everything */}
          <div className="pointer-events-none absolute inset-0 z-[6]" aria-hidden>
            <div className="absolute overflow-hidden" style={bgCloneStyle(promote, 'new')}>
              <Backdrop item={promote.item} zoom reduced={reduced} />
              <div className="absolute inset-0 bg-black/15" />
              <div className="absolute inset-0 bg-gradient-to-r from-black/75 via-black/25 to-transparent" />
              <div className="absolute inset-x-0 bottom-0 h-3/5 bg-gradient-to-t from-black/90 via-black/25 to-transparent" />
            </div>
          </div>
          <div className="pointer-events-none absolute inset-0 z-30" aria-hidden>
            <CloneArt item={promote.item} style={posterCloneStyle(promote, 'new')} />
          </div>
        </>
      ) : null}

      {/* autoplay progress bars */}
      {count > 1 ? (
        <div className="absolute bottom-4 right-4 z-20 flex items-center gap-1.5 rounded-full bg-black/35 px-2 py-1.5 backdrop-blur">
          {slides.map((s, i) => (
            <button
              key={s.id}
              type="button"
              onClick={() => goTo(i)}
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
      ) : null}
    </section>
  );
}

function posterLeft(p: Promote, which: 'old' | 'new'): number {
  const distance = p.from.x - p.to.x;
  if (which === 'old') return p.phase === 'move' ? p.to.x - distance : p.to.x;
  return p.phase === 'move' ? p.to.x : p.from.x;
}

function posterCloneStyle(p: Promote, which: 'old' | 'new'): CSSProperties {
  return {
    left: posterLeft(p, which),
    top: p.to.y,
    width: p.to.w,
    height: p.to.h,
    opacity: which === 'old' && p.phase === 'move' ? 0 : 1,
    transition: p.phase === 'move' ? `left ${TRANSITION_MS}ms ${EASE}, opacity ${TRANSITION_MS}ms linear` : 'none',
  };
}

function bgCloneStyle(p: Promote, which: 'old' | 'new'): CSSProperties {
  return {
    left: posterLeft(p, which) + p.to.w,
    top: p.to.y,
    height: p.to.h,
    aspectRatio: '16 / 9',
    transition: p.phase === 'move' ? `left ${TRANSITION_MS}ms ${EASE}` : 'none',
  };
}

function CloneArt({ item, style }: { item: Item; style: CSSProperties }) {
  const className =
    'absolute rounded-[5px] object-cover shadow-[0_18px_50px_rgba(0,0,0,0.55)] ring-1 ring-white/10';
  if (item.posterUrl) return <img src={item.posterUrl} alt="" className={className} style={style} />;
  return <div className={className} style={{ ...style, background: posterGradient(item.title) }} />;
}

function Backdrop({ item, zoom, reduced }: { item: Item; zoom: boolean; reduced: boolean }) {
  if (item.backdropUrl) {
    return (
      <div className="relative h-full w-full">
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
