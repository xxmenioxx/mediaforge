import type { UpscaleMode, UpscaleSharpen, VideoWorkerConfig } from './api/types';

export type UpscaleRequest = {
  upscaleMode: UpscaleMode;
  upscaleSharpen: UpscaleSharpen;
  upscaleCustomHeight?: number;
};

const upscaleModes: UpscaleMode[] = ['disabled', 'auto', '720p', '1080p', 'custom'];
const sharpenModes: UpscaleSharpen[] = ['off', 'light', 'medium'];

export function upscaleRequestFromWorkerConfig(config?: VideoWorkerConfig): UpscaleRequest {
  const mode = typeof config?.upscaleMode === 'string' ? config.upscaleMode : 'disabled';
  const sharpen = typeof config?.upscaleSharpen === 'string' ? config.upscaleSharpen : 'off';
  if (!upscaleModes.includes(mode as UpscaleMode)) throw new Error('Invalid upscale mode');
  if (!sharpenModes.includes(sharpen as UpscaleSharpen)) throw new Error('Invalid upscale sharpen mode');
  const customHeight = typeof config?.upscaleCustomHeight === 'number' ? config.upscaleCustomHeight : undefined;
  if (mode === 'custom' && (!customHeight || customHeight < 2 || customHeight % 2 !== 0)) {
    throw new Error('Custom upscale requires a positive even target height');
  }
  return { upscaleMode: mode as UpscaleMode, upscaleSharpen: sharpen as UpscaleSharpen, ...(mode === 'custom' ? { upscaleCustomHeight: customHeight } : {}) };
}

export function workerConfigWithUpscaleRequest(config: VideoWorkerConfig, request: UpscaleRequest): VideoWorkerConfig {
  const validated = upscaleRequestFromWorkerConfig(request);
  const next: VideoWorkerConfig = { ...config, upscaleMode: validated.upscaleMode, upscaleSharpen: validated.upscaleSharpen };
  if (validated.upscaleMode === 'custom') next.upscaleCustomHeight = validated.upscaleCustomHeight;
  else delete next.upscaleCustomHeight;
  delete next.resolvedUpscaleDecision;
  return next;
}
