import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boxIn, locate, scaleOf, stylesOf, textOfHtml, Tree, type DomNode } from '#src/pick-dom.ts';

let next = 1;
/** A node as the page describes it; its backend id is its node id plus 100. */
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

/** The whole of a small page held at once: a node that comes with its children is held with them. */
const held = (root: DomNode): Tree => {
  const tree = new Tree(10_000);
  tree.document(root);
  return tree;
};
/** A node as the page sends it in a list of children: without its own. */
const bare = (n: DomNode): DomNode => ({ ...n, children: undefined });

test('the tree is built from the document and the children the page sends for each node on the way down, and no more', () => {
  const p = page();
  const [, html] = p.root.children!;
  const body = html!.children![1]!;
  const tree = new Tree(10_000);
  tree.children(html!.nodeId, [bare(body)]); // before any document: nothing to hang it on
  tree.document(bare(p.root));
  assert.equal(tree.chainTo(html!.nodeId), null);
  tree.children(p.root.nodeId, p.root.children!.map(bare));
  tree.children(html!.nodeId, html!.children!.map(bare));
  tree.children(body.nodeId, body.children!.map(bare));
  tree.children(p.card.nodeId, p.card.children!.map(bare));
  const names = (id: number) => tree.chainTo(id)?.map((c) => c.localName ?? `#${c.nodeType}`);
  assert.deepEqual(names(p.pay.nodeId), ['#9', 'html', 'body', 'div', 'button']);
  // What was not sent is not held: the list's items, and nodes under the picked one.
  assert.equal(tree.chainTo(p.third.nodeId), null);
  assert.equal(tree.chainTo(p.pay.children![0]!.nodeId), null);
  assert.equal(tree.chainTo(99_999), null);
  // The path is counted among the siblings that came with it.
  assert.equal(locate(tree.chainTo(p.keep.nodeId)!)!.path, 'div#cart > button:nth-of-type(2)');
  // Children of a node that is not held are let go.
  tree.children(99_999, [bare(p.third)]);
  assert.equal(tree.chainTo(p.third.nodeId), null);
  // A new document starts over.
  tree.document(bare(p.root));
  assert.equal(tree.chainTo(p.pay.nodeId), null);
});

test('a frame carries its document and a shadow host its roots, so the chain runs through them', () => {
  const p = page();
  const tree = held(p.root);
  const names = (n: DomNode) => tree.chainTo(n.nodeId)!.map((c) => c.localName ?? `#${c.nodeType}`);
  assert.deepEqual(names(p.inner), ['#9', 'html', 'body', 'iframe', '#9', 'html', 'body', 'button']);
  assert.deepEqual(names(p.deep), ['#9', 'html', 'body', 'x-cart', '#11', 'div', 'b']);
});

test('the tree holds no more nodes than its budget, says when it is full, and a loop in what it was sent ends', () => {
  const root = doc();
  const many = Array.from({ length: 50 }, () => el('span'));
  const tree = new Tree(10);
  tree.document(bare(root));
  assert.equal(tree.full, false);
  tree.children(root.nodeId, many);
  assert.equal(tree.full, true);
  // The document and nine of them make ten: the tenth child is the first not held.
  assert.equal(tree.chainTo(many[8]!.nodeId)!.length, 2);
  assert.equal(tree.chainTo(many[9]!.nodeId), null);
  // Full: what comes after is not taken, nor kept hold of.
  const late = el('b');
  tree.children(many[0]!.nodeId, [late]);
  assert.equal(tree.chainTo(late.nodeId), null);
  assert.deepEqual(many[0]!.children, []);
  tree.document(bare(root));
  assert.equal(tree.full, false);
  // Exactly its budget is not too many.
  const exact = new Tree(3);
  exact.document(bare(root));
  exact.children(root.nodeId, many.slice(0, 2));
  assert.deepEqual([exact.full, exact.chainTo(many[1]!.nodeId)?.length], [false, 2]);

  // A page nested a hundred thousand deep is walked without a deeper stack.
  const top = doc();
  let last = top;
  const deep = new Tree(200_000);
  deep.document(top);
  for (let i = 0; i < 100_000; i++) {
    const next = el('div');
    deep.children(last.nodeId, [next]);
    last = next;
  }
  assert.equal(deep.chainTo(last.nodeId)!.length, 100_001);

  // A node sent as its own ancestor: no chain, and no endless walk.
  const a = el('div');
  const loop = new Tree(100);
  loop.document(bare(root));
  loop.children(root.nodeId, [a]);
  loop.children(a.nodeId, [bare(root)]);
  assert.equal(loop.chainTo(a.nodeId), null);
});

