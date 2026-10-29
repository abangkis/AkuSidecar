// Private operational brands, independent of diagnostic JSON and exception text.
// Only trusted acquisition/transport code may mark a known transient failure.
const transient = new WeakMap();
export function markSourceTransient(error, kind) {
  if (error && typeof error === 'object' && ['timeout', 'network'].includes(kind)) transient.set(error, kind);
  return error;
}
export const sourceTransientClass = error => error && typeof error === 'object' ? transient.get(error) ?? '' : '';
export const SOURCE_RETRY_LIMIT = 2;
