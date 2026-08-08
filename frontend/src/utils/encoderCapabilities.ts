export function qsvAdaptiveCapabilities(
  capability: Record<string, unknown> | undefined,
  main10Selected: boolean,
) {
  return {
    adaptiveI:
      main10Selected &&
      capability?.qsvAdaptiveIMain10 === true,

    adaptiveB:
      main10Selected &&
      capability?.qsvAdaptiveBMain10 === true,
  };
}
