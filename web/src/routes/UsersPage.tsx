import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Shield, Trash2, UserPlus } from 'lucide-react';
import { api, type User, type UserRole } from '../lib/api';
import { useCurrentUser } from '../lib/auth';
import { Dialog, DialogButton } from '../components/Dialog';
import { ErrorState, Loading } from '../components/State';

const inputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';

export function UsersPage() {
  const qc = useQueryClient();
  const { user: current } = useCurrentUser();
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
          <h1 className="text-[20px] font-semibold tracking-tight text-fg-heading">Users</h1>
          <p className="mt-1 text-[12px] text-fg-dim">
            Admins manage libraries, metadata and settings. Users can browse, play and keep their own watch state.
          </p>
        </div>
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="inline-flex items-center gap-2 rounded-lg bg-progress/90 px-3.5 py-2 text-[12.5px] font-medium text-[#062027] hover:bg-progress"
        >
          <UserPlus className="size-4" aria-hidden /> Add user
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

  const save = useMutation({
    mutationFn: () => {
      if (isEdit && user) {
        return api.updateUser(user.id, {
          displayName: displayName.trim(),
          role,
          active,
          ...(password ? { password } : {}),
        });
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
            <DialogButton variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>
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
