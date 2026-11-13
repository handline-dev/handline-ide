// What a right-click on anything that opens a tab offers, from what it opens. Pure.
import { opened } from '#src/layout/kinds.tsx';
import { SHORTCUT } from '#src/layout/openTab.tsx';

export type OpenItem =
  | { id: 'tab' | 'side' | 'below'; label: string; shortcut?: string }
  | { id: 'copy'; label: string; text: string; name: string }
  | { id: 'browser'; label: string; url: string };

/** The three ways to open, with their shortcuts; then a way to copy what it names (a task's T-number, a link, a file's path), and a web
 * page also opens in the browser. */
export function openItems(opens: string): OpenItem[] {
  const items: OpenItem[] = [
    { id: 'tab', label: 'Open in new tab', shortcut: SHORTCUT.tab },
    { id: 'side', label: 'Open to the side', shortcut: SHORTCUT.side },
    { id: 'below', label: 'Open below' },
  ];
  const task = opened(opens, 'task');
  if (task) items.push({ id: 'copy', label: `Copy T${task}`, text: `T${task}`, name: 'the task number' });
  const url = opened(opens, 'web');
  if (url) items.push({ id: 'copy', label: 'Copy link', text: url, name: 'the link' }, { id: 'browser', label: 'Open in browser', url });
  const file = opened(opens, 'file');
  if (file) items.push({ id: 'copy', label: 'Copy path', text: file, name: 'the path' });
  return items;
}
