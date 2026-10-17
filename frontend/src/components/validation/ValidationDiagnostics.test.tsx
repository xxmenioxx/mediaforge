// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ValidationFinding, ValidationOverride, ValidationRun } from '../../api/types';
import { ValidationFindingsTable } from './ValidationFindingsTable';
import { ValidationTechnicalEvidence } from './ValidationTechnicalEvidence';

afterEach(() => cleanup());

function finding(key: string, state: ValidationFinding['state'] = 'FAIL', values: Partial<ValidationFinding> = {}): ValidationFinding {
  return {
    key, expected: 'expected value', actual: 'observed value', severity: 'error', disposition: 'blocking', state,
    actionable: state !== 'PASS', overrideable: state !== 'PASS', explanation: `${key} needs review.`, evidenceReferences: ['result.outputProbe'],
    ...values,
  };
}

function run(): ValidationRun {
  return {
    id: 11, runId: 'validation/run:technical evidence', jobId: 165, executionNumber: 4, executionPlanId: 3,
    executionPlanVersion: 1, resultArtifactId: 'result:test', resultGenerationToken: 'generation:test',
    resultGenerationVersion: 1, resultOutputRevision: 'revision:test', schemaVersion: 'validation-run-v1',
    validatorVersion: 'quality-gate-v1', rulesetId: 'mvforge-post-conversion', rulesetVersion: '1.0.0',
    policySchemaVersion: 1, policyVersion: 'validation-policy-v1', policyPreset: 'default', policyFingerprint: 'sha256:run-policy',
    actorSource: 'deployment', actorId: 'local_operator', outcome: 'FAIL', score: 60, actionableWarnings: true,
    findings: [finding('timeline.av_alignment', 'FAIL', { details: { tracks: [{ streamIndex: 1, introducedOffsetMs: 42 }] } })],
    report: { outputProbe: { streams: [{ codecType: 'audio', codecName: 'aac' }] } }, createdAt: '2026-10-10T10:00:00Z',
  };
}

describe('ValidationFindingsTable', () => {
  it('orders findings by canonical state, keeps every status distinct, and lets users filter passed checks', async () => {
    const user = userEvent.setup();
    const findings = [
      finding('pass.check', 'PASS'),
      finding('unknown.metric', 'UNVERIFIED', { severity: 'critical', explanation: '' }),
      finding('review.check', 'REVIEW_REQUIRED'),
      finding('warning.check', 'WARNING'),
      finding('na.check', 'NOT_APPLICABLE'),
      finding('fail.check', 'FAIL'),
    ];
    render(<ValidationFindingsTable findings={findings} overridesByKey={new Map()} onSkip={vi.fn()} />);

    expect(screen.getByText('FAIL')).toBeTruthy();
    expect(screen.getByText('WARNING')).toBeTruthy();
    expect(screen.getByText('UNVERIFIED')).toBeTruthy();
    expect(screen.queryByText('PASS')).toBeNull();
    const orderedText = screen.getByRole('table', { name: 'Validation diagnostics' }).textContent ?? '';
    expect(orderedText.indexOf('fail.check')).toBeLessThan(orderedText.indexOf('warning.check'));
    expect(orderedText.indexOf('warning.check')).toBeLessThan(orderedText.indexOf('unknown.metric'));
    expect(screen.getByText('Unknown Metric')).toBeTruthy();

    await user.click(screen.getByRole('tab', { name: 'All checks (6)' }));
    expect(screen.getByText('REVIEW REQUIRED')).toBeTruthy();
    expect(screen.getByText('NOT APPLICABLE')).toBeTruthy();
    await user.click(screen.getByRole('tab', { name: 'Passed (1)' }));
    expect(screen.getByText('PASS')).toBeTruthy();
    expect(screen.queryByText('FAIL')).toBeNull();
  });

  it('shows A/V offsets, video property evidence, missing-evidence status, long text, and override audit without healing FAIL', async () => {
    const user = userEvent.setup();
    const override: ValidationOverride = {
      eventId: 'validation-override:test', validationRunId: 'validation-run:test', scope: 'rule', resultArtifactId: 'result:test',
      resultGenerationToken: 'generation:test', resultGenerationVersion: 1, resultOutputRevision: 'revision:test',
      policySchemaVersion: 1, policyVersion: 'validation-policy-v1', policyFingerprint: 'sha256:run-policy', actorSource: 'deployment',
      actorId: 'local_operator', reason: 'Reviewed by operator', ruleKeys: ['timeline.av_alignment'], requestedHandoff: 'publisher',
      overrideVersion: 'validation-override-v1', createdAt: '2026-10-10T10:01:00Z',
    };
    const findings = [
      finding('timeline.av_alignment', 'FAIL', { details: { tracks: [{ streamIndex: 2, trackLabel: 'English audio track with a deliberately long label', introducedOffsetMs: 42, toleranceMs: 20 }] } }),
      finding('video.properties', 'FAIL', { expected: { fields: { width: { expected: 1920, actual: 1280, status: 'mismatch', reason: 'Geometry differs from the selected expectation.' } } }, actual: { fields: { width: { expected: 1920, actual: 1280, status: 'mismatch', reason: 'Geometry differs from the selected expectation.' } } } }),
      finding('historical.timeline', 'REVIEW_REQUIRED', { expected: undefined, actual: undefined, details: undefined }),
    ];
    render(<ValidationFindingsTable findings={findings} overridesByKey={new Map([['timeline.av_alignment', override]])} onSkip={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Show details for Audio/video timing alignment' }));
    await screen.findByText(/Manual SKIPPED override recorded/);
    expect(screen.getByText(/local_operator/)).toBeTruthy();
    expect(screen.getByText(/Reviewed by operator/)).toBeTruthy();
    expect(screen.getByText('SKIPPED')).toBeTruthy();
    expect(screen.getAllByText('FAIL').length).toBeGreaterThan(0);
    expect(screen.getByText('42')).toBeTruthy();
    expect(screen.getByText('20')).toBeTruthy();
    expect(screen.getByText(/English audio track with a deliberately long label/)).toBeTruthy();
    expect(screen.getByText(/Per-track/)).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Show details for Video properties' }));
    expect((await screen.findAllByText(/Geometry differs from the selected expectation/)).length).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: 'Show details for Historical Timeline' }));
    expect(await screen.findByText(/Evidence availability: UNVERIFIED/)).toBeTruthy();
    expect(screen.getByText(/canonical finding state remains REVIEW_REQUIRED/i)).toBeTruthy();
  });
});

