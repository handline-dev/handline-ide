import { describe, expect, it } from 'vitest';
import { appLink, codeFrom, invite } from '#src/connection/pairing.ts';

const FP = 'ab'.repeat(32);

describe('a pairing link', () => {
  it('carries the code, and the fingerprint of a self-made certificate', () => {
    expect(invite('#pair=5S3K-R92R')).toEqual({ code: '5S3K-R92R', fp: null });
    expect(invite(`#pair=5S3K%2DR92R&fp=${FP.toUpperCase()}`)).toEqual({ code: '5S3K-R92R', fp: FP });
    expect(invite('#settings')).toBeNull();
    expect(invite('#pair=%E0%A4%A')).toBeNull();
  });

  it('opens in the app as the same listener and code', () => {
    expect(appLink('home.tail1.ts.net:7443', { code: 'A B', fp: null })).toBe('handline://pair?host=home.tail1.ts.net%3A7443&code=A%20B');
    expect(appLink('10.0.0.2:7443', { code: 'C', fp: FP })).toBe(`handline://pair?host=10.0.0.2%3A7443&code=C&fp=${FP}`);
  });

  it('pasted whole, a bare code or an old handline:// line gives the code', () => {
    expect(codeFrom(` https://home.tail1.ts.net:7443/#pair=5S3K&fp=${FP} `)).toBe('5S3K');
    expect(codeFrom('handline://pair?host=h&code=7Q2X')).toBe('7Q2X');
    expect(codeFrom(' 7Q2X ')).toBe('7Q2X');
  });
});
