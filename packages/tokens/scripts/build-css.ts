// Writes apps/web/legacy/src/tokens.css from the token values.
import { writeFileSync } from 'node:fs';
import { tokensCss } from '#src/css.ts';

writeFileSync(new URL('../../../apps/web/legacy/src/tokens.css', import.meta.url), tokensCss());
