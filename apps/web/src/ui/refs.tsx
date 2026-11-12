// Daemon text names agents as <@key> tokens; Markdown renders them, and plain-text spots go through these.
import { renderRefs } from '@handline/app-core';
import { useDirectory } from '@handline/app-data';
import { useCallback } from 'react';

/** Writes each <@key> in a daemon string as the name it stands for. */
export function useRefs(): (text: string) => string {
  const dir = useDirectory();
  return useCallback((text: string) => renderRefs(dir, text), [dir]);
}

/** A daemon string with its <@key> tokens as names. */
export function Refs({ text }: { text: string | null | undefined }) {
  const refs = useRefs();
  return text ? <>{refs(text)}</> : null;
}
