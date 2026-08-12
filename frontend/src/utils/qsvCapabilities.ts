export type QSVRateControl = 'icq' | 'la_icq' | 'cqp' | 'vbr' | 'cbr';

export type QSVSelection = {
  adaptiveI?: boolean;
  adaptiveB?: boolean;
  extendedBRC?: boolean;
};

export function qsvPStrategySupported(capability: Record<string, unknown> | undefined, main10: boolean, value: 1 | 2) {
  const testedModes = capability?.testedModes;
  if (!testedModes || typeof testedModes !== 'object' || Array.isArray(testedModes)) return false;
  const mode = value === 1 ? 'Simple' : 'Pyramid';
  return (testedModes as Record<string, unknown>)[`qsvPStrategy${mode}${main10 ? 'Main10' : 'Main8'}`] === true;
}

export function resolveQSVFeatures(
  capability: Record<string, unknown> | undefined,
  options: {
    main10: boolean;
    rateControl?: string;
  },
) {
  const { main10 } = options;
  const rateControl = (options.rateControl ?? 'icq').toLowerCase();
  const rateControls = {
    icq: main10
      ? capability?.qsvIcqMain10 === true
      : capability?.qsvIcqMain8 === true,
    laIcq: main10 ? capability?.qsvLaIcqMain10 === true : capability?.qsvLaIcqMain8 === true,
    cqp: main10
      ? capability?.qsvCqpMain10 === true
      : capability?.qsvCqpMain8 === true,
    vbr: main10
      ? capability?.qsvVbrMain10 === true
      : capability?.qsvVbrMain8 === true,
    cbr: main10
      ? capability?.qsvCbrMain10 === true
      : capability?.qsvCbrMain8 === true,
  };

  return {
    probed: capability !== undefined,
    main10,
    rateControl,
    rateControls,
    adaptiveI:
      main10
        ? capability?.qsvAdaptiveIMain10 === true
        : capability?.qsvAdaptiveIMain8 === true,

    adaptiveB:
      main10
        ? capability?.qsvAdaptiveBMain10 === true
        : capability?.qsvAdaptiveBMain8 === true,

    extBrc:
      (
        (rateControl === 'vbr' &&
          (main10 ? capability?.qsvVbrExtBrcMain10 === true : capability?.qsvVbrExtBrcMain8 === true)) ||
        (rateControl === 'cbr' &&
          (main10 ? capability?.qsvCbrExtBrcMain10 === true : capability?.qsvCbrExtBrcMain8 === true))
      ),

    lookAhead:
      (
        (rateControl === 'vbr' &&
          (main10 ? capability?.qsvVbrLookAheadMain10 === true : capability?.qsvVbrLookAheadMain8 === true)) ||
        (rateControl === 'cbr' &&
          (main10 ? capability?.qsvCbrLookAheadMain10 === true : capability?.qsvCbrLookAheadMain8 === true)) ||
        (rateControl === 'la_icq' &&
          (main10 ? capability?.qsvLaIcqMain10 === true : capability?.qsvLaIcqMain8 === true))
      ),
  };
}

export function qsvSelectionWarnings(
  features: ReturnType<typeof resolveQSVFeatures>,
  selection: QSVSelection,
) {
  const warnings: string[] = [];
  if (!features.probed) return warnings;
  const selectedRateControlAvailable = {
    icq: features.rateControls.icq,
    la_icq: features.rateControls.laIcq,
    cqp: features.rateControls.cqp,
    vbr: features.rateControls.vbr,
    cbr: features.rateControls.cbr,
  }[features.rateControl as QSVRateControl];

  if (selectedRateControlAvailable === false) {
    warnings.push(`${features.rateControl.toUpperCase().replace('_', '-')} is not validated for the selected bit depth on the active worker; the backend may apply a tested fallback.`);
  }
  if (selection.extendedBRC && !features.extBrc) {
    warnings.push('Extended BRC is only effective for a validated VBR/CBR combination at the selected bit depth. It is not used as a GOP or B-frame correction.');
  }
  if (selection.adaptiveI && !features.adaptiveI) {
    warnings.push('Adaptive I is requested but not validated for this worker/bit-depth combination, so it will be omitted from the effective command.');
  }
  if (selection.adaptiveB && !features.adaptiveB) {
    warnings.push('Adaptive B is requested but not validated for this worker/bit-depth combination, so it will be omitted from the effective command.');
  }
  return warnings;
}
