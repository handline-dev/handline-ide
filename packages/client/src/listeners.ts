/** A set of handlers for one kind of notice: add returns the undo, emit calls each in turn. */
export class Listeners<T> {
  private fns = new Set<(value: T) => void>();
  add(fn: (value: T) => void): () => void {
    this.fns.add(fn);
    return () => {
      this.fns.delete(fn);
    };
  }
  emit(value: T): void {
    for (const fn of this.fns) fn(value);
  }
}
