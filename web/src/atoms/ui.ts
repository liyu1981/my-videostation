import { atom } from 'jotai';
import { atomWithStorage } from 'jotai/utils';

export type ViewMode = 'poster' | 'tile' | 'list';
export type Density = 'comfortable' | 'compact';

export const viewModeAtom = atomWithStorage<ViewMode>('mvs.viewMode', 'poster');
export const densityAtom = atomWithStorage<Density>('mvs.density', 'comfortable');

/** Item currently being played in the modal player. */
export const playerAtom = atom<{ itemId: number; title: string } | null>(null);
