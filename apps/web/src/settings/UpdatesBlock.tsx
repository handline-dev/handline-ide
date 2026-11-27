// The Updates block of Settings > General: the running version, the channel the installed app follows and where it reads it, and the update when one is ready.
import { UPDATE_CHANNELS, type ShellUpdate, type UpdateChannel as Channel } from '@handline/app-core';
import { useRead } from '@handline/app-data';
import { Download } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { shell } from '#src/shell.ts';
import { Button } from '#src/ui/Button.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';

const WORD: Record<Channel, string> = { stable: 'Stable', nightly: 'Nightly', canary: 'Canary' };

type Installed = { channel: Channel | null; version: string | null; source: string | null };

/** A GitHub release address as its repository and tag; any other address without its scheme. */
function shortSource(url: string): string {
  const m = /github\.com\/([^/]+\/[^/]+)\/releases\/download\/([^/?#]+)/.exec(url);
  return m ? `${m[1]} · ${m[2]}` : url.replace(/^https?:\/\//, '');
}

/** The installed app's version (null for a checkout), channel and update address; null in a browser. */
function useInstalled(): [Installed | null, (c: Channel) => void] {
  const [now, setNow] = useState<Installed | null>(null);
  const [read, setRead] = useState(0);
  useEffect(() => {
    let live = true;
    void shell()
      ?.updateChannel?.()
      .then((r) => live && r.ok && setNow({ channel: r.channel, version: r.version, source: r.source }));
    return () => {
      live = false;
    };
  }, [read]);
  const pick = (c: Channel) => {
    setNow((n) => n && { ...n, channel: c });
    void shell()
      ?.setUpdateChannel?.(c)
      .finally(() => setRead((k) => k + 1));
  };
  return [now, pick];
}

function line(u: ShellUpdate | null): string {
  if (u?.phase === 'ready' || u?.phase === 'later') return 'An update is ready.';
  if (u?.phase === 'preparing') return u.step || 'Preparing an update…';
  if (u?.phase === 'failed') return `The update did not finish. ${u.reason}`;
  return 'Handline is up to date.';
}

export function UpdatesBlock() {
  const [app, pick] = useInstalled();
  const daemon = useRead('daemon.status', {}).data?.version;
  const [u, setU] = useState<ShellUpdate | null>(null);
  useEffect(() => shell()?.onUpdate?.(setU), []);
  const version = app?.version ?? daemon;
  if (!version) return null;
  const channel = app?.channel ?? null;
  const ready = u?.phase === 'ready' || u?.phase === 'later';
  const failed = u?.phase === 'failed';
  return (
    <Block title="Updates" icon={<Download size={16} />}>
      <Setting
        title={`Handline ${version}${channel ? ` · ${WORD[channel]}` : ''}`}
        note={app?.source ? shortSource(app.source) : undefined}
      >
        {null}
      </Setting>
      {channel && (
        <>
          <Setting title="Channel" note="Stable moves when it is promoted by hand, Nightly once a day, Canary every 30 minutes.">
            <Segmented label="Update channel" value={channel} options={UPDATE_CHANNELS} word={(c) => WORD[c]} onChange={pick} />
          </Setting>
          <Setting title={line(u)}>
            {(ready || failed) && (
              <Button variant="primary" onClick={() => void shell()?.updateAct?.(ready ? 'restart' : 'retry')}>
                {ready ? 'Restart to update' : 'Try again'}
              </Button>
            )}
            {failed && <Button onClick={() => void shell()?.updateAct?.('details')}>Details</Button>}
          </Setting>
        </>
      )}
    </Block>
  );
}