describe('ValidationTechnicalEvidence', () => {
  it('renders the complete run and falls back to manual copy when the clipboard rejects the request', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockRejectedValue(new Error('clipboard denied'));
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    const immutableRun = run();
    render(<ValidationTechnicalEvidence run={immutableRun} />);

    const json = screen.getByLabelText('Validation run JSON');
    expect(json.textContent).toContain('"report"');
    expect(json.textContent).toContain('"introducedOffsetMs": 42');
    expect(json.textContent).toContain('"outputProbe"');
    await user.click(screen.getByRole('button', { name: 'Copy JSON' }));
    expect(writeText).toHaveBeenCalledWith(JSON.stringify(immutableRun, null, 2));
    expect((await screen.findByRole('status')).textContent).toMatch(/Select the JSON below and copy it manually/);
    expect(json.textContent).toContain('"validation/run:technical evidence"');
  });

  it('keeps the full JSON visible and reports when download APIs are unavailable', async () => {
    const user = userEvent.setup();
    const createObjectURL = URL.createObjectURL;
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: undefined });
    try {
      render(<ValidationTechnicalEvidence run={run()} />);
      await user.click(screen.getByRole('button', { name: 'Download JSON' }));
      expect((await screen.findByRole('status')).textContent).toMatch(/Download is unavailable/);
      expect(screen.getByLabelText('Validation run JSON').textContent).toContain('"findings"');
    } finally {
      Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    }
  });

  it('downloads the immutable run with a sanitized run-identified filename', async () => {
    vi.useFakeTimers();
    const originalCreate = Object.getOwnPropertyDescriptor(URL, 'createObjectURL');
    const originalRevoke = Object.getOwnPropertyDescriptor(URL, 'revokeObjectURL');
    const createObjectURL = vi.fn(() => 'blob:validation-run');
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });
    let downloadAtClick: { href: string; name: string } | undefined;
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {
      const anchor = document.querySelector<HTMLAnchorElement>('a[download]');
      if (anchor) downloadAtClick = { href: anchor.href, name: anchor.download };
      expect(revokeObjectURL).not.toHaveBeenCalled();
    });
    try {
      render(<ValidationTechnicalEvidence run={run()} />);
      fireEvent.click(screen.getByRole('button', { name: 'Download JSON' }));
      expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
      expect(click).toHaveBeenCalledTimes(1);
      expect(downloadAtClick).toEqual({ href: 'blob:validation-run', name: 'validation-validation-run-technical-evidence.json' });
      expect(revokeObjectURL).not.toHaveBeenCalled();
      await vi.runAllTimersAsync();
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:validation-run');
      expect(screen.getByRole('status').textContent).toContain('validation-validation-run-technical-evidence.json');
    } finally {
      click.mockRestore();
      if (originalCreate) Object.defineProperty(URL, 'createObjectURL', originalCreate);
      else delete (URL as { createObjectURL?: typeof URL.createObjectURL }).createObjectURL;
      if (originalRevoke) Object.defineProperty(URL, 'revokeObjectURL', originalRevoke);
      else delete (URL as { revokeObjectURL?: typeof URL.revokeObjectURL }).revokeObjectURL;
      vi.useRealTimers();
    }
  });
});
