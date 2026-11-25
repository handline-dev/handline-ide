import { useSize } from '#src/ui/useSize.ts';
import { AnimatePresence, motion, useReducedMotion } from 'framer-motion';
import type { LucideIcon } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame } from '#src/ui/modal.tsx';

export type StepAction = { label: string; onClick: () => void; disabled?: boolean; busy?: boolean };
export type Step = {
  body: ReactNode;
  /** The last page: a large icon over what came of it, no buttons, and the modal closes by itself a moment later. */
  finish?: { icon: LucideIcon; tone?: 'success' | 'warning' };
  /** Bottom left. */
  back?: StepAction;
  /** Bottom right: the primary button (Next, Done). */
  next?: StepAction;
};

/** A modal that is one flow: `step` picks the page; Back sits bottom left and Next bottom right. One page shows at a time,
 * the old one slides out, the new one in, and the frame grows or shrinks to the page's height. */
export function StepModal({
  title,
  description,
  size = 'lg',
  onClose,
  step,
  steps,
}: {
  title: ReactNode;
  description?: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  onClose: () => void;
  step: number;
  steps: Step[];
}) {
  const still = useReducedMotion();
  const [page, setPage] = useState<HTMLDivElement | null>(null);
  const height = useSize<HTMLDivElement, number | 'auto'>(page, (el) => el.offsetHeight, 'auto');
  const [seen, setSeen] = useState(step);
  const [dir, setDir] = useState(1);
  if (step !== seen) {
    setSeen(step);
    setDir(step > seen ? 1 : -1);
  }
  const { back, next, finish } = steps[step];
  const closing = useRef(onClose);
  useEffect(() => void (closing.current = onClose));
  const finished = !!finish;
  useEffect(() => {
    if (!finished) return;
    const t = setTimeout(() => closing.current(), FINISH_MS);
    return () => clearTimeout(t);
  }, [finished]);
  const x = still ? 0 : 32;
  const duration = still ? 0 : 0.15;
  return (
    <ModalFrame
      title={title}
      description={description}
      size={size}
      onClose={onClose}
      footer={
        finish ? undefined : (
          <div className="flex w-full justify-between gap-2">
            {back ? (
              <Button onClick={back.onClick} disabled={back.disabled}>
                {back.label}
              </Button>
            ) : (
              <span />
            )}
            {next && (
              <Button variant="primary" onClick={next.onClick} disabled={next.disabled} busy={next.busy}>
                {next.label}
              </Button>
            )}
          </div>
        )
      }
    >
      <motion.div animate={{ height }} transition={{ duration: still ? 0 : 0.2 }} className="overflow-hidden">
        {/* "wait": the old page is gone before the new one comes in, so nothing shows through. */}
        <AnimatePresence initial={false} mode="wait" custom={dir}>
          <motion.div
            key={step}
            ref={setPage}
            custom={dir}
            variants={{
              in: (d: number) => ({ x: x * d, opacity: 0 }),
              on: { x: 0, opacity: 1 },
              out: (d: number) => ({ x: -x * d, opacity: 0 }),
            }}
            initial="in"
            animate="on"
            exit="out"
            transition={{ duration }}
          >
            {finish ? <Finished {...finish}>{steps[step].body}</Finished> : steps[step].body}
          </motion.div>
        </AnimatePresence>
      </motion.div>
    </ModalFrame>
  );
}

/** How long the last page stays before the modal closes by itself. */
const FINISH_MS = 2500;

function Finished({ icon: Icon, tone = 'success', children }: { icon: LucideIcon; tone?: 'success' | 'warning'; children: ReactNode }) {
  return (
    <div role="status" className="flex flex-col items-center gap-3 py-6 text-center text-sm text-fg">
      <Icon size={48} strokeWidth={1.5} className={tone === 'success' ? 'text-success' : 'text-warning'} />
      {children}
    </div>
  );
}
