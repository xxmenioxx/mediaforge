// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ValidationPolicy } from '../api/types';
import { ValidationPolicySettingsCard } from './ValidationPolicySettingsCard';

afterEach(cleanup);

function policyValue(): ValidationPolicy {
  const defaults: Record<string, ValidationPolicy> = {
    disabled: { schemaVersion: 1, policyVersion: 'validation-policy-v1', preset: 'disabled', rules: {
      'duration.timeline': { enabled: false, disposition: 'blocking', overrideable: false, thresholds: { maxAbsoluteSeconds: 2, maxPercent: 2 }, warningPenalty: 5, failurePenalty: 25 },
    } },
    basic: { schemaVersion: 1, policyVersion: 'validation-policy-v1', preset: 'basic', rules: {
      'duration.timeline': { enabled: true, disposition: 'blocking', overrideable: true, thresholds: { maxAbsoluteSeconds: 2, maxPercent: 2 }, warningPenalty: 5, failurePenalty: 25 },
    } },
    default: { schemaVersion: 1, policyVersion: 'validation-policy-v1', preset: 'default', rules: {
      'duration.timeline': { enabled: true, disposition: 'blocking', overrideable: true, thresholds: { maxAbsoluteSeconds: 2, maxPercent: 2 }, warningPenalty: 5, failurePenalty: 25 },
    } },
    strict: { schemaVersion: 1, policyVersion: 'validation-policy-v1', preset: 'strict', rules: {
      'duration.timeline': { enabled: true, disposition: 'blocking', overrideable: true, thresholds: { maxAbsoluteSeconds: 1, maxPercent: 1 }, warningPenalty: 10, failurePenalty: 40 },
    } },
  };
  return { ...defaults.default, presetDefaults: defaults };
}

describe('ValidationPolicySettingsCard', () => {
  it('shows the recommended Default preset and explains rule values', () => {
    render(<ValidationPolicySettingsCard value={policyValue() as unknown as Record<string, unknown>} saving={false} onSave={vi.fn()} />);

    expect(screen.getByRole('combobox', { name: 'Validation Mode' }).textContent).toContain('Default');
    expect(screen.getByText('Recommended')).toBeTruthy();
    expect(screen.getByText(/recommended balance between safety, compatibility, fidelity, and false positives/i)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Advanced rule editor' })).toBeTruthy();
  });

  it('shows Pixel semantics for the canonical pixelFormat policy field', async () => {
    const value = policyValue();
    const videoRule = {
      enabled: true, disposition: 'blocking' as const, overrideable: true,
      fields: { pixelFormat: true, bitDepth: true }, warningPenalty: 5, failurePenalty: 25,
    };
    value.rules['video.properties'] = videoRule;
    Object.values(value.presetDefaults ?? {}).forEach((preset) => {
      if (preset) preset.rules['video.properties'] = videoRule;
    });
    const user = userEvent.setup();
    render(<ValidationPolicySettingsCard value={value as unknown as Record<string, unknown>} saving={false} onSave={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Advanced rule editor' }));
    expect(screen.getByText('Pixel semantics')).toBeTruthy();
    expect(screen.getByText('Bit depth')).toBeTruthy();
  });

  it('selects named preset defaults, transitions edits to Custom, and restores the base preset', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<ValidationPolicySettingsCard value={policyValue() as unknown as Record<string, unknown>} saving={false} onSave={onSave} />);

    await user.click(screen.getByRole('combobox', { name: 'Validation Mode' }));
    await user.click(screen.getByRole('option', { name: 'Strict' }));
    expect(screen.getByRole('combobox', { name: 'Validation Mode' }).textContent).toContain('Strict');

    await user.click(screen.getByRole('combobox', { name: 'Validation Mode' }));
    await user.click(screen.getByRole('option', { name: 'Default · Recommended' }));
    await user.click(screen.getByRole('button', { name: 'Advanced rule editor' }));
    await user.click(screen.getByRole('combobox', { name: 'Disposition' }));
    await user.click(screen.getByRole('option', { name: 'Advisory' }));

    expect(await screen.findByText(/Custom · Based on: Default · 1 value differs/i)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Show differences' }));
    expect(screen.getByText('duration · timeline · disposition')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Save Validation policy' }));
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      preset: 'custom', basedOn: 'default',
      rules: expect.objectContaining({ 'duration.timeline': expect.objectContaining({ disposition: 'advisory' }) }),
    })));

    await user.click(screen.getByRole('button', { name: 'Restore preset defaults' }));
    expect(screen.getByRole('combobox', { name: 'Validation Mode' }).textContent).toContain('Default');
    await user.click(screen.getByRole('button', { name: 'Save Validation policy' }));
    await waitFor(() => expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ preset: 'default' })));
  });
});
