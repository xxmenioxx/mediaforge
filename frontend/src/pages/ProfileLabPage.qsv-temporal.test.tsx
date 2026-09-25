import { describe, expect, it } from 'vitest';
import type { LabFidelityOperation, QSVTemporalExperimentSummary } from '../api/types';
import {
  buildQSVTemporalExperimentRequest,
  buildQSVTemporalRequestedDelta,
  qsvTemporalEvidenceProjection,
  qsvTemporalRequestFromSummary,
} from '../utils/profileLabTemporal';
import { labFidelityOperationMatchesRun } from '../utils/profileLabFidelity';

function summary(): QSVTemporalExperimentSummary {
  return {
    experimentId: 'experiment:1',
    sourceRevision: 'source:1',
    samplingPlanIdentity: 'plan:1',
    targets: [
      {
        windowIdentity: 'window:primary',
        primary: true,
        baseline: { candidateIdentity: 'baseline:primary', sourceRevision: 'source:1', samplingWindowIdentity: 'window:primary' },
        variants: [{
          variantIdentity: 'variant:gop',
          changedVariable: 'gop',
          requestedDelta: { gopFrames: 48 },
          isolation: { result: 'valid', declaredVariable: 'gop' },
          candidate: { candidateIdentity: 'candidate:gop', sourceRevision: 'source:1', samplingWindowIdentity: 'window:primary' },
          temporalEvidence: [{
            targetWindowIdentity: 'window:primary',
            variantIdentity: 'variant:gop',
            availability: 'measured',
            interpretation: 'not_observed',
            interpretationPolicyVersion: 'qsv-temporal-v1',
            baselineInterpretation: 'likely_pumping',
            variantInterpretation: 'not_observed',
            mitigationDirection: 'improved',
          }],
        }],
        controlCandidates: [{
          windowIdentity: 'window:primary',
          requestId: 'control-request:1',
          artifactIdentity: 'control-artifact:1',
          generationIdentity: 'control-generation:1',
          evidenceIdentity: 'control-evidence:1',
          comparable: 'comparable',
        }],
        classification: 'reproducible_qsv_specific_pumping',
      },
      {
        windowIdentity: 'window:texture',
        primary: false,
        baseline: { candidateIdentity: 'baseline:texture', sourceRevision: 'source:1', samplingWindowIdentity: 'window:texture' },
        variants: [],
      },
    ],
  };
}

describe('QSV temporal LAB projection and request intent', () => {
  it('builds one explicit variable request without a Cartesian product', () => {
    const windows = [{ windowIdentity: 'window:primary' }, { windowIdentity: 'window:texture' }, { windowIdentity: 'window:worst' }, { windowIdentity: 'window:ignored' }];
    expect(buildQSVTemporalExperimentRequest(windows, 'gop', { gopFrames: 48 })).toEqual({
      variantsByWindow: [
        { windowIdentity: 'window:primary', variants: [{ changedVariable: 'gop', requestedDelta: { gopFrames: 48 } }] },
        { windowIdentity: 'window:texture', variants: [{ changedVariable: 'gop', requestedDelta: { gopFrames: 48 } }] },
        { windowIdentity: 'window:worst', variants: [{ changedVariable: 'gop', requestedDelta: { gopFrames: 48 } }] },
      ],
    });
    expect(buildQSVTemporalRequestedDelta('quality_rate_control', '22')).toEqual({ globalQuality: 22 });
    expect(buildQSVTemporalRequestedDelta('b_frame_strategy', '0')).toEqual({ maxBFrames: 0 });
    expect(buildQSVTemporalRequestedDelta('qsv_feature', 'true', 'adaptiveB')).toEqual({ adaptiveB: true });
    expect(buildQSVTemporalRequestedDelta('qsv_feature', 'enabled', 'MBBRC')).toEqual({ mbbrcMode: 'enabled' });
  });

  it('retries with the same frozen target/variant/control intent and fresh operation identity', () => {
    const request = qsvTemporalRequestFromSummary(summary());
    expect(request.variantsByWindow).toEqual([
      { windowIdentity: 'window:primary', variants: [{ variantIdentity: 'variant:gop', changedVariable: 'gop', requestedDelta: { gopFrames: 48 } }] },
      { windowIdentity: 'window:texture', variants: [] },
    ]);
    expect(request.controls).toEqual([{ windowIdentity: 'window:primary', requestId: 'control-request:1', generationIdentity: 'control-generation:1', evidenceIdentity: 'control-evidence:1' }]);
    expect(request).not.toHaveProperty('operationId');
  });

  it('projects the actual variant evidence instead of target-level legacy evidence', () => {
    const target = summary().targets[0];
    target.temporalEvidence = [{ targetWindowIdentity: target.windowIdentity, variantIdentity: 'legacy', availability: 'measured', interpretation: 'likely_pumping', interpretationPolicyVersion: 'old' }];
    const projection = qsvTemporalEvidenceProjection(target);
    expect(projection.variant?.variantIdentity).toBe('variant:gop');
    expect(projection.evidence?.variantInterpretation).toBe('not_observed');
    expect(projection.evidence?.mitigationDirection).toBe('improved');
    expect(projection.control?.requestId).toBe('control-request:1');
  });

  it('keeps stale fencing strict and does not treat rerender/tab changes as work', () => {
    const operation = { id: 'operation:1', primaryRequestId: 'request:1', primaryArtifactId: 'artifact:1', primaryGeneration: 'generation:1', materialSignature: 'material:1' } as LabFidelityOperation;
    const run = { operationId: 'operation:1', primaryRequestId: 'request:1', primaryArtifactId: 'artifact:1', primaryGeneration: 'generation:1', materialSignature: 'material:1', localRequestSignature: 'request:signature' };
    expect(labFidelityOperationMatchesRun(operation, run)).toBe(true);
    expect(labFidelityOperationMatchesRun({ ...operation, id: 'operation:stale' }, run)).toBe(false);
  });
});
