// The Updates block of This Mac's page: which release channel the installed app follows, and the update when one is ready.
import type { ShellUpdate } from '@handline/app-core';
import { Download } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { shell } from '#src/shell.ts';
import { Button } from '#src/ui/Button.tsx';
import { Switch } from '#src/usage/UsageViews.tsx';

const CHANNELS = ['stable', 'nightly', 'canary'] as const;
type Channel = (typeof CHANNELS)[number];
const WORD: Record<Channel, string> = { stable: 'Stable', nightly: 'Nightly', canary: 'Canary' };

/** The channel the installed app reads updates from, null in a browser and in a build that reads none. */
function useChannel(): [Channel | null, (c: Channel) => void] {
  const [channel, setChannel] = useState<Channel | null>(null);
  useEffect(() => {
    let live = true;
    void shell()
      ?.updateChannel?.()
      .then((r) => live && r.ok && setChannel(r.channel));
    return () => {
      live = false;
    };
  }, []);
  const pick = (c: Channel) => {
    setChannel(c);
    void shell()?.setUpdateChannel?.(c);
  };
  return [channel, pick];
}

function line(u: ShellUpdate | null): string {
  if (u?.phase === 'ready' || u?.phase === 'later') return 'An update is ready.';
  if (u?.phase === 'preparing') return u.step || 'Preparing an update…';
  if (u?.phase === 'failed') return `The update did not finish. ${u.reason}`;
  return 'Handline is up to date.';
}

export function UpdatesBlock() {
  const [channel, pick] = useChannel();
  const [u, setU] = useState<ShellUpdate | null>(null);
  useEffect(() => shell()?.onUpdate?.(setU), []);
  if (!channel) return null;
  const ready = u?.phase === 'ready' || u?.phase === 'later';
  const failed = u?.phase === 'failed';
  return (
    <Block title="Updates" icon={<Download size={16} />}>
      <Setting title="Channel" note="Stable moves when it is promoted by hand, Nightly once a day, Canary every 30 minutes.">
        <Switch label="Update channel" value={channel} options={CHANNELS} word={(c) => WORD[c]} onChange={pick} />
      </Setting>
      <Setting title={line(u)}>
        {(ready || failed) && (
          <Button variant="primary" onClick={() => void shell()?.updateAct?.(ready ? 'restart' : 'retry')}>
            {ready ? 'Restart to update' : 'Try again'}
          </Button>
        )}
      </Setting>
    </Block>
  );
}
