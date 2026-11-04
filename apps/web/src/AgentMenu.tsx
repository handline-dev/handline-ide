import { deleteSaid, heldWords, type HeldWorktree, type WorktreeAnswer } from '@handline/app-core';
import { classify, sentence, useAgentDetail, useNav, useRetireAgent } from '@handline/app-data';
import type { AgentView } from '@handline/client';
import { Copy, FolderOpen, Pin, PinOff, Settings, Trash2 } from 'lucide-react';
import type { ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { ContextMenu, MenuItem, MenuSeparator, SubMenu } from '#src/ui/Menu.tsx';
import { confirm, ModalFrame, openModal } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

const copy = (text: string, what: string) =>
  void navigator.clipboard?.writeText(text).then(
    () => toast(`Copied the ${what}.`),
    () => toast(`Could not copy the ${what}.`, { tone: 'warning' }),
  );

/** A worktree that holds work: how to delete (the safe answer first), or undefined to keep the agent. */
function askWorktree(name: string, held: HeldWorktree[]) {
  return openModal<WorktreeAnswer>((close) => (
    <ModalFrame
      title={`${name} has work in its worktree`}
      description={`${name} has ${heldWords(held)}. Nothing is deleted until you pick.`}
      onClose={() => close()}
      footer={
        <>
          <Button onClick={() => close()}>Cancel</Button>
          <Button variant="danger" onClick={() => close('force')}>
            Delete and lose the files
          </Button>
          <Button onClick={() => close('keep')}>Delete, keep the worktree</Button>
          <Button variant="primary" autoFocus onClick={() => close('save')}>
            Save to a branch, then delete
          </Button>
        </>
      }
    />
  ));
}

/** Right-click on an agent row: pin, settings, open in, delete. */
export function AgentMenu({ agent, onSettings, children }: { agent: AgentView; onSettings: () => void; children: ReactNode }) {
  const name = agent.name;
  const { nav, togglePin } = useNav();
  const pinned = nav.pins.includes(name);
  const detail = useAgentDetail(name).data;
  const retire = useRetireAgent(name);

  const remove = async () => {
    if (
      !(await confirm({
        title: `Delete ${name}?`,
        message: 'Its terminal closes and it leaves the team.',
        confirmText: 'Delete',
        danger: true,
      }))
    )
      return;
    const attempt = (worktree?: WorktreeAnswer) =>
      retire(
        {
          onSuccess: () => toast(worktree ? deleteSaid(worktree, name) : `${name} deleted.`),
          onError: (e) => {
            const f = classify(e);
            if (f.kind === 'refused' && f.held?.length) void askWorktree(name, f.held).then((a) => a && attempt(a));
            else toast(sentence(f), { tone: 'warning' });
          },
        },
        worktree,
      );
    attempt();
  };

  return (
    <ContextMenu
      items={
        <>
          <MenuItem icon={pinned ? <PinOff size={14} /> : <Pin size={14} />} onClick={() => togglePin(name)}>
            {pinned ? 'Unpin' : 'Pin to top'}
          </MenuItem>
          <MenuItem icon={<Settings size={14} />} onClick={onSettings}>
            Settings…
          </MenuItem>
          <MenuSeparator />
          {/* Opening a folder or an app needs the desktop app; in a browser the paths can be copied. */}
          <SubMenu label="Open in" icon={<FolderOpen size={14} />}>
            <MenuItem icon={<Copy size={14} />} disabled={!detail?.cwd} onClick={() => detail && copy(detail.cwd, 'folder path')}>
              Copy folder path
            </MenuItem>
            {detail?.worktree && (
              <MenuItem
                icon={<Copy size={14} />}
                onClick={() => copy(detail.worktree!.path.split('/').pop() ?? detail.worktree!.path, 'worktree name')}
              >
                Copy worktree name
              </MenuItem>
            )}
          </SubMenu>
          <MenuSeparator />
          <MenuItem icon={<Trash2 size={14} />} danger onClick={() => void remove()}>
            Delete {name}
          </MenuItem>
        </>
      }
    >
      {children}
    </ContextMenu>
  );
}
