import { AnimatePresence, motion, useReducedMotion } from 'framer-motion';
import { useEffect, useState, type ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame } from '#src/ui/modal.tsx';

export type StepAction = { label: string; onClick: () => void; disabled?: boolean };
export type Step = {
  body: ReactNode;
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
  const [height, setHeight] = useState<number | 'auto'>('auto');
  const [seen, setSeen] = useState(step);
  const [dir, setDir] = useState(1);
  if (step !== seen) {
    setSeen(step);
    setDir(step > seen ? 1 : -1);
  }
  useEffect(() => {
    if (!page) return;
    const ro = new ResizeObserver(() => setHeight(page.offsetHeight));
    ro.observe(page);
    return () => ro.disconnect();
  }, [page]);
  const { back, next } = steps[step]!;
  const x = still ? 0 : 32;
  const duration = still ? 0 : 0.15;
  return (
    <ModalFrame
      title={title}
      description={description}
      size={size}
      onClose={onClose}
      footer={
        <div className="flex w-full justify-between gap-2">
          {back ? (
            <Button onClick={back.onClick} disabled={back.disabled}>
              {back.label}
            </Button>
          ) : (
            <span />
          )}
          {next && (
            <Button variant="primary" onClick={next.onClick} disabled={next.disabled}>
              {next.label}
            </Button>
          )}
        </div>
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
            {steps[step]!.body}
          </motion.div>
        </AnimatePresence>
      </motion.div>
    </ModalFrame>
  );
}
