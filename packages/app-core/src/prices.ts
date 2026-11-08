// List prices the screens estimate with: go/accounts/usage/prices.go's InputPrices and DefaultPattern, which a Go test holds this copy
// to. Most specific pattern first; the daemon lays config.yaml's token_prices over its copy, this one is the list only.
const INPUT_USD: [string, number][] = [
  ['fable-5-1', 10],
  ['mythos-5-1', 10],
  ['fable', 10],
  ['mythos', 10],
  ['opus-5-5', 4],
  ['opus', 5],
  ['sonnet-5', 2],
  ['sonnet', 3],
  ['haiku', 1],
];

const DEFAULT_PATTERN = 'sonnet';

/** USD per million input tokens for a model id or alias; the default pattern's for one that matches none, as the daemon does. */
export function inputPrice(model: string): number {
  const m = model.toLowerCase();
  return (INPUT_USD.find(([p]) => m.includes(p)) ?? INPUT_USD.find(([p]) => p === DEFAULT_PATTERN)!)[1];
}

/** What writing tokens into the hour-long prompt cache once costs on model: 2x its input price. */
export const cacheRewriteUsd = (tokens: number, model: string): number => (tokens / 1e6) * inputPrice(model) * 2;
