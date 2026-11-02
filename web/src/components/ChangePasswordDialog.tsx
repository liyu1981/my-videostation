import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { api } from '../lib/api';
import { Dialog, DialogButton } from './Dialog';

const inputClass =
  'w-full rounded-lg border border-hairline bg-surface-2 px-3 py-2 text-[13px] text-fg outline-none placeholder:text-fg-dim focus:border-hairline-strong focus:bg-surface-3';

export function ChangePasswordDialog({ onClose }: { onClose: () => void }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [localError, setLocalError] = useState('');
  const change = useMutation({
    mutationFn: () => api.changePassword(current, next),
    onSuccess: onClose,
  });

  return (
    <Dialog
      title="Change password"
      size="max-w-md"
      onClose={onClose}
      footer={
        <>
          <span className="text-[10.5px] text-fg-dim">Other sessions will be signed out.</span>
          <span className="flex gap-2">
            <DialogButton onClick={onClose}>Cancel</DialogButton>
            <DialogButton variant="primary" onClick={() => change.mutate()} disabled={change.isPending}>
              {change.isPending ? 'Saving…' : 'Change password'}
            </DialogButton>
          </span>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setLocalError('');
          if (next !== confirm) {
            setLocalError('Passwords do not match');
            return;
          }
          if (next.length < 8) {
            setLocalError('Password must be at least 8 characters');
            return;
          }
          change.mutate();
        }}
        className="space-y-3 p-5"
      >
        <label className="block">
          <span className="mb-1 block text-[12px] text-fg-muted">Current password</span>
          <input
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            required
            className={inputClass}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-[12px] text-fg-muted">New password</span>
          <input
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            minLength={8}
            required
            className={inputClass}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-[12px] text-fg-muted">Confirm new password</span>
          <input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            required
            className={inputClass}
          />
        </label>
        {localError || change.isError ? (
          <p className="text-[12px] text-active">{localError || (change.error as Error).message}</p>
        ) : null}
      </form>
    </Dialog>
  );
}
