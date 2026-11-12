import { Globe, Hammer, Plug, Smartphone } from 'lucide-react';

const ICON = { heavy: Hammer, simulator: Smartphone, browser: Globe, port: Plug } as const;

/** The resource a task waits in line for, as an icon before its words; a kind the app does not know has none. */
export function LeaseIcon({ kind, size = 12 }: { kind: string; size?: number }) {
  const Icon = ICON[kind as keyof typeof ICON];
  return Icon ? <Icon size={size} className="inline shrink-0" /> : null;
}
