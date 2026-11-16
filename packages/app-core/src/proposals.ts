// Which proposals wait on the founder.
import type { Proposal } from '@handline/client';

/** The states that wait for the founder; any other word is drawn as text, offers no action and is not counted. */
export const isWaiting = (p: Proposal): boolean => p.state === 'open' || p.state === 'apply_failed';
export const waitingCount = (l: readonly Proposal[]): number => l.filter(isWaiting).length;
