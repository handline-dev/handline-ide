import { Ago } from '#src/ui/Ago.tsx';
import type { ActivityItem } from '@handline/app-core';
import { AnimatePresence, motion } from 'framer-motion';
import { Check } from 'lucide-react';
import { Who } from '#src/ui/Who.tsx';
import { spring } from '#src/motion.ts';

/** Under a founder's comment: each agent it was told to that read it, and when ("legacy read it 09:34"); one still waiting says so.
 *  A line that comes or goes opens or closes its height. */
export function ReadBy({ readers }: { readers?: ActivityItem['readers'] }) {
  return (
    <div className="flex flex-col text-xs text-muted">
      <AnimatePresence initial={false}>
        {(readers ?? []).map((r) => (
          <motion.span
            key={r.agent}
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={spring}
            className="overflow-hidden"
          >
            <span className="flex items-center gap-1 pt-1">
              {r.state === 'read' && <Check size={12} className="text-success" />}
              <Who k={r.agent} />
              {r.state === 'read' ? (
                <>
                  {' read it'}
                  {r.at && (
                    <>
                      {' '}
                      <Ago at={r.at} />
                    </>
                  )}
                </>
              ) : r.state === 'held' ? (
                ' gets it with its next digest'
              ) : (
                ' has not read it yet'
              )}
            </span>
          </motion.span>
        ))}
      </AnimatePresence>
    </div>
  );
}
