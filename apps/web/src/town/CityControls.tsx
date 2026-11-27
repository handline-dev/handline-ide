// The city's zoom buttons, in a corner of its pane: the whole town or back to following the founder, farther and closer. The wheel and
// the M key do the same (camera.tsx, CityView.tsx).
import { Minus, Plus } from 'lucide-react';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';

const OFF = 'disabled:pointer-events-none disabled:opacity-40';

export function CityControls({
  level,
  onStep,
  onToggle,
}: {
  /** How far in the city is zoomed: 0 the whole town, 1 the close-up. */
  level: number;
  onStep: (dir: 1 | -1) => void;
  onToggle: () => void;
}) {
  const whole = level === 0;
  return (
    <div className="absolute right-3 bottom-3 z-10 flex items-center gap-0.5 rounded-lg border border-border bg-bg p-1 shadow-popover">
      <Button variant="ghost" onClick={onToggle} title={whole ? 'Back to the close-up on you (M)' : 'See the whole town (M)'}>
        {whole ? 'Follow me' : 'Whole town'}
      </Button>
      <span className="mx-0.5 h-4 w-px bg-border" />
      <IconButton label="Zoom out" className={OFF} disabled={whole} onClick={() => onStep(-1)}>
        <Minus size={16} />
      </IconButton>
      <IconButton label="Zoom in" className={OFF} disabled={level === 1} onClick={() => onStep(1)}>
        <Plus size={16} />
      </IconButton>
    </div>
  );
}
