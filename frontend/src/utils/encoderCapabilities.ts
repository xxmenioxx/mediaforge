import { resolveQSVFeatures } from './qsvCapabilities';

export function qsvAdaptiveCapabilities(
  capability: Record<string, unknown> | undefined,
  main10Selected: boolean,
) {
  const features = resolveQSVFeatures(capability, {
    main10: main10Selected,
  });
  return {
    adaptiveI: features.adaptiveI,
    adaptiveB: features.adaptiveB,
  };
}
