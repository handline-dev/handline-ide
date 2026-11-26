// A letter shortcut read by the key pressed, not the letter typed: with a Korean (or any non-Latin) input on, M types 'ㅡ' but its
// code is still KeyM. A key with no letter code falls back to what it typed.
export const letterOf = (e: { code?: string; key: string }): string =>
  /^Key[A-Z]$/.test(e.code ?? '') ? e.code!.slice(3).toLowerCase() : e.key.toLowerCase();
