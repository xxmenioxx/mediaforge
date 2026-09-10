import { describe, expect, it } from 'vitest';
import {
  chromaNRWindowError,
  restorationConfigFromLegacyFilters,
  structuredRestorationFilters,
  withStructuredRestorationFilters,
} from './restorationFilters';

describe('structured restoration filters', () => {
  it('requires odd Custom Chroma NR windows in the supported range', () => {
    expect(chromaNRWindowError(3)).toBe('');
    expect(chromaNRWindowError(5)).toBe('');
    expect(chromaNRWindowError(4)).toContain('odd integer');
    expect(chromaNRWindowError(0)).toContain('odd integer');
  });

  it('renders the required custom examples in canonical cleanup order', () => {
    expect(structuredRestorationFilters({
      deblockFilter: 'custom',
      deblockCustomFilter: 'strong',
      deblockCustomBlockSize: 8,
      denoise: 'custom',
      hqdn3dLumaSpatial: 4,
      hqdn3dChromaSpatial: 3,
      hqdn3dLumaTemporal: 6,
      hqdn3dChromaTemporal: 4.5,
      chromaNR: 'custom',
      chromaNRThreshold: 25,
      chromaNRWindowWidth: 3,
      chromaNRWindowHeight: 3,
      deband: 'custom',
      debandThreshold: 0.024,
    })).toEqual([
      'deblock=filter=strong:block=8',
      'chromanr=thres=25:sizew=3:sizeh=3',
      'hqdn3d=4:3:6:4.5',
      'deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024',
    ]);
  });

  it('preserves existing preset meanings', () => {
    expect(structuredRestorationFilters({ deblockFilter: 'light', denoise: 'medium', deband: 'light' })).toEqual([
      'deblock=filter=weak:block=8',
      'hqdn3d=2:2:7:7',
      'deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018',
    ]);
  });

  it('renders Regrain presets exactly and keeps missing configuration off', () => {
    expect(structuredRestorationFilters({})).not.toContain(expect.stringContaining('noise='));
    expect(structuredRestorationFilters({ regrain: 'light' })).toEqual(['noise=c0s=1:c0f=t']);
    expect(structuredRestorationFilters({ regrain: 'medium' })).toEqual(['noise=c0s=2:c0f=t']);
    expect(structuredRestorationFilters({ regrain: 'strong' })).toEqual(['noise=c0s=3:c0f=t']);
  });

  it('renders Custom Regrain deterministically and rejects invalid values', () => {
    expect(structuredRestorationFilters({ regrain: 'custom', regrainLumaStrength: 2.5, regrainChromaStrength: 0.5, regrainTemporal: true, regrainDistribution: 'uniform' }))
      .toEqual(['noise=c0s=2.5:c1s=0.5:c2s=0.5:c0f=tu:c1f=tu:c2f=tu']);
    expect(structuredRestorationFilters({ regrain: 'custom', regrainLumaStrength: Number.NaN, regrainChromaStrength: 0, regrainTemporal: true, regrainDistribution: 'gaussian' })).toEqual([]);
  });

  it('replaces duplicate canonical noise while preserving unknown filters', () => {
    const config = withStructuredRestorationFilters({ videoFilters: 'noise=c0s=1:c0f=t,mystery_filter=keep,noise=c0s=3:c0f=t', regrain: 'medium' });
    expect(config.videoFilters).toBe('mystery_filter=keep,noise=c0s=2:c0f=t');
  });

  it('replaces only controlled filters and keeps the advanced escape hatch', () => {
    const config = withStructuredRestorationFilters({
      videoFilters: 'bwdif=mode=send_frame,mystery_filter=keep,hqdn3d=1.5:1.5:6:6,deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018',
      denoise: 'custom',
      deband: 'custom',
    });
    expect(config.videoFilters).toBe('bwdif=mode=send_frame,mystery_filter=keep,hqdn3d=4:3:6:4.5,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024');
  });

  it('preserves quoted and escaped commas while replacing controlled filters', () => {
    const quoted = withStructuredRestorationFilters({
      videoFilters: "mystery_filter=text='hello,world',hqdn3d=2:2:7:7,deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018",
      denoise: 'custom',
      deband: 'custom',
    });
    expect(quoted.videoFilters).toBe("mystery_filter=text='hello,world',hqdn3d=4:3:6:4.5,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024");

    const escaped = withStructuredRestorationFilters({
      videoFilters: String.raw`mystery_filter=text=hello\,world,hqdn3d=2:2:7:7`,
      denoise: 'custom',
    });
    expect(escaped.videoFilters).toBe(String.raw`mystery_filter=text=hello\,world,hqdn3d=4:3:6:4.5`);
  });

  it('hydrates recognizable legacy filters without changing the raw chain', () => {
    const raw = 'deblock=filter=strong:block=8,hqdn3d=4:3:6:4.5,chromanr=thres=25:sizew=3:sizeh=3,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024';
    const config = restorationConfigFromLegacyFilters({ videoFilters: raw });
    expect(config.videoFilters).toBe(raw);
    expect(config).toMatchObject({
      deblockFilter: 'medium',
      denoise: 'custom',
      chromaNR: 'medium',
      deband: 'custom',
      hqdn3dChromaTemporal: 4.5,
      debandThreshold: 0.024,
    });
  });

  it('hydrates controlled legacy filters after an unknown quoted-comma barrier', () => {
    const raw = "mystery_filter=text='hello,world',hqdn3d=4:3:6:4.5";
    const config = restorationConfigFromLegacyFilters({ videoFilters: raw });
    expect(config.videoFilters).toBe(raw);
    expect(config).toMatchObject({ denoise: 'custom', hqdn3dLumaSpatial: 4, hqdn3dChromaTemporal: 4.5 });
  });
});
