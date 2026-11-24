import { useEffect, useState } from 'react';

/** A short time since `iso`, for tight spots like a sidebar row: now, 5m, 3h, 2d, then the date (11/2,
 * with the year once it is not this year's). A time in the future reads now. */
export function ago(iso: string, now: number = Date.now()): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  const s = Math.max(0, (now - t) / 1000);
  if (s < 60) return 'now';
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  if (s < 7 * 86400) return `${Math.floor(s / 86400)}d`;
  const d = new Date(t);
  const md = `${d.getMonth() + 1}/${d.getDate()}`;
  return d.getFullYear() === new Date(now).getFullYear() ? md : `${d.getFullYear()}/${md}`;
}

/** The current time, updated every `ms` (a minute by default; null: not updated), so relative times stay true. */
export function useNow(ms: number | null = 60_000): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (ms === null) return;
    const id = setInterval(() => setNow(Date.now()), ms);
    return () => clearInterval(id);
  }, [ms]);
  return now;
}

/** The time of day, as hours and minutes in the reader's locale. */
export const clock = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

/** How long ago `iso` was, in words: just now, 5m ago, 3h ago, 2d ago, then the date (ago). */
export function since(iso: string, now: number = Date.now()): string {
  const a = ago(iso, now);
  if (a === 'now') return 'just now';
  return /^\d+[mhd]$/.test(a) ? `${a} ago` : a;
}

/** A date as month and day: Nov 2. */
export const day = (iso: string) => new Date(iso).toLocaleDateString([], { month: 'short', day: 'numeric' });

/** A time today as its clock (14:02), an older one with its day first (Nov 2 14:02). */
export function clockOrDay(iso: string): string {
  const hm = clock(iso);
  return new Date(iso).toDateString() === new Date().toDateString() ? hm : `${day(iso)} ${hm}`;
}

/** Milliseconds left as a countdown: 4:05. */
export const countdown = (ms: number) => `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}`;
