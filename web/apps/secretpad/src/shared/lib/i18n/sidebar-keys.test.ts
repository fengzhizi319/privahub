import { describe, expect, it } from 'vitest';
import { getMenu } from '../access';
import { toPlatformContext, type PlatformUser } from '../platform';
import { dictionaries, type Locale } from './dictionaries';

/** Resolves a dotted key ("sidebar.dashboard") in a dictionary. */
function lookup(locale: Locale, key: string): unknown {
  return key.split('.').reduce<unknown>(
    (node, part) => (node && typeof node === 'object' ? (node as Record<string, unknown>)[part] : undefined),
    dictionaries[locale],
  );
}

const users: PlatformUser[] = [
  { platformType: 'CENTER', ownerType: 'CENTER', ownerId: 'kuscia-system' },
  { platformType: 'CENTER', ownerType: 'EDGE', ownerId: 'alice' },
  { platformType: 'EDGE', ownerType: 'EDGE', ownerId: 'alice' },
  { platformType: 'AUTONOMY', ownerType: 'P2P', ownerId: 'inst-a' },
  { platformType: 'P2P', ownerId: 'bob' },
  { platformType: 'TEST' },
];

// Regression: sidebar.privacyScenes had no translation, so the sidebar showed
// the raw key "sidebar.privacyScenes" in both locales.
describe('sidebar menu labels', () => {
  const keys = new Set<string>();
  for (const u of users) {
    for (const item of getMenu(toPlatformContext(u))) {
      if (item.labelKey) keys.add(item.labelKey);
      if (item.section) keys.add(item.section);
    }
  }
  for (const locale of ['zh-CN', 'en-US'] as Locale[]) {
    it(`every menu key is translated in ${locale}`, () => {
      const missing = [...keys].filter((k) => typeof lookup(locale, k) !== 'string');
      expect(missing).toEqual([]);
    });
  }
});
