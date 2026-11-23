import { describe, expect, it } from 'vitest';
import { spaceOf, splitSpace, tabIn } from '#src/connection/clusters.tsx';

describe('a joined cluster in workspace keys and tabs', () => {
  it('names a joined agent @<cluster>/<agent>, this Mac’s by its key', () => {
    expect(spaceOf('a1b2c3d4', 'shop.backend')).toBe('@a1b2c3d4/shop.backend');
    expect(spaceOf('', 'handline.lead')).toBe('handline.lead');
    expect(splitSpace('@a1b2c3d4/shop.backend')).toEqual({ cluster: 'a1b2c3d4', agent: 'shop.backend' });
    expect(splitSpace('page/requests')).toEqual({ cluster: '', agent: 'page/requests' });
  });
  it('draws a tab another cluster’s row opened as its kind, in that cluster', () => {
    expect(tabIn({ id: 't1', opens: '@a1b2c3d4/task:41' })).toEqual({ tab: { id: 't1', opens: 'task:41' }, cluster: 'a1b2c3d4' });
    expect(tabIn({ id: 't2', opens: 'chat' })).toEqual({ tab: { id: 't2', opens: 'chat' }, cluster: '' });
  });
});
