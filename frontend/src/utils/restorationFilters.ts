export type RestorationMode = 'off' | 'light' | 'medium' | 'strong' | 'custom';

export type RestorationConfig = Record<string, unknown>;

export type StructuredRestorationStages = {
  deblock: string;
  chromaCleanup: string;
  denoise: string;
  deband: string;
};

const controlledFilterNames = new Set(['deblock', 'hqdn3d', 'nlmeans', 'chromanr', 'deband']);

export function structuredRestorationFilters(config: RestorationConfig): string[] {
  const stages = structuredRestorationStages(config);
  return [stages.deblock, stages.chromaCleanup, stages.denoise, stages.deband].filter((value): value is string => Boolean(value));
}

export function structuredRestorationStages(config: RestorationConfig): StructuredRestorationStages {
  return {
    deblock: renderDeblock(config),
    chromaCleanup: renderChromaNR(config),
    denoise: renderDenoise(config),
    deband: renderDeband(config),
  };
}

export function withStructuredRestorationFilters(config: RestorationConfig): RestorationConfig {
  const existing = splitFilterChain(stringValue(config.videoFilters));
  const preserved = existing.filter((filter) => !controlledFilterNames.has(filterName(filter)));
  return {
    ...config,
    videoFilters: [...preserved, ...structuredRestorationFilters(config)].join(','),
  };
}

export function chromaNRWindowError(value: unknown) {
  const parsed = typeof value === 'number' ? value : typeof value === 'string' && value.trim() !== '' ? Number(value) : NaN;
  if (!Number.isInteger(parsed) || parsed < 1 || parsed > 99 || parsed % 2 === 0) {
    return 'Window size must be an odd integer from 1 to 99.';
  }
  return '';
}

export function restorationConfigFromLegacyFilters(config: RestorationConfig): RestorationConfig {
  const result = { ...config };
  for (const filter of splitFilterChain(stringValue(config.videoFilters))) {
    const name = filterName(filter);
    const value = filter.slice(filter.indexOf('=') + 1);
    if (name === 'deblock' && result.deblockFilter === undefined) {
      const match = /^filter=(weak|strong):block=(\d+)$/.exec(value);
      if (!match) continue;
      if (match[1] === 'weak' && match[2] === '8') result.deblockFilter = 'light';
      else if (match[1] === 'strong' && match[2] === '8') result.deblockFilter = 'medium';
      else if (match[1] === 'strong' && match[2] === '4') result.deblockFilter = 'strong';
      else {
        result.deblockFilter = 'custom';
        result.deblockCustomFilter = match[1];
        result.deblockCustomBlockSize = Number(match[2]);
      }
    }
    if (name === 'hqdn3d' && result.denoise === undefined) {
      if (value === '1.0:1.0:3.0:3.0') result.denoise = 'film-grain';
      else if (value === '1.2:1.0:4.0:3.0') result.denoise = 'film-restore';
      else if (value === '1.5:1.5:6:6') result.denoise = 'light';
      else if (value === '2:2:7:7') result.denoise = 'medium';
      else {
        const values = value.split(':').map(Number);
        if (values.length === 4 && values.every(Number.isFinite)) {
          result.denoise = 'custom';
          [result.hqdn3dLumaSpatial, result.hqdn3dChromaSpatial, result.hqdn3dLumaTemporal, result.hqdn3dChromaTemporal] = values;
        }
      }
    }
    if (name === 'nlmeans' && result.denoise === undefined && value === 's=2:p=7:r=15') result.denoise = 'strong';
    if (name === 'chromanr' && result.chromaNR === undefined) {
      const match = /^thres=([^:]+):sizew=(\d+):sizeh=(\d+)$/.exec(value);
      if (!match) continue;
      if (match[1] === '15' && match[2] === '3' && match[3] === '3') result.chromaNR = 'light';
      else if (match[1] === '25' && match[2] === '3' && match[3] === '3') result.chromaNR = 'medium';
      else if (match[1] === '35' && match[2] === '5' && match[3] === '5') result.chromaNR = 'strong';
      else {
        result.chromaNR = 'custom';
        result.chromaNRThreshold = Number(match[1]);
        result.chromaNRWindowWidth = Number(match[2]);
        result.chromaNRWindowHeight = Number(match[3]);
      }
    }
    if (name === 'deband' && result.deband === undefined) {
      const match = /^1thr=([^:]+):2thr=\1:3thr=\1:4thr=\1$/.exec(value);
      if (!match) continue;
      if (match[1] === '0.018') result.deband = 'light';
      else if (match[1] === '0.028') result.deband = 'medium';
      else if (match[1] === '0.04') result.deband = 'strong';
      else {
        result.deband = 'custom';
        result.debandThreshold = Number(match[1]);
      }
    }
  }
  return result;
}

