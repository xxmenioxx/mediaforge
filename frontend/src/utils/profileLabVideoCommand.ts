import type { EffectiveVideoDecision, ProfileInput, ScanResult } from '../api/types';
import { qsvQualityRangeForCrf } from './qsv';
import {
  buildVideoFilterChain,
  combinedAutomaticMotionFilters,
  isTerminalFieldMetadataFilter,
} from './profileLabVideoUtilities';

type VideoToolboxRates = { target: string; maxrate: string; buffer: string };
type VideoColorConversion = {
  filter?: string;
  outputColor?: { space: string; transfer: string; primaries: string; range: string };
};
type FrameStructureRecommendation = { targetGopFrames: number; maxBFrames: number };

export type CombinedVideoCommandContext = {
  videoWorkerValue: (profile: ProfileInput, key: string, fallback?: string) => string;
  videoWorkerBool: (profile: ProfileInput, key: string, fallback?: boolean) => boolean;
  numberWorkerValue: (profile: ProfileInput, key: string, fallback: number) => number;
  softwareEncoderForLabCodec: (codec: string) => string;
  adaptiveVideoToolboxPreviewRates: (profile: ProfileInput, scan: ScanResult) => VideoToolboxRates;
  isTenBitDraft: (profile: ProfileInput) => boolean;
  combinedVideoToolboxColorConversion: (scan: ScanResult) => VideoColorConversion;
  isHardwareEncoderOption: (encoder: string) => boolean;
  effectiveLabX265Params: (profile: ProfileInput) => string;
  frameStructureRecommendationForLab: (
    analysis: ScanResult['frameStructureAnalysis'],
    scan: ScanResult,
    profile: ProfileInput,
  ) => FrameStructureRecommendation;
};

function shellQuote(value: string) {
  return `'${value.replaceAll("'", "'\"'\"'")}'`;
}

function splitResolvedAutomaticMotionFilters(resolvedVideoDecision?: EffectiveVideoDecision) {
  const filters = combinedAutomaticMotionFilters(resolvedVideoDecision);
  return {
    motion: filters.filter((filter) => !isTerminalFieldMetadataFilter(filter)),
    fieldMetadata: filters.filter(isTerminalFieldMetadataFilter),
  };
}

function splitFilterStages(filters: string) {
  return filters.split(',').map((filter) => filter.trim()).filter(Boolean);
}

