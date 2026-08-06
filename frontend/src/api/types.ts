export type Library = {
  id: number;
  name: string;
  sourcePath: string;
  destinationPath: string;
  type: string;
  validationRules: Record<string, unknown>;
  defaultProfileId?: number;
  createdAt: string;
  updatedAt: string;
};

export type LibraryInput = {
  name: string;
  sourcePath: string;
  destinationPath: string;
  type: string;
  validationRules?: Record<string, unknown>;
  defaultProfileId?: number;
};

export type UpdateLibraryInput = LibraryInput & {
  id: number;
};

export type PathEntry = {
  path: string;
  relativePath: string;
  name: string;
};

export type PathBrowseResponse = {
  root: string;
  rootKey: 'raw' | 'library' | 'staging';
  paths: PathEntry[];
};

export type AudioEnhancementProfile = {
  key: string;
  name: string;
  description: string;
  intent: string;
  filters: string;
  rnnoiseModelPath: string;
  channelMode: 'preserve' | 'dual-mono' | 'force-stereo' | 'downmix-mono' | 'light-stereo';
  forceStereoMode: 'auto' | 'first-two' | 'duplicate-first';
  stereoDelayMs: number;
  stereoWidth: number;
  eqBands: Record<string, number>;
  preserveOriginalTrack: boolean;
  outputCodec: string;
  targetLoudness: number;
  truePeak: number;
  notes: string;
  disabled?: boolean;
  deletedAt?: string;
};

export type Asset = {
  libraryId: number;
  libraryName: string;
  path: string;
  relativePath: string;
  groupPath: string;
  fileName: string;
  extension: string;
  sizeBytes: number;
  modifiedAt: string;
  status: 'unprocessed' | 'library' | 'published_as_is' | 'converted' | 'unverified' | 'archive';
  missing: boolean;
  expiresAt?: string;
  review: AssetReviewState;
  metadata: AssetMetadataState;
  conversion: AssetConversionOverrideState;
  publicationMode?: 'as_is';
  technical?: {
    videoCodec: string;
    encoder?: string;
    width: number;
    height: number;
    duration: number;
    bitrate: number;
    hdr: boolean;
  };
};

export type AssetGroup = {
  id: string;
  libraryId: number;
  libraryName: string;
  path: string;
  relativePath: string;
  status: 'unprocessed' | 'library' | 'published_as_is' | 'converted' | 'unverified' | 'archive';
  fileCount: number;
  sizeBytes: number;
  modifiedAt: string;
  assets: Asset[];
  review: AssetReviewState;
  pathReview: AssetReviewState;
  metadata: AssetMetadataState;
  pathMetadata: AssetMetadataState;
};

export type AssetReviewState = {
  requiresReview: boolean;
  reason: string;
  source: string;
  tags: string[];
  updatedAt: string;
};

export type AssetMetadataState = {
  categories: string[];
  tags: string[];
  updatedAt: string;
};

