import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { AlertTriangle, Clapperboard, Play, Tv, Video } from 'lucide-react';
import { api, type Item } from '../lib/api';
import { kindLabel } from '../lib/format';
import { EmptyState, ErrorState, Loading } from '../components/State';
import { HomeHero } from '../components/HomeHero';
import { MediaRow } from '../components/MediaRow';

const ROW_ORDER = ['movie', 'show', 'home_video'] as const;
const ROW_ICON = {
  movie: <Clapperboard className="size-[18px]" aria-hidden />,
  show: <Tv className="size-[18px]" aria-hidden />,
  home_video: <Video className="size-[18px]" aria-hidden />,
};

export function HomePage() {
  const home = useQuery({ queryKey: ['home'], queryFn: api.home });
  const unmatched = useQuery({
    queryKey: ['browse', 'unmatched', 'home'],
    queryFn: () => api.browse({ metadataStatus: 'unmatched', perPage: 12 }),
  });

  const data = home.data;

  const rows = useMemo(() => {
    if (!data) return [];
    const groups: Record<string, Item[]> = { movie: [], show: [], home_video: [] };
    const seen = new Set<number>();
    for (const shelf of data.shelves) {
      const bucket = groups[shelf.kind];
      if (!bucket) continue;
      for (const item of shelf.items) {
        if (seen.has(item.id)) continue;
        seen.add(item.id);
        bucket.push(item);
      }
    }
    return ROW_ORDER.filter((kind) => (groups[kind]?.length ?? 0) > 0).map((kind) => ({
      kind,
      title: kindLabel(kind),
      items: groups[kind]!,
    }));
  }, [data]);

  if (home.isLoading) return <Loading />;
  if (home.isError) return <ErrorState error={home.error} />;

  const heroItems = [...(data?.recent ?? []), ...(data?.shelves ?? []).flatMap((s) => s.items)];
  const hasContent = heroItems.length > 0 || (data?.continue.length ?? 0) > 0;

  return (
    <div className="pb-12">
      {hasContent ? <HomeHero items={heroItems} /> : null}

      <div className="space-y-7 px-6 pt-6 lg:px-8">
        {!hasContent ? (
          <EmptyState
            icon={<Clapperboard className="size-8" />}
            title="No media yet"
            description="Add a library folder, then scan it. MyVideoStation reads your files and never modifies them."
            action={
              <Link
                to="/libraries"
                className="mt-2 rounded-lg bg-progress/90 px-3.5 py-2 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
              >
                Add a library
              </Link>
            }
          />
        ) : null}

        {(data?.continue.length ?? 0) > 0 ? (
          <MediaRow title="Continue watching" icon={<Play className="size-[18px]" aria-hidden />} items={data!.continue} />
        ) : null}

        {rows.map((row) => (
          <MediaRow
            key={row.kind}
            title={row.title}
            icon={ROW_ICON[row.kind]}
            items={row.items}
            viewAll={{ kind: row.kind }}
          />
        ))}

        {unmatched.data && unmatched.data.items.length > 0 ? (
          <MediaRow
            title={`Needs attention (${unmatched.data.total})`}
            showCount={false}
            icon={<AlertTriangle className="size-[18px]" aria-hidden />}
            items={unmatched.data.items}
            viewAll={{ kind: 'movie', status: 'unmatched' }}
          />
        ) : null}
      </div>
    </div>
  );
}