function renderDeblock(config: RestorationConfig) {
  switch (stringValue(config.deblockFilter, 'off')) {
    case 'light': return 'deblock=filter=weak:block=8';
    case 'medium': return 'deblock=filter=strong:block=8';
    case 'strong': return 'deblock=filter=strong:block=4';
    case 'custom': {
      const filter = stringValue(config.deblockCustomFilter, 'strong') === 'weak' ? 'weak' : 'strong';
      const block = integerValue(config.deblockCustomBlockSize, 8, 4, 64);
      return `deblock=filter=${filter}:block=${block}`;
    }
    default: return '';
  }
}

function renderDenoise(config: RestorationConfig) {
  switch (stringValue(config.denoise, 'off')) {
    case 'film-grain': return 'hqdn3d=1.0:1.0:3.0:3.0';
    case 'film-restore': return 'hqdn3d=1.2:1.0:4.0:3.0';
    case 'light': return 'hqdn3d=1.5:1.5:6:6';
    case 'medium': return 'hqdn3d=2:2:7:7';
    case 'strong': return 'nlmeans=s=2:p=7:r=15';
    case 'custom': return `hqdn3d=${numberValue(config.hqdn3dLumaSpatial, 4, 0, 100)}:${numberValue(config.hqdn3dChromaSpatial, 3, 0, 100)}:${numberValue(config.hqdn3dLumaTemporal, 6, 0, 100)}:${numberValue(config.hqdn3dChromaTemporal, 4.5, 0, 100)}`;
    default: return '';
  }
}

function renderChromaNR(config: RestorationConfig) {
  switch (stringValue(config.chromaNR, 'off')) {
    case 'light': return 'chromanr=thres=15:sizew=3:sizeh=3';
    case 'medium': return 'chromanr=thres=25:sizew=3:sizeh=3';
    case 'strong': return 'chromanr=thres=35:sizew=5:sizeh=5';
    case 'custom': return `chromanr=thres=${numberValue(config.chromaNRThreshold, 25, 1, 100)}:sizew=${oddIntegerValue(config.chromaNRWindowWidth, 3)}:sizeh=${oddIntegerValue(config.chromaNRWindowHeight, 3)}`;
    default: return '';
  }
}

function renderDeband(config: RestorationConfig) {
  const mode = stringValue(config.deband, 'off');
  const threshold = mode === 'light' ? 0.018 : mode === 'medium' ? 0.028 : mode === 'strong' ? 0.04 : mode === 'custom' ? numberValue(config.debandThreshold, 0.024, 0.001, 1) : 0;
  if (threshold === 0) return '';
  return `deband=1thr=${threshold}:2thr=${threshold}:3thr=${threshold}:4thr=${threshold}`;
}

function splitFilterChain(value: string) {
  const filters: string[] = [];
  let start = 0;
  let quote = '';
  let escaped = false;
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (character === '\\') {
      escaped = true;
      continue;
    }
    if (quote) {
      if (character === quote) quote = '';
      continue;
    }
    if (character === "'" || character === '"') {
      quote = character;
      continue;
    }
    if (character === ',') {
      filters.push(value.slice(start, index));
      start = index + 1;
    }
  }
  filters.push(value.slice(start));
  return filters.map((filter) => filter.trim()).filter(Boolean);
}

function filterName(filter: string) {
  return filter.split('=', 1)[0].trim().toLowerCase();
}

function stringValue(value: unknown, fallback = '') {
  return typeof value === 'string' ? value : fallback;
}

function numberValue(value: unknown, fallback: number, min: number, max: number) {
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
  return Number.isFinite(parsed) && parsed >= min && parsed <= max ? parsed : fallback;
}

function integerValue(value: unknown, fallback: number, min: number, max: number) {
  const parsed = numberValue(value, fallback, min, max);
  return Number.isInteger(parsed) ? parsed : fallback;
}

function oddIntegerValue(value: unknown, fallback: number) {
  const parsed = integerValue(value, fallback, 1, 99);
  return parsed % 2 === 1 ? parsed : fallback;
}
