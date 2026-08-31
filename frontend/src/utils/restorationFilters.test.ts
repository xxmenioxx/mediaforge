import { describe, expect, it } from 'vitest';
import {
  restorationConfigFromLegacyFilters,
  structuredRestorationFilters,
  withStructuredRestorationFilters,
} from './restorationFilters';

describe('structured restoration filters', () => {
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

  it('replaces only controlled filters and keeps the advanced escape hatch', () => {
    const config = withStructuredRestorationFilters({
      videoFilters: 'bwdif=mode=send_frame,mystery_filter=keep,hqdn3d=1.5:1.5:6:6,deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018',
      denoise: 'custom',
      deband: 'custom',
    });
    expect(config.videoFilters).toBe('bwdif=mode=send_frame,mystery_filter=keep,hqdn3d=4:3:6:4.5,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024');
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
});
