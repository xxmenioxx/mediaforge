import type {
  EncoderControlCandidate,
  QSVTemporalControlRequest,
  QSVTemporalEvidence,
  QSVTemporalExperimentRequest,
  QSVTemporalExperimentSummary,
  QSVTemporalTargetRequest,
  QSVTemporalTargetSummary,
  QSVTemporalVariant,
  QSVTemporalVariantRequest,
} from '../api/types';

export type QSVTemporalChangedVariable = 'quality_rate_control' | 'gop' | 'b_frame_strategy' | 'qsv_feature';

export const QSV_TEMPORAL_LAB_BOUNDS = {
  maxTargets: 3,
  maxVariantsPerTarget: 3,
  maxTotalVariants: 6,
  maxOptionalControls: 3,
} as const;

export function buildQSVTemporalRequestedDelta(
  changedVariable: QSVTemporalChangedVariable,
  value: string,
  feature = 'adaptiveB',
): Record<string, unknown> | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  if (changedVariable === 'quality_rate_control') {
    const globalQuality = Number(trimmed);
    return Number.isFinite(globalQuality) ? { globalQuality } : null;
  }
  if (changedVariable === 'gop') {
    const gopFrames = Number(trimmed);
    return Number.isInteger(gopFrames) && gopFrames > 0 ? { gopFrames } : null;
  }
  if (changedVariable === 'b_frame_strategy') {
    const maxBFrames = Number(trimmed);
    return Number.isInteger(maxBFrames) && maxBFrames >= 0 ? { maxBFrames } : null;
  }
  if (feature === 'adaptiveB' || feature === 'adaptiveI' || feature === 'extendedBRC') {
    if (!['true', 'false'].includes(trimmed.toLowerCase())) return null;
    return { [feature]: trimmed.toLowerCase() === 'true' };
  }
  if (feature === 'MBBRC') return { mbbrcMode: trimmed };
  if (feature === 'rdo') return { rdoMode: trimmed };
  const numericValue = Number(trimmed);
  return Number.isFinite(numericValue) ? { [feature]: numericValue } : { [feature]: trimmed };
}

export function buildQSVTemporalExperimentRequest(
  windows: Array<{ windowIdentity?: string | null }>,
  changedVariable: QSVTemporalChangedVariable,
  requestedDelta: Record<string, unknown>,
): QSVTemporalExperimentRequest {
  return {
    variantsByWindow: windows
      .filter((window): window is { windowIdentity: string } => typeof window.windowIdentity === 'string' && Boolean(window.windowIdentity))
      .slice(0, QSV_TEMPORAL_LAB_BOUNDS.maxTargets)
      .map((window): QSVTemporalTargetRequest => ({
        windowIdentity: window.windowIdentity,
        variants: [{ changedVariable, requestedDelta } satisfies QSVTemporalVariantRequest],
      })),
  };
}

export function qsvTemporalRequestFromSummary(summary: QSVTemporalExperimentSummary): QSVTemporalExperimentRequest {
  let totalVariants = 0;
  const variantsByWindow = summary.targets.slice(0, QSV_TEMPORAL_LAB_BOUNDS.maxTargets).map((target): QSVTemporalTargetRequest => {
    const variants = (target.variants ?? []).slice(0, QSV_TEMPORAL_LAB_BOUNDS.maxVariantsPerTarget).flatMap((variant): QSVTemporalVariantRequest[] => {
      if (totalVariants >= QSV_TEMPORAL_LAB_BOUNDS.maxTotalVariants) return [];
      totalVariants += 1;
      return [{
        variantIdentity: variant.variantIdentity,
        changedVariable: variant.changedVariable,
        requestedDelta: variant.requestedDelta ?? {},
      }];
    });
    return { windowIdentity: target.windowIdentity, variants };
  });
  const controls = summary.targets.flatMap((target) => (target.controlCandidates ?? []).map((control): QSVTemporalControlRequest => ({
    windowIdentity: target.windowIdentity,
    requestId: control.requestId,
    generationIdentity: control.generationIdentity,
    evidenceIdentity: control.evidenceIdentity,
  })).filter((control) => Boolean(control.requestId)));
  return { variantsByWindow, controls: controls.length ? controls.slice(0, QSV_TEMPORAL_LAB_BOUNDS.maxOptionalControls) : undefined };
}

export function qsvTemporalEvidenceProjection(target: QSVTemporalTargetSummary): {
  variant?: QSVTemporalVariant;
  evidence?: QSVTemporalEvidence;
  control?: EncoderControlCandidate;
} {
  const variant = (target.variants ?? []).find((candidate) => (candidate.temporalEvidence ?? []).length > 0) ?? target.variants?.[0];
  return {
    variant,
    evidence: variant?.temporalEvidence?.[0],
    control: target.controlCandidates?.[0],
  };
}
