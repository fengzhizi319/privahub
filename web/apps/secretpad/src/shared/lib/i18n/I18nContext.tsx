import React, { useEffect, useMemo, useState } from 'react';
import type { Dictionary, Locale } from './dictionaries';
import { dictionaries } from './dictionaries';
import { I18nContext } from './use-translation';

const STORAGE_KEY = 'secretpad-locale';

function getInitialLocale(): Locale {
  if (typeof localStorage !== 'undefined') {
    const stored = localStorage.getItem(STORAGE_KEY) as Locale | null;
    if (stored && dictionaries[stored]) return stored;
  }
  if (typeof navigator !== 'undefined' && navigator.language.startsWith('zh')) {
    return 'zh-CN';
  }
  return 'en-US';
}


function getValue(dict: Dictionary, key: string): string | undefined {
  const parts = key.split('.');
  let current: Dictionary | string | undefined = dict;
  for (const part of parts) {
    if (typeof current !== 'object' || current === null) return undefined;
    current = current[part];
  }
  return typeof current === 'string' ? current : undefined;
}

export const I18nProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [locale, setLocaleState] = useState<Locale>(() => getInitialLocale());

  useEffect(() => {
    document.documentElement.lang = locale === 'zh-CN' ? 'zh' : 'en';
  }, [locale]);

  const setLocale = (next: Locale) => {
    setLocaleState(next);
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, next);
    }
  };

  const t = useMemo(() => {
    return (key: string, params?: Record<string, string | number>): string => {
      const dict = dictionaries[locale];
      let text = getValue(dict, key) ?? getValue(dictionaries['en-US'], key) ?? key;
      if (params) {
        Object.entries(params).forEach(([k, v]) => {
          text = text.replace(new RegExp(`\\{\\{${k}\\}\\}`, 'g'), String(v));
        });
      }
      return text;
    };
  }, [locale]);

  return (
    <I18nContext.Provider value={{ locale, setLocale, t }}>
      {children}
    </I18nContext.Provider>
  );
};
