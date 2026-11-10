import type { Activity } from '#src/layout/agentTree.ts';
import { Circle, CircleAlert, CircleHelp, Clock, LoaderCircle } from 'lucide-react';
import type { ReactNode } from 'react';

const LABEL: Record<Activity, string> = {
  working: 'Working',
  waking: 'Starting',
  asleep: 'Asleep',
  you: 'Needs you',
  held: 'Waiting',
  error: 'Needs attention',
  resting: 'On',
};

/** An agent's mark, in the sidebar row and at its chat's top: red only for a real fault; a wait by design is a calm clock. */
const activityIcon: Record<Activity, ReactNode> = {
  // An arc turning at a steady size.
  working: <LoaderCircle size={14} strokeWidth={3.5} className="animate-spin text-success" />,
  // Starting: the asleep ring, turning.
  waking: <Circle size={12} strokeWidth={3.5} strokeDasharray="3 4.85" className="spin-slow text-success" />,
  // Asleep is a dashed ring; on and resting, a solid one.
  asleep: <Circle size={12} strokeWidth={3.5} strokeDasharray="3 4.85" className="text-muted" />,
  you: <CircleHelp size={16} className="text-warning" />,
  held: <Clock size={14} className="text-muted" />,
  error: <CircleAlert size={16} className="text-danger" />,
  resting: <Circle size={12} strokeWidth={3.5} className="text-muted" />,
};

/** The mark, with its state's name on hover and for a screen reader. */
export function ActivityIcon({ act }: { act: Activity }) {
  return (
    <span title={LABEL[act]} role="img" aria-label={LABEL[act]} className="inline-grid place-items-center">
      {activityIcon[act]}
    </span>
  );
}
