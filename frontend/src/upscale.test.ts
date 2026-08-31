import { describe, expect, it } from 'vitest';
import { smartUpscaleControlsDisabled, upscaleRequestFromWorkerConfig, workerConfigWithUpscaleRequest } from './upscale';

describe('smart upscale profile domain', () => {
  it('disables controls for Copy and preserves the independent active-job lock', () => {
    expect(smartUpscaleControlsDisabled('copy')).toBe(true);
    expect(smartUpscaleControlsDisabled('COPY')).toBe(true);
    expect(smartUpscaleControlsDisabled('x265')).toBe(false);
    expect(smartUpscaleControlsDisabled('x265', true)).toBe(true);
  });
  it('treats missing legacy fields as disabled without changing the stored config', () => {
    const legacy = { videoEncoder: 'hevc_qsv' };
    expect(upscaleRequestFromWorkerConfig(legacy)).toEqual({ upscaleMode: 'disabled', upscaleSharpen: 'off' });
    expect(legacy).toEqual({ videoEncoder: 'hevc_qsv' });
  });

  it('round-trips a DAR-safe custom height and clears stale resolved state', () => {
    const config = workerConfigWithUpscaleRequest(
      { resolvedUpscaleDecision: { requestedMode: 'auto', resolvedMode: 'disabled' } as never },
      { upscaleMode: 'custom', upscaleSharpen: 'light', upscaleCustomHeight: 900 },
    );
    expect(config).toEqual({ upscaleMode: 'custom', upscaleSharpen: 'light', upscaleCustomHeight: 900 });
    expect(upscaleRequestFromWorkerConfig(config)).toEqual({ upscaleMode: 'custom', upscaleSharpen: 'light', upscaleCustomHeight: 900 });
  });

  it.each([
    [{ upscaleMode: 'invalid' }, 'Invalid upscale mode'],
    [{ upscaleMode: 'auto', upscaleSharpen: 'strong' }, 'Invalid upscale sharpen mode'],
    [{ upscaleMode: 'custom', upscaleCustomHeight: 721 }, 'positive even target height'],
  ])('rejects invalid request %#j', (config, message) => {
    expect(() => upscaleRequestFromWorkerConfig(config as never)).toThrow(message);
  });
});
