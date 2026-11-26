import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boxIn, chainTo, locate, scaleOf, stylesOf, type DomNode } from '#src/pick-dom.ts';

let next = 1;
/** A node as DOM.getDocument gives it; its backend id is its node id plus 100. */
const node = (nodeType: number, over: Partial<DomNode> = {}): DomNode => {
  const nodeId = next++;
  return { nodeId, backendNodeId: nodeId + 100, nodeType, ...over };
};
const el = (tag: string, attrs: Record<string, string> = {}, children: DomNode[] = [], over: Partial<DomNode> = {}): DomNode =>
  node(1, { localName: tag, attributes: Object.entries(attrs).flat(), children, ...over });
const text = (nodeValue: string): DomNode => node(3, { nodeValue });
const doc = (...children: DomNode[]): DomNode => node(9, { children });

/** A page with a card, a list, a frame of the same site and a custom element with a shadow root. */
function page() {
  const pay = el('button', { class: 'btn primary', id: 'pay' }, [text('Pay now')]);
  const keep = el('button', { class: 'btn  ghost' }, [text('Keep\n   shopping')]);
  const third = el('span', {}, [text('Third')]);
  const before = node(1, { localName: '::before', pseudoType: 'before' });
  const list = el('ul', { class: 'list' }, [
    el('li', {}, [text('First')]),
    el('li', {}, [text('Second')]),
    el('li', {}, [third], { pseudoElements: [before] }),
  ]);
  const inner = el('button', { id: 'inner-btn' }, [text('Inside the frame')]);
  const frame = el('iframe', { id: 'same' }, [], { contentDocument: doc(el('html', {}, [el('body', {}, [inner])])) });
  const deep = el('b', {}, [text('Deep')]);
  const host = el('x-cart', {}, [], { shadowRoots: [node(11, { children: [el('div', {}, [el('i'), deep])] })] });
  const card = el('div', { class: 'card', id: 'cart' }, [
    el('p', { class: 'line' }, [text('Two items.'), el('script', {}, [text('ignored()')]), el('style', {}, [text('.x{}')])]),
    pay,
    keep,
  ]);
  const odd = el('div', { id: 'has space' }, [el('em', {}, [text('odd')])]);
  const root = doc(node(10), el('html', {}, [el('head'), el('body', {}, [el('h1', { id: 'title' }), card, list, frame, host, odd])]));
  return { root, pay, keep, third, before, inner, deep, card, odd };
}

test('the chain to a node runs from the document down, through a frame and a shadow root', () => {
  const p = page();
  const names = (n: DomNode) => chainTo(p.root, n.backendNodeId)!.map((c) => c.localName ?? `#${c.nodeType}`);
  assert.deepEqual(names(p.pay), ['#9', 'html', 'body', 'div', 'button']);
  assert.deepEqual(names(p.inner), ['#9', 'html', 'body', 'iframe', '#9', 'html', 'body', 'button']);
  assert.deepEqual(names(p.deep), ['#9', 'html', 'body', 'x-cart', '#11', 'div', 'b']);
  assert.equal(chainTo(p.root, 99_999), null);
});

test('a path starts at the nearest plain id, and counts an element among its siblings of the same tag only when there are several', () => {
  const p = page();
  const at = (n: DomNode) => locate(chainTo(p.root, n.backendNodeId)!, 300)!;
  assert.equal(at(p.pay).path, 'button#pay');
  assert.equal(at(p.keep).path, 'div#cart > button:nth-of-type(2)');
  assert.equal(at(p.third).path, 'html > body > ul > li:nth-of-type(3) > span');
  // An id that is not a plain name is not used: the element is counted like any other.
  assert.equal(at(p.odd.children![0]!).path, 'html > body > div:nth-of-type(2) > em');
  assert.deepEqual([at(p.keep).tag, at(p.keep).id, at(p.keep).classes], ['button', '', ['btn', 'ghost']]);
});

