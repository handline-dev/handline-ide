// A workspace's folders in a window of their own: each with its git or plain mark, the default tagged, the agents working in it, a
// remove on hover; Add folder at the end.
import { Hooks, useAgents, useNameOf, useWrite, why } from '@handline/app-data';
import type { Workspace } from '@handline/client';
import { Folder, FolderPlus, GitBranch, X } from 'lucide-react';
import { useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { shell } from '#src/shell.ts';

/** The last row of a folder list: Add folder, by the system's folder chooser (a typed path in a browser). `onAdded` hears the path once
 * the workspace has it. A row, not a form, so it can sit inside another form. */
export function AddFolder({ wsKey, onAdded }: { wsKey: string; onAdded?: (path: string) => void }) {
  const set = useWrite('workspace.set');
  const [typing, setTyping] = useState<string | null>(null);
  const send = (path: string) =>
    set.mutate(
      { workspace: wsKey, add_folder: path },
      {
        onSuccess: () => {
          setTyping(null);
          onAdded?.(path);
        },
      },
    );
  const add = async () => {
    const pick = shell()?.pickFolder;
    if (!pick) return setTyping('');
    // A chooser that fails falls back to typing the path.
    const r = await pick().catch(() => null);
    if (r?.ok && r.path) send(r.path);
    else if (!r?.ok) setTyping('');
  };
  return (
    <>
      {typing === null ? (
        <button
          type="button"
          onClick={() => void add()}
          disabled={set.isPending}
          className="flex h-9 items-center gap-2 px-3 text-left text-sm text-muted outline-none hover:bg-hover hover:text-fg focus-visible:bg-hover"
        >
          <FolderPlus size={14} className="shrink-0" />
          {set.isPending ? 'Adding…' : 'Add folder'}
        </button>
      ) : (
        <div className="flex items-center gap-2 px-2 py-1">
          <Input
            autoFocus
            value={typing}
            onChange={(e) => setTyping(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                if (typing.trim()) send(typing.trim());
              }
            }}
            placeholder="/path/to/folder"
            className="flex-1"
          />
          <Button variant="primary" disabled={!typing.trim() || set.isPending} onClick={() => send(typing.trim())}>
            Add
          </Button>
        </div>
      )}
      {!!set.error && <p className="px-3 py-1 text-xs text-danger">{why(set.error)}</p>}
    </>
  );
}

/** A folder's row start: a git or plain-folder mark, its name, and Default on the default one. */
export function FolderName({ folder: d }: { folder: { name: string; git: boolean; default: boolean } }) {
  const Mark = d.git ? GitBranch : Folder;
  return (
    <>
      <Mark size={14} className="shrink-0 text-muted" aria-label={d.git ? 'git' : 'folder'} />
      <span className="min-w-0 truncate">{d.name}</span>
      {d.default && <span className="shrink-0 rounded bg-hover px-1.5 text-xs text-muted">Default</span>}
    </>
  );
}

export function Folders({ wsKey }: { wsKey: string }) {
  // Read live, so an added or removed folder shows at once.
  const ws = Hooks.useWorkspacesList().data?.workspaces.find((w) => w.key === wsKey);
  const set = useWrite('workspace.set');
  const nameOf = useNameOf();
  const agents = useAgents().data?.agents ?? [];
  // An agent with no folder works in the default.
  const workingIn = (path: string, isDefault: boolean) =>
    agents.filter((a) => a.workspace === wsKey && (a.folder ?? (isDefault ? path : null)) === path);
  if (!ws) return null;
  return (
    <div className="flex flex-col gap-3 pb-4 text-sm">
      <div className="flex flex-col overflow-hidden rounded-md border border-border">
        {ws.folders.map((d) => {
          const here = workingIn(d.path, d.default);
          return (
            <div key={d.path} title={d.path} className="group flex h-9 items-center gap-2 border-b border-border px-3">
              <FolderName folder={d} />
              <span className="min-w-0 flex-1 truncate text-right text-xs text-muted">{here.map((a) => nameOf(a.key)).join(', ')}</span>
              <button
                type="button"
                aria-label={`Remove ${d.name}`}
                disabled={here.length > 0 || set.isPending}
                onClick={() => set.mutate({ workspace: ws.key, remove_folder: d.path })}
                className="grid size-6 shrink-0 place-items-center rounded text-muted opacity-0 outline-none group-hover:opacity-100 hover:bg-hover hover:text-fg focus-visible:opacity-100 disabled:invisible"
              >
                <X size={14} />
              </button>
            </div>
          );
        })}
        <AddFolder wsKey={ws.key} />
      </div>
      {!!set.error && <p className="text-danger">{why(set.error)}</p>}
    </div>
  );
}

/** The window of the workspace's folders. */
export function workspaceFolders(ws: Workspace) {
  void openModal<void>((close) => (
    <ModalFrame title={`${ws.name} folders`} onClose={() => close()}>
      <Folders wsKey={ws.key} />
    </ModalFrame>
  ));
}
