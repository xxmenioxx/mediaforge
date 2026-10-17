// @vitest-environment jsdom

import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiRequestError, api } from '../api/client';
import type { ValidationFinding, ValidationRunDetail, ValidationSummary } from '../api/types';
import { ValidationPage } from './ValidationPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function job(validationStatus: ValidationSummary['validationStatus'], validationScore = 0): ValidationSummary {
  return {
    id: 165,
    mediaPath: '/media/raw/validation-test.mkv',
    status: 'completed',
    outputPath: '/media/staging/validation-test.mkv',
    validationStatus,
    validationScore,
    validationRunId: '',
    validationOutcome: '',
    validationOverridePresent: false,
    validationHasActionableWarnings: false,
    createdAt: '2026-09-25T10:00:00Z',
    updatedAt: '2026-09-25T10:00:00Z',
  };
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <ValidationPage />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

function runDetail(findings: ValidationFinding[], overrides: ValidationRunDetail['overrides'] = [], effectiveHandoff: ValidationRunDetail['effectiveHandoff'] = 'BLOCKED'): ValidationRunDetail {
  return {
    run: {
      id: 11, runId: 'validation-run:test', jobId: 165, executionNumber: 1, executionPlanId: 3,
      executionPlanVersion: 1, resultArtifactId: 'result:test', resultGenerationToken: 'generation:test',
      resultGenerationVersion: 1, resultOutputRevision: 'revision:test', schemaVersion: 'validation-run-v1',
      validatorVersion: 'quality-gate-v1', rulesetId: 'mvforge-post-conversion', rulesetVersion: '1.0.0',
      policySchemaVersion: 1, policyVersion: 'validation-policy-v1', policyPreset: 'default',
      policyFingerprint: 'sha256:run-policy', policySnapshot: { schemaVersion: 1, policyVersion: 'validation-policy-v1', preset: 'default', rules: {} },
      actorSource: 'deployment', actorId: 'local_operator', outcome: 'FAIL', score: 60, actionableWarnings: true,
      findings, report: {}, createdAt: '2026-10-10T10:00:00Z',
    },
    overrides,
    effectiveHandoff,
    currentPolicy: { schemaVersion: 1, policyVersion: 'validation-policy-v1', preset: 'strict', rules: {} },
    policyCurrent: false,
  };
}

function finding(key: string, overrideable = true): ValidationFinding {
  return { key, expected: 'expected', actual: 'actual', severity: 'error', disposition: 'blocking', state: 'FAIL', actionable: true, overrideable, explanation: `${key} needs review.`, evidenceReferences: ['result.outputProbe'] };
}

