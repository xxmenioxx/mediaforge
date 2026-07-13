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
  status: 'unprocessed' | 'converted' | 'archive';
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
  status: 'unprocessed' | 'converted' | 'archive';
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
  converted: Asset[];
  archive: Asset[];
  unprocessedGroups: AssetGroup[];
  convertedGroups: AssetGroup[];
  archiveGroups: AssetGroup[];
  reports: AssetReports;
  sync: AssetSyncInfo;
};

export type AssetReports = {
  unprocessedFiles: number;
  convertedFiles: number;
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
  convertedFiles: number;
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

export type QueueJob = {
  id: number;
  batchId: string;
  batchName: string;
  mediaPath: string;
  libraryId: number;
  profileId: number;
  audioProfileKey: string;
  priority: number;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled';
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