export function combinedVideoCommandArgs(
  profile: ProfileInput,
  scan: ScanResult,
  resolvedVideoDecision: EffectiveVideoDecision | undefined,
  context: CombinedVideoCommandContext,
) {
  const {
    videoWorkerValue,
    videoWorkerBool,
    numberWorkerValue,
    softwareEncoderForLabCodec,
    adaptiveVideoToolboxPreviewRates,
    isTenBitDraft,
    combinedVideoToolboxColorConversion,
    isHardwareEncoderOption,
    effectiveLabX265Params,
    frameStructureRecommendationForLab,
  } = context;
  if (profile.videoCodec === 'copy') return ['-c:v', 'copy'];
  const preference = videoWorkerValue(profile, 'preferredEncoder', 'software');
  const configuredEncoder = videoWorkerValue(profile, 'videoEncoder', 'auto');
  const encoder = preference === 'hardware' && configuredEncoder !== 'auto'
    ? configuredEncoder
    : softwareEncoderForLabCodec(profile.videoCodec);
  const worker = profile.workerConfig ?? {};
  const restorationFilters = buildVideoFilterChain(worker);
  const args = ['-c:v', encoder];
  const automaticMotion = splitResolvedAutomaticMotionFilters(resolvedVideoDecision);
  const restorationStages = splitFilterStages(restorationFilters);
  const restorationFieldMetadata = restorationStages.filter(isTerminalFieldMetadataFilter);
  const terminalFieldMetadata = automaticMotion.fieldMetadata.length
    ? automaticMotion.fieldMetadata
    : restorationFieldMetadata;
  let filters = [
    automaticMotion.motion.join(','),
    restorationStages.filter((filter) => !isTerminalFieldMetadataFilter(filter)).join(','),
  ].filter(Boolean).join(',');

  if (encoder === 'hevc_qsv') {
    args.push('-preset', videoWorkerValue(profile, 'videoPreset', 'medium'));
    const configuredPixFmt = videoWorkerValue(profile, 'pixFmt', 'yuv420p10le').toLowerCase();
    const qsvTenBit = configuredPixFmt === 'p010' || configuredPixFmt === 'p010le' ||
      configuredPixFmt === 'yuv420p10' || configuredPixFmt === 'yuv420p10le' || configuredPixFmt.includes('10');
    args.push('-pix_fmt', qsvTenBit ? 'p010le' : 'nv12');
    args.push('-profile:v', qsvTenBit ? 'main10' : 'main');
    if (videoWorkerBool(profile, 'qsvLowPower')) args.push('-low_power', '1');

    const qsvRateControl = videoWorkerValue(profile, 'qsvEffectiveRateControl', videoWorkerValue(profile, 'qsvRateControl', 'icq'));
    const qsvGlobalQuality = numberWorkerValue(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue).recommended);
    if (qsvRateControl === 'vbr' || qsvRateControl === 'cbr') {
      const targetBitrate = numberWorkerValue(profile, 'qsvTargetBitrate', 0);
      const maxrate = numberWorkerValue(profile, 'qsvMaxrate', targetBitrate);
      const buffer = numberWorkerValue(profile, 'qsvBuffer', targetBitrate * 2);
      if (targetBitrate > 0) args.push('-b:v', String(targetBitrate), '-maxrate', String(maxrate), '-bufsize', String(buffer));
      else args.push('-global_quality', String(qsvGlobalQuality));
    } else {
      args.push('-global_quality', String(qsvGlobalQuality));
      if (qsvRateControl === 'cqp') args.push('-flags', '+qscale');
    }
    const qsvLookAheadDepth = numberWorkerValue(profile, 'qsvLookAheadDepth', 40);
    if (qsvRateControl === 'la_icq') {
      args.push('-look_ahead', '1');
      if (qsvLookAheadDepth > 0) args.push('-look_ahead_depth', String(Math.min(100, Math.max(10, qsvLookAheadDepth))));
    }
    if (qsvTenBit) {
      if (videoWorkerBool(profile, 'qsvExtendedBRC')) {
        args.push('-extbrc', '1');
        if (qsvRateControl !== 'la_icq' && qsvLookAheadDepth > 0) {
          args.push('-look_ahead_depth', String(Math.min(100, Math.max(10, qsvLookAheadDepth))));
        }
      }
      if (videoWorkerBool(profile, 'qsvAdaptiveI')) args.push('-adaptive_i', '1');
      if (videoWorkerBool(profile, 'qsvAdaptiveB')) args.push('-adaptive_b', '1');
      const pStrategy = numberWorkerValue(profile, 'qsvPStrategy', 0);
      if (pStrategy > 0 && videoWorkerValue(profile, 'frameStructureBFrameMode', 'auto') === 'off') {
        args.push('-p_strategy', String(Math.min(2, pStrategy)));
      }
    }
  } else if (encoder === 'hevc_vaapi') {
    const format = videoWorkerValue(profile, 'pixFmt', 'yuv420p10le').includes('10') ? 'p010le' : 'nv12';
    filters = [filters, `format=${format}`, 'hwupload'].filter(Boolean).join(',');
    args.push('-vaapi_device', '/dev/dri/renderD128');
    args.push('-global_quality', String(numberWorkerValue(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue).recommended)));
  } else if (encoder === 'hevc_videotoolbox') {
    const rates = adaptiveVideoToolboxPreviewRates(profile, scan);
    args.push('-b:v', rates.target, '-maxrate', rates.maxrate, '-bufsize', rates.buffer);
    const tenBit = isTenBitDraft(profile);
    args.push('-profile:v', tenBit ? 'main10' : 'main', '-pix_fmt', tenBit ? 'p010le' : 'yuv420p');
    const realtime = videoWorkerBool(profile, 'videoToolboxRealtime');
    args.push('-realtime', realtime ? '1' : '0');
    if (!videoWorkerValue(profile, 'frameStructureBFrameMode')) {
      const bFramePolicy = videoWorkerValue(profile, 'videoToolboxBFramePolicy', 'auto');
      if (bFramePolicy === 'enabled') args.push('-bf', String(Math.max(1, Math.min(4, numberWorkerValue(profile, 'videoToolboxBFrames', 3)))));
      if (bFramePolicy === 'disabled' || (bFramePolicy === 'auto' && realtime)) args.push('-bf', '0');
    }
    const color = combinedVideoToolboxColorConversion(scan);
    if (color.filter) filters = [filters, color.filter].filter(Boolean).join(',');
    if (color.outputColor) {
      args.push('-colorspace', color.outputColor.space, '-color_trc', color.outputColor.transfer,
        '-color_primaries', color.outputColor.primaries, '-color_range', color.outputColor.range);
    }
  } else if (isHardwareEncoderOption(encoder)) {
    args.push('-cq', String(numberWorkerValue(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue).recommended)));
  } else {
    args.push('-crf', String(profile.qualityValue || 20));
    args.push('-preset', videoWorkerValue(profile, 'videoPreset', 'medium'));
    const pixFmt = videoWorkerValue(profile, 'pixFmt', 'auto');
    if (pixFmt !== 'auto') args.push('-pix_fmt', pixFmt);
    if (encoder === 'libx265') {
      const x265Params = effectiveLabX265Params(profile);
      if (x265Params) args.push('-x265-params', shellQuote(x265Params));
    }
  }

  const frameStructureMode = videoWorkerValue(profile, 'frameStructureMode', 'custom');
  if (frameStructureMode === 'auto') {
    const recommendation = frameStructureRecommendationForLab(scan.frameStructureAnalysis, scan, profile);
    if (recommendation.targetGopFrames > 0) args.push('-g', String(Math.max(1, Math.min(1000, recommendation.targetGopFrames))));
    if (recommendation.maxBFrames > 0) {
      args.push('-bf', String(Math.max(1, Math.min(encoder === 'hevc_videotoolbox' ? 4 : 16, recommendation.maxBFrames))));
    }
  } else {
    const gopMode = videoWorkerValue(profile, 'frameStructureGopMode', 'auto');
    if (gopMode === 'recommended' || gopMode === 'custom') {
      args.push('-g', String(Math.max(1, Math.min(1000, numberWorkerValue(profile, 'frameStructureGopFrames', 120)))));
    }
    const bFrameMode = videoWorkerValue(profile, 'frameStructureBFrameMode', 'auto');
    if (bFrameMode === 'recommended' || bFrameMode === 'custom') {
      args.push('-bf', String(Math.max(1, Math.min(encoder === 'hevc_videotoolbox' ? 4 : 16,
        numberWorkerValue(profile, 'frameStructureMaxBFrames', 3)))));
    }
    if (bFrameMode === 'off') args.push('-bf', '0');
  }
  filters = [filters, terminalFieldMetadata.join(',')].filter(Boolean).join(',');
  if (filters) args.push('-vf', shellQuote(filters));
  return args;
}
