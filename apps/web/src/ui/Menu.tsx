import { ContextMenu as CM } from '@base-ui/react/context-menu';
import { Menu as M } from '@base-ui/react/menu';
import { Check, ChevronRight } from 'lucide-react';
import type { MouseEvent, ReactElement, ReactNode } from 'react';
import { floatingSurface } from '#src/ui/Popover.tsx';

/** A list of actions on the popover surface; arrow keys and type-ahead come with it. */
export function Menu({
  trigger,
  align = 'start',
  children,
}: {
  trigger: ReactElement;
  align?: 'start' | 'center' | 'end';
  children: ReactNode;
}) {
  return (
    <M.Root>
      <M.Trigger render={trigger} />
      <M.Portal>
        <M.Positioner side="bottom" align={align} sideOffset={4} collisionPadding={8} className="z-50">
          <M.Popup className={`${floatingSurface} min-w-40 p-1`}>{children}</M.Popup>
        </M.Positioner>
      </M.Portal>
    </M.Root>
  );
}

/** The same menu, opened by right-click (or long press) anywhere in `children`, at the pointer. */
export function ContextMenu({ items, children }: { items: ReactNode; children: ReactNode }) {
  return (
    <CM.Root>
      <CM.Trigger className="contents">{children}</CM.Trigger>
      <CM.Portal>
        <CM.Positioner collisionPadding={8} className="z-50">
          <CM.Popup className={`${floatingSurface} min-w-40 p-1`}>{items}</CM.Popup>
        </CM.Positioner>
      </CM.Portal>
    </CM.Root>
  );
}

export function MenuItem({
  onClick,
  icon,
  danger,
  disabled,
  checked,
  children,
}: {
  onClick?: (e: MouseEvent) => void;
  icon?: ReactNode;
  danger?: boolean;
  disabled?: boolean;
  /** Marks the current choice with a check at the end. */
  checked?: boolean;
  children: ReactNode;
}) {
  return (
    <M.Item
      onClick={onClick}
      disabled={disabled}
      className={`flex h-7 cursor-default items-center gap-2 rounded-sm px-2 text-sm outline-none select-none data-[disabled]:opacity-40 data-[highlighted]:bg-hover ${
        danger ? 'text-danger' : 'text-fg'
      }`}
    >
      {icon && <span className="grid size-4 place-items-center text-muted">{icon}</span>}
      <span className="flex-1">{children}</span>
      {checked && <Check size={14} className="text-muted" />}
    </M.Item>
  );
}

/** A nested menu inside a Menu or ContextMenu: `label` opens `children` beside it. */
export function SubMenu({ label, icon, children }: { label: ReactNode; icon?: ReactNode; children: ReactNode }) {
  return (
    <M.SubmenuRoot>
      <M.SubmenuTrigger className="flex h-7 cursor-default items-center gap-2 rounded-sm px-2 text-sm text-fg outline-none select-none data-[highlighted]:bg-hover data-[popup-open]:bg-hover">
        {icon && <span className="grid size-4 place-items-center text-muted">{icon}</span>}
        <span className="flex-1">{label}</span>
        <ChevronRight size={14} className="text-muted" />
      </M.SubmenuTrigger>
      <M.Portal>
        <M.Positioner side="right" align="start" sideOffset={2} collisionPadding={8} className="z-50">
          <M.Popup className={`${floatingSurface} min-w-40 p-1`}>{children}</M.Popup>
        </M.Positioner>
      </M.Portal>
    </M.SubmenuRoot>
  );
}

export function MenuSeparator() {
  return <M.Separator className="my-1 h-px bg-border" />;
}
