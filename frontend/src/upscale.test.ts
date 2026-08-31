import { describe, expect, it } from 'vitest';
import { upscaleRequestFromWorkerConfig, workerConfigWithUpscaleRequest } from './upscale';

describe('smart upscale profile domain', () => {
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
