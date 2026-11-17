// Settings > Notifications: whether this browser may show them, which events raise one and how (the daemon's settings, for every device), and a test.
import { classify, sentence, useSettings, useWrite } from '@handline/app-data';
import { AppWindow, Bell, BellRing, CircleCheck, CircleHelp, Gauge, Monitor, TriangleAlert, Volume2 } from 'lucide-react';
import type { ReactNode } from 'react';
import { useOnboarding } from '#src/onboarding/data/progress.ts';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

const HOW = [
  { key: 'push_only_away', title: 'Only when Handline is not in front', on: false, icon: <AppWindow size={16} /> },
  { key: 'push_sound', title: 'Play a sound', on: true, icon: <Volume2 size={16} /> },
] as const satisfies readonly { key: string; title: string; on: boolean; icon: ReactNode }[];

const KINDS = [
  { key: 'push_task_done', title: 'An agent finished its task', icon: <CircleCheck size={16} /> },
  { key: 'push_prompt_asked', title: 'An agent asks you something or waits for approval', icon: <CircleHelp size={16} /> },
  { key: 'push_stall', title: 'An agent hit an error or stopped', icon: <TriangleAlert size={16} /> },
  { key: 'push_account_out', title: 'An account ran out of usage', icon: <Gauge size={16} /> },
] as const;

export function NotificationsTab() {
  const o = useOnboarding();
  const settings = useSettings();
  const save = useWrite('settings.set');
  const values: Record<string, unknown> = settings.data?.values ?? {};
  return (
    <SettingsPage keep="settings-notifications" title="Notifications" note="What Handline tells you about, and how.">
      <Block
        title="This browser"
        icon={<Monitor size={16} />}
        note={
          {
            granted: 'Notifications are on.',
            denied: 'Blocked: change it in the browser or system settings.',
            default: 'Not asked yet.',
            unsupported: 'Not available here.',
          }[o.notify]
        }
      >
        <div className="flex flex-wrap gap-2">
          {o.notify === 'default' && (
            <Button variant="primary" onClick={() => void o.askNotify()}>
              <Bell size={14} />
              Allow notifications
            </Button>
          )}
          <Button
            disabled={o.notify !== 'granted'}
            onClick={() => {
              try {
                new Notification('Handline', { body: 'This is how a notification looks.' });
              } catch {
                // Some platforms only show them from a service worker.
              }
            }}
          >
            Send test notification
          </Button>
        </div>
      </Block>
      <Block title="Tell me when" icon={<BellRing size={16} />} note="Each is a setting of the daemon, so it holds for every device.">
        {KINDS.map((k) => (
          <Setting key={k.key} title={k.title} icon={k.icon}>
            <Toggle
              label={k.title}
              on={values[k.key] !== false}
              disabled={!settings.isSuccess || save.isPending}
              onChange={(on) => save.mutate({ changes: { [k.key]: on } })}
            />
          </Setting>
        ))}
      </Block>
      <Block title="How" icon={<Bell size={16} />} note="In the desktop app.">
        {HOW.map((k) => {
          const v = values[k.key];
          return (
            <Setting key={k.key} title={k.title} icon={k.icon}>
              <Toggle
                label={k.title}
                on={typeof v === 'boolean' ? v : k.on}
                disabled={!settings.isSuccess || save.isPending}
                onChange={(on) => save.mutate({ changes: { [k.key]: on } })}
              />
            </Setting>
          );
        })}
      </Block>
      {!!save.error && (
        <p className="text-xs text-danger" role="alert">
          {sentence(classify(save.error))}
        </p>
      )}
    </SettingsPage>
  );
}