export type AssetConversionOverrideState = {
  trackProfileKey?: string;
  keepVideoStreams?: number[] | null;
  keepAudioStreams?: number[] | null;
  keepSubtitleStreams?: number[] | null;
  videoMetadata?: Record<string, StreamMetadataOverride>;
  audioMetadata?: Record<string, StreamMetadataOverride>;
  subtitleMetadata?: Record<string, StreamMetadataOverride>;
  subtitleTransforms?: Array<{
    streamIndex: number;
    format: 'srt' | 'ass';
    removeEmbedded: boolean;
    makeDefault: boolean;
    language: string;
    ocrLanguage?: string;
    ocrMode?: 'raw' | 'clean' | 'accurate';
    title?: string;
  }>;
  videoCodec?: string;
  audioCodec?: string;
  qualityMode?: string;
  qualityValue?: number;
  videoPreset?: string;
  pixFmt?: string;
  videoFilters?: string;
  deinterlaceMode?: 'auto' | 'off' | 'force' | 'ivtc_tff' | 'ivtc_bff';
  x265Params?: string;
  preserveHdr?: boolean;
  preserveSubtitles?: boolean;
  preserveChapters?: boolean;
  externalSubtitleFormat?: 'disabled' | 'source' | 'srt' | 'ass' | 'remove';
  finalColorPolicy?: 'automatic' | 'preserve' | 'normalize_bt709';
  addAacStereoTrack?: boolean;
  aacStereoDefault?: boolean;
  enhancedAudioSourceStreamIndex?: number;
  useHardwareIfAvailable?: boolean;
  videoEncoder?: string;
  preferredEncoder?: 'software' | 'hardware' | 'auto';
  globalQuality?: number;
  qsvRateControl?: 'icq' | 'la_icq';
  qsvLookAheadDepth?: number;
  qsvExtendedBrc?: boolean;
  qsvAdaptiveI?: boolean;
  qsvAdaptiveB?: boolean;
  videoToolboxBitrateMbps?: number;
  videoToolboxMaxrateMbps?: number;
  videoToolboxBufferMbps?: number;
  videoToolboxQualityProfile?: number;
  videoToolboxProfile?: string;
  videoToolboxGop?: number;
  videoToolboxRealtime?: boolean;
  videoToolboxBFramePolicy?: 'auto' | 'enabled' | 'disabled';
  videoToolboxBFrames?: number;
  videoToolboxAutoAdjustBitrate?: boolean;
  videoToolboxAllowFrameReordering?: boolean;
  videoToolboxPowerEfficiency?: boolean;
  hardwareQualityPreset?: string;
  updatedAt?: string;
};

export type ExternalSubtitle = {
  path: string;
  fileName: string;
  format: 'srt' | 'ass';
  language?: string;
  default: boolean;
  forced: boolean;
  sizeBytes: number;
  modifiedAt: string;
};

export type StreamMetadataOverride = {
  title?: string;
  language?: string;
  default?: boolean;
  forced?: boolean;
};

export type AssetReviewUpdateInput = {
  path: string;
  requiresReview: boolean;
  reason?: string;
  source?: string;
  tags?: string[];
};

export type AssetMetadataUpdateInput = {
  path: string;
  categories: string[];
  tags: string[];
};

export type AssetConversionUpdateInput = AssetConversionOverrideState & {
  path: string;
};

export type AssetInventory = {
  unprocessed: Asset[];
  library: Asset[];
  converted: Asset[];
  unverified: Asset[];
  archive: Asset[];
  missing: Asset[];
  unprocessedGroups: AssetGroup[];
  libraryGroups: AssetGroup[];
  convertedGroups: AssetGroup[];
  unverifiedGroups: AssetGroup[];
  archiveGroups: AssetGroup[];
  reports: AssetReports;
  sync: AssetSyncInfo;
};

export type AssetReports = {
  unprocessedFiles: number;
  libraryFiles: number;
  convertedFiles: number;
  unverifiedFiles: number;
  archiveFiles: number;
  archiveBytes: number;
  expiredArchive: number;
  missingFiles: number;
  missingActionable: number;
  missingHistorical: number;
};

export type AssetSyncInfo = {
  lastSyncedAt: string;
  totalRecords: number;
  missingFiles: number;
  missingActionable: number;
  missingHistorical: number;
};

export type AssetSyncResult = {
  syncedAt: string;
  unprocessedFiles: number;
  libraryFiles: number;
  convertedFiles: number;
  unverifiedFiles: number;
  archiveFiles: number;
  expiredDeleted: number;
  reconciledFiles: number;
  reviewMatches: number;
};

export type AdvisorRequest = {
  mediaPath: string;
  profileId: number;
};

export type AdvisorResponse = {
  recommendation: 'worth_it' | 'maybe' | 'not_recommended';
  score: number;
  summary: string;
  reasons: string[];
  warnings: string[];
  estimated: {
    currentSizeBytes: number;
    targetCodec: string;
    targetContainer: string;
  };
  scan: ScanResult;
  profile: Profile;
};

