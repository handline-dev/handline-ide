// The founder's requests across every cluster, read once at the app's top, outside any pane's cluster: the Requests mark and the
// Requests tab both read this one copy, so their numbers cannot drift apart.
import { turnCount } from '@handline/app-core';
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { Across, type Of } from '#src/connection/clusters.tsx';
import { useRequests } from '#src/requests/data.ts';
import { useServiceDown } from '#src/requests/ServiceRow.tsx';

export type AllRequests = Of<ReturnType<typeof useRequests>>[];

const Ctx = createContext<AllRequests | null>(null);

const sig = (r: ReturnType<typeof useRequests>) => JSON.stringify([r.loaded, r.failed, r.turn, r.notes, r.later, r.unseen]);

export function RequestsAcross({ children }: { children: ReactNode }) {
  const [all, setAll] = useState<AllRequests | null>(null);
  return (
    <>
      <Across hook={useRequests} sig={sig}>
        {(a) => <Put all={a} set={setAll} />}
      </Across>
      <Ctx.Provider value={all}>{children}</Ctx.Provider>
    </>
  );
}

function Put({ all, set }: { all: AllRequests; set: (a: AllRequests) => void }) {
  useEffect(() => set(all), [all, set]);
  return null;
}

/** Every cluster's requests, this Mac's first; null until this Mac's have been read once. */
export const useAllRequests = () => useContext(Ctx);

/** The one number of what waits on the founder: every cluster's Your turn rows, and this Mac's stopped service. */
export function useTurnCount(): number {
  const all = useAllRequests() ?? [];
  return turnCount(
    all.map((a) => a.data.turn.length),
    useServiceDown(),
  );
}
