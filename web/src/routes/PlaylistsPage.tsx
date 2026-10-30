import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { Bookmark, Heart, ListVideo, Plus } from 'lucide-react';
import { api, type Collection } from '../lib/api';
import { Loading } from '../components/State';

export function PlaylistsPage() {
  const qc = useQueryClient();
  const collections = useQuery({ queryKey: ['collections'], queryFn: api.collections });
  const [name, setName] = useState('');
  const [creating, setCreating] = useState(false);

  const create = useMutation({
    mutationFn: () => api.createCollection(name.trim()),
    onSuccess: () => {
      setName('');
      setCreating(false);
      void qc.invalidateQueries({ queryKey: ['collections'] });
    },
  });

  if (collections.isLoading) return <Loading />;

  const all = collections.data?.collections ?? [];
  const system = all.filter((c) => c.systemKey);
  const user = all.filter((c) => !c.systemKey);

  return (
    <div className="px-6 py-6 lg:px-8">
      <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Playlists</h1>
          <p className="mt-1 text-[12px] text-fg-dim">Favorites and Watchlist are built in; add your own collections below.</p>
        </div>
        <button
          type="button"
          onClick={() => setCreating((v) => !v)}
          className="inline-flex items-center gap-1.5 rounded-lg bg-progress/90 px-3.5 py-2 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
        >
          <Plus className="size-4" aria-hidden /> New collection
        </button>
      </header>

      {creating ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
          className="mb-5 flex flex-wrap items-center gap-2 rounded-[var(--radius-panel)] border border-hairline bg-surface p-3"
        >
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Collection name"
            autoFocus
            className="h-9 min-w-0 flex-1 rounded-lg border border-hairline bg-surface-2 px-3 text-[13px] outline-none placeholder:text-fg-dim focus:border-hairline-strong"
          />
          <button type="submit" disabled={!name.trim() || create.isPending} className="rounded-lg bg-surface-3 px-3 py-1.5 text-[12px] font-medium disabled:opacity-50">
            Create
          </button>
          <button type="button" onClick={() => setCreating(false)} className="rounded-lg border border-hairline px-3 py-1.5 text-[12px] text-fg-muted hover:text-fg">
            Cancel
          </button>
          {create.isError ? <p className="w-full text-[12px] text-active">{(create.error as Error).message}</p> : null}
        </form>
      ) : null}

      <section className="mb-8">
        <h2 className="mb-3 text-[13px] font-medium uppercase tracking-wide text-fg-dim">Built in</h2>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4">
          {system.map((c) => (
            <CollectionCard key={c.id} collection={c} />
          ))}
        </div>
      </section>

      <section>
        <h2 className="mb-3 text-[13px] font-medium uppercase tracking-wide text-fg-dim">My collections</h2>
        {user.length === 0 ? (
          <p className="text-[12.5px] text-fg-dim">No collections yet — create one, then add items from any detail page.</p>
        ) : (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4">
            {user.map((c) => (
              <CollectionCard key={c.id} collection={c} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function CollectionCard({ collection }: { collection: Collection }) {
  const Icon = collection.systemKey === 'favorites' ? Heart : collection.systemKey === 'watchlist' ? Bookmark : ListVideo;
  return (
    <Link
      to="/collections/$collectionId"
      params={{ collectionId: String(collection.id) }}
      className="group flex flex-col gap-3 rounded-[var(--radius-panel)] border border-hairline bg-surface p-4 transition-colors hover:border-hairline-strong hover:bg-surface-2"
    >
      <span className="grid size-10 place-items-center rounded-lg bg-surface-3">
        <Icon className={['size-5', collection.systemKey === 'favorites' ? 'text-active' : 'text-fg-muted'].join(' ')} aria-hidden />
      </span>
      <div>
        <p className="truncate text-[13.5px] font-medium text-fg">{collection.name}</p>
        <p className="text-[11.5px] text-fg-dim">{collection.itemCount} items</p>
      </div>
    </Link>
  );
}
