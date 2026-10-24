import { describe, expect, it } from 'vitest';
import type { AppSetting } from './api/types';
import { getMVForgePreferences } from './mvforgePreferences';

describe('MVForge preferred track languages', () => {
  it('normalizes aliases in order when settings are reloaded', () => {
    const settings = [{ key: 'mvforgePreferences', value: {
      preferredLanguages: ['KO', 'kor', 'fr-FR', 'fre', 'EN', 'fr-CA', 'UND'],
    } }] as unknown as AppSetting[];

    expect(getMVForgePreferences(settings).preferredLanguages).toEqual(['kor', 'fra', 'eng', 'fr-ca', 'und']);
  });
});
