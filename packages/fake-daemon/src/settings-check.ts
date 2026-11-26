// What settings.set refuses of one key's value, as the daemon does: each key takes only its own kind.
import { badRequest } from '#src/errors.ts';
import { checkHeavyRuns } from '#src/runload.ts';
import { EFFORTS } from '#src/settings-defaults.ts';

/** Throws bad_request, naming the key, when value is not what key takes; null (back to the default) always passes. */
export function checkValue(key: string, value: unknown): void {
  if (key === 'park_timeout_s' && value !== null && (!Number.isInteger(value) || (value as number) < 0))
    throw badRequest('park_timeout_s is a whole number of seconds, 0 for never', 'park_timeout_s');
  if (key === 'keep_awake' && value !== null && value !== 'off' && value !== 'on' && value !== 'working')
    throw badRequest('keep_awake is off, on or working', 'keep_awake');
  checkHeavyRuns(key, value);
  if (key === 'reap_orphans' && value !== null && value !== 'dry' && value !== 'on' && value !== 'off')
    throw badRequest('reap_orphans is dry, on or off', 'reap_orphans');
  if (key.startsWith('default_effort_') && value !== null && !EFFORTS.includes(value as string))
    throw badRequest(`${key} is one of ${EFFORTS.join(', ')}`, key);
  if (key.startsWith('default_model_') && value !== null && (typeof value !== 'string' || !/^(?!-)[^\s'"]{1,100}$/.test(value)))
    throw badRequest(`${key} is a model name of at most 100 characters, no spaces, not starting with -`, key);
}
