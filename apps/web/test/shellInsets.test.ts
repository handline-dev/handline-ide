import { expect, test } from 'vitest';
import { insetsFor } from '#src/shellInsets.ts';

test('the room for the window buttons follows the zoom, since the buttons do not', () => {
  expect(insetsFor(1, false)).toEqual({ '--lights-w': '86px', '--controls-top': '8px' });
  expect(insetsFor(2, false)).toEqual({ '--lights-w': '43px', '--controls-top': '-3px' });
  expect(insetsFor(0.5, false)).toEqual({ '--lights-w': '172px', '--controls-top': '30px' });
});

test('in full screen the buttons are gone, and so is their room', () => {
  expect(insetsFor(1.5, true)).toEqual({ '--lights-w': '0px', '--controls-top': '6px' });
});
