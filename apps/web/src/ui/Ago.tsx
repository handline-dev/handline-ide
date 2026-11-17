import { since, useNow } from '#src/format/time.ts';

/** How long ago `at` was (5m ago), kept true as time passes: the one way a past moment is written. */
export function Ago({ at }: { at: string }) {
  return <>{since(at, useNow())}</>;
}
