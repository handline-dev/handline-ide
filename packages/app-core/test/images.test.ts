import { test } from 'node:test';
import assert from 'node:assert/strict';
import { findImageRefs, imageRef, withImages } from '#src/images.ts';

test('an image is named with the attachment scheme and a clean caption', () => {
  assert.equal(imageRef('9f2c41d0a7b3e6c8', 'login [x]'), '![login  x](attachment:9f2c41d0a7b3e6c8)');
});

test('image references are found with their caption and id', () => {
  const t = 'a ![login](attachment:9f2c) b ![](attachment:ab_1)';
  assert.deepEqual(
    findImageRefs(t).map((r) => [t.slice(r.start, r.end), r.caption, r.id]),
    [
      ['![login](attachment:9f2c)', 'login', '9f2c'],
      ['![](attachment:ab_1)', '', 'ab_1'],
    ],
  );
  assert.deepEqual(findImageRefs('![x](http://a/b.png)'), []);
});

test('a capture note carries one reference per image', () => {
  assert.equal(
    withImages(' look ', [
      { id: 'a1', name: 'shot.png' },
      { id: 'b2', name: 'notes.md' },
    ]),
    'look\n![shot.png](attachment:a1)\n![notes.md](attachment:b2)',
  );
});
