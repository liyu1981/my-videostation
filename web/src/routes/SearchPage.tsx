import { useQuery } from '@tanstack/react-query';
import { useSearch } from '@tanstack/react-router';
import { SearchX } from 'lucide-react';
import { api } from '../lib/api';
import { EmptyState, ErrorState, Loading } from '../components/State';
import { ItemViews } from '../components/ItemViews';

export function SearchPage() {
  const { q } = useSearch({ strict: false }) as { q?: string };
  const query = (q ?? '').trim();

  const results = useQuery({
    queryKey: ['search', query],
    queryFn: () => api.search(query),
    enabled: query.length > 0,
  });

  return (
    <div className="px-6 py-6 lg:px-8">
      <header className="mb-5">
        <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Search</h1>
        <p className="mt-1 text-[12px] text-fg-dim">
          {query ? (
            <>
              {results.data ? `${results.data.items.length} results for ` : 'Searching for '}
              <span className="text-fg-muted">“{query}”</span>
            </>
          ) : (
            'Type in the search box above to search titles, overviews and people.'
          )}
        </p>
      </header>

      {!query ? null : results.isLoading ? (
        <Loading label="Searching…" />
      ) : results.isError ? (
        <ErrorState error={results.error} />
      ) : results.data && results.data.items.length === 0 ? (
        <EmptyState
          icon={<SearchX className="size-7" />}
          title={`No matches for “${query}”`}
          description="Try a shorter query, or check that the library has been scanned."
        />
      ) : results.data ? (
        <ItemViews items={results.data.items} view="poster" />
      ) : null}
    </div>
  );
}
