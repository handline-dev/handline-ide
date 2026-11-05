// The editor's calls for one agent's files: a plain read and write by etag, and the daemon's shared doc (open, push, pull,
// close and its notices). Each takes the path at call time, so a new file is written where it was just named.
import type { CallParams, DocNotice } from '@handline/client';
import { useStore } from '@handline/app-react';
import { newOperationId } from '#src/send-ops.ts';

export function useDocCalls(agent: string) {
  const { client } = useStore();
  return {
    read: (path: string) => client.call('file.read', { agent, path }),
    write: (path: string, text: string, etag: string) =>
      client.call('file.write', { agent, path, text, etag, request_id: newOperationId() }),
    open: (path: string) => client.call('doc.open', { agent, path }),
    push: async (path: string, doc: string, version: number, updates: CallParams<'doc.push'>['updates']) =>
      (
        await client.call('doc.push', {
          agent,
          path,
          doc,
          version,
          updates,
          request_id: newOperationId(),
        })
      ).version,
    pull: (path: string, doc: string, version: number) => client.call('doc.pull', { agent, path, doc, version }),
    close: (path: string) => void client.call('doc.close', { agent, path }).catch(() => {}),
    /** The doc notices for this agent's file at `path()`, read when each one comes. */
    listen: (path: () => string, f: (n: DocNotice) => void) =>
      client.onDoc((n) => {
        if (n.agent === agent && n.path === path()) f(n);
      }),
  };
}
