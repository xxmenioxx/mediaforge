import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { LabResolutionAssistant } from './LabResolutionAssistant';
import type { LabResolutionRequired } from '../api/types';

const response: LabResolutionRequired = {
  version: 1,
  status: 'resolution_required',
  planningStatus: 'NEEDS_REVIEW',
  blockingFields: [{ field: 'effectiveFrameRate', source: 'cadence', expected: 'known output FPS', value: 'unknown', authority: 'Deep Cadence', authorityState: 'UNVERIFIED', reason: 'Ambiguous hard telecine has no validated output rate.' }],
  evidence: { path: '/raw/example.mkv', sourceSizeBytes: 100, sourceModifiedAtNs: 1, sourceMode: 33188, cadenceAnalysisVersion: 1, sourceFrameRate: '30000/1001', profileDecisionSignature: 'sig', requestedProfileSignature: 'requested-sig' },
  requested: { cadence: 'auto', fieldStructure: 'auto', gop: 'auto', gopStrategy: 'compatible' },
  effective: { frameRate: '', frameRateKnown: false, progressive: false, progressiveReliable: false, cadenceOperation: 'review', fieldOrder: 'unknown' },
  actions: [
    { id: 'preserve_source', label: 'Preserve source cadence', eligible: true, recommended: true, reason: 'Source timing is supported.', tradeoffs: 'Telecine may remain.' },
    { id: 'reanalyze_cadence', label: 'Run bounded cadence analysis', eligible: true, reason: 'Current evidence is insufficient.' },
    { id: 'compare_field_order', label: 'Compare field order', eligible: true, reason: 'Authority is ambiguous.' },
  ],
};

describe('LAB resolution assistant', () => {
  it('shows evidence and eligible actions without claiming Safe', () => {
    const onApply = vi.fn();
    const onTest = vi.fn();
    const onReanalyze = vi.fn();
    render(<LabResolutionAssistant resolution={response} onApply={onApply} onTest={onTest} onReanalyze={onReanalyze} />);
    expect(screen.getByText('Why Auto needs review')).toBeTruthy();
    expect(screen.getByText(/UNVERIFIED · Deep Cadence/)).toBeTruthy();
    expect(screen.getByText(/Telecine may remain/)).toBeTruthy();
    expect(screen.getByText(/Plan status: NEEDS_REVIEW/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Apply to this asset' }));
    fireEvent.click(screen.getByRole('button', { name: 'Test alternative' }));
    fireEvent.click(screen.getByRole('button', { name: 'Re-analyze cadence' }));
    fireEvent.click(screen.getByRole('button', { name: 'Test TFF alternative' }));
    fireEvent.click(screen.getByRole('button', { name: 'Test BFF alternative' }));
    expect(onApply).toHaveBeenCalledWith(response.actions[0], undefined);
    expect(onTest).toHaveBeenCalledWith(response.actions[0], undefined);
    expect(onReanalyze).toHaveBeenCalledOnce();
    expect(onTest).toHaveBeenCalledWith(response.actions[2], 'tff');
    expect(onTest).toHaveBeenCalledWith(response.actions[2], 'bff');
  });

  it('does not show ineligible actions', () => {
    render(<LabResolutionAssistant resolution={{ ...response, actions: [{ ...response.actions[1], eligible: false }] }} onApply={vi.fn()} onTest={vi.fn()} onReanalyze={vi.fn()} />);
    expect(screen.queryByRole('button', { name: 'Re-analyze cadence' })).toBeNull();
  });
});
