// A write that names what it writes: `pending` is a key in flight (its row dims); the promise rejects with the refusal as a sentence.
import { classify, sentence } from '@handline/app-data';
import { useCallback, useState } from 'react';

export function useKeyed() {
  const [flying, setFlying] = useState<string[]>([]);
  const run = useCallback(async <T>(key: string, call: () => Promise<T>): Promise<T> => {
    setFlying((f) => [...f, key]);
    try {
      return await call();
    } catch (e) {
      throw sentence(classify(e));
    } finally {
      setFlying((f) => f.filter((_, i) => i !== f.indexOf(key)));
    }
  }, []);
  return { pending: flying.at(-1) ?? null, run };
}
