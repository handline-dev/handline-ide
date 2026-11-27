// How a seated person moves, from what the agents list already says: the tool of its newest call (the first word of AgentView.doing's
// text) while it works, and its state otherwise. One reading sways along the lines, one writing types, one running something leans back
// and rocks, one waiting for the founder fidgets. The motion is a CSS animation on the figure (Cast.tsx), so nothing runs per frame here.
import type { Activity } from '#src/layout/agentList.ts';

export type Motion = 'reading' | 'writing' | 'running' | 'waiting';

/** Tools that look at things without changing them. */
const READS = new Set(['Read', 'Grep', 'Glob', 'LS', 'WebFetch', 'WebSearch', 'NotebookRead', 'ToolSearch']);
/** Tools that change files. */
const WRITES = new Set(['Edit', 'Write', 'MultiEdit', 'NotebookEdit']);

/** The motion of one doing `activity`, its newest call's line `doing` (null when no turn runs or it has called no tool yet); null sits
 *  still. A turn with no call yet is thinking, read as reading; background work at the prompt, or any other tool (a command, a
 *  subagent), is running. */
export function motionOf(activity: Activity, doing: string | null | undefined, background = false): Motion | null {
  if (activity === 'you') return 'waiting';
  if (activity !== 'working') return null;
  if (!doing && background) return 'running';
  const tool = doing?.trim().split(/\s/, 1)[0] ?? '';
  // An MCP tool's name says which server: mcp__<server>__<tool>; its own name is what it does.
  const name = tool.startsWith('mcp__') ? tool.slice(tool.lastIndexOf('__') + 2) : tool;
  if (!name || READS.has(name) || /^(read|get|list|search|find|query|fetch)/i.test(name)) return 'reading';
  if (WRITES.has(name) || /^(write|edit|update|create)/i.test(name)) return 'writing';
  return 'running';
}

/** The CSS animation of each motion, in picture pixels (a seated person shows about 17 over its chair), turning on the neck so what the
 *  chair hides stays hidden. Writing bobs as it types; reading leans in and tilts along a line, then back to the next; running sits
 *  back and swivels its chair while it watches; waiting sits still and now and then looks round over its shoulder. Its screen shows
 *  the same (monitors.tsx). */
export const MOTION_CSS = [
  '@keyframes town-writing{0%,100%{translate:0 0}50%{translate:0 1px}}',
  '@keyframes town-reading{0%{translate:0 0.8px;rotate:-5deg}75%{translate:0 0.8px;rotate:5deg}85%,100%{translate:0 0.8px;rotate:-5deg}}',
  '@keyframes town-running{0%,100%{translate:-1px -0.6px}50%{translate:1px -0.6px}}',
  '@keyframes town-waiting{0%,60%,100%{rotate:0deg;translate:0 0}66%,84%{rotate:-9deg;translate:-0.5px -0.5px}90%{rotate:0deg;translate:0 0}}',
  '@media (prefers-reduced-motion: reduce){.town-motion{animation:none!important}}',
].join('');

const TIMING: Record<Motion, string> = {
  writing: '700ms steps(2, jump-none) infinite',
  reading: '2400ms linear infinite',
  running: '2200ms ease-in-out infinite',
  waiting: '3200ms ease-in-out infinite',
};

/** A seated figure's style for its motion, turning on `neck` (picture pixels); `delay` (seconds) keeps a room from moving in step. */
export const motionStyle = (motion: Motion, [x, y]: [number, number], delay: number): React.CSSProperties => ({
  // The delay inside the shorthand: set on its own, a change of motion would reset it and the room would move in step.
  animation: `town-${motion} ${TIMING[motion]} ${-delay}s`,
  transformBox: 'view-box',
  transformOrigin: `${x}px ${y}px`,
});
