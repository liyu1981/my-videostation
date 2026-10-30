import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from '@tanstack/react-router';
import { Trash2, X } from 'lucide-react';
import { api } from '../lib/api';
import { EmptyState, ErrorState, Loading } from '../components/State';
import { Pagination } from '../components/ItemViews';
import { PosterCard } from '../components/Poster';

export function CollectionPage() {
  const params = useParams({ strict: false }) as { collectionId?: string };
  const id = Number(params.collectionId ?? 0);
  const qc = useQueryClient();
  const [page, setPage] = useState(1);

  const collections = useQuery({ queryKey: ['collections'], queryFn: api.collections });
  const items = useQuery({
    queryKey: ['collection', id, page],
    queryFn: () => api.collectionItems(id, page),
    enabled: id > 0,
  });

  const remove = useMutation({
    mutationFn: (itemId: number) => api.removeFromCollection(id, itemId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['collection', id] });
      void qc.invalidateQueries({ queryKey: ['collections'] });
    },
  });

  const collection = collections.data?.collections.find((c) => c.id === id);
  const isSystem = Boolean(collection?.systemKey);

  const rename = useMutation({
    mutationFn: (name: string) => api.renameCollection(id, name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['collections'] }),
  });
  const destroy = useMutation({
    mutationFn: () => api.deleteCollection(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['collections'] });
      window.history.back();
    },
  });

  if (collections.isLoading || items.isLoading) return <Loading />;
  if (items.isError) return <ErrorState error={items.error} />;
  if (!collection) return <EmptyState title="Collection not found" />;

  return (
    <div className="px-6 py-6 lg:px-8">
      <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">{collection.name}</h1>
          <p className="mt-1 text-[12px] text-fg-dim">
            {items.data?.total ?? 0} items{isSystem ? ' · built-in collection' : ''}
          </p>
        </div>
        {!isSystem ? (
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => {
                const name = prompt('Rename collection', collection.name);
                if (name && name.trim() && name.trim() !== collection.name) rename.mutate(name.trim());
              }}
              className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:border-hairline-strong hover:text-fg"
            >
              Rename
            </button>
            <button
              type="button"
              onClick={() => {
                if (confirm(`Delete collection “${collection.name}”? Items are kept.`)) destroy.mutate();
              }}
              className="rounded-lg border border-hairline p-1.5 text-fg-dim hover:border-active/50 hover:text-active"
              aria-label="Delete collection"
            >
              <Trash2 className="size-3.5" aria-hidden />
            </button>
          </div>
        ) : null}
      </header>

      {items.data && items.data.items.length === 0 ? (
        <EmptyState title="This collection is empty" description="Add items from a detail page or with selection mode." />
      ) : null}

      {items.data && items.data.items.length > 0 ? (
        <>
          <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
            {items.data.items.map((item) => (
              <div key={item.id} className="group relative">
                <PosterCard item={item} />
                <button
                  type="button"
                  onClick={() => remove.mutate(item.id)}
                  aria-label={`Remove ${item.title} from collection`}
                  className="absolute right-1 top-1 z-10 grid size-6 place-items-center rounded-full bg-black/70 text-fg opacity-0 ring-1 ring-white/20 transition-opacity hover:bg-active group-hover:opacity-100"
                >
                  <X className="size-3.5" aria-hidden />
                </button>
              </div>
            ))}
          </div>
          <div className="mt-6">
            <Pagination page={page} totalPages={Math.ceil((items.data.total || 0) / 60)} total={items.data.total} onChange={setPage} />
          </div>
        </>
      ) : null}
    </div>
  );
}
