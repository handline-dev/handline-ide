import { act, type DragEvent, type KeyboardEvent, type MouseEvent } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PaneOpen, type How } from '#src/layout/openTab.tsx';
import { openItems } from '#src/layout/openItems.ts';
import { useOpenable } from '#src/layout/openable.tsx';
import { initial, panes, reduce, whereTo } from '#src/layout/tree.ts';
import { HOLD, useSpringOpen } from '#src/ui/springOpen.ts';

let root: Root;
beforeEach(() => {
  const el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
});
afterEach(() => act(() => root.unmount()));

/** Renders `use` inside a pane whose opens are recorded. */
function mount<T>(use: () => T) {
  const opened: [string, How | undefined][] = [];
  const out = {} as { current: T };
  function Probe() {
    out.current = use();
    return null;
  }
  act(() =>
    root.render(
      <PaneOpen.Provider value={(opens, how) => opened.push([opens, how])}>
        <Probe />
      </PaneOpen.Provider>,
    ),
  );
  return { out, opened };
}

const mouse = (over: Partial<MouseEvent> = {}) => ({ button: 0, preventDefault() {}, stopPropagation() {}, ...over }) as MouseEvent;

describe('openItems', () => {
  it('offers the three ways to open, and a task its T-number to copy', () => {
    expect(openItems('task:8503').map((i) => i.label)).toEqual(['Open in new tab', 'Open to the side', 'Open below', 'Copy T8503']);
  });
  it('a web page copies its link and opens in the browser', () => {
    const items = openItems('web:https://example.com/a');
    expect(items.slice(3)).toEqual([
      { id: 'copy', label: 'Copy link', text: 'https://example.com/a', name: 'the link' },
      { id: 'browser', label: 'Open in browser', url: 'https://example.com/a' },
    ]);
  });
  it('a file copies its path; a plain kind has only the ways to open', () => {
    expect(openItems('file:docs/a.md').at(-1)).toEqual({ id: 'copy', label: 'Copy path', text: 'docs/a.md', name: 'the path' });
    expect(openItems('diff')).toHaveLength(3);
  });
});

describe('useOpenable', () => {
  it('a click opens here; Cmd, Ctrl or a middle click a new tab; Option beside', () => {
    const here = vi.fn();
    const { out, opened } = mount(() => useOpenable('task:1', { here }));
    out.current.onClick(mouse());
    out.current.onClick(mouse({ metaKey: true }));
    out.current.onClick(mouse({ ctrlKey: true }));
    out.current.onAuxClick(mouse({ button: 1 }));
    out.current.onAuxClick(mouse({ button: 2 }));
    out.current.onClick(mouse({ altKey: true }));
    expect(here).toHaveBeenCalledOnce();
    expect(opened).toEqual([
      ['task:1', 'here'],
      ['task:1', 'here'],
      ['task:1', 'here'],
      ['task:1', 'side'],
    ]);
  });

  it('Option+Enter opens beside, Cmd+Enter a new tab; plain Enter is a button’s own click, but opens a div', () => {
    const { out, opened } = mount(() => useOpenable('diff'));
    const key = (target: Element, over: Partial<KeyboardEvent> = {}) =>
      ({ key: 'Enter', currentTarget: target, preventDefault() {}, ...over }) as unknown as KeyboardEvent;
    out.current.onKeyDown(key(document.createElement('button'), { altKey: true }));
    out.current.onKeyDown(key(document.createElement('button'), { metaKey: true }));
    out.current.onKeyDown(key(document.createElement('button')));
    out.current.onKeyDown(key(document.createElement('div')));
    expect(opened).toEqual([
      ['diff', 'side'],
      ['diff', 'here'],
      ['diff', 'here'],
    ]);
  });

  it('`here` takes a plain open; `onOpen` runs before each, `focus` a drag’s too, and a drag carries what it opens', () => {
    const here = vi.fn();
    const onOpen = vi.fn();
    const focus = vi.fn();
    const { out, opened } = mount(() => useOpenable('chat:a', { here, onOpen, focus }));
    out.current.onClick(mouse());
    out.current.onClick(mouse({ altKey: true }));
    expect(here).toHaveBeenCalledOnce();
    expect(opened).toEqual([['chat:a', 'side']]);
    const setData = vi.fn();
    out.current.onDragStart?.({ dataTransfer: { setData, effectAllowed: '' } } as unknown as DragEvent);
    expect(onOpen).toHaveBeenCalledTimes(2);
    expect(focus).toHaveBeenCalledTimes(3);
    expect(setData.mock.calls.some(([, v]) => String(v).includes('chat:a'))).toBe(true);
  });

  it('a right-click has the app’s menu, unless the element has its own; nothing to open, no drag nor menu', () => {
    expect(mount(() => useOpenable('diff')).out.current.onContextMenu).toBeTypeOf('function');
    expect(mount(() => useOpenable('diff', { menu: false })).out.current.onContextMenu).toBeUndefined();
    const none = mount(() => useOpenable(null, { here: () => {} })).out.current;
    expect(none.onDragStart).toBeUndefined();
    expect(none.onContextMenu).toBeUndefined();
  });
});

describe('spring-open', () => {
  const files = { dataTransfer: { types: ['Files'] } } as unknown as DragEvent;

  it('opens once, after the hold', () => {
    vi.useFakeTimers();
    const open = vi.fn();
    const { out } = mount(() => useSpringOpen(open));
    act(() => out.current.props.onDragEnter(files));
    act(() => {
      vi.advanceTimersByTime(HOLD - 1);
    });
    expect(open).not.toHaveBeenCalled();
    act(() => {
      vi.advanceTimersByTime(HOLD * 2);
    });
    expect(open).toHaveBeenCalledOnce();
    vi.useRealTimers();
  });

  it('files taken away before the hold ends open nothing', () => {
    vi.useFakeTimers();
    const open = vi.fn();
    const { out } = mount(() => useSpringOpen(open));
    act(() => out.current.props.onDragEnter(files));
    act(() => out.current.props.onDragLeave(files));
    act(() => {
      vi.advanceTimersByTime(HOLD * 2);
    });
    expect(open).not.toHaveBeenCalled();
    expect(out.current.props['data-spring']).toBeUndefined();
    vi.useRealTimers();
  });
});

describe('whereTo', () => {
  it('below opens a new split under the pane; beside, the next pane or a split to the right', () => {
    const one = initial();
    const p = panes(one)[0]!.id;
    expect(whereTo(one, p, 'below')).toEqual({ pane: p, edge: 'down' });
    expect(whereTo(one, p, 'side')).toEqual({ pane: p, edge: 'right' });
    const two = reduce(one, { type: 'split', pane: p, edge: 'right' });
    expect(whereTo(two, p, 'side')).toEqual({ pane: panes(two)[1]!.id });
    const below = reduce(one, { type: 'open', opens: 'diff', title: 'Changes', to: whereTo(one, p, 'below') });
    expect(below.kind === 'split' && below.axis).toBe('y');
  });
});
