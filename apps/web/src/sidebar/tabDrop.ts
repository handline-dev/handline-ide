import { createContext } from 'react';
import type { TabMove } from '#src/ui/dnd.tsx';

/** Moves a dragged tab to workspace `to` (an agent's key, or `page/<kind>`), dropped on its sidebar row; set by the app. */
export const TabsTo = createContext<((move: TabMove, to: string) => void) | null>(null);