describe('ValidationPage request errors', () => {
  it('shows the backend error message when the validation request fails', async () => {
    vi.spyOn(api, 'settings').mockResolvedValue([{ key: 'validation', value: { preset: 'default' } }] as never);
    vi.spyOn(api, 'validationReview').mockResolvedValue({ items: [job('pending')], total: 1, limit: 25, offset: 0 });
    vi.spyOn(api, 'validateJob').mockRejectedValue(
      new ApiRequestError('The output could not be inspected by the backend.', 500),
    );
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Run validation' }));

    expect(await screen.findByText('The output could not be inspected by the backend.')).toBeTruthy();
  });

  it('keeps a normal failed Validation result separate from request errors', async () => {
    vi.spyOn(api, 'settings').mockResolvedValue([{ key: 'validation', value: { preset: 'default' } }] as never);
    const jobs = vi.spyOn(api, 'validationReview')
      .mockResolvedValueOnce({ items: [job('pending')], total: 1, limit: 25, offset: 0 })
      .mockResolvedValue({ items: [{ ...job('failed', 60), validationOutcome: 'FAIL', validationRunId: 'validation-run:test' }], total: 1, limit: 25, offset: 0 });
    vi.spyOn(api, 'validateJob').mockResolvedValue({
      jobId: 165,
      status: 'failed',
      outcome: 'FAIL',
      runId: 'validation-run:test',
      score: 60,
      checks: [{ key: 'video.properties', label: 'Video properties', status: 'failed', message: 'Output geometry mismatch.' }],
      warnings: [],
      report: {},
      findings: [],
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Run validation' }));
    await waitFor(() => expect(jobs).toHaveBeenCalledTimes(2));

    expect(screen.queryByText('Validation could not be completed.')).toBeNull();
    expect(await screen.findByText('FAIL')).toBeTruthy();
    expect(await screen.findByText('60/100')).toBeTruthy();
  });

  it('shows frozen run policy separately and allows an individual blank-reason Skip without changing FAIL', async () => {
    vi.spyOn(api, 'settings').mockResolvedValue([{ key: 'validation', value: { preset: 'strict', differenceCount: 0 } }] as never);
    vi.spyOn(api, 'validationReview').mockResolvedValue({
      items: [{ ...job('failed', 60), validationOutcome: 'FAIL', validationRunId: 'validation-run:test' }], total: 1, limit: 25, offset: 0,
    });
    const event = {
      eventId: 'validation-override:test', validationRunId: 'validation-run:test', scope: 'rule',
      resultArtifactId: 'result:test', resultGenerationToken: 'generation:test', resultGenerationVersion: 1,
      resultOutputRevision: 'revision:test', policySchemaVersion: 1, policyVersion: 'validation-policy-v1', policyFingerprint: 'sha256:run-policy',
      actorSource: 'deployment', actorId: 'local_operator', reason: '', ruleKeys: ['duration.timeline'],
      requestedHandoff: 'publisher' as const, overrideVersion: 'validation-override-v1', createdAt: '2026-10-10T10:01:00Z',
    };
    vi.spyOn(api, 'validationRun')
      .mockResolvedValueOnce(runDetail([finding('duration.timeline')]))
      .mockResolvedValue(runDetail([finding('duration.timeline')], [event], 'ALLOWED_BY_OVERRIDE'));
    const createOverride = vi.spyOn(api, 'createValidationOverride').mockResolvedValue(event as never);
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Details' }));
    const detailDialog = await screen.findByRole('dialog');
    expect(detailDialog.textContent).toContain('Canonical Validation: FAIL');
    expect(detailDialog.textContent).toContain('Effective Publisher handoff');
    expect(detailDialog.textContent).toContain('Backend result: BLOCKED');
    await waitFor(() => expect(detailDialog.textContent).toContain('Policy used by this run: Default · sha256:run-policy'));
    expect(detailDialog.textContent).toContain('Current Validation Settings: Strict · different from this run');
    await user.click(within(detailDialog).getByRole('button', { name: 'Skip' }));
    const overrideDialog = await screen.findByRole('dialog', { name: 'Skip validation rule' });
    expect(overrideDialog.textContent).toContain('Canonical result: FAIL');
    expect(within(overrideDialog).getByRole('textbox', { name: 'Reason (optional)' })).toBeTruthy();
    await user.click(within(overrideDialog).getByRole('button', { name: 'Skip rule' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Skip validation rule' })).toBeNull());

    await waitFor(() => expect(createOverride).toHaveBeenCalledWith({
      jobId: 165, scope: 'rule', reason: '', ruleKeys: ['duration.timeline'], requestedHandoff: 'publisher',
    }));
    expect(await screen.findByText('ALLOWED BY OVERRIDE')).toBeTruthy();
    expect(await screen.findByText('Backend result: ALLOWED_BY_OVERRIDE')).toBeTruthy();
    expect(screen.getAllByText('FAIL').length).toBeGreaterThan(0);
    expect(await screen.findByText('SKIPPED')).toBeTruthy();
    expect(await screen.findByText(/Manual SKIPPED event by local_operator ·/)).toBeTruthy();
    await user.click(within(detailDialog).getByRole('tab', { name: 'Technical Evidence' }));
    const technicalJson = await within(detailDialog).findByLabelText('Validation run JSON');
    expect(technicalJson.textContent).toContain('"findings"');
    expect(technicalJson.textContent).toContain('validation-run:test');
  });

  it('confirms Skip all with the exact unresolved overrideable rule keys', async () => {
    vi.spyOn(api, 'settings').mockResolvedValue([{ key: 'validation', value: { preset: 'default' } }] as never);
    vi.spyOn(api, 'validationReview').mockResolvedValue({
      items: [{ ...job('failed', 40), validationOutcome: 'FAIL', validationRunId: 'validation-run:test' }], total: 1, limit: 25, offset: 0,
    });
    const findings = [finding('duration.timeline'), finding('video.properties'), finding('artifact.integrity', false)];
    const event = {
      eventId: 'validation-override:all', validationRunId: 'validation-run:test', scope: 'all_overrideable',
      resultArtifactId: 'result:test', resultGenerationToken: 'generation:test', resultGenerationVersion: 1,
      resultOutputRevision: 'revision:test', policySchemaVersion: 1, policyVersion: 'validation-policy-v1', policyFingerprint: 'sha256:run-policy',
      actorSource: 'deployment', actorId: 'local_operator', reason: 'Reviewed together', ruleKeys: ['duration.timeline', 'video.properties'],
      requestedHandoff: 'publisher' as const, overrideVersion: 'validation-override-v1', createdAt: '2026-10-10T10:01:00Z',
    };
    vi.spyOn(api, 'validationRun')
      .mockResolvedValueOnce(runDetail(findings))
      .mockResolvedValue(runDetail(findings, [event], 'ALLOWED_BY_OVERRIDE'));
    const createOverride = vi.spyOn(api, 'createValidationOverride').mockResolvedValue(event as never);
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Details' }));
    const detailDialog = await screen.findByRole('dialog');
    await user.click(await within(detailDialog).findByRole('button', { name: 'Skip all overrideable findings' }));
    const overrideDialog = await screen.findByRole('dialog', { name: 'Skip all validation findings' });
    expect(overrideDialog.textContent).toContain('duration.timeline');
    expect(overrideDialog.textContent).toContain('video.properties');
    expect(overrideDialog.textContent).not.toContain('artifact.integrity');
    await user.click(within(overrideDialog).getByRole('button', { name: 'Skip all' }));

    await waitFor(() => expect(createOverride).toHaveBeenCalledWith({
      jobId: 165, scope: 'all_overrideable', reason: '', ruleKeys: ['duration.timeline', 'video.properties'], requestedHandoff: 'publisher',
    }));
    expect(await screen.findByText('ALLOWED BY OVERRIDE')).toBeTruthy();
    expect(screen.getAllByText('FAIL').length).toBeGreaterThan(0);
  });
});
