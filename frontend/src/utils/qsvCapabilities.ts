export function resolveQSVFeatures(
  capability: Record<string, unknown> | undefined,
  options: {
    main10: boolean;
    rateControl?: string;
  },
) {
  const { main10, rateControl } = options;

  return {
    adaptiveI:
      main10 &&
      capability?.qsvAdaptiveIMain10 === true,

    adaptiveB:
      main10 &&
      capability?.qsvAdaptiveBMain10 === true,

    extBrc:
      main10 &&
      (
        (rateControl === 'vbr' &&
          capability?.qsvVbrExtBrcMain10 === true) ||
        (rateControl === 'cbr' &&
          capability?.qsvCbrExtBrcMain10 === true)
      ),

    lookAhead:
      main10 &&
      (
        (rateControl === 'vbr' &&
          capability?.qsvVbrLookAheadMain10 === true) ||
        (rateControl === 'cbr' &&
          capability?.qsvCbrLookAheadMain10 === true) ||
        (rateControl === 'la_icq' &&
          capability?.qsvLaIcqMain10 === true)
      ),
  };
}