test('an element in a frame or a shadow root has its path in its own document, and what it is inside', () => {
  const p = page();
  const inner = locate(chainTo(p.root, p.inner.backendNodeId)!, 300)!;
  assert.deepEqual([inner.path, inner.within], ['button#inner-btn', ['iframe#same']]);
  const deep = locate(chainTo(p.root, p.deep.backendNodeId)!, 300)!;
  assert.deepEqual([deep.path, deep.within], ['div > b', ['html > body > x-cart (shadow)']]);
});

test('a pseudo-element is its element, and a chain with no element is nothing', () => {
  const p = page();
  const at = locate(chainTo(p.root, p.before.backendNodeId)!, 300)!;
  assert.equal(at.node.localName, 'li');
  assert.equal(at.path, 'html > body > ul > li:nth-of-type(3)');
  assert.equal(locate([p.root], 300), null);
});

test('the text is what the element holds, without scripts and styles, spaces collapsed, read only as far as needed', () => {
  const p = page();
  const at = (n: DomNode, max = 300) => locate(chainTo(p.root, n.backendNodeId)!, max)!.text;
  assert.equal(at(p.keep), 'Keep shopping');
  assert.equal(at(p.card), 'Two items. Pay now Keep shopping');
  assert.equal(at(p.card, 5), 'Two items.');
});

test('the scale is device pixels per CSS pixel as the layout says it, else the picture over the viewport', () => {
  assert.equal(scaleOf({ layoutViewport: { clientWidth: 2048 }, cssLayoutViewport: { clientWidth: 1024 } }, 2078), 2);
  assert.equal(scaleOf({ layoutViewport: { clientWidth: 1536 }, cssLayoutViewport: { clientWidth: 512 } }, 1560), 3);
  assert.equal(scaleOf({ cssLayoutViewport: { clientWidth: 1000 } }, 1500), 1.5);
  assert.equal(scaleOf({}, 1500), 1);
  assert.equal(scaleOf({ cssLayoutViewport: { clientWidth: 0 } }, 1500), 1);
});

test('a box is its quad in picture pixels, cut to the picture; one outside it is nothing', () => {
  const picture = { width: 2078, height: 1510 };
  assert.deepEqual(boxIn([41, 171.4375, 129.85, 171.4375, 129.85, 204.4375, 41, 204.4375], 2, picture), { x: 82, y: 343, w: 178, h: 66 });
  // Partly off the left and the bottom.
  assert.deepEqual(boxIn([-50, 700, 100, 700, 100, 900, -50, 900], 2, picture), { x: 0, y: 1400, w: 200, h: 110 });
  assert.equal(boxIn([3000, 10, 3100, 10, 3100, 20, 3000, 20], 2, picture), null);
  assert.equal(boxIn([0, 0, 10, 0, 10, 10], 2, picture), null);
  assert.equal(boxIn([0, 0, Number.NaN, 0, 10, 10, 0, 10], 2, picture), null);
  assert.equal(boxIn(undefined, 2, picture), null);
  assert.equal(boxIn([0, 0, 10, 0, 10, 10, 0, 10], 0, picture), null);
});

test('only the listed styles are read, and margin and padding are made of their four sides', () => {
  const side = (name: string, t: string, r: string, b: string, l: string) => [
    { name: `${name}-top`, value: t },
    { name: `${name}-right`, value: r },
    { name: `${name}-bottom`, value: b },
    { name: `${name}-left`, value: l },
  ];
  const out = stylesOf([
    { name: 'color', value: 'rgb(255, 255, 255)' },
    { name: 'cursor', value: 'pointer' },
    { name: 'background-image', value: 'url(https://example.com/x.png)' },
    { name: 'display', value: 'block' },
    ...side('margin', '0px', '0px', '0px', '0px'),
    ...side('padding', '8px', '14px', '8px', '14px'),
  ]);
  assert.deepEqual(out, { color: 'rgb(255, 255, 255)', display: 'block', margin: '0px', padding: '8px 14px' });
  assert.equal(stylesOf(side('margin', '1px', '2px', '3px', '2px')).margin, '1px 2px 3px');
  assert.equal(stylesOf(side('margin', '1px', '2px', '3px', '4px')).margin, '1px 2px 3px 4px');
  assert.deepEqual(stylesOf(side('margin', '1px', '2px', '3px', '').slice(0, 3)), {});
});
