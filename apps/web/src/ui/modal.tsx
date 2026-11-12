import { Dialog } from '@base-ui/react/dialog';
import { ArrowLeft, X } from 'lucide-react';
import { useState, useSyncExternalStore, type ReactNode } from 'react';
import { useWindowLayer } from '#src/layout/windowLayer.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { useBackCloses } from '#src/ui/backCloses.ts';
import { useNarrow } from '#src/ui/narrow.ts';

/** A modal is a full-screen layer: nothing under it moves the window. */
function Layer({ open, onBack }: { open: boolean; onBack: () => void }) {
  useWindowLayer(open);
  useBackCloses(open, onBack);
  return null;
}

/** Promise-based modals: `const ok = await confirm({ ... })`. openModal works from anywhere (a handler, a
 * plain function) - the store lives here, and one <ModalHost /> at the root draws it. */

export type Close<R> = (value?: R) => void;

export type ModalOptions = {
  /** Esc and backdrop click close it (resolving undefined). Default true. */
  dismissible?: boolean;
  /** Asked before a dismissal (Esc, backdrop, close() with no value); false keeps the modal open - an unsaved-changes guard. A close with a result skips it. */
  canClose?: () => boolean | Promise<boolean>;
  /** Aborting closes the modal from outside, resolving undefined. */
  signal?: AbortSignal;
};

type Entry = {
  id: number;
  node: ReactNode;
  open: boolean;
  options: ModalOptions;
  resolve: (value: unknown) => void;
  /** Drops the abort listener once the modal is closed. */
  detach: () => void;
};

/** Default button text, in one place so a locale can replace it. */
export const modalText = { ok: 'OK', cancel: 'Cancel', close: 'Close' };

let entries: Entry[] = [];
let nextId = 0;
const listeners = new Set<() => void>();
const set = (next: Entry[]) => {
  entries = next;
  listeners.forEach((l) => l());
};
// Ids whose canClose is being asked: a second close meanwhile is ignored, so the guard shows once.
const guarding = new Set<number>();
const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

async function close(id: number, value: unknown, force = false) {
  const e = entries.find((x) => x.id === id);
  if (!e?.open || (guarding.has(id) && !force)) return;
  if (!force && value === undefined && e.options.canClose) {
    guarding.add(id);
    const ok = await Promise.resolve()
      .then(e.options.canClose)
      .finally(() => guarding.delete(id));
    if (!ok) return;
    if (!entries.find((x) => x.id === id)?.open) return; // closed another way while asking
  }
  e.detach();
  e.resolve(value);
  // The entry stays mounted, closed, until its exit motion ends (onOpenChangeComplete).
  set(entries.map((x) => (x.id === id ? { ...x, open: false } : x)));
}

/** Opens `render(close)` as a modal; resolves with the value passed to close, or undefined when dismissed. */
export function openModal<R>(render: (close: Close<R>) => ReactNode, options: ModalOptions = {}): Promise<R | undefined> {
  const id = ++nextId;
  if (listeners.size === 0) console.error('openModal: no <ModalHost /> is mounted, so this modal never shows.');
  return new Promise((resolve) => {
    const node = render((value) => void close(id, value));
    const { signal } = options;
    const onAbort = () => void close(id, undefined, true);
    signal?.addEventListener('abort', onAbort, { once: true });
    const detach = () => signal?.removeEventListener('abort', onAbort);
    set([...entries, { id, node, open: true, options, resolve: resolve as (v: unknown) => void, detach }]);
    if (signal?.aborted) onAbort();
  });
}

export function ModalHost() {
  const list = useSyncExternalStore(subscribe, () => entries);
  const narrow = useNarrow();
  const top = [...list].reverse().find((e) => e.open)?.id;
  return list.map((e) => (
    <Dialog.Root
      key={e.id}
      open={e.open}
      disablePointerDismissal={e.options.dismissible === false}
      onOpenChange={(open) => {
        // Only the top modal takes Esc and outside clicks; one under it would see the top's backdrop as outside.
        if (!open && e.id === top && e.options.dismissible !== false) void close(e.id, undefined);
      }}
      onOpenChangeComplete={(open) => {
        if (!open) set(entries.filter((x) => x.id !== e.id));
      }}
    >
      <Dialog.Portal>
        {/* Back closes it as Esc would; one that must be answered stays. */}
        <Layer open={e.open} onBack={() => (e.options.dismissible === false ? undefined : void close(e.id, undefined))} />
        <Dialog.Backdrop className="fixed inset-0 z-50 bg-fg/40 transition-opacity duration-150 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
        <Dialog.Viewport className={`fixed inset-0 z-50 flex items-center justify-center ${narrow ? '' : 'p-4'}`}>{e.node}</Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  ));
}

const widths = { sm: 'max-w-sm', md: 'max-w-md', lg: 'max-w-lg', xl: 'max-w-2xl' };

