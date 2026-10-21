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

function job(validationStatus: ValidationSummary['validationStatus'], validationScore = 0, overrides: Partial<ValidationSummary> = {}): ValidationSummary {
  return {
    id: 165,
    mediaPath: '/media/raw/validation-test.mkv',
    status: 'completed',
    stage: 'validation_review',
    outputPath: '/media/staging/validation-test.mkv',
    publishedPath: '',
    validationStatus,
    validationScore,
    validationRunId: '',
    validationOutcome: '',
    validationOverridePresent: false,
    validationHasActionableWarnings: false,
    originalArchivedPath: '',
    originalArchiveStatus: '',
    createdAt: '2026-09-25T10:00:00Z',
    updatedAt: '2026-09-25T10:00:00Z',
    ...overrides,
  };
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return { ...render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <ValidationPage />
      </QueryClientProvider>
    </MemoryRouter>,
  ), queryClient };
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

  it('previews staging, defaults cleanup on, then dismisses before scoped cleanup', async () => {
    vi.spyOn(api, 'settings').mockResolvedValue([{ key: 'validation', value: { preset: 'default' } }] as never);
    vi.spyOn(api, 'validationReview').mockResolvedValue({
      items: [
        job('failed', 55, { validationOutcome: 'FAIL', validationRunId: 'validation-run:fail' }),
        job('passed', 100, { validationOutcome: 'PASS', validationRunId: 'validation-run:pass', id: 166 }),
        job('failed', 50, { validationOutcome: 'REVIEW_REQUIRED', validationRunId: 'validation-run:published', publishedAt: '2026-10-12T10:00:00Z', id: 167 }),
      ],
      total: 3, limit: 25, offset: 0,
    });
    const dismissed = vi.spyOn(api, 'dismissQueueJob').mockResolvedValue({ ...job('failed', 55, { validationOutcome: 'FAIL', validationRunId: 'validation-run:fail' }) } as never);
    const preview = vi.spyOn(api, 'previewValidationStagingCleanup').mockResolvedValue({
      jobId: 165, executionNumber: 1, validationRunId: 'validation-run:fail', previewId: 'preview:165', expiresAt: '2026-10-20T10:00:00Z',
      state: 'ready', workspacePath: '/work/job-165', ownership: 'QueueJob #165', reason: 'Owned job workspace',
      files: [{ path: 'output.mkv', sizeBytes: 4096, kind: 'file' }], totalBytes: 4096, blockers: [],
    });
    const cleanup = vi.spyOn(api, 'cleanupValidationStaging').mockResolvedValue({ jobId: 165, previewId: 'preview:165', outcome: 'cleaned', removedPaths: ['output.mkv'], recoveredBytes: 4096, reason: '', evidenceId: 'evidence:1' });
    const user = userEvent.setup();
    const { queryClient } = renderPage();
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');

    const dismissButtons = await screen.findAllByRole('button', { name: 'Dismiss' });
    expect(dismissButtons).toHaveLength(1);
    await user.click(dismissButtons[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Dismiss job' });
    await waitFor(() => expect(preview).toHaveBeenCalledWith(165));
    expect((within(dialog).getByRole('checkbox', { name: 'Also clean staged output' }) as HTMLInputElement).checked).toBe(true);
    expect(within(dialog).getByText('output.mkv')).toBeTruthy();
    expect(within(dialog).getByText('4,096 bytes')).toBeTruthy();
    await user.click(within(dialog).getByRole('button', { name: 'Dismiss' }));
    await waitFor(() => expect(cleanup).toHaveBeenCalledWith({ jobId: 165, previewId: 'preview:165' }));
    expect(dismissed.mock.calls[0]?.[0]).toBe(165);
    expect(dismissed.mock.invocationCallOrder[0]).toBeLessThan(cleanup.mock.invocationCallOrder[0]);
    for (const key of ['validationReview', 'validationHistory', 'queueJobs', 'assets', 'queueBatchPreflight', 'publisherEligibility']) {
      expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: [key] });
    }
    expect(await screen.findByText(/staging cleaned/)).toBeTruthy();
  });

  it('allows Dismiss without invoking cleanup when the operator unchecks the option', async () => {
    vi.spyOn(api, 'settings').mockResolvedValue([]);
    vi.spyOn(api, 'validationReview').mockResolvedValue({ items: [{ ...job('failed'), validationOutcome: 'FAIL', validationRunId: 'run' }], total: 1, limit: 25, offset: 0 });
    vi.spyOn(api, 'previewValidationStagingCleanup').mockResolvedValue({ jobId: 165, executionNumber: 1, validationRunId: 'run', previewId: 'p', expiresAt: '', state: 'ready', workspacePath: '/work/job-165', ownership: 'QueueJob #165', reason: '', files: [], totalBytes: 0, blockers: [] });
    const dismiss = vi.spyOn(api, 'dismissQueueJob').mockResolvedValue({} as never);
    const cleanup = vi.spyOn(api, 'cleanupValidationStaging');
    const user = userEvent.setup(); renderPage();
    await user.click((await screen.findAllByRole('button', { name: 'Dismiss' }))[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Dismiss job' });
    const checkbox = within(dialog).getByRole('checkbox', { name: 'Also clean staged output' });
    expect((checkbox as HTMLInputElement).checked).toBe(true);
    await user.click(checkbox);
    await user.click(within(dialog).getByRole('button', { name: 'Dismiss' }));
    await waitFor(() => expect(dismiss.mock.calls[0]?.[0]).toBe(165));
    expect(await within(dialog).findByText('Dismissed; staging retained.')).toBeTruthy();
    expect(cleanup).not.toHaveBeenCalled();
  });

  it('keeps Dismiss available and defaults cleanup off when the preview is blocked', async () => {
    vi.spyOn(api, 'settings').mockResolvedValue([]);
    vi.spyOn(api, 'validationReview').mockResolvedValue({ items: [{ ...job('failed'), validationOutcome: 'FAIL', validationRunId: 'run' }], total: 1, limit: 25, offset: 0 });
    vi.spyOn(api, 'previewValidationStagingCleanup').mockResolvedValue({ jobId: 165, executionNumber: 1, validationRunId: 'run', previewId: '', expiresAt: '', state: 'blocked', workspacePath: '/work/job-165', ownership: 'unavailable', reason: '', files: [], totalBytes: 0, blockers: ['workspace is unsafe'] });
    const dismiss = vi.spyOn(api, 'dismissQueueJob').mockResolvedValue({} as never);
    const cleanup = vi.spyOn(api, 'cleanupValidationStaging');
    const user = userEvent.setup(); renderPage();
    await user.click((await screen.findAllByRole('button', { name: 'Dismiss' }))[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Dismiss job' });
    const checkbox = await within(dialog).findByRole('checkbox', { name: 'Also clean staged output' });
    await waitFor(() => expect((checkbox as HTMLInputElement).checked).toBe(false));
    expect(within(dialog).getByRole('button', { name: 'Dismiss' }).hasAttribute('disabled')).toBe(false);
    await user.click(within(dialog).getByRole('button', { name: 'Dismiss' }));
    await waitFor(() => expect(dismiss.mock.calls[0]?.[0]).toBe(165));
    expect(cleanup).not.toHaveBeenCalled();
  });
});