export type ProfileCandidate = {
  profile: Profile;
  score: number;
  reasons: string[];
};

export type ProfileSuggestion = {
  matchType: 'existing' | 'create';
  summary: string;
  scan: ScanResult;
  suggestedProfile?: Profile;
  candidates: ProfileCandidate[];
  proposedProfile: ProfileInput;
  insights: {
    recommendedCrf: number;
    estimatedMinBytes: number;
    estimatedMaxBytes: number;
    estimatedSavingsLow: number;
    estimatedSavingsHigh: number;
    recommendations: string[];
  };
};

export type Profile = {
  id: number;
  name: string;
  description: string;
  container: string;
  videoCodec: string;
  codecFamily: string;
  encoderPolicy: 'locked' | 'restricted' | 'automatic';
  preferredEncoder: string;
  allowedEncoders: string[];
  fallbackPolicy: 'wait' | 'allowed_only';
  bitDepth: number;
  pixelFormat: string;
  qualityStrategy: string;
  profileVersion: number;
  audioCodec: string;
  qualityMode: string;
  qualityValue: number;
  preserveHdr: boolean;
  preserveSubtitles: boolean;
  preserveChapters: boolean;
  workerConfig: Record<string, unknown>;
  disabled: boolean;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string | null;
};

export type ProfileInput = {
  name: string;
  description: string;
  container: string;
  videoCodec: string;
  codecFamily?: string;
  encoderPolicy?: 'locked' | 'restricted' | 'automatic';
  preferredEncoder?: string;
  allowedEncoders?: string[];
  fallbackPolicy?: 'wait' | 'allowed_only';
  bitDepth?: number;
  pixelFormat?: string;
  qualityStrategy?: string;
  audioCodec: string;
  qualityMode: string;
  qualityValue: number;
  preserveHdr: boolean;
  preserveSubtitles: boolean;
  preserveChapters: boolean;
  workerConfig?: Record<string, unknown>;
  disabled?: boolean;
};

export type UpdateProfileInput = ProfileInput & {
  id: number;
};

export type AppSetting = {
  key: string;
  value: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
};

export type UpdateSettingInput = {
  key: string;
  value: Record<string, unknown>;
};

export type MWPImportSummary = {
  profilesCreated: number;
  profilesUpdated: number;
  libraryTypesCreated: number;
  libraryTypesUpdated: number;
  settingsUpdated: string[];
};

export type SoftwareComponent = {
  name: string;
  version: string;
  source: string;
};

export type SoftwareVersions = {
  components: SoftwareComponent[];
};

export type RuntimeSnapshot = {
  id: number;
  detectedAt: string;
  os: string;
  architecture: string;
  container: boolean;
  cpuCores: number;
  cpuLoad1: number;
  gpuDetected: boolean;
  gpuUsageAvailable: boolean;
  gpuUsagePercent: number;
  gpuMediaUsagePercent: number;
  gpuRenderUsagePercent: number;
  gpuMetricSource: string;
  totalMemoryBytes: number;
  availableMemoryBytes: number;
  batteryPresent: boolean;
  batteryPercent: number;
  powerSource: string;
  onBattery: boolean;
  disks: Record<string, unknown>;
  encoders: Record<string, { listed: boolean; usable: boolean; reason: string; main10?: boolean; icq?: boolean; lowPower?: boolean; lookAhead?: boolean; extendedBrc?: boolean; adaptiveI?: boolean; adaptiveB?: boolean; qsvFullCombination?: boolean; qsvIcqMain8?: boolean; qsvIcqMain10?: boolean; qsvLaIcqMain10?: boolean; qsvCqpMain8?: boolean; qsvCqpMain10?: boolean; qsvVbrMain8?: boolean; qsvVbrMain10?: boolean; qsvCbrMain8?: boolean; qsvCbrMain10?: boolean; qsvLowPowerMain10?: boolean; videoToolboxMain?: boolean; videoToolboxMain10?: boolean; videoToolboxBFrames?: boolean; videoToolboxBFramesVerified?: boolean; videoToolboxBFramesDisabled?: boolean; videoToolboxObservedBFrames?: number; videoToolboxPowerEfficient?: boolean; testedModes?: Record<string, boolean>; modeReasons?: Record<string, string> }>;
  recommendedProfile: string;
  selectedProfile: string;
  preferredProfile: string;
  fallbackProfile: string;
  appliedOverrides: string[];
  effectivePolicy: Record<string, unknown>;
  selectionReasons: unknown[];
  warnings: unknown[];
  createdAt: string;
};

