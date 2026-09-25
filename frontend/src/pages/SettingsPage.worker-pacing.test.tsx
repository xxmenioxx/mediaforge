// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { afterEach, describe, expect, it } from 'vitest';

import {
  WorkersCard,
} from './SettingsPage';
import type { SettingsForm } from './SettingsPage';
import { settingsToForm, workerProgressPercentError, workerSettingsPayload } from '../utils/settingsPageHelpers';

afterEach(cleanup);

function WorkersHarness({ initialForm = settingsToForm([]) }: { initialForm?: SettingsForm }) {
  const [form, setForm] = useState(initialForm);
  return <WorkersCard form={form} setForm={setForm} />;
}

describe('worker progress pacing settings', () => {
  it('defaults legacy worker settings to time delay and a 75 percent threshold', () => {
    const form = settingsToForm([
      {
        key: 'workers',
        value: { delaySecondsBetweenJobs: 30 },
      },
    ]);

    expect(form.workers.jobStartPacingMode).toBe('time');
    expect(form.workers.startNextJobAtProgressPercent).toBe(75);
    expect(form.workers.delaySecondsBetweenJobs).toBe(30);
  });

  it('switches the authoritative control without erasing the inactive value', async () => {
    const user = userEvent.setup();
    const form = settingsToForm([
      {
        key: 'workers',
        value: {
          delaySecondsBetweenJobs: 30,
          jobStartPacingMode: 'time',
          startNextJobAtProgressPercent: 82,
        },
      },
    ]);

    render(<WorkersHarness initialForm={form} />);

    expect((screen.getByRole('radio', { name: 'Time delay' }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText('Delay between job starts') as HTMLInputElement).value).toBe('30');
    expect(screen.queryByLabelText('Progress trigger')).toBeNull();

    await user.click(screen.getByRole('radio', { name: 'Job progress' }));
    expect((screen.getByLabelText('Progress trigger') as HTMLInputElement).value).toBe('82');
    expect(screen.queryByLabelText('Delay between job starts')).toBeNull();
    expect(screen.getByText('%')).toBeTruthy();

    await user.clear(screen.getByLabelText('Progress trigger'));
    await user.type(screen.getByLabelText('Progress trigger'), '90');
    await user.click(screen.getByRole('radio', { name: 'Time delay' }));
    expect((screen.getByLabelText('Delay between job starts') as HTMLInputElement).value).toBe('30');

    await user.click(screen.getByRole('radio', { name: 'Job progress' }));
    expect((screen.getByLabelText('Progress trigger') as HTMLInputElement).value).toBe('90');
  });

  it('validates progress thresholds and round-trips both worker fields', () => {
    expect(workerProgressPercentError(1)).toBeUndefined();
    expect(workerProgressPercentError(100)).toBeUndefined();
    expect(workerProgressPercentError(0)).toBeDefined();
    expect(workerProgressPercentError(101)).toBeDefined();
    expect(workerProgressPercentError(75.5)).toBeDefined();

    const form = settingsToForm([
      {
        key: 'workers',
        value: {
          delaySecondsBetweenJobs: 30,
          jobStartPacingMode: 'progress',
          startNextJobAtProgressPercent: 82,
        },
      },
    ]);
    const advancedWorkers = JSON.parse(form.advancedJson.workers) as Record<string, unknown>;
    const payload = workerSettingsPayload(form.workers);

    expect(advancedWorkers.jobStartPacingMode).toBe('progress');
    expect(advancedWorkers.startNextJobAtProgressPercent).toBe(82);
    expect(payload.delaySecondsBetweenJobs).toBe(30);
    expect(payload.jobStartPacingMode).toBe('progress');
    expect(payload.startNextJobAtProgressPercent).toBe(82);
  });
});
