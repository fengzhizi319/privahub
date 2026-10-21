import type { Dictionary } from '../dictionaries';

/** Deep-merge `extra` into a copy of `base`; leaf strings in `extra` win. */
export function mergeDictionaries(base: Dictionary, ...extras: Dictionary[]): Dictionary {
  const out: Dictionary = { ...base };
  for (const extra of extras) {
    for (const [key, value] of Object.entries(extra)) {
      const prev = out[key];
      if (typeof value === 'object' && value !== null && typeof prev === 'object' && prev !== null) {
        out[key] = mergeDictionaries(prev, value);
      } else {
        out[key] = value;
      }
    }
  }
  return out;
}