/** The one modal frame: title, optional description, body, footer actions, close button. */
export function ModalFrame({
  title,
  description,
  size = 'md',
  onClose,
  footer,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  size?: keyof typeof widths;
  /** Shows the X; leave out for a modal that must be answered. */
  onClose?: () => void;
  footer?: ReactNode;
  children?: ReactNode;
}) {
  // On a phone-sized window every modal fills it.
  if (useNarrow())
    return (
      <Dialog.Popup className="flex size-full flex-col bg-bg text-fg outline-none transition-opacity duration-150 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0">
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          {onClose ? (
            <IconButton label={modalText.close} onClick={onClose}>
              <ArrowLeft size={16} />
            </IconButton>
          ) : (
            <span className="size-7" />
          )}
          <Dialog.Title className="min-w-0 flex-1 truncate text-center text-sm font-semibold">{title}</Dialog.Title>
          <span className="size-7" />
        </div>
        <ScrollFade className="scrollbar-auto min-h-0 flex-1 px-4 pt-3 pb-4 text-sm">
          {description && <Dialog.Description className="pb-3 text-sm text-muted">{description}</Dialog.Description>}
          {children}
        </ScrollFade>
        {footer && <div className="flex gap-2 border-t border-border p-4 [&>*]:flex-1 [&>*]:justify-center">{footer}</div>}
      </Dialog.Popup>
    );
  return (
    <Dialog.Popup
      className={`flex max-h-full w-full flex-col rounded-md border border-border bg-bg text-fg shadow-popover outline-none transition duration-150 data-[ending-style]:scale-95 data-[ending-style]:opacity-0 data-[starting-style]:scale-95 data-[starting-style]:opacity-0 ${widths[size]}`}
    >
      <div className="flex items-start gap-2 px-4 pt-4">
        <div className="flex flex-1 flex-col gap-1">
          <Dialog.Title className="text-base font-semibold">{title}</Dialog.Title>
          {description && <Dialog.Description className="text-sm text-muted">{description}</Dialog.Description>}
        </div>
        {onClose && (
          <IconButton label={modalText.close} onClick={onClose}>
            <X size={16} />
          </IconButton>
        )}
      </div>
      {/* The body scrolls with its bar showing, its padding inside it, so it scrolls to the frame's edge. */}
      {children && <ScrollFade className={`scrollbar-auto min-h-0 px-4 pt-3 text-sm ${footer ? '' : 'pb-4'}`}>{children}</ScrollFade>}
      {footer && <div className="flex justify-end gap-2 p-4">{footer}</div>}
      {!footer && !children && <div className="h-4" />}
    </Dialog.Popup>
  );
}

export function confirm({
  title,
  message,
  confirmText = modalText.ok,
  cancelText = modalText.cancel,
  danger = false,
}: {
  title: ReactNode;
  message?: ReactNode;
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
}): Promise<boolean> {
  return openModal<boolean>((close) => (
    <ModalFrame
      title={title}
      description={message}
      size="sm"
      footer={
        <>
          <Button onClick={() => close(false)}>{cancelText}</Button>
          <Button variant={danger ? 'danger' : 'primary'} autoFocus onClick={() => close(true)}>
            {confirmText}
          </Button>
        </>
      }
    />
  )).then((v) => v === true);
}

export function alert({
  title,
  message,
  okText = modalText.ok,
}: {
  title: ReactNode;
  message?: ReactNode;
  okText?: string;
}): Promise<void> {
  return openModal<void>((close) => (
    <ModalFrame
      title={title}
      description={message}
      size="sm"
      footer={
        <Button variant="primary" autoFocus onClick={() => close()}>
          {okText}
        </Button>
      }
    />
  )).then(() => undefined);
}

function PromptBody({
  initial,
  placeholder,
  submitText,
  close,
}: {
  initial: string;
  placeholder?: string;
  submitText: string;
  close: Close<string>;
}) {
  const [value, setValue] = useState(initial);
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(ev) => {
        ev.preventDefault();
        close(value);
      }}
    >
      <input
        autoFocus
        value={value}
        placeholder={placeholder}
        onChange={(ev) => setValue(ev.target.value)}
        className="h-8 rounded-md border border-border bg-bg px-2 text-sm text-fg outline-none placeholder:text-muted focus:border-accent"
      />
      <div className="flex justify-end gap-2 pb-4">
        <Button onClick={() => close()}>{modalText.cancel}</Button>
        <Button type="submit" variant="primary">
          {submitText}
        </Button>
      </div>
    </form>
  );
}

/** Resolves the text entered, or undefined when cancelled. */
export function prompt({
  title,
  message,
  initial = '',
  placeholder,
  submitText = modalText.ok,
}: {
  title: ReactNode;
  message?: ReactNode;
  initial?: string;
  placeholder?: string;
  submitText?: string;
}): Promise<string | undefined> {
  return openModal<string>((close) => (
    <ModalFrame title={title} description={message} size="sm">
      <PromptBody initial={initial} placeholder={placeholder} submitText={submitText} close={close} />
    </ModalFrame>
  ));
}
