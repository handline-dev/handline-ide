// The frame, fitted whole to the pane, with the agent's marks over it; while the person holds it, their pointer, wheel and keys go in.
import { useScreenInput, type Watched } from '@handline/app-data';
import type { ScreenSource } from '@handline/client';
import { useRef, type PointerEvent, type WheelEvent } from 'react';
import { useSize } from '#src/ui/useSize.ts';
import { fitOf, keyOf, mouseOf, toFrame } from '#src/tools/screenInput.ts';
import { ScreenMarks } from '#src/tools/ScreenMarks.tsx';

const MOVE_MS = 50; // pointer moves sent at most this often

export function ScreenFrameView({
  agent,
  source,
  watched,
  holds,
}: {
  agent: string;
  source: ScreenSource;
  watched: Watched;
  holds: boolean;
}) {
  const box = useRef<HTMLDivElement>(null);
  const size = useSize(box, (el) => ({ width: el.clientWidth, height: el.clientHeight }), null as { width: number; height: number } | null);
  const lastMove = useRef(0);
  const { frame } = watched;
  const send = useScreenInput(agent, source);
  const at = (e: PointerEvent | WheelEvent, clamp = false) => {
    const r = e.currentTarget.getBoundingClientRect();
    return frame ? toFrame(e.clientX - r.left, e.clientY - r.top, frame.width, frame.height, r.width, r.height, clamp) : null;
  };
  function pointer(action: 'move' | 'down' | 'up', e: PointerEvent) {
    if (!holds) return;
    const p = at(e, action === 'up'); // a release past the edge still lets go of the button
    if (!p) return;
    if (action === 'move') {
      if (e.timeStamp - lastMove.current < MOVE_MS) return;
      lastMove.current = e.timeStamp;
    }
    if (action === 'down') {
      (e.currentTarget as HTMLElement).focus();
      e.currentTarget.setPointerCapture(e.pointerId); // its release comes here even outside the view
    }
    send(mouseOf(action, p, e)); // a touch's down and up are a click
  }
  const fit = frame && size ? fitOf(frame.width, frame.height, size.width, size.height) : { scale: 1, left: 0, top: 0 };
  return (
    <div
      ref={box}
      tabIndex={holds ? 0 : -1}
      aria-label="Screen"
      className={`relative min-h-0 min-w-0 flex-1 touch-none overflow-hidden bg-surface outline-none ${holds ? 'cursor-default' : ''}`}
      onPointerMove={(e) => pointer('move', e)}
      onPointerDown={(e) => pointer('down', e)}
      onPointerUp={(e) => pointer('up', e)}
      onContextMenu={(e) => holds && e.preventDefault()}
      onWheel={(e) => {
        const p = holds && at(e);
        if (p) send({ kind: 'wheel', ...p, dx: Math.round(e.deltaX), dy: Math.round(e.deltaY) });
      }}
      onKeyDown={(e) => {
        if (!holds || e.nativeEvent.isComposing) return;
        if ((e.metaKey || e.ctrlKey) && e.key === 'v') return; // the paste below sends the text
        e.preventDefault();
        send(keyOf('down', e));
      }}
      onKeyUp={(e) => holds && send(keyOf('up', e))}
      onPaste={(e) => {
        const text = e.clipboardData.getData('text');
        if (holds && text) send({ kind: 'text', text });
      }}
    >
      {frame && (
        <img
          alt=""
          draggable={false}
          src={`data:image/jpeg;base64,${frame.data}`}
          className="pointer-events-none absolute select-none"
          style={{ left: fit.left, top: fit.top, width: frame.width * fit.scale, height: frame.height * fit.scale }}
        />
      )}
      {!frame && (
        <span className="absolute inset-0 grid place-items-center text-xs text-muted">
          {watched.failed || (watched.ended ? 'Closed' : '')}
        </span>
      )}
      {frame && !holds && <ScreenMarks action={watched.action} fit={fit} />}
    </div>
  );
}
