// The person's nav prefs (the daemon's, on every device).
import { useRef } from 'react';
import { addPin, movePin, navOf, toggleIn, togglePin } from '@handline/app-core';
import { useQueryClient } from '@tanstack/react-query';
import type { NavPrefs, NavPrefsChanges, Prefs } from '@handline/client';
import { useData } from '#src/provider.ts';
import { usePrefs } from '#src/read.ts';
import { qk } from '#src/reads.ts';
import { useWrite } from '#src/write.ts';

/** The nav prefs, the defaults until they load; nothing is sent before then, since a key is replaced whole. */
export function useNav(): {
  nav: NavPrefs;
  ready: boolean;
  change: (changes: NavPrefsChanges) => void;
  togglePin: (agent: string) => void;
  addPin: (agent: string) => void;
  movePin: (agent: string, index: number) => void;
  toggleFold: (workspace: string) => void;
} {
  const prefs = usePrefs();
  const qc = useQueryClient();
  const { daemon } = useData();
  const set = useWrite('prefs.set');
  const ready = prefs.data !== undefined;
  const sent = useRef<string[] | null>(null); // the pins still on their way, so two changes in one tick both count
  const change = (changes: NavPrefsChanges) => {
    if (ready) set.mutate({ changes: { nav: changes } });
  };
  const pin = (next: (pins: string[]) => string[]) => {
    if (!ready) return;
    const pins = next(sent.current ?? navOf(qc.getQueryData<Prefs>(qk.prefs(daemon))).pins);
    sent.current = pins;
    set.mutate(
      { changes: { nav: { pins } } },
      {
        onSettled: () => {
          if (sent.current === pins) sent.current = null;
        },
      },
    );
  };
  return {
    nav: navOf(prefs.data),
    ready,
    change,
    togglePin: (agent) => pin((p) => togglePin(p, agent)),
    addPin: (agent) => pin((p) => addPin(p, agent)),
    movePin: (agent, index) => pin((p) => movePin(p, agent, index)),
    // From the latest prefs, so two quick folds both count.
    toggleFold: (workspace) => change({ folded: toggleIn(navOf(qc.getQueryData<Prefs>(qk.prefs(daemon))).folded, workspace) }),
  };
}
