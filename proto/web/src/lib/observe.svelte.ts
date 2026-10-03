import type { Observable } from "rxjs";

/** Mirrors the latest value of an observable into reactive state. `source` is
 *  tracked, so the subscription follows when its dependencies change. */
export function observe<T, I = T>(source: () => Observable<T> | undefined, initial: I): { readonly current: T | I } {
  let value = $state<T | I>(initial);
  $effect(() => {
    const sub = source()?.subscribe((v) => (value = v));
    return () => sub?.unsubscribe();
  });
  return {
    get current() {
      return value;
    },
  };
}
