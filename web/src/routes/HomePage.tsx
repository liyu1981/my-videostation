import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { Clapperboard } from 'lucide-react';
import { api } from '../lib/api';
import { EmptyState, ErrorState, Loading, SectionHeader } from '../components/State';
import { ItemViews } from '../components/ItemViews';

export function HomePage() {
  const home = useQuery({ queryKey: ['home'], queryFn: api.home });

  if (home.isLoading) return <Loading />;
  if (home.isError) return <ErrorState error={home.error} />;

  const data = home.data!;
  const hasContent = data.continue.length + data.recent.length + data.shelves.length > 0;

  return (
    <div className="space-y-9 px-6 py-7 lg:px-8">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight text-fg-heading">Your library</h1>
          <p className="mt-1 text-[12.5px] text-fg-dim">
            {data.recent.length} recently added · {data.shelves.length} libraries
          </p>
        </div>
        <Link
          to="/libraries"
          className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted transition-colors hover:border-hairline-strong hover:text-fg"
        >
          Manage libraries
        </Link>
      </div>

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

      {data.continue.length > 0 ? (
        <section>
          <SectionHeader title="Continue watching" />
          <ItemViews items={data.continue} view="poster" bare />
        </section>
      ) : null}

      {data.recent.length > 0 ? (
        <section>
          <SectionHeader title="Recently added" />
          <ItemViews items={data.recent} view="poster" />
        </section>
      ) : null}

      {data.shelves.map((shelf) => (
        <section key={shelf.title}>
          <SectionHeader
            title={shelf.title}
            aside={
              <Link
                to="/browse"
                search={{
                  kind: shelf.kind === 'show' ? 'show' : shelf.kind === 'home_video' ? 'home_video' : 'movie',
                  libraryId: undefined,
                  sort: 'added',
                  page: 1,
                  q: '',
                }}
                className="text-[11.5px] text-fg-dim hover:text-fg"
              >
                View all
              </Link>
            }
          />
          <ItemViews items={shelf.items} view="poster" bare />
        </section>
      ))}
    </div>
  );
}
