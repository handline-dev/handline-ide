import { useNameOf } from '@handline/app-data';

/** A who key (an agent, a person, a workspace) written out as its name. */
export function Who({ k }: { k: string }) {
  return <>{useNameOf()(k)}</>;
}
