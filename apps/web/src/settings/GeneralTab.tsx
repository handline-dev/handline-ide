// Settings > General: the cluster's name, renamed by its owner (setting cluster_name).
import { useServes, useSettings, useWrite, why } from '@handline/app-data';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Button } from '#src/ui/Button.tsx';
import { prompt } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

export function GeneralTab() {
  const name = useSettings().data?.values.cluster_name ?? '';
  const save = useWrite('settings.set');
  const owner = useServes('settings.set').served;
  const rename = async () => {
    const next = (await prompt({ title: 'Rename the cluster', initial: name, submitText: 'Rename' }))?.trim();
    if (!next || next === name) return;
    save.mutate({ changes: { cluster_name: next } }, { onError: (e) => toast(`It was not renamed: ${why(e)}`, { tone: 'warning' }) });
  };
  return (
    <SettingsPage keep="settings-general" title="General" note="This cluster.">
      <Block title="Cluster">
        <Setting title="Cluster name">
          <span className="flex items-center gap-3 text-sm">
            <span className="text-fg">{name}</span>
            {owner && <Button onClick={() => void rename()}>Rename</Button>}
          </span>
        </Setting>
      </Block>
    </SettingsPage>
  );
}
