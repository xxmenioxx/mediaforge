import type { UpscaleMode, UpscaleSharpen, VideoWorkerConfig } from './api/types';

export type UpscaleRequest = {
  upscaleMode: UpscaleMode;
  upscaleSharpen: UpscaleSharpen;
  upscaleCustomHeight?: number;
  upscaleSharpenCustomStrength?: number;
};

export const upscaleModeOptions: ReadonlyArray<{ value: UpscaleMode; label: string }> = [
  { value: 'disabled', label: 'Disabled' },
  { value: 'auto', label: 'Auto / Recommended' },
  { value: '720p', label: '720p' },
  { value: '1080p', label: '1080p' },
  { value: 'custom', label: 'Custom' },
];

export const upscaleSharpenOptions: ReadonlyArray<{ value: UpscaleSharpen; label: string }> = [
  { value: 'off', label: 'Off' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
  { value: 'custom', label: 'Custom' },
];

const upscaleModes = upscaleModeOptions.map((option) => option.value);
const sharpenModes = upscaleSharpenOptions.map((option) => option.value);

export function upscaleModeLabel(mode: UpscaleMode | 'keep_source') {
  if (mode === 'keep_source') return 'Keep Source';
  return upscaleModeOptions.find((option) => option.value === mode)?.label ?? mode;
}

export function upscaleSharpenLabel(mode: UpscaleSharpen) {
  return upscaleSharpenOptions.find((option) => option.value === mode)?.label ?? mode;
}

export function customUpscaleHeightError(mode: UpscaleMode | undefined, height: number | undefined) {
  if (mode !== 'custom') return '';
  if (!height || height < 2) return 'Target height is required and must be positive.';
  if (height % 2 !== 0) return 'Target height must be even.';
  return '';
}

export function customUpscaleSharpenError(mode: UpscaleSharpen | undefined, strength: number | undefined) {
  if (mode !== 'custom') return '';
  if (strength === undefined || !Number.isFinite(strength)) return 'Custom CAS strength is required.';
  if (strength < 0 || strength > 0.4) return 'Custom CAS strength must be between 0.00 and 0.40.';
  return '';
}

export function smartUpscaleControlsDisabled(videoCodec: string | undefined, readOnly = false) {
  return readOnly || videoCodec?.trim().toLowerCase() === 'copy';
}

const reasonLabels: Record<string, string> = {
  upscale_disabled: 'Smart Upscale is disabled.',
  keep_source_geometry_unresolved: 'Source geometry could not be resolved reliably.',
  keep_source_above_sd: 'The effective source resolution is already above SD.',
  keep_source_evidence_insufficient: 'Available asset evidence is insufficient for a safe upscale recommendation.',
  keep_source_video_copy: 'Video Codec is Copy, so Smart Upscale cannot modify the video stream.',
  reliable_sd_progressive_output: 'The source is SD and the effective output is progressive.',
  conservative_720p_target: '720p is the conservative recommended target.',
  keep_source_target_is_not_an_upscale: 'The requested target would not increase the effective source resolution.',
  target_width_derived_from_effective_dar: 'Target width was derived from the effective display aspect ratio.',
  square_pixel_output: 'The resolved output uses square pixels.',
};

export function upscaleEvidenceLabel(value: string) {
  const normalized = value.trim();
  if (!normalized) return 'Unknown';
  return reasonLabels[normalized] ?? normalized.replaceAll('_', ' ').replace(/^./, (character) => character.toUpperCase());
}

export function upscaleRequestFromWorkerConfig(config?: VideoWorkerConfig): UpscaleRequest {
  const mode = typeof config?.upscaleMode === 'string' ? config.upscaleMode : 'disabled';
  const sharpen = typeof config?.upscaleSharpen === 'string' ? config.upscaleSharpen : 'off';
  if (!upscaleModes.includes(mode as UpscaleMode)) throw new Error('Invalid upscale mode');
  if (!sharpenModes.includes(sharpen as UpscaleSharpen)) throw new Error('Invalid upscale sharpen mode');
  const customHeight = typeof config?.upscaleCustomHeight === 'number' ? config.upscaleCustomHeight : undefined;
  if (mode === 'custom' && (!customHeight || customHeight < 2 || customHeight % 2 !== 0)) {
    throw new Error('Custom upscale requires a positive even target height');
  }
  const customSharpenStrength = typeof config?.upscaleSharpenCustomStrength === 'number' ? config.upscaleSharpenCustomStrength : undefined;
  if (sharpen === 'custom' && customUpscaleSharpenError(sharpen as UpscaleSharpen, customSharpenStrength)) {
    throw new Error('Custom sharpen requires a CAS strength between 0.00 and 0.40');
  }
  return {
    upscaleMode: mode as UpscaleMode,
    upscaleSharpen: sharpen as UpscaleSharpen,
    ...(mode === 'custom' ? { upscaleCustomHeight: customHeight } : {}),
    ...(sharpen === 'custom' ? { upscaleSharpenCustomStrength: customSharpenStrength } : {}),
  };
}

export function workerConfigWithUpscaleRequest(config: VideoWorkerConfig, request: UpscaleRequest): VideoWorkerConfig {
  const validated = upscaleRequestFromWorkerConfig(request);
  const next: VideoWorkerConfig = { ...config, upscaleMode: validated.upscaleMode, upscaleSharpen: validated.upscaleSharpen };
  if (validated.upscaleMode === 'custom') next.upscaleCustomHeight = validated.upscaleCustomHeight;
  else delete next.upscaleCustomHeight;
  if (validated.upscaleSharpen === 'custom') next.upscaleSharpenCustomStrength = validated.upscaleSharpenCustomStrength;
  else delete next.upscaleSharpenCustomStrength;
  delete next.resolvedUpscaleDecision;
	delete next.resolvedRestorationPlan;
  return next;
}
