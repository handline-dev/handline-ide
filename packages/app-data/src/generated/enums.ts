// Generated from contract/spec/ by contract/generate.py; do not edit.
export type StateWord = "running" | "idle" | "queued" | "starting" | "parked" | "waiting" | "stalled" | "blocked" | "no_terminal";
export type StatusState = "preparing" | "starting" | "running" | "waiting" | "stopped" | "idle" | "off" | "done";
export type StatusReason = "work" | "restart" | "retry" | "launch" | "turn" | "watch" | "handoff" | "background" | "you" | "opener" | "task" | "owner" | "clock" | "move" | "load" | "pace" | "paused" | "wall" | "cap" | "stalled" | "none" | "done" | "discarded";
