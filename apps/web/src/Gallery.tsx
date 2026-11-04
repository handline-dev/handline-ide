import { Copy, Ellipsis, Pencil, Plus, Settings, Trash2, X } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Menu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { Popover } from '#src/ui/Popover.tsx';
import { Button } from '#src/ui/Button.tsx';
import { alert, confirm, ModalFrame, openModal, prompt } from '#src/ui/modal.tsx';
import { Tab, TabBar, type TabItem } from '#src/ui/Tabs.tsx';

const roles = ['bg', 'surface', 'hover', 'selected', 'border', 'fg', 'muted', 'accent', 'accent-fg', 'danger', 'warning', 'success'];
// Spelled out so Tailwind sees each class.
const swatch: Record<string, string> = {
  bg: 'bg-bg',
  surface: 'bg-surface',
  hover: 'bg-hover',
  selected: 'bg-selected',
  border: 'bg-border',
  fg: 'bg-fg',
  muted: 'bg-muted',
  accent: 'bg-accent',
  'accent-fg': 'bg-accent-fg',
  danger: 'bg-danger',
  warning: 'bg-warning',
  success: 'bg-success',
};

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-medium text-muted">{title}</h2>
      {children}
    </section>
  );
}

function Sheet() {
  const [tabs, setTabs] = useState<TabItem[]>([
    { id: 'home', title: 'Home', closable: false },
    { id: 'a', title: 'Agent handline' },
    { id: 'b', title: 'A long tab title that truncates' },
  ]);
  const [active, setActive] = useState<string | null>('a');
  const noop = () => {};
  const [result, setResult] = useState('-');
  const show = (p: Promise<unknown>) => void p.then((v) => setResult(String(v)));
  return (
    <div className="flex flex-col gap-8 bg-bg p-6 text-fg">
      <Section title="Color roles">
        <div className="flex flex-wrap gap-3">
          {roles.map((r) => (
            <div key={r} className="flex flex-col items-center gap-1">
              <div className={`size-10 rounded-md border border-border ${swatch[r]}`} />
              <span className="text-xs text-muted">{r}</span>
            </div>
          ))}
        </div>
      </Section>
      <Section title="Type">
        <p className="text-lg">lg 16 - 다람쥐 헌 쳇바퀴</p>
        <p className="text-base">base 14 - 다람쥐 헌 쳇바퀴</p>
        <p className="text-sm">sm 13 - 다람쥐 헌 쳇바퀴</p>
        <p className="text-xs">xs 11 - 다람쥐 헌 쳇바퀴</p>
      </Section>
      <Section title="IconButton">
        <div className="flex items-center gap-2">
          <IconButton label="md" onClick={noop}>
            <Plus size={16} />
          </IconButton>
          <IconButton label="sm" size="sm" onClick={noop}>
            <X size={12} />
          </IconButton>
        </div>
      </Section>
      <Section title="Tab: active / inactive / pinned">
        <div className="flex gap-1 bg-surface p-2">
          <Tab tab={{ id: '1', title: 'Active' }} active onSelect={noop} onClose={noop} />
          <Tab tab={{ id: '2', title: 'Inactive' }} active={false} onSelect={noop} onClose={noop} />
          <Tab tab={{ id: '3', title: 'Pinned', closable: false }} active={false} onSelect={noop} onClose={noop} />
          <Tab tab={{ id: '4', title: 'Pinned active', closable: false }} active onSelect={noop} onClose={noop} />
        </div>
      </Section>
      <Section title="Popover / Menu">
        <div className="flex items-center gap-2">
          <Popover
            trigger={
              <IconButton label="Settings">
                <Settings size={16} />
              </IconButton>
            }
          >
            <p className="w-56">팝오버 본문. 바깥 클릭이나 Esc로 닫힙니다.</p>
          </Popover>
          <Menu
            trigger={
              <IconButton label="More">
                <Ellipsis size={16} />
              </IconButton>
            }
          >
            <MenuItem icon={<Pencil size={14} />}>Rename</MenuItem>
            <MenuItem icon={<Copy size={14} />}>Duplicate</MenuItem>
            <MenuItem disabled>Disabled</MenuItem>
            <MenuSeparator />
            <MenuItem icon={<Trash2 size={14} />} danger>
              Delete
            </MenuItem>
          </Menu>
        </div>
      </Section>
      <Section title="Button">
        <div className="flex items-center gap-2">
          <Button variant="primary">Primary</Button>
          <Button>Secondary</Button>
          <Button variant="danger">Danger</Button>
          <Button variant="soft">Soft</Button>
          <Button variant="ghost">Ghost</Button>
          <Button disabled>Disabled</Button>
          <Button size="lg" variant="soft">
            Large
          </Button>
        </div>
      </Section>
      <Section title="Modal (await → result)">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            onClick={() =>
              show(confirm({ title: 'Delete this tab?', message: 'It cannot be undone.', confirmText: 'Delete', danger: true }))
            }
          >
            confirm
          </Button>
          <Button onClick={() => show(prompt({ title: 'Rename', initial: 'Agent handline' }))}>prompt</Button>
          <Button onClick={() => show(alert({ title: 'Saved' }).then(() => 'alert closed'))}>alert</Button>
          <Button
            onClick={() =>
              show(
                openModal<string>(
                  (close) => (
                    <ModalFrame
                      title="Unsaved changes guard"
                      description="Esc/backdrop ask first (canClose)."
                      onClose={() => close()}
                      footer={
                        <Button variant="primary" onClick={() => close('saved')}>
                          Save
                        </Button>
                      }
                    >
                      <Button onClick={() => show(confirm({ title: 'Stacked confirm' }))}>Open a stacked modal</Button>
                    </ModalFrame>
                  ),
                  { canClose: () => confirm({ title: 'Discard changes?', confirmText: 'Discard', danger: true }) },
                ),
              )
            }
          >
            custom + guard + stack
          </Button>
          <span className="text-sm text-muted">result: {result}</span>
        </div>
      </Section>
      <Section title="TabBar (live)">
        <TabBar
          tabs={tabs}
          active={active}
          onSelect={setActive}
          onClose={(id) => setTabs((ts) => ts.filter((t) => t.id !== id))}
          newTabKinds={[
            { kind: 'agent', label: 'Agent' },
            { kind: 'task', label: 'Task' },
          ]}
          onAdd={(kind) => {
            const id = crypto.randomUUID();
            setTabs((ts) => [...ts, { id, title: kind }]);
            setActive(id);
          }}
        />
      </Section>
    </div>
  );
}

/** Every token and component in every state. Open /gallery; the toggle flips <html> to dark, which
 * floating panels (portaled to <body>) follow too. */
export function Gallery() {
  const [dark, setDark] = useState(false);
  return (
    <div className="min-h-full bg-bg">
      <div className="flex justify-end p-2">
        <button
          type="button"
          className="rounded-md px-2 py-1 text-sm text-muted hover:bg-hover hover:text-fg"
          onClick={() => {
            document.documentElement.classList.toggle('dark', !dark);
            setDark(!dark);
          }}
        >
          {dark ? 'Light' : 'Dark'}
        </button>
      </div>
      <Sheet />
    </div>
  );
}
