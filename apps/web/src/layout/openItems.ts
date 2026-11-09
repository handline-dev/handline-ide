// What a right-click on anything that opens a tab offers, from what it opens. Pure.
import { opened } from '#src/layout/kinds.tsx';

export type OpenItem =
  | { id: 'here' | 'side' | 'below'; label: string }
  | { id: 'copy'; label: string; text: string; name: string }
  | { id: 'browser'; label: string; url: string };

/** Open, Open to the side, Open below; then a way to copy what it names (a task's T-number, a link, a file's path), and a web
 * page also opens in the browser. */
export function openItems(opens: string): OpenItem[] {
  const items: OpenItem[] = [
    { id: 'here', label: 'Open' },
    { id: 'side', label: 'Open to the side' },
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
