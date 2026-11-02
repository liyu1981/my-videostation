import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Shield, Trash2, UserPlus } from 'lucide-react';
import { api, type User, type UserRole } from '../lib/api';
import { useCurrentUser } from '../lib/auth';
import { useI18n } from '../lib/i18n';
import { Dialog, DialogButton } from '../components/Dialog';
import { ErrorState, Loading } from '../components/State';

const inputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';

export function UsersPage() {
  const qc = useQueryClient();
  const { user: current } = useCurrentUser();
  const { t } = useI18n();
  const users = useQuery({ queryKey: ['users'], queryFn: api.users });
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteUser(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['users'] }),
  });

  if (users.isLoading) return <Loading />;
  if (users.isError) return <ErrorState error={users.error} />;

  const list = users.data!.users;
  return (
    <div className="mx-auto max-w-5xl px-6 py-7 lg:px-8">
      <header className="mb-6 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">{t('users.title')}</h1>
          <p className="mt-1 text-[12px] text-fg-dim">{t('users.subtitle')}</p>
        </div>
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="inline-flex items-center gap-2 rounded-lg bg-progress/90 px-3.5 py-2 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
        >
          <UserPlus className="size-4" aria-hidden /> {t('users.add')}
        </button>
      </header>

      <div className="overflow-hidden rounded-[var(--radius-panel)] border border-hairline bg-surface">
        {list.map((user) => {
          const self = user.id === current?.id;
          return (
            <div key={user.id} className="flex flex-wrap items-center gap-3 border-b border-hairline px-4 py-3 last:border-b-0">
              <span className="grid size-9 place-items-center rounded-full bg-surface-3 text-[12px] font-medium text-fg">
                {(user.displayName || user.username).slice(0, 1).toUpperCase()}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="truncate text-[13.5px] font-medium text-fg">{user.displayName || user.username}</p>
                  {user.role === 'admin' ? (
                    <span className="inline-flex items-center gap-1 rounded-full border border-active/40 bg-active/10 px-2 py-0.5 text-[10.5px] text-active">
                      <Shield className="size-3" aria-hidden /> Admin
                    </span>
                  ) : (
                    <span className="rounded-full border border-hairline px-2 py-0.5 text-[10.5px] text-fg-muted">User</span>
                  )}
                  {!user.active ? (
                    <span className="rounded-full border border-hairline px-2 py-0.5 text-[10.5px] text-fg-dim">Disabled</span>
                  ) : null}
                  {user.ratingCeiling ? (
                    <span className="rounded-full border border-hairline px-2 py-0.5 text-[10.5px] text-fg-dim">{user.ratingCeiling}</span>
                  ) : null}
                  {self ? <span className="text-[10.5px] text-fg-dim">you</span> : null}
                </div>
                <p className="mt-0.5 truncate font-mono text-[11px] text-fg-dim">@{user.username}</p>
              </div>
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => setEditing(user)}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-hairline px-2.5 py-1.5 text-[11.5px] text-fg-muted hover:border-hairline-strong hover:text-fg"
                >
                  <Pencil className="size-3.5" aria-hidden /> Edit
                </button>
                <button
                  type="button"
                  disabled={self || remove.isPending}
                  onClick={() => {
                    if (confirm(`Delete account “${user.username}”? Their watch state and collections are removed.`)) {
                      remove.mutate(user.id);
                    }
                  }}
                  className="rounded-lg border border-hairline p-1.5 text-fg-dim hover:border-active/50 hover:text-active disabled:cursor-not-allowed disabled:opacity-30"
                  aria-label={`Delete ${user.username}`}
                >
                  <Trash2 className="size-3.5" aria-hidden />
                </button>
              </div>
            </div>
          );
        })}
      </div>
      {remove.isError ? <p className="mt-2 text-[12px] text-active">{(remove.error as Error).message}</p> : null}

      {creating ? (
        <UserDialog
          currentUserId={current?.id}
          onClose={() => setCreating(false)}
          onSaved={() => {
            setCreating(false);
            void qc.invalidateQueries({ queryKey: ['users'] });
          }}
        />
      ) : null}
      {editing ? (
        <UserDialog
          user={editing}
          currentUserId={current?.id}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['users'] });
            void qc.invalidateQueries({ queryKey: ['auth', 'me'] });
          }}
        />
      ) : null}
    </div>
  );
}

