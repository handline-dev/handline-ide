// Settings > Notifications: whether this browser may show them, which events raise one (the daemon's settings, for every device), and a test.
import { classify, sentence, useSettings, useWrite } from '@handline/app-data';
import { Bell } from 'lucide-react';
import { useOnboarding } from '#src/onboarding/data/progress.ts';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

const KINDS = [
  { key: 'push_task_done', title: 'An agent finished its task' },
  { key: 'push_prompt_asked', title: 'An agent asks you something or waits for approval' },
  { key: 'push_stall', title: 'An agent hit an error or stopped' },
  { key: 'push_account_out', title: 'An account ran out of usage' },
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
      <Block title="Tell me when" note="Each is a setting of the daemon, so it holds for every device.">
        {KINDS.map((k) => (
          <Setting key={k.key} title={k.title}>
            <Toggle
              label={k.title}
              on={values[k.key] !== false}
              disabled={!settings.isSuccess || save.isPending}
              onChange={(on) => save.mutate({ changes: { [k.key]: on } })}
            />
          </Setting>
        ))}
        {!!save.error && (
          <p className="text-xs text-danger" role="alert">
            {sentence(classify(save.error))}
          </p>
        )}
      </Block>
    </SettingsPage>
  );
}
