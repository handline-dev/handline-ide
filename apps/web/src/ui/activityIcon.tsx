import type { Activity } from '#src/layout/agentTree.ts';
import { Circle, CircleAlert, CircleHelp, Clock, LoaderCircle } from 'lucide-react';
import type { ReactNode } from 'react';

/** An agent's mark, in the sidebar row and at its chat's top: red only for a real fault; a wait by design is a calm clock. */
export const activityIcon: Record<Activity, ReactNode> = {
  // An arc turning at a steady size.
  working: <LoaderCircle size={14} strokeWidth={3.5} className="animate-spin text-success" aria-label="working" />,
  // Asleep is the hollow ring of the resting dot.
  asleep: <Circle size={12} strokeWidth={3.5} className="text-muted" aria-label="asleep" />,
  you: <CircleHelp size={16} className="text-warning" aria-label="needs you" />,
  held: <Clock size={14} className="text-muted" aria-label="waiting" />,
  error: <CircleAlert size={16} className="text-danger" aria-label="needs attention" />,
  resting: <Circle size={12} className="fill-muted text-muted" aria-label="resting" />,
};
