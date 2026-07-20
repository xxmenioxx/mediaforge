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
  status: 'unprocessed' | 'library' | 'converted' | 'unverified' | 'archive';
  missing: boolean;
  expiresAt?: string;
  review: AssetReviewState;
  metadata: AssetMetadataState;
  conversion: AssetConversionOverrideState;
};

export type AssetGroup = {
  id: string;
  libraryId: number;
  libraryName: string;
  path: string;
  relativePath: string;
  status: 'unprocessed' | 'library' | 'converted' | 'unverified' | 'archive';
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
  videoCodec?: string;
  audioCodec?: string;
  qualityMode?: string;
  qualityValue?: number;
  videoPreset?: string;
  pixFmt?: string;
  videoFilters?: string;
  x265Params?: string;
  processingMode?: string;
  preserveHdr?: boolean;
  preserveSubtitles?: boolean;
  preserveChapters?: boolean;
  addAacStereoTrack?: boolean;
  aacStereoDefault?: boolean;
  updatedAt?: string;
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
};

export type AssetSyncInfo = {
  lastSyncedAt: string;
  totalRecords: number;
  missingFiles: number;
};

export type AssetSyncResult = {
  syncedAt: string;
  unprocessedFiles: number;
  libraryFiles: number;
  convertedFiles: number;
  unverifiedFiles: number;
  archiveFiles: number;
  expiredDeleted: number;
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
  totalMemoryBytes: number;
  availableMemoryBytes: number;
  batteryPresent: boolean;
  batteryPercent: number;
  powerSource: string;
  onBattery: boolean;
  disks: Record<string, unknown>;
  encoders: Record<string, { listed: boolean; usable: boolean; reason: string }>;
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
  batchId: string;
  batchName: string;
  mediaPath: string;
  libraryId: number;
  profileId: number;
  profileVersion: number;
  profileSnapshot: Record<string, unknown>;
  profileCapturedAt?: string;
  activeExecutionPlanId?: number;
  audioProfileKey: string;
  priority: number;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled';
  stage: string;
  stageUpdatedAt?: string;
  stageHistory: Array<Record<string, unknown>>;
  progress: number;
  workerName: string;
  outputPath: string;
  errorMessage: string;
  notes: string;
  validationStatus: 'pending' | 'passed' | 'warning' | 'failed' | '';
  validationScore: number;
  validationReport: Record<string, unknown>;
  publishedPath: string;
  publishedAt?: string;
  startedAt?: string;
  finishedAt?: string;
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
  batchId?: string;
  batchName?: string;
  libraryId: number;
  profileId: number;
  audioProfileKey?: string;
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
  default: boolean;
  forced: boolean;
  comment: boolean;
  hearingImpaired: boolean;
  width?: number;
  height?: number;
  pixFmt?: string;
  colorTransfer?: string;
  colorPrimaries?: string;
  bitsPerRawSample?: string;
  avgFrameRate?: string;
  realFrameRate?: string;
  sampleAspectRatio?: string;
  displayAspectRatio?: string;
  hdr?: boolean;
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
  rawProbe: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
};
