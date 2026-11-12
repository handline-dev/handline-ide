import { afterEach, describe, expect, it } from 'vitest';
import { setup } from '@handline/fake-daemon/helpers';
import { Store } from '@handline/app-core';
import { RULE_KEYS, ruleRows, useWorkspaceRules, useWrite } from '#src/index.ts';
import { mount, probe, unmount, until } from '#test/harness.tsx';

afterEach(unmount);

describe('ruleRows', () => {
  it('gives every key once, in RULE_KEYS order, a key set nowhere with no value and no source', () => {
    const rows = ruleRows([{ key: 'mode_max', value: 'plan', on: true, source: 'machine', set_by: 'p1', set_at: 1 }]);
    expect(rows.map((r) => r.key)).toEqual([...RULE_KEYS]);
    expect(rows.find((r) => r.key === 'mode_max')).toMatchObject({ set: true, value: 'plan', source: 'machine', on: true });
    expect(rows.find((r) => r.key === 'agent_cap')).toEqual({ key: 'agent_cap', set: false, on: false, value: null, source: null });
  });
});

describe('useWorkspaceRules', () => {
  it("reads a workspace's rules and its rules text's size, and reads them again after a rule is set on it or on the machine", async () => {
    const s = setup();
    const store = new Store(s.client);
    await store.start();
    const [ws] = (await store.client.call('workspaces.list', {})).workspaces;
    if (!ws) throw new Error('a seeded workspace');
    s.daemon.rules.textBytes.set(ws.key, 812);
    const v = probe(() => ({ rules: useWorkspaceRules(ws.key), set: useWrite('workspace.rule.set') }));
    await mount(store, v.node);
    const row = (key: string) => v.get().rules.data?.rules.find((r) => r.key === key);
    await until(() => v.get().rules.data, 'the rules');
    expect(row('agent_cap')).toMatchObject({ set: false });
    expect(v.get().rules.data).toMatchObject({ textBytes: 812, limitBytes: 2048 });

    await v.get().set.mutateAsync({ key: 'agent_cap', value: 4 });
    await until(() => row('agent_cap')?.set, 'the machine default');
    expect(row('agent_cap')).toMatchObject({ value: 4, source: 'machine', on: true });

    await v.get().set.mutateAsync({ workspace: ws.key, key: 'agent_cap', value: 2 });
    await until(() => row('agent_cap')?.source === 'workspace', "the workspace's own value");
    expect(row('agent_cap')).toMatchObject({ value: 2, on: true });
  });
});