test('a path starts at the nearest plain id, and counts an element among its siblings of the same tag only when there are several', () => {
  const p = page();
  const tree = held(p.root);
  const at = (n: DomNode) => locate(tree.chainTo(n.nodeId)!)!;
  assert.equal(at(p.pay).path, 'button#pay');
  assert.equal(at(p.keep).path, 'div#cart > button:nth-of-type(2)');
  assert.equal(at(p.third).path, 'html > body > ul > li:nth-of-type(3) > span');
  // An id that is not a plain name is not used: the element is counted like any other.
  assert.equal(at(p.odd.children![0]!).path, 'html > body > div:nth-of-type(2) > em');
  assert.deepEqual([at(p.keep).tag, at(p.keep).id, at(p.keep).classes], ['button', '', ['btn', 'ghost']]);
});

test('an element in a frame or a shadow root has its path in its own document, and what it is inside', () => {
  const p = page();
  const tree = held(p.root);
  const inner = locate(tree.chainTo(p.inner.nodeId)!)!;
  assert.deepEqual([inner.path, inner.within], ['button#inner-btn', ['iframe#same']]);
  const deep = locate(tree.chainTo(p.deep.nodeId)!)!;
  assert.deepEqual([deep.path, deep.within], ['div > b', ['html > body > x-cart (shadow)']]);
});

test('a pseudo-element is its element, and a chain with no element is nothing', () => {
  const p = page();
  const at = locate(held(p.root).chainTo(p.before.nodeId)!)!;
  assert.equal(at.node.localName, 'li');
  assert.equal(at.path, 'html > body > ul > li:nth-of-type(3)');
  assert.equal(locate([p.root]), null);
});

test("the text is read from the element's HTML: what is between the tags, without scripts, styles and comments, spaces collapsed", () => {
  assert.equal(textOfHtml('<button class="btn" id="pay">Pay now</button>', 300), 'Pay now');
  assert.equal(textOfHtml('<button>Keep\n   shopping</button>', 300), 'Keep shopping');
  const card =
    '<div id="cart"><p class="line">Two items.<script>ignored("</p>")</script><STYLE>.x{}</STYLE><!-- not <b>this</b> --></p>' +
    '<button>Pay now</button><template><i>nor this</i></template><noscript>or this</noscript><button>Keep shopping</button></div>';
  assert.equal(textOfHtml(card, 300), 'Two items. Pay now Keep shopping');
  // A tag's own quoted values may hold what looks like its end.
  assert.equal(textOfHtml('<a title="a > b" data-x=\'c > "d"\'>link</a> after', 300), 'link after');
  assert.equal(
    textOfHtml('Tom &amp; Jerry &lt;3 &quot;x&quot; &#39;y&#x27; a&nbsp;b &copy; &#0; &#99999999;', 300),
    'Tom & Jerry <3 "x" \'y\' a b &copy; &#0; &#99999999;',
  );
  assert.equal(textOfHtml('', 300), '');
  assert.equal(textOfHtml('<script>only()</script>', 300), '');
  assert.equal(textOfHtml('<p>never closed <b', 300), 'never closed');
});

test('the text is read from the front and only as far as needed, in one pass however deep or long the HTML is', () => {
  const many = '<li>item</li>'.repeat(100_000);
  assert.equal(textOfHtml(`<ul>${many}</ul>`, 10), 'item item item');
  // A hundred thousand levels deep: no stack to run out of.
  const deep = `${'<div>'.repeat(100_000)}bottom${'</div>'.repeat(100_000)}`;
  assert.equal(textOfHtml(deep, 300), 'bottom');
  // One long run of text is not taken whole.
  assert.ok(textOfHtml(`<p>${'x'.repeat(5_000_000)}</p>`, 300).length <= 2400);
  // Past the first million characters nothing is read: text there is not found, and a value that long is not walked.
  assert.equal(textOfHtml(`<img src="${'A'.repeat(3_000_000)}"><p>late</p>`, 300), '');
  assert.equal(textOfHtml(`<p>early</p>${'<i></i>'.repeat(200_000)}<p>late</p>`, 300), 'early');
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
