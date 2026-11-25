import { act, useEffect } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it } from 'vitest';
import { TabId } from '#src/persist/tabState.ts';
import { PickProvider, usePick } from '#src/requests/RowMenu.tsx';

const seen: { pick: ReturnType<typeof usePick> } = { pick: null };
function Probe() {
  const p = usePick();
  useEffect(() => {
    seen.pick = p;
  });
  return null;
}
const draw = async (tab: string) => {
  const root = createRoot(document.createElement('div'));
  await act(async () =>
    root.render(
      <TabId.Provider value={tab}>
        <PickProvider>
          <Probe />
        </PickProvider>
      </TabId.Provider>,
    ),
  );
  return () => act(async () => root.unmount());
};

describe('Requests kept with its tab', () => {
  it('keeps the checked rows when the tab is left and drawn again, and not in another tab', async () => {
    const gone = await draw('req-1');
    await act(async () => seen.pick!.toggle({ key: 'turn:task:1' }));
    expect([...seen.pick!.picked.keys()]).toEqual(['turn:task:1']);
    await gone();
    await draw('req-1');
    expect([...seen.pick!.picked.keys()]).toEqual(['turn:task:1']);
    await draw('req-2');
    expect(seen.pick!.picked.size).toBe(0);
  });
});