export type RuntimeProfileValues = {
  maxRunningJobs: number; maxVideoJobs: number; maxSoftwareX265Jobs: number; maxHardwareEncodeJobs: number;
  maxAudioJobs: number; maxLabJobs: number; minFreeRamGb: number; minFreeWorkGb: number;
  minFreeLibraryGb: number; maxWorkspaceGb: number; allowDirectMode: boolean;
  pauseWhenOnBattery: boolean; preventSleepDuringJobs: boolean;
};

export type RuntimeProfileDefinition = { key: string; name: string; description: string; official: boolean; values: RuntimeProfileValues };
export type RuntimeProfileOverride = Partial<RuntimeProfileValues>;
export type EffectiveRuntimePolicy = {
  mode: 'automatic' | 'manual'; detectedProfile: string; preferredProfile: string; fallbackProfile: string; baseProfile: string;
  values: RuntimeProfileValues; overrides: RuntimeProfileOverride; overriddenFields: string[]; selectionReasons: unknown[];
};
export type RuntimeProfilesResponse = { profiles: RuntimeProfileDefinition[]; effective: EffectiveRuntimePolicy };

export type QueueJob = {
  id: number;
  executionNumber?: number;
  batchId: string;
  batchName: string;
  mediaPath: string;
  publishMode: 'standard' | 'replace_library_asset';
  libraryId: number;
  profileId: number;
  profileVersion: number;
  profileSnapshot: Record<string, unknown>;
  profileCapturedAt?: string;
  activeExecutionPlanId?: number;
  audioProfileKey: string;
  trackProfileKey: string;
  processingMode: 'full_encode' | 'audio_only' | '';
  priority: number;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled';
  stage: string;
  stageUpdatedAt?: string;
  stageHistory: Array<Record<string, unknown>>;
  progress: number;
  workerName: string;
  outputPath: string;
  plannedPublishedPath: string;
  errorMessage: string;
  notes: string;
  validationStatus: 'pending' | 'passed' | 'warning' | 'failed' | '';
  validationScore: number;
  validationReport: Record<string, unknown>;
  publishedPath: string;
  publishedAt?: string;
  publicationRetiredAt?: string;
  replacementTargetPath: string;
  originalArchivedPath: string;
  subtitleArtifacts?: Array<{
    streamIndex: number;
    sourceCodec: string;
    format: 'srt' | 'ass';
    language: string;
    default: boolean;
    stagedPath: string;
    publishedPath?: string;
    sizeBytes: number;
  }>;
  startedAt?: string;
  finishedAt?: string;
  dismissedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type WorkerNode = {
  id: number; name: string; status: 'online' | 'offline'; maxConcurrentJobs: number; encoders: unknown[];
  runtimeProfile: string; lastSeenAt: string; createdAt: string; updatedAt: string;
};

export type SchedulerRecoveryReport = {
  ranAt: string; interruptedJobs: number; partialOutputsPreserved: number; reservationsReleased: number;
  workersMarkedOffline: number; missingCompletedOutputs: number; missingPublishedOutputs: number;
  orphanWorkspacePaths: string[]; warnings: string[];
};

export type HousekeepingCandidate = { path: string; jobId?: number; reason: string; sizeBytes: number; modifiedAt: string };
export type HousekeepingReport = { ranAt: string; dryRun: boolean; candidates: HousekeepingCandidate[]; removedPaths: string[]; recoveredBytes: number; errors: string[] };

export type ExecutionPlan = {
  id: number;
  jobId: number;
  version: number;
  status: 'pending_evaluation' | 'ready' | 'waiting' | 'dispatched' | 'superseded';
  profileVersion: number;
  constraints: Record<string, unknown>;
  codecFamily: string;
  selectedEncoder: string;
  bitDepth: number;
  pixelFormat: string;
  qualityMode: string;
  qualityValue: number;
  runtimeProfile: string;
  runtimeSnapshotId?: number;
  workspaceMode: string;
  waitingState: string;
  decisionReasons: unknown[];
  decisionSources: Record<string, unknown>;
  warnings: unknown[];
  reservation: Record<string, unknown>;
  evaluation: Record<string, unknown>;
  outputPath: string;
  inputSizeBytes: number;
  estimatedOutputMinBytes: number;
  estimatedOutputMaxBytes: number;
  estimatedWorkspaceBytes: number;
  estimateConfidence: string;
  approvalStatus: 'pending' | 'auto_approved' | 'manually_approved' | 'rejected' | '';
  approvalMode: 'manual' | 'automatic' | 'conditional' | '';
  approvedAt?: string;
  rejectedAt?: string;
  supersededAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type JobArtifactsResponse = {
  jobId: number;
  asIs?: Record<string, unknown>;
  result?: Record<string, unknown>;
  asIsPath?: string;
  resultPath?: string;
  warnings?: string[];
};

export type AnalysisBackfillResponse = {
  imported: number;
  corrected: number;
  skipped: number;
  total: number;
};

export type ValidationCheck = {
  key: string;
  label: string;
  status: 'passed' | 'failed';
  message: string;
};

export type ValidationResult = {
  jobId: number;
  status: 'passed' | 'warning' | 'failed';
  score: number;
  checks: ValidationCheck[];
  warnings: string[];
  report: Record<string, unknown>;
};

export type PublishResult = {
  jobId: number;
  status: 'published';
  sourcePath: string;
  publishedPath: string;
  message: string;
};

export type LogFile = {
  name: string;
  category: 'system' | 'scheduler' | 'workers' | 'pipeline' | 'jobs';
  description: string;
  sizeBytes: number;
  modifiedAt: string;
};

export type LogFileContent = {
  name: string;
  content: string;
  truncated: boolean;
};

export type QueueJobInput = {
  mediaPath: string;
  publishMode?: 'standard' | 'replace_library_asset';
  batchId?: string;
  batchName?: string;
  libraryId: number;
  profileId: number;
  audioProfileKey?: string;
  trackProfileKey?: string;
  processingMode?: 'full_encode' | 'audio_only';
  priority: number;
  notes?: string;
};

export type ClaimJobInput = {
  workerName: string;
};

export type UpdateJobStatusInput = {
  jobId: number;
  status: QueueJob['status'];
  progress?: number;
  outputPath?: string;
  errorMessage?: string;
};

export type QueueJobUpdateInput = {
  jobId: number;
  mediaPath?: string;
  libraryId?: number;
  profileId?: number;
  audioProfileKey?: string;
  trackProfileKey?: string;
  processingMode?: 'full_encode' | 'audio_only';
  priority?: number;
  status?: QueueJob['status'];
  notes?: string;
};

export type ExecuteJobInput = {
  jobId: number;
  overwrite?: boolean;
};

export type MediaStreamInfo = {
  index: number;
  type: 'video' | 'audio' | 'subtitle';
  codec: string;
  codecLong: string;
  profile: string;
  language: string;
  title: string;
  duration: number;
  bitrate: number;
  sizeBytes?: number;
  sizeEstimated?: boolean;
  default: boolean;
  forced: boolean;
  comment: boolean;
  hearingImpaired: boolean;
  width?: number;
  height?: number;
  pixFmt?: string;
  colorSpace?: string;
  colorRange?: string;
  colorTransfer?: string;
  colorPrimaries?: string;
  chromaLocation?: string;
  bitsPerRawSample?: string;
  avgFrameRate?: string;
  realFrameRate?: string;
  sampleAspectRatio?: string;
  displayAspectRatio?: string;
  hdr?: boolean;
  fieldOrder?: string;
  channels?: number;
  channelLayout?: string;
  sampleRate?: number;
  bitDepth?: number;
};

export type ScanResult = {
  id: number;
  path: string;
  fileName: string;
  container: string;
  sizeBytes: number;
  duration: number;
  bitrate: number;
  videoCodec: string;
  width: number;
  height: number;
  hdr: boolean;
  audioTracks: number;
  subtitleTracks: number;
  chapters: number;
  videoStreams: MediaStreamInfo[];
  audioStreams: MediaStreamInfo[];
  subtitleStreams: MediaStreamInfo[];
  compatibilityAnalysis: {
    version?: number;
    target?: string;
    overall?: 'direct_play_likely' | 'client_dependent' | 'transcode_likely' | string;
    score?: number;
    video?: string;
    audio?: string;
    subtitles?: string;
    reasons?: string[];
    warnings?: string[];
    recommendations?: string[];
  };
  interlaceAnalysis: {
    version?: number;
    status?: 'progressive' | 'interlaced' | 'mixed' | 'telecine_suspected' | 'unknown';
    fieldOrder?: string;
    containerFieldOrder?: string;
    detectedFieldOrder?: 'tff' | 'bff' | string;
    fieldOrderMismatch?: boolean;
    source?: string;
    confidence?: number;
    tff?: number;
    bff?: number;
    progressive?: number;
    undetermined?: number;
    sampledFrames?: number;
    recommendedFilter?: string;
    windowStart?: number;
    windowSeconds?: number;
    sampleCount?: number;
    sampledAt?: number[];
    recommendedMode?: 'force' | 'ivtc_tff' | 'ivtc_bff' | string;
    recommendedFieldMetadataMode?: 'progressive' | 'tff' | 'bff' | string;
    ivtcValidation?: {
      tffProgressive?: number;
      tffClassified?: number;
      tffProgressiveRatio?: number;
      bffProgressive?: number;
      bffClassified?: number;
      bffProgressiveRatio?: number;
      selectedOrder?: 'tff' | 'bff' | string;
      confidence?: number;
    };
  };
  cropAnalysis: {
    version?: number;
    status?: 'none' | 'detected' | 'variable' | 'unknown';
    source?: string;
    confidence?: number;
    recommendedCrop?: string;
    originalWidth?: number;
    originalHeight?: number;
    outputWidth?: number;
    outputHeight?: number;
    x?: number;
    y?: number;
    windows?: number;
    matchingWindows?: number;
    sampledAt?: number[];
    reason?: string;
  };
  rawProbe: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
};

export type PreviewVideoCharacteristics = {
  codec: string;
  profile?: string;
  pixelFormat?: string;
  bitDepth?: number;
  width: number;
  height: number;
  sampleAspectRatio?: string;
  displayAspectRatio?: string;
  frameRate?: string;
  fieldOrder?: string;
  colorRange?: string;
  colorSpace?: string;
  colorTransfer?: string;
  colorPrimaries?: string;
  chromaLocation?: string;
};

export type PreviewInspection = {
  source: PreviewVideoCharacteristics;
  output: PreviewVideoCharacteristics;
  cacheHit: boolean;
  previewMode: 'quick' | 'quality';
  start: string;
  seconds: number;
  generatedPath: string;
  requestedEncoder: string;
  effectiveEncoder: string;
  requestedQSVRateControl: string;
  effectiveQSVRateControl: string;
  ffmpegArgs: string[];
  normalization: {
    mode: 'preserve' | 'normalize_bt709';
    applied: boolean;
    filter?: string;
    reason: string;
    inputColor: PreviewVideoCharacteristics;
    outputColor: PreviewVideoCharacteristics;
    sarPreserved: boolean;
    aliasWarnings?: string[];
  };
};

export type PreviewFrameMetrics = {
  comparable: boolean;
  reason: string;
  sourceDimensions: string;
  outputDimensions: string;
  ssim?: number;
  psnr?: number;
};

export type CompatiblePreviewOptions = {
  path: string;
  profileId?: number;
  profile?: ProfileInput;

  start?: string;
  seconds?: number;
  videoCodec?: string;
  qualityValue?: number;
  videoPreset?: string;
  pixFmt?: string;
  videoFilters?: string;
  x265Params?: string;
  videoEncoder?: string;
  useHardwareIfAvailable?: boolean;
  hardwareQualityPreset?: string;
  videoToolboxBitrateMbps?: number;
  videoToolboxMaxrateMbps?: number;
  videoToolboxBufferMbps?: number;
  videoToolboxQualityProfile?: number;
  videoToolboxProfile?: string;
  videoToolboxGop?: number;
  videoToolboxRealtime?: boolean;
  videoToolboxBFramePolicy?: 'auto' | 'enabled' | 'disabled';
  videoToolboxBFrames?: number;
  videoToolboxAutoAdjustBitrate?: boolean;
  videoToolboxAllowFrameReordering?: boolean;
  videoToolboxPowerEfficiency?: boolean;
  globalQuality?: number;
  qsvRateControl?: string;
  qsvLookAheadDepth?: number;
  qsvExtendedBRC?: boolean;
  qsvAdaptiveI?: boolean;
  qsvAdaptiveB?: boolean;
  mode?: 'quick' | 'quality';
  previewNormalization?: 'preserve' | 'normalize_bt709';
  subtitleStreamIndex?: number;
};

export type ProfileSampleEstimate = {
  assetPath: string;
  durationSeconds: number;
  sampleSeconds: number;
  sampleStarts: number[];
  sampleCount: number;
  measuredVideoBytes: number;
  estimatedVideoBytes: number;
  measuredVideoBitrate: number;
  confidence: 'high';
  source: string;
  effectiveEncoder: string;
  persisted: boolean;
};

export type EncoderRecommendation = {
  encoder: string;
  requestedRateControl: string;
  effectiveRateControl: string;
  rateControlFallback?: string;
  targetBitrate?: number;
  requestedGlobalQuality?: number;
  globalQuality?: number;
  qualityAdjustment: number;
  qualityReasons: string[];
  maxrate?: number;
  buffer?: number;
  profile: string;
  pixelFormat: string;
  lookAhead: boolean;
  lookAheadDepth: number;
  lowPower: boolean;
  extendedBRC: boolean;
  adaptiveI: boolean;
  adaptiveB: boolean;
  estimatedVideoBitrate?: number;
  estimatedOutputSize?: number;
  estimateConfidence: 'high' | 'medium' | 'low';
  warnings: string[];
  requestedRealtime: boolean;
  effectiveRealtime: boolean;
  requestedBFramePolicy?: string;
  effectiveBFramePolicy?: string;
  requestedBFrames?: number;
  observedBFrameCount?: number;
  bFrameEfficiencyMultiplier?: number;
  bFrameDowngradeReason?: string;
  baseTargetBitrate?: number;
};

export type QualityRecommendationResponse = {
  requestedProfile: Profile;
  effectiveProfile: Profile;
  recommendation: EncoderRecommendation;
  capabilitySource: 'active_runtime_snapshot' | 'live_backend_probe';
  ffmpegVideoArguments: string[];
  estimatedOutputMinBytes: number;
  estimatedOutputMaxBytes: number;
  estimatedSavingsMinBytes: number;
  estimatedSavingsMaxBytes: number;
};
