// A daemon warning's icon and word in Requests.
import type { PendingRequest } from '@handline/client';
import { Bell, HardDrive, KeyRound, ListChecks, MailWarning } from 'lucide-react';

export const WARN: Record<NonNullable<PendingRequest['warning']>['kind'], typeof HardDrive> = {
  disk: HardDrive,
  login: KeyRound,
  delivery: MailWarning,
  undelivered: MailWarning,
  notice: Bell,
  review: ListChecks,
};

/** In Requests, beside the icon of a warning: what it is about. */
export const WARN_WORD: Record<NonNullable<PendingRequest['warning']>['kind'], string> = {
  disk: 'Disk',
  login: 'Account',
  delivery: 'Delivery',
  undelivered: 'Delivery',
  notice: 'Notice',
  review: 'Review',
};