function UserDialog({
  user,
  currentUserId,
  onClose,
  onSaved,
}: {
  user?: User;
  currentUserId?: number;
  onClose: () => void;
  onSaved: () => void;
}) {
  const isEdit = Boolean(user);
  const isSelf = user?.id === currentUserId;
  const [username, setUsername] = useState(user?.username ?? '');
  const [displayName, setDisplayName] = useState(user?.displayName ?? '');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<UserRole>(user?.role ?? 'user');
  const [active, setActive] = useState(user?.active ?? true);
  const [rating, setRating] = useState(user?.ratingCeiling ?? '');
  const [libraryIDs, setLibraryIDs] = useState<number[] | null>(null);
  const [permissionsDirty, setPermissionsDirty] = useState(false);
  const access = useQuery({
    queryKey: ['user-libraries', user?.id],
    queryFn: () => api.userLibraries(user!.id),
    enabled: isEdit,
  });

  useEffect(() => {
    if (!access.data) return;
    setLibraryIDs(access.data.libraries.filter((l) => l.allowed).map((l) => l.id));
  }, [access.data]);

  const unrestrict = useMutation({
    mutationFn: () => api.setUserLibraries(user!.id, [], true),
    onSuccess: onSaved,
  });

  const save = useMutation({
    mutationFn: async () => {
      if (isEdit && user) {
        await api.updateUser(user.id, {
          displayName: displayName.trim(),
          role,
          active,
          ratingCeiling: rating,
          ...(password ? { password } : {}),
        });
        if (libraryIDs !== null && permissionsDirty) await api.setUserLibraries(user.id, libraryIDs);
        return;
      }
      return api.createUser({ username: username.trim(), displayName: displayName.trim(), password, role });
    },
    onSuccess: onSaved,
  });

  return (
    <Dialog
      title={isEdit ? 'Edit account' : 'New account'}
      size="max-w-lg"
      onClose={onClose}
      footer={
        <>
          <span className="text-[10.5px] text-fg-dim">{isSelf ? 'This is your account.' : ''}</span>
          <span className="flex gap-2">
            <DialogButton onClick={onClose}>Cancel</DialogButton>
            <DialogButton
              variant="primary"
              onClick={() => save.mutate()}
              disabled={save.isPending || (isEdit && !isSelf && access.isLoading)}
            >
              {save.isPending ? 'Saving…' : isEdit ? 'Save' : 'Create'}
            </DialogButton>
          </span>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
        className="space-y-3 p-5"
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Username</span>
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              disabled={isEdit}
              required
              minLength={3}
              maxLength={32}
              className={`${inputClass} disabled:opacity-60`}
            />
          </label>
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Display name</span>
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className={inputClass} />
          </label>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Role</span>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value as UserRole)}
              disabled={isSelf}
              className={`${inputClass} disabled:opacity-60`}
            >
              <option value="user">User</option>
              <option value="admin">Admin</option>
            </select>
          </label>
          <label className="flex items-center gap-2 self-end pb-2 text-[12px] text-fg-muted">
            <input
              type="checkbox"
              checked={active}
              onChange={(e) => setActive(e.target.checked)}
              disabled={isSelf}
              className="size-4 rounded border-hairline bg-surface-2"
            />
            Account enabled
          </label>
        </div>
        {isEdit ? (
          <label className="block">
            <span className="mb-1 block text-[12px] text-fg-muted">Rating ceiling</span>
            <select value={rating} onChange={(e) => setRating(e.target.value)} className={inputClass}>
              <option value="">No limit (all ratings)</option>
              <option value="G">G / TV-Y / TV-G</option>
              <option value="PG">PG / TV-PG</option>
              <option value="PG-13">PG-13 / TV-14</option>
              <option value="R">R / TV-MA</option>
              <option value="NC-17">NC-17</option>
            </select>
            <span className="mt-1 block text-[10.5px] text-fg-dim">
              Unrated titles remain visible; unknown ratings are hidden when a ceiling is set. Admins bypass this.
            </span>
          </label>
        ) : null}
        {isEdit && !isSelf ? (
          <div>
            <span className="mb-1 block text-[12px] text-fg-muted">Library access</span>
            {access.isLoading ? (
              <p className="text-[11px] text-fg-dim">Loading libraries…</p>
            ) : access.isError ? (
              <p className="text-[11px] text-active">{(access.error as Error).message}</p>
            ) : access.data && access.data.libraries.length > 0 ? (
              <div className="grid gap-1.5 sm:grid-cols-2">
                {access.data.libraries.map((lib) => (
                  <label key={lib.id} className="flex items-center gap-2 text-[12px] text-fg-muted">
                    <input
                      type="checkbox"
                      checked={libraryIDs?.includes(lib.id) ?? true}
                      onChange={(e) =>
                        {
                          setPermissionsDirty(true);
                          setLibraryIDs((current) => {
                            const base = current ?? access.data!.libraries.filter((l) => l.allowed).map((l) => l.id);
                            return e.target.checked ? [...new Set([...base, lib.id])] : base.filter((id) => id !== lib.id);
                          });
                        }
                      }
                      className="size-4 rounded border-hairline bg-surface-2"
                    />
                    <span className="truncate" title={lib.name}>
                      {lib.name}
                    </span>
                  </label>
                ))}
              </div>
            ) : (
              <p className="text-[11px] text-fg-dim">No libraries yet.</p>
            )}
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <p className="text-[10.5px] text-fg-dim">
                Saving an explicit list denies libraries added later; an unrestricted user sees all libraries.
              </p>
              <button
                type="button"
                onClick={() => unrestrict.mutate()}
                disabled={unrestrict.isPending}
                className="text-[10.5px] text-progress hover:underline disabled:opacity-50"
              >
                Allow all libraries
              </button>
              {unrestrict.isError ? <span className="text-[10.5px] text-active">{(unrestrict.error as Error).message}</span> : null}
            </div>
          </div>
        ) : null}
        <label className="block">
          <span className="mb-1 block text-[12px] text-fg-muted">
            {isEdit ? 'New password (leave blank to keep current)' : 'Password'}
          </span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required={!isEdit}
            minLength={8}
            autoComplete="new-password"
            className={inputClass}
          />
          <span className="mt-1 block text-[10.5px] text-fg-dim">At least 8 characters.</span>
        </label>
        {save.isError ? <p className="text-[12px] text-active">{(save.error as Error).message}</p> : null}
      </form>
    </Dialog>
  );
}
