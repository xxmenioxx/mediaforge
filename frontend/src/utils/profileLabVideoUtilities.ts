import type { EffectiveVideoDecision } from '../api/types';
import { structuredRestorationStages } from './restorationFilters';
import { formatFilterNumber } from './imageAdjustmentPrecision';

function stringValue(value: unknown, fallback = '') {
  return typeof value === 'string' ? value : fallback;
}

function numericWorkerConfigValue(workerConfig: Record<string, unknown>, key: string, fallback: number) {
  const value = workerConfig[key];
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
  return Number.isFinite(parsed) ? parsed : fallback;
}

function trimGain(value: number) {
  return formatFilterNumber(value);
}

export function buildVideoFilterChain(workerConfig: Record<string, unknown>) {
  const filters: string[] = [];
  const correctProgressiveFieldMetadata = workerConfig.correctProgressiveFieldMetadata === true;
  const deflicker = stringValue(workerConfig.deflicker, 'off');
  const unsharp = stringValue(workerConfig.unsharp, 'off');
  const crop = stringValue(workerConfig.crop, 'off');
  const cropValue = stringValue(workerConfig.cropValue, '');
  const brightness = numericWorkerConfigValue(workerConfig, 'brightness', 0);
  const contrast = numericWorkerConfigValue(workerConfig, 'contrast', 100);
  const saturation = numericWorkerConfigValue(workerConfig, 'saturation', 100);
  const gamma = numericWorkerConfigValue(workerConfig, 'gamma', 100);
  const temperature = numericWorkerConfigValue(workerConfig, 'temperature', 0);
  const tint = numericWorkerConfigValue(workerConfig, 'tint', 0);
  const exposure = numericWorkerConfigValue(workerConfig, 'exposure', 0);
  const vibrance = numericWorkerConfigValue(workerConfig, 'vibrance', 0);
  const blackPoint = numericWorkerConfigValue(workerConfig, 'blackPoint', 0);
  const whitePoint = numericWorkerConfigValue(workerConfig, 'whitePoint', 100);
  const restoration = structuredRestorationStages(workerConfig);

  if (deflicker === 'light') filters.push('deflicker=size=5:mode=pm');
  else if (deflicker === 'medium') filters.push('deflicker=size=9:mode=pm');
  if (restoration.deblock) filters.push(restoration.deblock);
  if (crop === 'manual' && cropValue.trim()) filters.push(`crop=${cropValue.trim()}`);
  if (restoration.chromaCleanup) filters.push(restoration.chromaCleanup);
  if (restoration.denoise) filters.push(restoration.denoise);
  if (restoration.deband) filters.push(restoration.deband);
  if (exposure !== 0) filters.push(`exposure=exposure=${trimGain(Math.max(-200, Math.min(200, exposure)) / 100)}`);
  if (blackPoint !== 0 || whitePoint !== 100) {
    const inputBlack = trimGain(Math.max(0, Math.min(10, blackPoint)) / 100);
    const inputWhite = trimGain(Math.max(90, Math.min(100, whitePoint)) / 100);
    filters.push(`colorlevels=rimin=${inputBlack}:gimin=${inputBlack}:bimin=${inputBlack}:rimax=${inputWhite}:gimax=${inputWhite}:bimax=${inputWhite}`);
  }
  if (brightness !== 0 || contrast !== 100 || saturation !== 100 || gamma !== 100) {
    const eqParts = [
      `brightness=${trimGain((Math.max(-100, Math.min(100, brightness)) / 100) * 0.12)}`,
      `contrast=${trimGain(Math.max(50, Math.min(150, contrast)) / 100)}`,
      `saturation=${trimGain(Math.max(50, Math.min(150, saturation)) / 100)}`,
      `gamma=${trimGain(Math.max(70, Math.min(130, gamma)) / 100)}`,
    ];
    filters.push(`eq=${eqParts.join(':')}`);
  }
  if (vibrance !== 0) filters.push(`vibrance=intensity=${trimGain(Math.max(-100, Math.min(100, vibrance)) / 100)}`);
  if (temperature !== 0 || tint !== 0) {
    const warm = Math.max(-100, Math.min(100, temperature)) / 100;
    const magenta = Math.max(-100, Math.min(100, tint)) / 100;
    const red = trimGain(warm * 0.08);
    const blue = trimGain(-warm * 0.08);
    const green = trimGain(-magenta * 0.06);
    filters.push(`colorbalance=rs=${red}:bs=${blue}:gs=${green}:rm=${red}:bm=${blue}:gm=${green}`);
  }
  if (unsharp === 'subtle') filters.push('unsharp=5:5:0.10:5:5:0.0');
  else if (unsharp === 'film-restore') filters.push('unsharp=5:5:0.15:5:5:0.0');
  else if (unsharp === 'light') filters.push('unsharp=5:5:0.25:5:5:0.0');
  if (restoration.regrain) filters.push(restoration.regrain);
  if (correctProgressiveFieldMetadata) filters.push('setfield=prog');
  return filters.join(',');
}

export function combinedAutomaticMotionFilters(resolvedVideoDecision?: EffectiveVideoDecision) {
  return (resolvedVideoDecision?.effectiveMotionFilters || '')
    .split(',')
    .map((filter) => filter.trim())
    .filter(Boolean);
}

export function isTerminalFieldMetadataFilter(filter: string) {
  return /^setfield\s*=/i.test(filter.trim());
}
