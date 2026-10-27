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
  scope?: 'asset' | 'path';
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
	id?: number;
	sourceGroupId?: number;
	logicalGroupPath?: string;
	sourcePath?: string;
  libraryId: number;
  libraryName: string;
  path: string;
  relativePath: string;
  groupPath: string;
  fileName: string;
  extension: string;
  sizeBytes: number;
  modifiedAt: string;
  status: 'unprocessed' | 'library' | 'published_as_is' | 'converted' | 'unverified' | 'accepted' | 'mixed';
  missing: boolean;
  expiresAt?: string;
  review: AssetReviewState;
  metadata: AssetMetadataState;
  conversion: AssetConversionOverrideState;
  publicationMode?: 'as_is';
  lifecycle?: AssetLifecycleProjection;
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
  status: 'unprocessed' | 'library' | 'published_as_is' | 'converted' | 'unverified' | 'accepted' | 'mixed';
  fileCount: number;
  sizeBytes: number;
  modifiedAt: string;
  assets: Asset[];
  review: AssetReviewState;
  pathReview: AssetReviewState;
  metadata: AssetMetadataState;
  pathMetadata: AssetMetadataState;
  lifecycleSummary?: AssetGroupLifecycleSummary;
};

export type AssetLifecycleProjection = {
  location: 'raw' | 'library' | string;
  lifecycle: string;
  provenance: string;
  validation: string;
  publication: string;
  original: string;
  activeOperation?: { kind?: string; status?: string; stage?: string };
  attention?: string[];
  currentPublication?: { state?: string; authority?: string; id?: number; publishedPath?: string; path?: string; publishedAt?: string };
  sourceProvenance?: Array<{ sourcePath?: string; authority?: string; recordId?: number; sourceAssetRecordId?: number; jobId?: number; publicationId?: number; publishedPath?: string; current?: boolean; publishedAt?: string }>;
  currentRawCorrelation?: { state?: string; rawAssetRecordId?: number; assetRecordId?: number; libraryAssetId?: number; path?: string };
  evidenceSummary?: {
    queueJobIds?: number[];
    directPublicationIds?: number[];
    hasAnalysis?: boolean;
    analysis?: AssetAnalysisEvidence;
    originalArchivedPath?: string;
  };
  archivedOriginals?: AssetArchivedOriginalProjection[];
  actions?: Record<string, { allowed: boolean; reason?: string }>;
};

export type AssetArchivedOriginalProjection = {
  assetRecordId?: number;
  queueJobId: number;
  path: string;
  sizeBytes: number;
  archivedAt?: string;
  availability: 'available' | 'missing' | string;
  relationship: 'current' | 'historical' | 'unresolved' | string;
  provenance: {
    authority?: string;
    recordId?: number;
    sourcePath?: string;
    publishedPath?: string;
    sourceAssetRecordId?: number;
    current?: boolean;
    publishedAt?: string;
  };
  retentionState: 'retained' | 'expired' | 'unknown' | string;
  retentionDeadline?: string;
  restore?: { allowed: boolean; reason?: string };
  deleteArchivedOriginal?: { allowed: boolean; reason?: string };
};

export type ArchivedOriginalMutationInput = {
  path: string;
  archiveAssetRecordId: number;
  libraryAssetRecordId: number;
};

export type AssetAnalysisEvidence = {
  status: 'missing' | 'ready' | 'stale' | 'unavailable';
  sourceIdentity?: {
    path: string;
    snapshotId?: number;
    sizeBytes: number;
    modifiedAtUnixNano: number;
    snapshotUpdatedAt?: string;
    matchesCurrentSourceRevision: boolean;
  };
  staleComponents?: string[];
  unavailableReason?: string;
};

export type AssetDimensionCount = { value: string; count: number };

export type AssetGroupLifecycleSummary = {
  total: number;
  lifecycle: AssetDimensionCount[];
  provenance: AssetDimensionCount[];
  validation: AssetDimensionCount[];
  publication: AssetDimensionCount[];
  attention: AssetDimensionCount[];
};

export type LibraryRenameSidecarPlan = {
  sourcePath: string;
  targetPath: string;
};

export type LibraryRenameAssetPlan = {
  assetId: number;
  sourcePath: string;
  targetPath: string;
  sidecars: LibraryRenameSidecarPlan[];
};

export type LibraryRenameConflict = {
  code: string;
  sourcePath?: string;
  targetPath?: string;
};

export type LibraryRenamePlan = {
  libraryId: number;
  sourcePath: string;
  targetPath: string;
  assets: LibraryRenameAssetPlan[];
  warnings: string[];
  conflicts: LibraryRenameConflict[];
  planHash: string;
};

export type LibraryRenameApplyResponse = {
  status: string;
  plan: LibraryRenamePlan;
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
  inheritedFrom?: string;
  updatedAt: string;
};

export type SubtitleSemanticRole = 'full' | 'forced' | 'signs' | 'commentary' | 'sdh' | 'unknown';

export type SubtitleRoleOverride = {
  role: SubtitleSemanticRole;
  sourceSnapshotFingerprint?: string;
  streamSignature?: string;
};

export type AssetConversionOverrideState = {
  trackProfileKey?: string;
  keepVideoStreams?: number[] | null;
  keepAudioStreams?: number[] | null;
  keepSubtitleStreams?: number[] | null;
  videoMetadata?: Record<string, StreamMetadataOverride>;
  audioMetadata?: Record<string, StreamMetadataOverride>;
  subtitleMetadata?: Record<string, StreamMetadataOverride>;
  subtitleRoleOverrides?: Record<string, SubtitleRoleOverride>;
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
  subtitleSidecarFormatsByStream?: Record<string, Array<'original' | 'srt'>>;
  videoCodec?: string;
  audioCodec?: string;
  qualityMode?: string;
  qualityValue?: number;
  optimizationIntent?: MVForgeQualityGoal;
  videoPreset?: string;
  pixFmt?: string;
  videoFilters?: string;
  cropAspectPolicy?: 'source_sar' | 'preserve_dar';
  upscaleMode?: UpscaleMode;
  upscaleSharpen?: UpscaleSharpen;
  upscaleSharpenCustomStrength?: number;
  upscaleCustomHeight?: number;
  deinterlaceMode?: 'auto' | 'off' | 'force' | 'ivtc_tff' | 'ivtc_bff';
  deinterlaceFieldOrder?: 'auto' | 'tff' | 'bff';
  fieldStructureMode?: 'preserve' | 'auto' | 'deinterlace';
  cadenceMode?: 'preserve' | 'auto' | 'remove_soft_telecine' | 'inverse_telecine';
  cadenceFieldOrder?: 'auto' | 'tff' | 'bff';
  x265Params?: string;
  frameStructureMode?: 'auto' | 'off' | 'compatible' | 'balanced' | 'maximum_compression' | 'custom';
  frameStructureGopMode?: 'auto' | 'recommended' | 'custom';
  frameStructureGopStrategy?: 'compatible' | 'balanced' | 'maximum_compression' | 'custom';
  frameStructureGopFrames?: number;
  frameStructureBFrameMode?: 'auto' | 'recommended' | 'custom' | 'off';
  frameStructureMaxBFrames?: number;
  hevcLevelMode?: 'auto' | 'recommended' | 'custom';
  hevcLevel?: string;
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
  targetWorkerName?: string;
  globalQuality?: number;
  qsvRateControl?: 'icq' | 'la_icq';
  qsvLookAheadDepth?: number;
  qsvExtendedBrc?: boolean;
  qsvAdaptiveI?: boolean;
  qsvAdaptiveB?: boolean;
  qsvMBBRCMode?: 'auto' | 'enabled' | 'disabled';
  qsvRDOMode?: 'auto' | 'enabled' | 'disabled';
  qsvPStrategy?: 0 | 1 | 2;
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

export type TestEncode = {
  id: number;
  sourceAssetId: number;
  sourcePath: string;
  sourceFingerprint: string;
  sourceSizeBytes: number;
  sourceModifiedAt?: string;
  libraryId: number;
  configurationSource: 'effective_asset' | 'lab_draft';
  requestedConfiguration: Record<string, unknown>;
  effectiveConfiguration: Record<string, unknown>;
  configurationHash: string;
  profileId: number;
  profileVersion: number;
  runtimeSnapshotId?: number;
  workerName: string;
  effectiveEncoder: string;
  startSeconds: number;
  durationSeconds: number;
  status: 'waiting' | 'generating' | 'ready' | 'failed' | 'canceled' | 'deleted' | 'expired';
  phase: string;
  progress: number;
  ffmpegCommand: string;
  outputPath: string;
  outputSizeBytes: number;
  subtitleArtifacts: Array<Record<string, unknown>>;
  validationReport: Record<string, unknown>;
  keep: boolean;
  expiresAt?: string;
  reconciliationStatus: string;
  reconciliationMessage?: string;
  lastReconciledAt?: string;
  outputExists: boolean;
  artifactCount: number;
  sidecarArtifactCount: number;
  knownDiskUsageBytes: number;
  unknownSizeArtifactCount: number;
  errorMessage: string;
  stale: boolean;
  staleReason?: string;
  profileQualification?: ProfileQualification;
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type ProfileQualificationRuleResult = {
  key: string;
  outcome: string;
  disposition: string;
  observed?: unknown;
  reference?: unknown;
  allowedTolerance?: unknown;
  windowIdentity?: string;
  sceneCategory?: string;
  explanation: string;
  evidenceProvenance?: Record<string, unknown>;
};

export type ProfileQualification = {
  id: string;
  testEncodeId: number;
  sourceAssetId: number;
  sourcePath: string;
  sourceRevision: string;
  sourceFingerprint: string;
  profileId: number;
  profileVersion: number;
  resolvedProfileFingerprint: string;
  candidateIdentity: string;
  candidateRevision: string;
  samplingPlanSchemaVersion: string;
  samplingPlanPolicyVersion: string;
  samplingPlanIdentity: string;
  perceptualEvidenceIdentity: string;
  referenceIdentity: string;
  qualityPolicyVersion: string;
  outcome: 'PASS' | 'PASS_WITH_WARNINGS' | 'REVIEW_REQUIRED' | 'FAIL';
  rules: ProfileQualificationRuleResult[];
  warnings: string[];
  qualifiedScope: string;
  evidenceProvenance: Record<string, unknown>;
  worstWindowIdentity?: string;
  worstSceneCategory?: string;
  worstRuleKeys?: string[];
  createdAt: string;
  stale?: boolean;
  staleReason?: string;
};

export type TestEncodePolicy = {
  defaultDurationSeconds: number;
  defaultStartMode: 'beginning' | 'middle' | 'representative';
  defaultRetention: 'temporary' | 'keep';
  temporaryRetentionHours: number;
  failedRetentionHours: number;
  maxReconcileBatchSize: number;
  maxCleanupBatchSize: number;
};

export type TestEncodeInventorySummary = {
  totalRuns: number;
  keepRuns: number;
  totalKnownBytes: number;
  keepKnownBytes: number;
  cleanupEligibleRuns: number;
  cleanupEligibleBytes: number;
  unknownSizeArtifacts: number;
};

export type TestEncodeDeletePreview = {
  testEncodeCount: number;
  mediaOutputCount: number;
  otherArtifactCount: number;
  estimatedBytes: number;
  keepCount: number;
};

export type TestEncodeDeleteSelectionResult = {
  results: Array<{ id: number; deleted: boolean; error?: string }>;
};

export type TestEncodeReconcileResponse = {
  processed: number;
  limit: number;
  results: Array<{
    id: number;
    previousStatus: string;
    reconciliationStatus: string;
    message?: string;
    outputExists: boolean;
    artifactCount: number;
    skippedActive?: boolean;
  }>;
  orphanedArtifacts: TestEncodeOrphanArtifact[];
};

export type TestEncodeOrphanArtifact = {
  id: number;
  testEncodeId: number;
  libraryId: number;
  path: string;
  type: string;
  sizeBytes: number;
  exists: boolean;
  safe: boolean;
  message?: string;
};

export type TestEncodeInput = {
  sourcePath: string;
  libraryId: number;
  configurationSource: 'effective_asset' | 'lab_draft' | 'batch_preflight';
  profileId?: number;
  audioProfileKey?: string;
  trackProfileKey?: string;
  processingMode?: 'full_encode' | 'audio_only';
  resolveAssignments?: boolean;
  labProfile?: ProfileInput;
  labAudioProfile?: Record<string, unknown>;
  labTrackProfile?: Record<string, unknown>;
  labTrackOverride?: AssetConversionOverrideState;
  startMode: 'representative' | 'beginning' | 'middle' | 'custom';
  startSeconds?: number;
  durationSeconds: number;
  configurationToken?: string;
  preflightCandidateId?: string;
  preflightRevision?: number;
  preflightAssetId?: string;
};

export type ExternalSubtitle = {
  path: string;
  fileName: string;
  format: 'srt' | 'ass' | 'ssa';
  language?: string;
  default: boolean;
  forced: boolean;
  sizeBytes: number;
  modifiedAt: string;
  ocrManaged?: boolean;
  ocrAnalysisAvailable?: boolean;
  logicalTransformKey?: string;
  sourceRevision?: string;
  eligibility?: SubtitleSourceEligibility;
};

export type AssetMetadataSidecar = {
  path: string;
  fileName: string;
  format: 'nfo';
  sizeBytes: number;
  modifiedAt: string;
};

export type SubtitleModeEligibility = {
  eligible: boolean;
  reason?: string;
};

export type SubtitleSourceEligibility = {
  restyle: SubtitleModeEligibility;
  srtToAss: SubtitleModeEligibility;
  regenerate: SubtitleModeEligibility;
  replaceEligible: boolean;
  nativeOverrideEligible: boolean;
  sourceIdentity: string;
  sourceRevision: string;
  representation: string;
  language?: string;
  role: SubtitleSemanticRole;
  roleProvenance: string;
  roleConfidence?: string;
  roleClassifierVersion?: string;
  roleOverrideState?: string;
  streamIndex?: number;
  policyVersion: string;
  compositionClass?: 'dialogue' | 'sign' | 'lyrics' | 'commentary' | 'unknown' | string;
  canonicalOcrArtifactIdentity?: string;
  canonicalOcrArtifactRevision?: string;
  canonicalOcrArtifactPath?: string;
  canonicalOcrLogicalTransform?: string;
};

export type SubtitleCompositionCandidate = {
  sourceIdentity: string;
  sourceRevision: string;
  representation: string;
  language: string;
  role: SubtitleSemanticRole;
  compositionClass: string;
  roleProvenance?: string;
  roleConfidence?: string;
  roleClassifierVersion?: string;
  roleOverrideState?: string;
  streamIndex?: number;
  sourceType: 'embedded' | 'external' | string;
  sourcePath?: string;
  canonicalOcrArtifactIdentity?: string;
  canonicalOcrArtifactRevision?: string;
  canonicalOcrArtifactPath?: string;
  canonicalOcrLogicalTransform?: string;
  eligible: boolean;
  reason?: string;
};

export type SubtitleCompositionEligibility = {
  eligible: boolean;
  reason?: string;
  policyVersion: string;
  maxSupplementals: number;
  primaryCandidates: SubtitleCompositionCandidate[];
  supplementalCandidates: SubtitleCompositionCandidate[];
  crossLanguageExplicit: boolean;
};

export type SubtitleEligibilityResponse = {
  external: ExternalSubtitle[];
  embedded: SubtitleSourceEligibility[];
  composition?: SubtitleCompositionEligibility;
};

export type SubtitleCompositionSourceRequest = {
  identity?: string;
  revision?: string;
  representation: 'srt' | 'ass' | 'ssa';
  language: string;
  role: SubtitleSemanticRole;
  cueClass: string;
  roleProvenance?: string;
  roleConfidence?: string;
  roleClassifierVersion?: string;
  roleOverrideState?: string;
  sourcePath: string;
  canonicalOcrArtifactIdentity?: string;
  canonicalOcrArtifactRevision?: string;
  canonicalOcrArtifactPath?: string;
  canonicalOcrLogicalTransform?: string;
};

export type SubtitleCompositionInput = {
  path: string;
  primary: SubtitleCompositionSourceRequest;
  supplementals: SubtitleCompositionSourceRequest[];
  observedRevisions: Record<string, string>;
  publicationMode?: string;
};

export type SubtitleTranslationSourceSelector =
  | {
      type: 'embedded';
      embedded: {
        streamIndex: number;
      };
    }
  | {
      type: 'external';
      external: {
        subtitlePath: string;
      };
    };

export type SubtitleTranslationSource = {
  type: 'embedded' | 'external';
  sourceIdentity: string;
  sourceRevision: string;
  representation: string;
  streamIndex?: number;
  streamId?: string;
  externalPath?: string;
  artifactId?: string;
  codec: string;
  language?: string;
  title?: string;
  default: boolean;
  sourceForced: boolean;
  effectiveForced: boolean;
  role: SubtitleSemanticRole;
  roleProvenance?: string;
  roleConfidence?: string;
  roleClassifierVersion?: string;
};

export type SubtitleTranslationCapability = {
  source: SubtitleTranslationSource;
  eligible: boolean;
  available: boolean;
  reason?: string;
  sourceLanguages: string[];
  targetLocales: string[];
  provider: {
    name?: string;
    model?: string;
    modelSelectable: boolean;
  };
  outputRepresentations: string[];
  context: {
    status: string;
    source?: string;
  };
};

export type SubtitleTranslationReadinessState =
  | 'disabled'
  | 'not_configured'
  | 'untested'
  | 'ready'
  | 'unavailable'
  | 'model_not_found'
  | 'invalid_configuration';

export type SubtitleTranslationReadiness = {
  configured: boolean;
  enabled: boolean;
  readiness: SubtitleTranslationReadinessState;
  lastCheckedAt?: string;
  diagnostic?: string;
};

export type SubtitleTranslationSettingsValue = {
  [key: string]: unknown;
	provider: string;
  ollamaEndpoint: string;
	ollamaModel: string;
	qualityReviewerModel: string;
  maxCuesPerRequest: number;
  maxRequestBytes: number;
  maxContextCues: number;
  maxContextChars: number;
  enabled: boolean;
  readiness: SubtitleTranslationReadinessState;
  readinessConfigIdentity?: string;
  readinessCheckedAt?: string;
  readinessDiagnostic?: string;
};

export type OCRAISettingsValue = {
  [key: string]: unknown;
  enabled: boolean;
  model: string;
};

export type OllamaInstalledModel = {
  id: string;
  name: string;
  digest?: string;
  sizeBytes?: number;
  modifiedAt?: string;
  details?: Record<string, unknown>;
};

export type OllamaModelsResponse = {
  state: string;
  discoveredAt?: string;
  installedModels: OllamaInstalledModel[];
  diagnostic?: string;
};

export type OllamaModelReadinessResponse = {
  state: string;
  modelId: string;
  available: boolean;
  checkedAt: string;
  diagnostic?: string;
  scope: string;
};

export type SubtitleTranslationCapabilityInput = {
  path: string;
  source: SubtitleTranslationSourceSelector;
};

export type SubtitleTranslationQualityDecisionInput = {
  cueId: string;
  action: 'keep' | 'edit';
  text?: string;
};

export type SubtitleTranslationQualityDecisionResponse = {
  operationId: string;
  cueId: string;
  status: 'open' | 'reviewed';
};

export type SubtitleTranslationQualityReviewResponse = {
  operationId: string;
  status: 'open' | 'reviewed' | 'recovery_required';
  quality: NonNullable<NonNullable<SubtitleExtractionOperation['translation']>['quality']>;
  decisions: Array<{ cueId: string; action: 'keep' | 'edit'; text?: string; at: string }>;
};

export type SubtitleTranslationQualityEditableCueResponse = {
  operationId: string;
  cueId: string;
  sourceIdentity: string;
  sourceRevision: string;
  targetRevisions: string[];
  text: string;
};

export type SubtitleTranslationQualityReviewListItem = SubtitleTranslationQualityReviewResponse & { updatedAt: string };

export type StartSubtitleTranslationInput = {
  path: string;
  source: SubtitleTranslationSourceSelector;
  sourceLanguage: string;
  targetLocale: string;
  outputRepresentation: string;
};

export type SubtitleSyncCapability = {
  enabled: boolean;
  fixedOffset: boolean;
  automaticAlignment: boolean;
  automaticAlignmentReason?: string;
  automaticFormats?: string[];
  segmentedAlignment: boolean;
  engines: Array<{ name: string; version: string }>;
};

export type SubtitleSyncAudioReference = {
  streamIndex: number;
  streamId?: string;
  codec: string;
  language?: string;
  title?: string;
};

export type SubtitleSyncCuePreview = {
  cueIndex: number;
  currentStartMs: number;
  currentEndMs: number;
  proposedStartMs: number;
  proposedEndMs: number;
};

export type SubtitleSyncProposal = {
  proposalId: string;
  subtitlePath: string;
  sourceIdentity: string;
  sourceRevision: string;
  representation: string;
  referenceAudio: SubtitleSyncAudioReference;
  engine: string;
  engineVersion: string;
  model: 'fixed_offset' | 'affine' | 'segmented' | string;
  offsetMilliseconds: number;
  confidence?: number;
  scale: number;
  segments: Array<{ startMilliseconds?: number; endMilliseconds?: number; offsetMilliseconds: number; scale: number }>;
  qualityState: string;
  analyzedAt: string;
  diagnostics: string[];
  preview: SubtitleSyncCuePreview[];
};

export type SubtitleSyncInput = {
  path: string;
  subtitlePath: string;
  referenceAudioStreamIndex: number;
  mode?: 'fixed_offset' | 'automatic';
  offsetMilliseconds?: number;
};

export type SubtitleSyncApplyInput = SubtitleSyncInput & {
  sourceRevision: string;
  proposalId: string;
};

export type SubtitleSyncProvenance = {
  operationId: string;
  assetPath: string;
  subtitlePath: string;
  sourceIdentity: string;
  sourceRevision: string;
  referenceAudioStreamIndex: number;
  referenceAudioStreamId?: string;
  referenceAudioCodec: string;
  referenceAudioLanguage?: string;
  referenceAudioTitle?: string;
  engine: string;
  engineVersion: string;
  model: string;
  offsetMilliseconds: number;
  scale: number;
  segmentsJson: string;
  qualityState: string;
  analysisAt: string;
  appliedAt: string;
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

export type AssetEditInput = {
  path: string;
  metadata: Omit<AssetMetadataState, 'inheritedFrom' | 'updatedAt'>;
  conversion: AssetConversionOverrideState;
};

export type AssetEditResponse = {
  path: string;
  metadata: AssetMetadataState;
  conversion: AssetConversionOverrideState;
};

export type AssetConversionUpdateInput = AssetConversionOverrideState & {
  path: string;
};

export type AssetTracksOverrideState = Pick<AssetConversionOverrideState,
  | 'keepVideoStreams'
  | 'keepAudioStreams'
  | 'keepSubtitleStreams'
  | 'videoMetadata'
  | 'audioMetadata'
  | 'subtitleMetadata'
  | 'subtitleRoleOverrides'
  | 'subtitleSidecarFormatsByStream'
>;

export type AssetTracksUpdateInput = Partial<AssetTracksOverrideState> & {
  path: string;
};

export type AssetTracksUpdateResponse = {
  path: string;
  conversion: AssetConversionOverrideState;
};

export type AssetInventory = {
	sourceGroups: AssetSourceGroup[];
  raw?: Asset[];
  unprocessed: Asset[];
  library: Asset[];
  converted: Asset[];
  unverified: Asset[];
  accepted: Asset[];
  missing: Asset[];
  unprocessedGroups: AssetGroup[];
  rawGroups?: AssetGroup[];
  libraryGroups: AssetGroup[];
  convertedGroups: AssetGroup[];
  unverifiedGroups: AssetGroup[];
  acceptedGroups: AssetGroup[];
  reports: AssetReports;
  sync: AssetSyncInfo;
  counts?: AssetInventoryCounts;
};

export type AssetInventoryCounts = {
  raw: number;
  library: number;
  processing: number;
  needsReview: number;
  validationReview: number;
  publicationBlocked: number;
  converted: number;
  originalsArchived: number;
  missing: number;
  historicalPaths: number;
};

export type AssetSourceGroup = {
	id: number;
	name: string;
	relativePath: string;
	sourcePath: string;
	fileCount: number;
	assetCount: number;
	titleCount: number;
	pathCount: number;
	sizeBytes: number;
	totalSizeBytes: number;
	logicalGroups: AssetLogicalGroup[];
};

export type AssetLogicalGroup = {
	id: string;
	name: string;
	path: string;
	relativePath: string;
	fileCount: number;
	assetCount: number;
	pathCount: number;
	sizeBytes: number;
	totalSizeBytes: number;
	assetPaths: AssetPath[];
};

export type AssetPath = {
	id: string;
	name: string;
	path: string;
	relativePath: string;
	displayPath: string;
	isLogicalGroupRoot: boolean;
	fileCount: number;
	assetCount: number;
	sizeBytes: number;
	totalSizeBytes: number;
	assets: Asset[];
};

export type AssetScopeConfiguration = {
	id: number;
	scopeType: 'source_group' | 'logical_group' | 'path' | 'asset';
	scopeKey: string;
	categorySelection: 'inherit' | 'value' | 'disabled';
	category: string;
	destinationSelection: 'inherit' | 'value' | 'disabled';
	destinationLibraryId?: number;
	createdAt: string;
	updatedAt: string;
};

export type AssetScopeConfigurationInput = Omit<AssetScopeConfiguration, 'id' | 'createdAt' | 'updatedAt'>;

export type LogicalGroupConfigurationChange = {
	mode: 'no_change' | 'inherit' | 'value' | 'disabled';
	videoProfileId?: number;
	profileKey?: string;
	category?: string;
	destinationLibraryId?: number;
};

export type ConfigureLogicalGroupsBatchInput = {
	logicalGroupPaths: string[];
	video: LogicalGroupConfigurationChange;
	audio: LogicalGroupConfigurationChange;
	tracks: LogicalGroupConfigurationChange;
	category: LogicalGroupConfigurationChange;
	destination: LogicalGroupConfigurationChange;
};

export type ConfigureLogicalGroupsBatchResponse = {
	logicalGroupPaths: string[];
	changedDimensions: Array<'video' | 'audio' | 'tracks' | 'category' | 'destination'>;
};

export type EffectiveAssetConfigurationValue = {
	selection: 'inherit' | 'profile' | 'value' | 'disabled' | 'override_only' | 'audio_only' | string;
	source?: 'source_group' | 'logical_group' | 'path' | 'asset' | string;
	sourceKey?: string;
	sourceName?: string;
	videoProfileId?: number;
	profileKey?: string;
	profileData?: Record<string, unknown>;
	category?: string;
	destinationLibraryId?: number;
};

export type EffectiveAssetConfiguration = {
	assetPath: string;
	video: EffectiveAssetConfigurationValue;
	audio: EffectiveAssetConfigurationValue;
	tracks: EffectiveAssetConfigurationValue;
	category: EffectiveAssetConfigurationValue;
	destination: EffectiveAssetConfigurationValue;
	warnings?: string[];
};

export type EffectiveAssetConfigurationBatchResponse = {
	configurations: Record<string, EffectiveAssetConfiguration>;
	missingAssetIds: number[];
};

export type AssetReports = {
  unprocessedFiles: number;
  libraryFiles: number;
  convertedFiles: number;
  unverifiedFiles: number;
  acceptedFiles: number;
  archiveFiles: number;
  archiveBytes: number;
  expiredArchive: number;
  missingFiles: number;
  missingActionable: number;
  missingHistorical: number;
  convertedComparedFiles: number;
  convertedOriginalBytes: number;
  convertedOutputBytes: number;
  convertedSpaceSavedBytes: number;
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
  sourceBitrateEvidence?: SourceBitrateEvidence;
  bpppfAssessment?: BPPPFAssessment;
};

export type ProfileCandidate = {
  profile: Profile;
  score: number;
  reasons: string[];
  source?: string;
};

export type UpscaleMode = 'disabled' | 'auto' | '720p' | '1080p' | 'custom';
export type UpscaleSharpen = 'off' | 'light' | 'medium' | 'custom';
export type ResolvedUpscaleMode = 'keep_source' | '720p' | '1080p' | 'custom';
export type UpscaleConfidence = 'unavailable' | 'low' | 'medium' | 'high';

export type VideoWorkerConfig = Record<string, unknown> & {
  upscaleMode?: UpscaleMode;
  upscaleSharpen?: UpscaleSharpen;
  upscaleSharpenCustomStrength?: number;
  upscaleCustomHeight?: number;
  resolvedUpscaleDecision?: ResolvedUpscaleDecision;
  resolvedRestorationPlan?: ResolvedRestorationPlan;
  restorationRecommendationProvenance?: RestorationRecommendationProvenance;
};

export type ResolvedRestorationStage = {
  stage: string;
  filter: string;
  parameters?: Record<string, unknown>;
};

export type RestorationRecommendationProvenance = {
  version: number;
  sourceAssetPath: string;
  appliedRecommendations: Array<Pick<RestorationRecommendation, 'id' | 'domain' | 'state' | 'recommendedValue' | 'confidence' | 'reasons' | 'warnings'>>;
  restorationEvidence: RestorationAnalysis;
};

export type ResolvedRestorationPlan = {
  version: number;
  requestedFilterChain?: string;
  resolvedFilterChain?: string;
  stages: ResolvedRestorationStage[];
  requiresVideoEncode: boolean;
  executable: boolean;
  sourceStorage: { width: number; height: number; sar?: string; dar?: string };
  effectiveGeometry: { width: number; height: number; sar?: string; dar?: string };
  resolvedOutput: { width: number; height: number; sar?: string; dar?: string };
  recommendationProvenance?: RestorationRecommendationProvenance;
  restorationEvidence?: RestorationAnalysis;
  warnings: string[];
};

export type ResolvedUpscaleDecision = {
  requestedMode: UpscaleMode;
  resolvedMode: ResolvedUpscaleMode;
  sourceWidth: number;
  sourceHeight: number;
  sourceSAR?: string;
  sourceDAR?: string;
  targetWidth: number;
  targetHeight: number;
  targetSAR?: string;
  upscaleApplied: boolean;
  sharpenMode: UpscaleSharpen;
  sharpenStrength?: number;
  confidence: UpscaleConfidence;
  reasons: string[];
  warnings: string[];
};

export type UpscaleAnalysisEvidence = {
  sourceWidth: number;
  sourceHeight: number;
  sourceSAR?: string;
  sourceDAR?: string;
  codec?: string;
  bitrate?: number;
  cropAvailable: boolean;
  cropStatus?: string;
  cropConfidence?: number;
  frameStructureAvailable: boolean;
  interlaceAvailable: boolean;
  interlaceStatus?: string;
  interlaceConfidence?: number;
  cadenceAvailable: boolean;
  cadenceType?: string;
  cadenceConfidence?: number;
  cadenceRecommendation?: string;
  effectiveFrameRate?: string;
  qualitySignals: {
    noise: 'unavailable';
    grain: 'unavailable';
    compressionArtifacts: 'unavailable';
    ringing: 'unavailable';
    edgeDetail: 'unavailable';
  };
};

export type ProfileSuggestion = {
  matchType: 'existing' | 'create' | 'assigned_asset' | 'assigned_path';
  summary: string;
  scan: ScanResult;
  sourceBitrateEvidence?: SourceBitrateEvidence;
  bpppfAssessment?: BPPPFAssessment;
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
  findings: AdvisorFinding[];
  restorationPlan: RestorationRecommendationPlan;
};

export type RestorationRecommendationState = 'recommended' | 'manual_review' | 'not_applicable' | 'no_recommendation';

export type RestorationRecommendation = {
  id: string;
  domain: string;
  state: RestorationRecommendationState;
  currentValue?: string;
  recommendedValue?: string;
  confidence: string;
  reasons: string[];
  warnings: string[];
  supportingEvidence: string[];
  patch?: Record<string, unknown>;
  resolvedOutput?: { width: number; height: number; sar?: string };
};

export type RestorationRecommendationPlan = {
  version: number;
  applyLocked: boolean;
  applyLockReason?: string;
  recommendations: RestorationRecommendation[];
  restorationEvidence: RestorationAnalysis;
};

export type AdvisorFinding = {
  id: string;
  category: 'video' | 'frame_structure' | 'color' | 'audio' | 'tracks' | 'encoder' | string;
  title: string;
  detail: string;
  severity: 'information' | 'recommended' | 'review' | 'unsafe' | string;
  confidence: 'low' | 'medium' | 'high' | string;
  actionable: boolean;
  defaultSelected: boolean;
  patch?: Record<string, unknown>;
  evidence?: string[];
};

export type Profile = {
  id: number;
  name: string;
  scope: 'asset' | 'path';
  description: string;
  templateKey?: string;
  templateVersion?: number;
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
  optimizationIntent?: MVForgeQualityGoal;
  profileVersion: number;
  audioCodec: string;
  qualityMode: string;
  qualityValue: number;
  preserveHdr: boolean;
  preserveSubtitles: boolean;
  preserveChapters: boolean;
  workerConfig: VideoWorkerConfig;
  disabled: boolean;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string | null;
};

export type ProfileInput = {
  name: string;
  scope?: 'asset' | 'path';
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
  optimizationIntent?: MVForgeQualityGoal;
  audioCodec: string;
  qualityMode: string;
  qualityValue: number;
  preserveHdr: boolean;
  preserveSubtitles: boolean;
  preserveChapters: boolean;
  workerConfig?: VideoWorkerConfig;
  disabled?: boolean;
};

export type LabResolutionBlockingField = {
  field: string;
  source?: unknown;
  expected?: unknown;
  value?: unknown;
  authority?: string;
  authorityState: string;
  reason: string;
};

export type LabResolutionAction = {
  id: 'reanalyze_cadence' | 'preserve_source' | 'compare_field_order';
  label: string;
  eligible: boolean;
  recommended?: boolean;
  reason: string;
  tradeoffs?: string;
};

export type LabResolutionRequired = {
  version: 1;
  status: 'resolution_required';
  gate?: 'planning' | 'canonical_fidelity';
  planningStatus: 'NEEDS_REVIEW' | 'RESOLVED';
  fidelityStatus?: 'UNVERIFIED';
  blockingFields: LabResolutionBlockingField[];
  evidence: {
    path: string;
    sourceSizeBytes: number;
    sourceModifiedAtNs: number;
    sourceMode: number;
    cadenceAnalysisVersion: number;
    sourceFrameRate: string;
    scanId?: number;
    scanUpdatedAt?: string;
    profileDecisionSignature: string;
    requestedProfileSignature: string;
  };
  requested: { cadence: string; fieldStructure: string; gop: string; gopStrategy: string };
  effective: { frameRate?: string | null; frameRateKnown: boolean; progressive?: boolean | null; progressiveReliable: boolean; cadenceOperation?: string | null; fieldOrder?: string | null };
  actions: LabResolutionAction[];
};

export type ProfileTemplate = {
  key: string;
  version: number;
  name: string;
  category: string;
  description: string;
  definition: Omit<ProfileInput, 'name' | 'description' | 'scope' | 'disabled'>;
};

export type CloneProfileTemplateInput = {
  key: string;
  templateVersion: number;
  name: string;
  scope?: 'asset' | 'path';
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

export type JellyfinLibrary = {
  id: string;
  name: string;
  collectionType?: string;
};

export type JellyfinConnectionResult = {
  reachable: boolean;
  authenticated: boolean;
  serverId?: string;
  serverName?: string;
  serverVersion?: string;
  versionAvailable: boolean;
  libraries: JellyfinLibrary[];
  failureClass?: string;
};

export type JellyfinIntegrationState = {
  enabled: boolean;
  credentialConfigured: boolean;
  configurationValid: boolean;
  configurationNeedsRevalidation: boolean;
  lastConnectionSuccessful: boolean;
  lastSuccessfulConnectionAt?: string;
  liveReachability: 'unknown' | 'reachable' | 'unavailable';
  liveFailureClass?: string;
  serverId?: string;
  serverName?: string;
  serverVersion?: string;
  librariesLoaded: boolean;
  librariesDiscoveredAt?: string;
  libraries: JellyfinLibrary[];
  mappings: Array<{
    mvforgeLibraryId: number;
    jellyfinLibraryId: string;
    jellyfinName?: string;
    jellyfinServerId?: string;
    needsRemap: boolean;
  }>;
};

export type JellyfinIntegrationOutcome = {
  id: number;
  reason: 'asset_published' | 'test_encode_ready' | 'test_encode_removed' | 'manual_refresh';
  status: 'pending' | 'success' | 'skipped_disabled' | 'skipped_unmapped' | 'retryable_failure' | 'permanent_configuration_error';
  effectiveScope?: 'server_wide';
  failureClass?: string;
  attemptCount: number;
  durationMillis: number;
  updatedAt: string;
  publicationId?: string;
  testEncodeId?: number;
  libraryId?: number;
  retryEligible: boolean;
};

export type AssetDefaultsSource = 'custom' | 'profile';
export type AssetDefaultsDomain = {
  source: AssetDefaultsSource;
  profileId?: number;
  profileKey?: string;
  custom: Record<string, unknown>;
  [key: string]: unknown;
};
export type PreferredSubtitleRule = {
  language: string;
  action: 'keep' | 'extract' | 'keep_and_extract';
};
export type AssetDefaultsValue = {
  schemaVersion: number;
  video: AssetDefaultsDomain;
  audio: AssetDefaultsDomain;
  tracks: AssetDefaultsDomain;
  [key: string]: unknown;
};

export type UpdateSettingInput = {
  key: string;
  value: Record<string, unknown>;
};

export type UpdateSettingsBatchInput = {
  settings: UpdateSettingInput[];
};

export type TrackProfilePreviewDecision = {
  index: number;
  type: 'video' | 'audio' | 'subtitle';
  kept: boolean;
  reason: string;
};

export type ResolvedDefaultTrack = {
  requestedLanguage?: string;
  streamIndex?: number;
  status: 'unchanged' | 'resolved' | 'missing';
};

export type TrackProfileResolutionPreview = {
  assetPath: string;
  keepVideoStreams: number[];
  keepAudioStreams: number[];
  keepSubtitleStreams: number[];
  video: TrackProfilePreviewDecision[];
  audio: TrackProfilePreviewDecision[];
  subtitle: TrackProfilePreviewDecision[];
  warnings: string[];
		resolvedTrackPlan: {
		videoStreams: Array<{ streamIndex: number; codec?: string; language?: string }>;
		audioStreams: Array<{ streamIndex: number; codec?: string; language?: string }>;
		removedAudioStreams: Array<{ streamIndex: number; codec?: string; language?: string }>;
		subtitleStreams: Array<{ streamIndex: number; codec?: string; language?: string; action: 'keep' | 'remove' | 'extract' | 'keep_and_extract'; actionSource?: string; role?: SubtitleSemanticRole; roleProvenance?: string; roleConfidence?: string; roleOverrideState?: string; forced?: boolean; effectiveForced?: boolean }>;
		attachmentPolicy: 'auto' | 'keep' | 'remove';
		attachmentsKept: boolean;
		attachmentReason: string;
		attachmentStreams: ResolvedAttachmentStream[];
		fontAttachmentExportPolicy: 'none' | 'all';
		fontAttachments: ResolvedFontAttachment[];
		fontAttachmentsExported: boolean;
		chapterPolicy: 'keep' | 'remove';
    defaultAudio: ResolvedDefaultTrack;
    defaultSubtitle: ResolvedDefaultTrack;
		chaptersKept: boolean;
		sidecarOutputs: Array<{ streamIndex: number; codec?: string; language?: string; format?: string; mode?: 'original' | 'converted'; forced: boolean; default: boolean; ocrLanguage?: string; ocrMode?: 'raw' | 'clean' | 'accurate'; logicalTransformKey?: string }>;
		warnings?: string[];
	};
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

export type RuntimeState = {
  id: number;
  /** The response represents the mutable current host state, not audit history. */
  stateKind?: 'live';
  detectedAt: string;
  stableFingerprint?: string;
  /** Set only when this response captured a new stable-transition audit row. */
  auditSnapshotId?: number | null;
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
  encoders: Record<string, { listed: boolean; usable: boolean; reason: string; main10?: boolean; icq?: boolean; lowPower?: boolean; lookAhead?: boolean; extendedBrc?: boolean; adaptiveI?: boolean; adaptiveB?: boolean; qsvFullCombination?: boolean; qsvIcqMain8?: boolean; qsvIcqMain10?: boolean; qsvLaIcqMain8?: boolean; qsvLaIcqMain10?: boolean; qsvCqpMain8?: boolean; qsvCqpMain10?: boolean; qsvVbrMain8?: boolean; qsvVbrMain10?: boolean; qsvCbrMain8?: boolean; qsvCbrMain10?: boolean; qsvLowPowerMain10?: boolean; qsvAdaptiveIMain8?: boolean; qsvAdaptiveBMain8?: boolean; qsvAdaptiveIMain10?: boolean; qsvAdaptiveBMain10?: boolean; qsvVbrExtBrcMain8?: boolean; qsvVbrExtBrcMain10?: boolean; qsvVbrLookAheadMain8?: boolean; qsvVbrLookAheadMain10?: boolean; qsvCbrExtBrcMain8?: boolean; qsvCbrExtBrcMain10?: boolean; qsvCbrLookAheadMain8?: boolean; qsvCbrLookAheadMain10?: boolean; videoToolboxMain?: boolean; videoToolboxMain10?: boolean; videoToolboxBFrames?: boolean; videoToolboxBFramesVerified?: boolean; videoToolboxBFramesDisabled?: boolean; videoToolboxObservedBFrames?: number; videoToolboxPowerEfficient?: boolean; testedModes?: Record<string, boolean>; modeReasons?: Record<string, string> }>;
  metricCapabilities?: Record<string, unknown>;
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

/** @deprecated Use RuntimeState for current host telemetry. */
export type RuntimeSnapshot = RuntimeState;

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
  sourceAssetRecordId?: number;
  executionNumber?: number;
  batchId: string;
  batchName: string;
  batchPosition: number;
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
  audioProfileSnapshot?: Record<string, unknown>;
  trackProfileSnapshot?: Record<string, unknown>;
  profileResolution?: Record<string, unknown>;
  processingMode: 'full_encode' | 'audio_only' | '';
  priority: number;
  queuePosition: number;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled';
  stage: string;
  stageUpdatedAt?: string;
  stageHistory: Array<Record<string, unknown>>;
  progress: number;
  workerName: string;
  outputPath: string;
  plannedLibraryRoot?: string;
  plannedPublishedPath: string;
  errorMessage: string;
  notes: string;
  validationStatus: 'pending' | 'passed' | 'warning' | 'failed' | '';
  validationScore: number;
  validationReport: Record<string, unknown>;
  validationRunId?: string;
  validationOutcome?: 'PASS' | 'PASS_WITH_WARNINGS' | 'REVIEW_REQUIRED' | 'FAIL' | '';
  validationOverridePresent?: boolean;
  validationHasActionableWarnings?: boolean;
  publishedPath: string;
  publishedAt?: string;
  publicationRetiredAt?: string;
  replacementTargetPath: string;
  originalArchivedPath: string;
  originalArchiveStatus?: string;
  publicationId?: string;
  publicationState?: 'READY' | 'HELD' | 'PUBLISHING' | 'PUBLISHED' | 'BLOCKED' | 'FAILED' | '';
  publicationSummary?: string;
  subtitleArtifacts?: Array<{
    artifactId?: string;
    type?: 'subtitle_sidecar' | 'font_attachment';
    streamIndex: number;
    sourceCodec: string;
    sourceType?: 'text' | 'bitmap';
    format?: 'srt' | 'ass' | 'ssa' | 'sup';
    requestedFormat?: string;
    mode?: 'original' | 'converted';
    ocrRequired?: boolean;
    language?: string;
    default?: boolean;
    forced?: boolean;
    title?: string;
    stagedPath: string;
    publishedPath?: string;
    sizeBytes: number;
    status: 'planned' | 'generating' | 'ready' | 'unsupported' | 'failed' | 'rolled_back' | 'skipped' | '';
    error?: string;
    displayName?: string;
    fontAttachmentsExported?: boolean;
	ocrLanguage?: string;
	effectiveOcrLanguage?: string;
	ocrMode?: 'raw' | 'clean' | 'accurate';
    stages?: string[];
    extension?: string;
    namingKey?: string;
    logicalTransformKey?: string;
    attachmentOrdinal?: number;
    originalName?: string;
    mimeType?: string;
    fontFormat?: 'TTF' | 'OTF' | 'TTC' | 'OTC';
    safeFilename?: string;
    relativePath?: string;
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

export type HousekeepingCandidateKind = 'job_workspace' | 'job_log' | 'stage_artifact' | 'empty_directory' | 'test_encode' | 'runtime_snapshot';

export type HousekeepingCandidate = {
  candidateId: string;
  kind: HousekeepingCandidateKind;
  path: string;
  jobId?: number;
  operationId?: string;
  testEncodeId?: number;
  reason: string;
  ownership?: string;
  sizeBytes: number;
  modifiedAt: string;
  safeToDelete: boolean;
  manualOnly?: boolean;
  outcome?: 'eligible' | 'already_absent' | 'cleaned' | 'blocked' | 'failed' | 'partial';
  blockedReason?: string;
  maintenance?: {
    assetPath: string;
    assetRecordId?: number;
    assetRecordStatus?: string;
    assetRecordMissing: boolean;
    status: string;
    phase: string;
    finishedAt?: string;
    temporaryPresence: 'present' | 'absent' | 'unknown';
    temporarySizeKnown: boolean;
    backupPath?: string;
    backupPresence: 'present' | 'absent' | 'unknown';
    recoveryState: string;
    resultFingerprint?: string;
    currentAssetFingerprint?: string;
    assetFingerprintState: string;
    warning?: string;
  };
  protectionReferences?: {
    source: string;
    recordId: string;
    field: string;
    path: string;
    state: string;
    reason: string;
  }[];
  protectionReferenceCount?: number;
};
export type RuntimeSnapshotRetentionReport = {
  rowsConsidered: number;
  referencedKept: number;
  latestKept: number;
  recentUnreferencedKept: number;
  pruneCandidates: number;
  deleted: number;
  errors: string[];
};
export type HousekeepingReport = {
  ranAt: string;
  dryRun: boolean;
  outcome?: 'preview' | 'cleaned' | 'already_absent' | 'blocked' | 'failed' | 'partial' | 'no_op';
  previewId?: string;
  previewExpiresAt?: string;
  candidates: HousekeepingCandidate[];
  summaries: { kind: HousekeepingCandidateKind; candidateCount: number; recoverableBytes: number }[];
  removedPaths: string[];
  recoveredBytes: number;
  errors: string[];
  runtimeSnapshots?: RuntimeSnapshotRetentionReport;
};

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

export type JobLogResponse = {
  jobId: number;
  available: boolean;
  content: string;
  truncated: boolean;
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
  outcome: 'PASS' | 'PASS_WITH_WARNINGS' | 'REVIEW_REQUIRED' | 'FAIL';
  runId: string;
  score: number;
  checks: ValidationCheck[];
  warnings: string[];
  report: Record<string, unknown>;
  findings: ValidationFinding[];
};

export type ValidationFinding = {
  key: string;
  expected: unknown;
  actual: unknown;
  details?: unknown;
  severity: 'critical' | 'error' | 'warning' | 'info';
  disposition: 'blocking' | 'advisory';
  state: 'PASS' | 'FAIL' | 'WARNING' | 'REVIEW_REQUIRED' | 'UNVERIFIED' | 'NOT_APPLICABLE';
  actionable: boolean;
  overrideable: boolean;
  explanation: string;
  evidenceReferences: string[];
};

export type ValidationPreset = 'disabled' | 'basic' | 'default' | 'strict' | 'custom';

export type ValidationRulePolicy = {
  enabled: boolean;
  disposition: 'blocking' | 'advisory';
  overrideable: boolean;
  thresholds?: Record<string, number>;
  fields?: Record<string, boolean>;
  warningPenalty: number;
  failurePenalty: number;
};

export type ValidationPolicy = {
  schemaVersion: number;
  policyVersion: string;
  preset: ValidationPreset;
  basedOn?: Exclude<ValidationPreset, 'custom'>;
  rules: Record<string, ValidationRulePolicy>;
  differenceCount?: number;
  differences?: string[];
  presetDefaults?: Partial<Record<Exclude<ValidationPreset, 'custom'>, ValidationPolicy>>;
  autoAdvanceOutcomes?: string[];
};

export type ValidationRun = {
  id: number;
  runId: string;
  jobId: number;
  executionNumber: number;
  executionPlanId: number;
  executionPlanVersion: number;
  resultArtifactId: string;
  resultGenerationToken: string;
  resultGenerationVersion: number;
  resultOutputRevision: string;
  schemaVersion: string;
  validatorVersion: string;
  rulesetId: string;
  rulesetVersion: string;
  policySchemaVersion: number;
  policyVersion: string;
  policyPreset: string;
  policyFingerprint: string;
  policySnapshot?: ValidationPolicy;
  actorSource: string;
  actorId: string;
  outcome: ValidationResult['outcome'];
  score: number;
  actionableWarnings: boolean;
  findings: ValidationFinding[];
  report: Record<string, unknown>;
  createdAt: string;
};

export type ValidationOverride = {
  eventId: string;
  validationRunId: string;
  scope: 'rule' | 'all_overrideable' | string;
  resultArtifactId: string;
  resultGenerationToken: string;
  resultGenerationVersion: number;
  resultOutputRevision: string;
  policySchemaVersion: number;
  policyVersion: string;
  policyFingerprint: string;
  actorSource: string;
  actorId: string;
  reason: string;
  ruleKeys: string[];
  requestedHandoff: 'publisher' | 'automatic';
  overrideVersion: string;
  createdAt: string;
};

export type ValidationRunDetail = {
  run: ValidationRun;
  overrides: ValidationOverride[];
  effectiveHandoff: 'ALLOWED' | 'ALLOWED_BY_OVERRIDE' | 'BLOCKED' | 'HISTORICAL_ONLY' | string;
  currentPolicy: ValidationPolicy;
  policyCurrent: boolean;
};

export type ValidationSummary = Pick<QueueJob,
  | 'id' | 'executionNumber' | 'mediaPath' | 'outputPath' | 'status' | 'finishedAt' | 'createdAt' | 'updatedAt'
  | 'stage' | 'validationStatus' | 'validationScore' | 'validationRunId' | 'validationOutcome' | 'validationOverridePresent'
  | 'validationHasActionableWarnings' | 'publishedPath' | 'publishedAt' | 'publicationId' | 'publicationState' | 'publicationRetiredAt'
  | 'originalArchivedPath' | 'originalArchiveStatus'
>;

export type ValidationSummaryPage = { items: ValidationSummary[]; total: number; limit: number; offset: number };

export type HistoryPublisherAttemptSummary = {
  phase: string;
  outcome: string;
  code?: string;
  summary?: string;
  createdAt: string;
};

export type HistoryPublisherSummary = {
  publicationId: string;
  state: string;
  destinationPath: string;
  publishMode: string;
  attemptCount: number;
  latestAttempt?: HistoryPublisherAttemptSummary;
  validationRunId: string;
  validationOverridePresent: boolean;
  validationOverrideEventIds?: string[];
};

export type ValidationHistorySummary = Pick<QueueJob,
  | 'id' | 'executionNumber' | 'mediaPath' | 'status' | 'publishedPath' | 'publishedAt' | 'createdAt' | 'updatedAt'
  | 'validationStatus' | 'validationScore' | 'validationRunId' | 'validationOutcome' | 'validationOverridePresent'
  | 'validationHasActionableWarnings' | 'originalArchivedPath' | 'originalArchiveStatus' | 'dismissedAt'
> & {
  publisher?: HistoryPublisherSummary;
  stagingState?: 'retained' | 'cleaned' | 'failed' | 'blocked' | 'no_workspace' | 'not_applicable';
  stagingCleanupOutcome?: 'cleaned' | 'noop' | 'blocked' | 'failed';
  stagingCleanupReason?: string;
  stagingRecoveredBytes?: number;
  stagingCleanupAt?: string;
};

export type ValidationStagingCleanupFile = { path: string; sizeBytes: number; kind: string };
export type ValidationStagingCleanupPreview = {
  jobId: number;
  executionNumber?: number;
  validationRunId: string;
  previewId: string;
  expiresAt: string;
  state: 'ready' | 'blocked' | 'noop';
  workspacePath: string;
  ownership: string;
  reason: string;
  files: ValidationStagingCleanupFile[];
  totalBytes: number;
  blockers: string[];
};
export type ValidationStagingCleanupResult = {
  jobId: number;
  previewId: string;
  outcome: 'cleaned' | 'noop' | 'blocked' | 'failed';
  removedPaths: string[];
  recoveredBytes: number;
  reason: string;
  evidenceId: string;
};

export type ValidationHistoryPage = { items: ValidationHistorySummary[]; total: number; limit: number; offset: number };

export type ManualPublishRequestResult = {
  jobId: number;
  requestId?: string;
  publicationId?: string;
  status: 'accepted' | 'processing' | 'already_published';
  message: string;
};

export type PublisherPublication = {
  publicationId: string;
  queueJobId: number;
  executionNumber: number;
  executionPlanId: number;
  libraryId: number;
  destinationRoot: string;
  destinationPath: string;
  publishMode: 'standard' | 'replace_library_asset';
  resultArtifactId: string;
  validationRunId: string;
  validationOverrideEventIds?: string[];
  state: 'READY' | 'HELD' | 'PUBLISHING' | 'PUBLISHED' | 'BLOCKED' | 'FAILED';
  manualRequestId?: string;
  manualRequestStatus?: 'accepted' | 'processing' | 'completed' | 'failed';
  manualRequestedAt?: string;
  manualRequestCompletedAt?: string;
  holdReason?: string;
  failureCode?: string;
  failureSummary?: string;
  attemptCount: number;
  fencingGeneration: number;
  publishedAt?: string;
  originalArchiveStatus: string;
  originalArchivedPath?: string;
  createdAt: string;
  updatedAt: string;
};

export type PublisherPublicationSummary = {
  publication: PublisherPublication;
  attempts: Array<{ attemptId: string; actorSource: string; actorId: string; createdAt: string }>;
  evidence: Array<{ attemptId: string; phase: string; outcome: string; code?: string; summary?: string; createdAt: string }>;
  audit: Array<{ action: string; actorSource: string; actorId: string; reason?: string; version: number; createdAt: string }>;
};

export type PublisherEligibilityStatus = 'ALLOWED' | 'ALLOWED_BY_OVERRIDE' | 'BLOCKED';

export type PublisherEligibilityCandidate = {
  job: Pick<QueueJob,
    'id' | 'executionNumber' | 'mediaPath' | 'outputPath' | 'publishedPath' | 'publishedAt' |
    'validationStatus' | 'validationScore' | 'validationOutcome' | 'validationRunId'> & { destinationPath?: string };
  status: PublisherEligibilityStatus;
  reason?: string;
};

export type PublisherEligibilityResponse = {
  items: PublisherEligibilityCandidate[];
  limit: number;
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
  batchPosition?: number;
  libraryId: number;
  profileId: number;
  audioProfileKey?: string;
  trackProfileKey?: string;
  candidateSubtitleDispositions?: CandidateSubtitleDispositionOverride[];
  cadenceModeOverride?: 'auto' | 'preserve' | 'remove_soft_telecine' | 'inverse_telecine';
  cadenceFieldOrder?: 'auto' | 'tff' | 'bff';
  processingMode?: 'full_encode' | 'audio_only';
  priority: number;
  notes?: string;
  resolveProfileAssignments?: boolean;
  profileResolution?: Record<string, unknown>;
};

export type CandidateSubtitleDispositionOverride = {
  streamIndex: number;
  streamSignature: string;
  sourceRevision: string;
  action: 'keep' | 'extract' | 'keep_and_extract' | 'remove';
};

export type QueueBatchInput = {
  batchId: string;
  batchName?: string;
  jobs: QueueJobInput[];
};

export type QueueBatchPreflightInput = QueueBatchInput & { revision?: number };

export type QueueBatchPreflightAssignmentValue = {
  selection?: string;
  source?: string;
  assignmentOrigin?: string;
  targetPath?: string;
  profileId?: number;
  profileName?: string;
  profileVersion?: number;
  profileKey?: string;
  libraryId?: number;
  libraryName?: string;
  destinationPath?: string;
  processingMode?: string;
  selectedEncoder?: string;
  candidateOverride?: boolean;
};

export type QueueBatchPreflightAssignmentChange = {
  domain: 'video' | 'audio' | 'tracks' | 'destination' | 'asset' | string;
  reason: string;
  before: QueueBatchPreflightAssignmentValue;
  current: QueueBatchPreflightAssignmentValue;
};

export type QueueBatchPreflightAssignmentReview = {
  required: boolean;
  changes: Array<{
    domain: 'video' | 'audio' | 'tracks' | 'destination' | string;
    before: QueueBatchPreflightAssignmentValue;
    after: QueueBatchPreflightAssignmentValue;
  }>;
};

export type QueueBatchPreflightUpdateInput = QueueBatchPreflightInput & { reResolveAssignments?: boolean };

export type QueueBatchPreflightPlan = {
  profile: Profile;
  profileSnapshot: Record<string, unknown> | null;
  profileVersion: number;
  profileCapturedAt?: string;
  profileResolution: Record<string, unknown> | null;
  audioProfileSnapshot: Record<string, unknown> | null;
  trackProfileSnapshot: Record<string, unknown> | null;
  constraints: Record<string, unknown> | null;
  qualityEvidence?: {
    sourceBitrateEvidence?: SourceBitrateEvidence | null;
    bpppfAssessment?: BPPPFAssessment | null;
    profileQualification?: {
      status: string;
      message?: string;
      qualificationId?: string;
      outcome?: string;
      policyVersion?: string;
      testEncodeId?: number;
      sourceAssetId?: number;
      sourceRevision?: string;
      sourceFingerprint?: string;
      resolvedProfileFingerprint?: string;
      perceptualEvidenceIdentity?: string;
      referenceIdentity?: string;
      candidateIdentity?: string;
      worstWindowIdentity?: string;
      worstSceneCategory?: string;
      rules?: ProfileQualificationRuleResult[];
      warnings?: string[];
    } | null;
  } | null;
  selectedEncoder: string;
  runtimeSnapshotId: number;
  runtimeProfile: string;
  destination: Record<string, unknown> | null;
  publishMode: string;
  processingMode: string;
  subtitlePlan?: QueueBatchPreflightSubtitlePlan;
};

export type QueueBatchPreflightSubtitleDecision = {
  streamIndex: number;
  streamSignature?: string;
  codec?: string;
  language?: string;
  title?: string;
  sourceDefault: boolean;
  action: 'keep' | 'extract' | 'keep_and_extract' | 'remove';
  inheritedAction?: 'keep' | 'extract' | 'keep_and_extract' | 'remove';
  role?: string;
  actionSource?: string;
  selectionSource?: string;
  queueOverrideAllowed: boolean;
  queueOverrideReason?: string;
  forced: boolean;
  effectiveForced: boolean;
};

export type QueueBatchPreflightSubtitleSidecar = {
  streamIndex: number;
  codec?: string;
  sourceType?: string;
  language?: string;
  format?: string;
  requestedFormat?: string;
  mode: string;
  ocrRequired: boolean;
  ocrLanguage?: string;
  ocrMode?: string;
  extension?: string;
  settingsSources?: Record<string, string>;
};

export type QueueBatchPreflightSubtitlePlan = {
  version: number;
  state: 'unavailable' | 'no_subtitle_streams' | 'no_subtitle_tracks_selected' | 'planned' | string;
  sourceStreamCount: number;
  keptInMKV: number;
  sidecarStreams: number;
  keepAndExtract: number;
  removed: number;
  policySource?: string;
  explicitAssetSelection: boolean;
  decisions: QueueBatchPreflightSubtitleDecision[];
  sidecars: QueueBatchPreflightSubtitleSidecar[];
  defaultSubtitle: { requestedLanguage?: string; streamIndex?: number; status: string };
  warnings?: string[];
};

export type QueueBatchPreflightAsset = {
  id: string;
  mediaPath: string;
  assetRecordId?: number;
  sourceRevision?: string;
  batchId: string;
  batchName: string;
  input: QueueJobInput;
  status: 'ready' | 'review' | 'requires_review' | 'blocked' | 'queued';
  reviewCode?: 'ivtc_field_order_required' | 'cadence_output_fps_unknown' | string;
  cadenceReview?: {
    cadenceType?: string;
    declaredFrameRate?: string;
    recommendedOperation?: string;
    recommendedFrameRate?: string;
    reportedFieldOrder?: string;
    validatedSelectedOrder?: string;
    validatedWindows: number;
    validWindows?: number;
    observedWindows: number;
    effectiveOutputFrameRate?: string;
    effectiveOutputFrameRateUnknown: boolean;
    hevcLevelResolutionState?: string;
    hevcLevelResolutionWarning?: string;
    hevcLevelEffective?: string;
    hevcLevelTier?: string;
    deepAnalysisAvailable?: boolean;
    deepAnalysisVersion?: number;
    deepAnalysisStatus?: string;
    sourceFrameRate?: string;
    expectedOutputFrameRate?: string;
    attemptedWindows?: number;
    inconclusiveWindows?: number;
    contradictoryWindows?: number;
    windows?: Array<{
      start: number;
      outcome?: string;
      reason?: string;
      selectionBasis?: string;
      selectedOrder?: string;
      diagnosticsAvailable?: boolean;
      tffResidualInterlaced?: number;
      tffRepeatedTop?: number;
      tffRepeatedBottom?: number;
      tffCanonicalOutputFrames?: number;
      tffCanonicalOutputProgressiveFrames?: number;
      tffCanonicalOutputFrameRate?: number;
      tffCanonicalTimestampAnomalies?: number;
      tffCanonicalIdetInterlacedFrames?: number;
      tffCanonicalIdetClassifiedFrames?: number;
      bffResidualInterlaced?: number;
      bffRepeatedTop?: number;
      bffRepeatedBottom?: number;
      bffCanonicalOutputFrames?: number;
      bffCanonicalOutputProgressiveFrames?: number;
      bffCanonicalOutputFrameRate?: number;
      bffCanonicalTimestampAnomalies?: number;
      bffCanonicalIdetInterlacedFrames?: number;
      bffCanonicalIdetClassifiedFrames?: number;
      tffProgressiveRatio: number;
      bffProgressiveRatio: number;
      confidence: number;
    }>;
  };
  blockers: string[];
  warnings: string[];
  queuedJobId?: number;
  assignmentChanges?: QueueBatchPreflightAssignmentChange[];
  assignmentReview?: QueueBatchPreflightAssignmentReview;
  plan?: QueueBatchPreflightPlan;
};

export type QueueBatchPreflightResponse = {
  id: string;
  revision: number;
  resolved: boolean;
  status: 'active' | 'completed' | 'discarded';
  batchId: string;
  batchName: string;
  assets: QueueBatchPreflightAsset[];
  jobs?: QueueJob[];
};

export type QueueBatchResponse = {
  batchId: string;
  batchName: string;
  jobs: QueueJob[];
};

export type QueueSelectedAssetsInput = {
  assetIds: number[];
  commit: boolean;
};

export type QueueSelectedAssetReason =
  | 'not_found'
  | 'not_unprocessed'
  | 'missing'
  | 'needs_review'
  | 'already_queued'
  | 'active_maintenance'
  | 'invalid_configuration'
  | 'reservation_conflict'
  | 'queue_creation_failed';

export type QueueSelectedAssetResult = {
  assetId: number;
  outcome: 'eligible' | 'queued' | 'skipped' | 'failed';
  reason?: QueueSelectedAssetReason;
  message?: string;
  batchId?: string;
  batchName?: string;
  jobId?: number;
  analysisEvidence?: AssetAnalysisEvidence;
};

export type QueueSelectedAssetsResponse = {
  summary: {
    selected: number;
    eligible: number;
    queued: number;
    skipped: number;
    failed: number;
    titleCount: number;
    sizeBytes: number;
  };
  results: QueueSelectedAssetResult[];
  batches: Array<{ batchId: string; batchName: string; jobCount: number }>;
};

export type ProfileAssignment = {
  id: number;
  targetType: 'source_group' | 'logical_group' | 'path' | 'asset';
  targetPath: string;
  mediaType: 'video' | 'audio' | 'tracks';
  selection:
    | 'profile'
    | 'disabled'
    | 'override_only'
    | 'audio_only';
  videoProfileId?: number;
  profileKey?: string;
  createdAt: string;
  updatedAt: string;
};

export type ProfileAssignmentInput =
  Omit<
    ProfileAssignment,
    'id' | 'createdAt' | 'updatedAt' | 'selection'
  > & {
    selection:
      | 'profile'
      | 'disabled'
      | 'inherit'
      | 'override_only'
      | 'audio_only';
  };

export type ClaimJobInput = {
  workerName: string;
  encoders?: string[];
  runtimeProfile?: string;
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

export type QueueReorderInput = {
  jobId: number;
  targetJobId: number;
  placement: 'before' | 'after';
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
  level?: number;
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
  role?: SubtitleSemanticRole;
  roleProvenance?: string;
  roleConfidence?: string;
  roleSourceSnapshotFingerprint?: string;
  roleStreamSignature?: string;
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

export type ColorPolicyPresentation = {
  status?: 'complete' | 'incomplete_unsupported' | 'not_applicable' | string;
  action?: 'none' | 'review' | string;
  reason?: string;
  warning?: string;
};

export type AttachmentStreamInfo = {
  index: number;
  type: 'attachment';
  codec: string;
  filename: string;
  mimeType: string;
  title?: string;
  attachmentKind: 'FONT' | 'IMAGE' | 'ATTACHMENT' | string;
  fontFormat: 'TTF' | 'OTF' | 'TTC' | 'OTC' | '' | string;
};

export type ResolvedAttachmentStream = {
  streamIndex: number;
  codec?: string;
  filename?: string;
  mimeType?: string;
  title?: string;
  attachmentKind: 'FONT' | 'IMAGE' | 'ATTACHMENT' | string;
  fontFormat?: 'TTF' | 'OTF' | 'TTC' | 'OTC' | string;
};

export type ResolvedFontAttachment = {
  artifactId: string;
  streamIndex: number;
  attachmentOrdinal: number;
  codec?: string;
  originalName?: string;
  mimeType?: string;
  fontFormat: 'TTF' | 'OTF' | 'TTC' | 'OTC';
  safeFilename: string;
};

export type ScanResult = {
  id: number;
  path: string;
  fileName: string;
  container: string;
  sizeBytes: number;
  duration: number;
  bitrate: number;
  sourceBitrateEvidence?: SourceBitrateEvidence;
  videoCodec: string;
  width: number;
  height: number;
  hdr: boolean;
  colorPolicy?: ColorPolicyPresentation;
  audioTracks: number;
  subtitleTracks: number;
  chapters: number;
  videoStreams: MediaStreamInfo[];
  audioStreams: MediaStreamInfo[];
  subtitleStreams: MediaStreamInfo[];
  attachmentStreams: AttachmentStreamInfo[];
  attachmentInventoryAvailable: boolean;
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
  directPlayAnalysis?: {
    enabled?: boolean;
    estimated?: boolean;
    strategy?: string;
    minimumScore?: number;
    lowestScore?: number;
    risk?: string;
    blocked?: boolean;
    status?: string;
    error?: string;
    clients?: Array<{ client: string; score: number; risk: string; warnings: string[] }>;
  };
  interlaceAnalysis: {
    version?: number;
    codec?: string;
    averageFrameRate?: string;
    realFrameRate?: string;
    status?: 'progressive' | 'interlaced' | 'mixed' | 'hybrid' | 'telecine' | 'telecine_suspected' | 'unknown';
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
    recommendedAction?: 'none' | 'deinterlace' | 'ivtc' | 'review' | string;
    decisionReason?: string;
    automaticFilter?: string;
    windowStart?: number;
    windowSeconds?: number;
    sampleCount?: number;
    regionCount?: number;
    ambiguousSampleCount?: number;
    contradictorySampleCount?: number;
    softTelecineSampleCount?: number;
    frameSignalSampleCount?: number;
    analysisDepth?: 'quick' | 'deep' | string;
    windowsRequested?: number;
    maximumWindows?: number;
    deepAnalysisTriggered?: boolean;
    depthReason?: string;
    sampledAt?: number[];
    windows?: Array<{
      start: number;
      seconds: number;
      status: 'progressive' | 'interlaced' | 'hybrid' | 'telecine_suspected' | 'unknown' | string;
      confidence: number;
      tff: number;
      bff: number;
      progressive: number;
      undetermined: number;
      repeatedTop: number;
      repeatedBottom: number;
      sampledFrames: number;
      frameSignals?: {
        decodedFrames: number;
        interlacedFrames: number;
        progressiveFrames: number;
        topFieldFirstFrames: number;
        bottomFirstFrames: number;
        repeatPictFrames: number;
        cadence?: string;
      };
    }>;
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
      validatedWindows?: number;
      windows?: Array<{
        start: number;
        outcome?: string;
        reason?: string;
        selectionBasis?: string;
        diagnosticsAvailable?: boolean;
        tffProgressiveRatio: number;
        bffProgressiveRatio: number;
        tffResidualInterlaced?: number;
        tffRepeatedTop?: number;
        tffRepeatedBottom?: number;
        tffCanonicalOutputFrames?: number;
        tffCanonicalOutputProgressiveFrames?: number;
        tffCanonicalOutputFrameRate?: number;
        tffCanonicalTimestampAnomalies?: number;
        tffCanonicalIdetInterlacedFrames?: number;
        tffCanonicalIdetClassifiedFrames?: number;
        bffResidualInterlaced?: number;
        bffRepeatedTop?: number;
        bffRepeatedBottom?: number;
        bffCanonicalOutputFrames?: number;
        bffCanonicalOutputProgressiveFrames?: number;
        bffCanonicalOutputFrameRate?: number;
        bffCanonicalTimestampAnomalies?: number;
        bffCanonicalIdetInterlacedFrames?: number;
        bffCanonicalIdetClassifiedFrames?: number;
        selectedOrder?: 'tff' | 'bff' | string;
        confidence: number;
      }>;
    };
  };
  cadenceAnalysis?: {
    version?: number;
    type?: 'native_progressive' | 'soft_telecine' | 'hard_telecine' | 'interlaced' | 'mixed' | 'unknown' | string;
    pattern?: string;
    declaredFrameRate?: string;
    declaredFps?: number;
    effectivePictureRate?: string;
    effectiveFps?: number;
    frameRateMismatch?: boolean;
    frameRateRatio?: number;
    repeatPictDetected?: boolean;
    confidence?: number;
    sampleCount?: number;
    regionCount?: number;
    ambiguousSampleCount?: number;
    contradictorySampleCount?: number;
    softTelecineSampleCount?: number;
    consistentSampleCount?: number;
    sampleEffectiveFps?: number[];
    decisionReason?: string;
  };
  cadenceRecommendation?: {
    version?: number;
    operation?: 'preserve' | 'remove_soft_telecine' | 'inverse_telecine' | 'review' | string;
    outputFrameRate?: string;
    confidence?: number;
    reason?: string;
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
  restorationAnalysis?: RestorationAnalysis;
  frameStructureAnalysis?: QSVFrameStructureAnalysis;
  frameStructureRecommendation?: FrameStructureRecommendationSet;
  hevcLevelRecommendation?: HEVCLevelRecommendation;
  rawProbe: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
};

export type RestorationSignalEvidence = {
  availability: 'available' | 'ambiguous' | 'unavailable' | string;
  severity: string;
  value?: number;
  confidence: string;
  supportingEvidence: string[];
};

export type RestorationAnalysis = {
  version: number;
  status: 'available' | 'unavailable' | string;
  source: string;
  windows: number;
  sampledFrames: number;
  blocking: RestorationSignalEvidence;
  noise: RestorationSignalEvidence;
  grain: RestorationSignalEvidence;
  chromaNoise: RestorationSignalEvidence;
  banding: RestorationSignalEvidence;
  ringing: RestorationSignalEvidence;
  edgeDetailConfidence: RestorationSignalEvidence;
};

export type MVForgeQualityGoal = 'maximum_savings' | 'balanced' | 'conservative' | 'maximum_quality' | 'archive';

export type MVForgePreferences = {
  qualityGoal: MVForgeQualityGoal;
  executionPreference: 'software' | 'hardware';
  preferredVideoEncoder: string;
  preferredLanguages: string[];
};

export type FrameStructureRecommendationSet = {
  version: number;
  sourceAnalysisVersion: number;
  fps: number;
  recommendedMaxBFrames: number;
  confidence: string;
  autoStrategy?: 'compatible' | 'balanced';
  analysisDriven?: boolean;
  sourceAnchorFrames?: number;
  sourceAnchorSeconds?: number;
  sourceFps?: number;
  byMode: Partial<Record<'compatible' | 'balanced' | 'maximum_compression', QSVFrameStructureRecommendation>>;
  warnings?: string[];
};

export type HEVCLevelRecommendation = {
  version: number;
  recommendedLevel?: string;
  levelIdc?: number;
  tier: 'main';
  width: number;
  height: number;
  fps: number;
  bitrate?: number;
  lumaPictureSize: number;
  lumaSampleRate: number;
  sourceLevel?: string;
  sourceLevelIdc?: number;
  confidence: string;
  limitingFactor?: 'picture_size' | 'sample_rate' | 'bitrate' | string;
  reasons: string[];
  warnings?: string[];
};

export type QSVFrameStructureRecommendation = {
  targetGopFrames: number;
  targetGopSeconds: number;
  maxBFrames: number;
  adaptiveI: boolean;
  adaptiveB: boolean;
  sourceAverageGop: number;
  sourceMaxBRun: number;
  sourceBRatio: number;
  confidence: string;
  reasons: string[];
  warnings: string[];
};

export type PreviewVideoCharacteristics = {
  codec: string;
  profile?: string;
  level?: number;
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

export type QSVFrameStructureAnalysis = {
  version?: number;
  sampleLimit?: number;
  framesAnalyzed: number;
  iFrames: number;
  pFrames: number;
  bFrames: number;
  keyFrames: number;
  bFrameRatio: number;
  hasBFrames: boolean;
  maxConsecutiveBFrames: number;
  averageGopLength: number;
  medianGopLength?: number;
  p25GopLength?: number;
  p75GopLength?: number;
  minimumGopLength?: number;
  maximumGopLength?: number;
  completeGops?: number;
  sampledSeconds?: number;
  assetDurationSeconds?: number;
  coverageRatio?: number;
  windowCount?: number;
  windowLengthSeconds?: number;
  positions?: number[];
  variability?: 'low' | 'medium' | 'high' | 'unknown' | string;
  confidence?: 'high' | 'medium' | 'low' | string;
  confidenceScore?: number;
  processCount?: number;
  windowsRequested?: number;
  earlyStopped?: boolean;
  deepAnalysisTriggered?: boolean;
  frameSignals?: FrameSignalSummary;
  windows?: Array<{ position: number; startSeconds: number; durationSeconds: number; analysis: QSVFrameStructureAnalysis; frameSignals?: FrameSignalSummary }>;
  assessment: string;
  source: string;
};

export type FrameSignalSummary = {
  decodedFrames: number;
  interlacedFrames: number;
  progressiveFrames: number;
  topFieldFirstFrames: number;
  bottomFirstFrames: number;
  repeatPictFrames: number;
  cadence?: string;
  actualTimespan?: number;
  effectiveFps?: number;
  timestampDeltas?: { count: number; minimum?: number; maximum?: number; dominant?: number[] };
};

export type SnapshotOperation = {
  id: string;
  assetPath: string;
  status: 'running' | 'completed' | 'error' | 'cancelled';
  phase: string;
  progress: number;
  message: string;
  stageTimingsMs?: Record<string, number>;
  durationMs: number;
  cacheHit: boolean;
  incrementalRefresh: boolean;
  deepCadence?: boolean;
  reusedComponents?: string[];
  refreshedComponents?: string[];
  componentStatuses?: Record<string, string>;
  fallbackReason?: string;
  result?: ScanResult;
  error?: string;
  createdAt: string;
  updatedAt: string;
};

export type TrackMaintenanceStream = {
  index: number;
  type: 'video' | 'audio' | 'subtitle' | 'attachment' | 'data' | string;
  codec: string;
  language?: string;
  title?: string;
  fileName?: string;
  mimeType?: string;
  attachmentKind?: 'FONT' | 'IMAGE' | 'ATTACHMENT' | string;
  fontFormat?: 'TTF' | 'OTF' | 'TTC' | 'OTC' | string;
  profile?: string;
  width?: number;
  height?: number;
  channels?: number;
  layout?: string;
  default: boolean;
  forced: boolean;
  attachedPic: boolean;
  stillImage: boolean;
};

export type TrackMaintenanceInventory = {
  path: string;
  fingerprint: string;
  streams: TrackMaintenanceStream[];
  chapters: number;
  durationSeconds: number;
  maintenanceAllowed?: boolean;
  maintenanceDisabledReason?: string;
};

export type AssetMaintenanceOperation = {
  id: string;
  operationType: string;
  assetPath: string;
  status: 'queued' | 'running' | 'completed' | 'failed' | string;
  phase: string;
  progress: number;
  errorMessage?: string;
  warning?: string;
};

export type QSVFeatureStatus = {
  requestedGopFrames?: number;
  requestedBFrames?: number;
  adaptiveIRequested: boolean;
  adaptiveIEffective: boolean;
  adaptiveBRequested: boolean;
  adaptiveBEffective: boolean;
  gpbKnown: boolean;
  gpbEffective: boolean;
  gopRefDist?: number;
  gopPicSize?: number;
  bRefType?: string;
  pRefType?: string;
  rateControlMethod?: string;
  targetUsage?: number;
  measuredKnown: boolean;
  reorderFrames?: number;
  dpbFrames?: number;
  estimatedDpbMiB?: number;
  temporalLayers?: number;
  interpretationMode?: 'qsv_gpb' | 'qsv_mixed_b_gpb';
  contextSource?: 'preview_encode' | 'worker_capability';
};

export type PreviewInspection = {
  source: PreviewVideoCharacteristics;
  output: PreviewVideoCharacteristics;
  sourceFrameStructure: QSVFrameStructureAnalysis;
  outputFrameStructure: QSVFrameStructureAnalysis;
  qsvFrameWarnings: string[];
  qsvFeatureStatus: QSVFeatureStatus;
  cacheHit: boolean;
  previewMode: 'quick' | 'quality';
  start: string;
  seconds: number;
  generatedPath: string;
  requestedEncoder: string;
  effectiveEncoder: string;
  requestedQSVRateControl: string;
  effectiveQSVRateControl: string;
  effectiveVideoDecision?: EffectiveVideoDecision;
  hevcBitstreamEvidence?: HEVCBitstreamEvidence;
  frameStructureRecommendation?: FrameStructureRecommendationSet;
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

export type EffectiveVideoDecision = {
  encoder?: string;
  codec?: string;
  profile?: string;
  pixelFormat?: string;
  frameStructureMode?: string;
  cadenceOperation?: string;
  cadenceFieldOrder?: 'tff' | 'bff' | string;
  residualCombingFallback?: 'selective' | 'none' | string;
  residualCombingFallbackReason?: string;
  effectiveMotionFilters?: string;
  effectiveFrameRate?: string;
  effectiveProgressive?: boolean;
  effectiveWidth?: number;
  effectiveHeight?: number;
  geometryUnknown?: boolean;
  gopFrames?: number;
  gopSeconds?: number;
  maxBFrames?: number;
  hevcLevel?: string;
  hevcTier?: string;
  hevcLevelMode?: string;
  hevcLevelInputFingerprint?: string;
  hevcLevelResolutionState?: string;
  hevcLevelEnforcement?: 'advisory' | 'hard' | string;
  hevcRateControlSource?: string;
  hevcRateControlBounded?: boolean;
  hevcRateControlTargetKbps?: number;
  hevcRateControlMaxrateKbps?: number;
  hevcRateControlBufferKbps?: number;
  hevcLevelWarning?: string;
  effectiveFinalColorPolicy?: string;
  colorPolicyDecision?: string;
  colorPolicyWarning?: string;
  upscale?: ResolvedUpscaleDecision;
  restoration?: ResolvedRestorationPlan;
};

export type HEVCBitstreamEvidence = {
  stream?: string;
  probe?: string;
  probeOutcome?: 'available' | 'unavailable' | string;
  probeStatus?: 'validated' | 'mismatch' | 'unavailable' | 'unverified' | string;
  levelEvidenceAvailable?: boolean;
  tierEvidenceAvailable?: boolean;
  generalLevelIdc?: number;
  generalLevel?: string;
  generalTierFlag?: number;
  generalTier?: 'main' | 'high' | string;
  error?: string;
};

export type PreviewFrameMetrics = {
  comparable: boolean;
  reason: string;
  sourceDimensions: string;
  outputDimensions: string;
  ssim?: number;
  psnr?: number;
};

export type PerceptualMetricValue = {
  kind: 'finite' | 'positive_infinity';
  value?: number;
};

export type PerceptualMetricResult = {
  identity: {
    identity: string;
    pairIdentity: string;
    metric: Record<string, unknown>;
  };
  status: 'available' | 'unsupported' | 'unavailable' | 'failed_measurement' | 'non_comparable' | string;
  value?: PerceptualMetricValue;
  reason?: string;
};

export type PerceptualWindowEvidence = {
  windowIdentity: string;
  startSeconds: number;
  durationSeconds: number;
  primary: boolean;
  roles?: string[];
  reasons?: string[];
  evidenceRefs?: string[];
  status: string;
  reference?: Record<string, unknown>;
  candidate?: Record<string, unknown>;
  coverage?: Record<string, unknown>;
  pairIdentity?: Record<string, unknown>;
  metrics?: PerceptualMetricResult[];
  comparableReason?: string;
};

export type PerceptualEvidenceSet = {
  status?: string;
  unavailableReason?: string;
  schemaVersion: string;
  policyVersion: string;
  identity: string;
  samplingPlanIdentity: string;
  sourceRevision: string;
  runtimeCapabilityIdentity: string;
  requestedMetrics: Array<Record<string, unknown>>;
  candidateManifest?: Array<Record<string, unknown>>;
  referenceManifest?: Array<Record<string, unknown>>;
  windows: PerceptualWindowEvidence[];
  aggregates?: Array<Record<string, unknown>>;
  capabilities?: Array<Record<string, unknown>>;
  warnings?: string[];
  unavailableCapabilities?: string[];
  artifactEvidence?: Array<Record<string, unknown>>;
  provenance?: Record<string, unknown>;
};

export type CompatiblePreviewOptions = {
  path: string;
  preflightCandidateId?: string;
  preflightRevision?: number;
  preflightAssetId?: string;
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
  qsvMBBRCMode?: 'auto' | 'enabled' | 'disabled';
  qsvRDOMode?: 'auto' | 'enabled' | 'disabled';
  qsvPStrategy?: 0 | 1 | 2;
  mode?: 'quick' | 'quality';
  previewNormalization?: 'preserve' | 'normalize_bt709';
  subtitleStreamIndex?: number;
  ephemeral?: boolean;
};

export type CompatiblePreviewRequest = {
  requestId: string;
  cacheIdentity: string;
  expiresInSeconds: number;
  path: string;
  artifactId?: string;
  sourceRevision?: Record<string, unknown>;
  start?: string;
  seconds?: number;
  normalization?: Record<string, unknown>;
  canonicalEncodeSignature?: string;
  generationToken?: string;
  generationVersion?: number;
};

export type CompatiblePreviewLifecycleState = 'idle' | 'generating' | 'ready' | 'failed' | 'canceled' | 'interrupted' | 'stale' | 'expired' | 'unknown';

export type CompatiblePreviewLifecycleResponse = {
  requestId: string;
  artifactId?: string;
  path?: string;
  sourceRevision?: Record<string, unknown>;
  start?: string;
  seconds?: number;
  normalization?: Record<string, unknown>;
  canonicalEncodeSignature?: string;
  generationToken?: string;
  generationVersion?: number;
  code: string;
  state: CompatiblePreviewLifecycleState | string;
  retryable?: boolean;
  recreate?: boolean;
  reason?: string;
  leaseToken?: string;
  ownerId?: string;
  leaseExpiresAt?: string;
  statusUrl?: string;
  renewUrl?: string;
  cancelUrl?: string;
  startedAt?: string;
  finishedAt?: string;
  elapsedSeconds?: number;
  encoder?: string;
  preset?: string;
  cacheHit?: boolean;
  slow?: boolean;
  retryAfterSeconds?: number;
  renewAfterSeconds?: number;
  leaseReleased?: boolean;
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

export type ProfileSampleEstimateOperation = {
  id: string;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled';
  phase: string;
  progress: number;
  currentSample: number;
  sampleCount: number;
  currentSampleProgress: number;
  encodedSeconds: number;
  totalSampleSeconds: number;
  speed: number;
  etaSeconds: number;
  result?: ProfileSampleEstimate;
  error?: string;
  createdAt: string;
  updatedAt: string;
};

export type LabFidelityOperationWindow = {
  position: number;
  startSeconds: number;
  durationSeconds: number;
  windowIdentity?: string;
  primary?: boolean;
  roles?: string[];
  source?: PreviewVideoCharacteristics;
  output?: PreviewVideoCharacteristics;
  metrics?: PreviewFrameMetrics;
  requestId: string;
  artifactId?: string;
  generationToken?: string;
  generationVersion?: number;
  status: string;
  canonicalFidelity?: Record<string, unknown>;
  perceptualWindowEvidence?: PerceptualWindowEvidence;
};

export type QSVTemporalVariantRequest = {
  variantIdentity?: string;
  changedVariable: 'quality_rate_control' | 'gop' | 'b_frame_strategy' | 'qsv_feature' | string;
  requestedDelta: Record<string, unknown>;
};

export type QSVTemporalTargetRequest = {
  windowIdentity: string;
  variants?: QSVTemporalVariantRequest[];
};

export type QSVTemporalControlRequest = {
  windowIdentity: string;
  requestId?: string;
  generationIdentity?: string;
  evidenceIdentity?: string;
};

export type QSVTemporalExperimentRequest = {
  variantsByWindow?: QSVTemporalTargetRequest[];
  controls?: QSVTemporalControlRequest[];
};

export type QSVTemporalEvidence = {
  targetWindowIdentity: string;
  variantIdentity: string;
  availability: string;
  interpretation: 'likely_pumping' | 'inconclusive' | 'not_observed' | string;
  interpretationPolicyVersion: string;
  samplingPolicyVersion?: string;
  samplingRateHz?: number;
  baselineInterpretation?: 'likely_pumping' | 'inconclusive' | 'not_observed' | string;
  variantInterpretation?: 'likely_pumping' | 'inconclusive' | 'not_observed' | string;
  mitigationDirection?: 'improved' | 'worsened' | 'unchanged' | 'inconclusive' | string;
  baselineEvidenceIdentity?: string;
  variantEvidenceIdentity?: string;
  frameCount?: number;
  lossRecoveryEvents?: Array<Record<string, unknown>>;
  temporalVariationSummary?: Record<string, unknown>;
  warnings?: string[];
};

export type QSVExperimentCandidate = {
  candidateIdentity: string;
  sourceRevision: string;
  samplingWindowIdentity: string;
  requestId?: string;
  artifactId?: string;
  generationToken?: string;
  generationVersion?: number;
  canonicalEncodeSignature?: string;
  effectiveEncoder?: string;
  requestedQSVSettings?: Record<string, unknown>;
  effectiveQSVSettings?: Record<string, unknown>;
  fallbackReason?: string;
  canonicalFidelity?: Record<string, unknown>;
  sizeBytes?: number;
  bitrate?: number;
  speed?: number;
};

export type QSVTemporalIsolation = {
  result: string;
  declaredVariable: string;
  changedFields?: string[];
  unexpectedFields?: string[];
  reason?: string;
};

export type QSVTemporalVariant = {
  variantIdentity: string;
  changedVariable: string;
  requestedDelta?: Record<string, unknown>;
  effectiveDelta?: Record<string, unknown>;
  isolation: QSVTemporalIsolation;
  candidate: QSVExperimentCandidate;
  temporalEvidence?: QSVTemporalEvidence[];
  perceptualEvidence?: Array<Record<string, unknown>>;
};

export type EncoderControlCandidate = {
  windowIdentity: string;
  requestId?: string;
  encoderFamily?: string;
  artifactIdentity?: string;
  generationIdentity?: string;
  artifactGeneration?: string;
  evidenceIdentity?: string;
  comparable: string;
  canonicalFidelity?: Record<string, unknown>;
  temporalInterpretation?: string;
  warnings?: string[];
};

export type QSVTemporalTargetSummary = {
  windowIdentity: string;
  interval?: { startSeconds: number; durationSeconds: number };
  roles?: string[];
  reasons?: string[];
  primary: boolean;
  baseline: QSVExperimentCandidate;
  variants?: QSVTemporalVariant[];
  temporalEvidence?: QSVTemporalEvidence[];
  controlCandidates?: EncoderControlCandidate[];
  classification?: string;
  recommendation?: Record<string, unknown>;
  warnings?: string[];
};

export type QSVTemporalExperimentSummary = {
  experimentId: string;
  sourceRevision: string;
  samplingPlanIdentity: string;
  unavailableRoles?: string[];
  targets: QSVTemporalTargetSummary[];
  warnings?: string[];
};

export type LabFidelityOperation = {
  id: string;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | string;
  phase: string;
  primaryRequestId: string;
  primaryArtifactId?: string;
  primaryGeneration?: string;
  sourceRevision?: Record<string, unknown>;
  primaryStart?: number;
  primaryDuration?: number;
  normalization?: Record<string, unknown>;
  frozenPlan?: Record<string, unknown>;
  planSignature?: string;
  materialSignature?: string;
  clientCorrelationSignature?: string;
  samplingPolicyVersion?: string;
  samplingPolicy?: Record<string, unknown>;
  temporalExperiment?: QSVTemporalExperimentSummary;
  completedWindows: number;
  totalWindows: number;
  currentStart?: number;
  currentDuration?: number;
  windows?: LabFidelityOperationWindow[];
  result?: Record<string, unknown>;
  error?: string;
  retryable?: boolean;
  createdAt: string;
  updatedAt: string;
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
  bpppfAssessment?: BPPPFAssessment;
};

export type SourceBitrateSampleWindow = {
  startSeconds: number;
  durationSeconds: number;
  bytes?: number;
  status: string;
  reason?: string;
};

export type SourceBitrateEvidence = {
  schemaVersion: number;
  samplingPolicyVersion: number;
  bitrate: number;
  minimumBitrate?: number;
  maximumBitrate?: number;
  method: 'stream_metadata' | 'matroska_statistics' | 'packet_sampling' | 'container_bitrate' | 'file_size_duration' | 'unavailable' | string;
  confidence: 'high' | 'medium' | 'low' | 'unavailable' | string;
  detail?: string;
  streamIndex?: number;
  durationSeconds?: number;
  requestedWindows: number;
  successfulWindows: number;
  sampledWindows: SourceBitrateSampleWindow[];
};

export type BPPPFAssessment = {
  policyVersion: number;
  encoderModelVersion: number;
  status: 'normal' | 'warning' | 'high-risk' | 'unavailable' | string;
  bpppf?: number;
  referenceBpppf?: number;
  referenceBitrate?: number;
  targetBitrate?: number;
  bitrateBasis?: string;
  outputWidth?: number;
  outputHeight?: number;
  effectiveFps?: number;
  codecFamily?: string;
  encoder?: string;
  encoderClass?: string;
  dynamicRange?: 'sdr' | 'hdr' | string;
  optimizationIntent?: string;
  explanation: string;
  heuristicOnly: boolean;
};

export type QualityRecommendationResponse = {
  requestedProfile: Profile;
  effectiveProfile: Profile;
  recommendation: EncoderRecommendation;
  sourceBitrateEvidence?: SourceBitrateEvidence;
  capabilitySource: 'active_runtime_state' | 'active_runtime_snapshot' | 'live_backend_probe';
  ffmpegVideoArguments: string[];
  estimatedOutputMinBytes: number;
  estimatedOutputMaxBytes: number;
  estimatedSavingsMinBytes: number;
  estimatedSavingsMaxBytes: number;
};

export type SubtitleExtractionResult = {
  created: string[];
  existing: string[];
  unsupported: string[];
  artifacts?: Array<{
    artifactId: string;
    type: 'subtitle_sidecar';
    streamIndex: number;
    sourceCodec: string;
    sourceType?: string;
    format: 'srt' | 'ass' | 'ssa' | 'sup';
    requestedFormat?: string;
    mode: 'original' | 'converted' | 'translation' | string;
    ocrRequired?: boolean;
    language?: string;
    stages?: string[];
    extension?: string;
    namingKey?: string;
    stagedPath: string;
    publishedPath?: string;
    sizeBytes: number;
    status: string;
    translation?: {
      identityKey: string;
      identityVersion: number;
      translationContractVersion: string;
      sourceArtifactIdentity: string;
      sourceRevision: string;
      sourceRepresentation: string;
      sourceProvenance: string;
      requestedSourceLanguage: string;
      effectiveSourceLanguage: string;
      targetLocale: string;
      provider: string;
      providerContractVersion: string;
      providerModel: string;
      contextContractVersion: string;
      contextMode: string;
      outputRepresentation: string;
    };
  }>;
};

export type SubtitleExtractionOperation = {
  id: string;
  assetPath: string;
  status: 'running' | 'completed' | 'cancelled' | 'error';
  phase:
    | 'preparing'
    | 'translating'
    | 'validating'
    | 'analyzing'
    | 'extracting'
    | 'ocr'
    | 'ocr_review'
    | 'reconciliation'
    | 'cleanup'
    | 'publishing'
    | 'completed'
    | 'cancelled'
    | 'error';
  progress: number;
  processed: number;
  total: number;
  streamIndex: number;
    format: 'original' | 'srt' | 'ass' | 'ssa' | 'both';
  message?: string;
  result?: SubtitleExtractionResult;
  error?: string;
  retryable?: boolean;
  kind?: 'extraction' | 'ocr_analysis' | 'ocr_correction' | 'translation' | 'proofreading' | string;
  composition?: {
    sources: Array<Record<string, unknown>>;
    policy: Record<string, unknown>;
    outputIdentity: string;
    outputPath: string;
    provenance?: Record<string, unknown>;
  };
  subtitleSync?: {
    proposal: SubtitleSyncProposal;
    appliedAt?: string;
  };
  translation?: {
    provider?: string;
    model?: string;
    requestedSourceLanguage?: string;
    targetLocale?: string;
    outputRepresentation?: string;
    contextMode?: string;
    contextSource?: string;
    failureClass?: string;
    diagnostic?: string;
    retryable: boolean;
    cancellable: boolean;
    ocrReviewTarget?: CanonicalOCRReviewTarget;
    quality?: {
      policyVersion: string;
      state: 'not_analyzed' | 'clean' | 'warnings' | 'review_required' | 'approved' | string;
      cueCount: number;
      cleanCount: number;
      warningCount: number;
      reviewCount: number;
      persisted?: boolean;
      persistenceWarning?: string;
      truncated?: boolean;
      reviewer?: {
        provider: string;
        model: string;
        state: string;
        diagnostic?: string;
        findings?: Array<{ cueId: string; verdict: string; reason?: string; suggestedText?: string }>;
      };
      findings?: Array<{
        cueId: string;
        sourceOrdinal: number;
        sourceText: string;
        translatedText: string;
        reasons: string[];
      }>;
    };
  };
  qualityReport?: SubtitleOCRQualityReport;
  proofreading?: SubtitleProofreadingReport;
  createdAt: string;
  updatedAt: string;
};

export type SubtitleProofreadingFinding = {
  cueId: string;
  ordinal: number;
  sourceTextHash: string;
  originalText: string;
  suggestedText?: string;
  status: 'clean' | 'suggested' | 'needs_review' | string;
  category: string;
  reasonCode: string;
  reason: string;
  confidence?: number;
  neighbors?: Array<{ cueId: string; text: string }>;
};

export type SubtitleProofreadingReport = {
  sidecarPath: string;
  format: 'srt' | 'ass' | 'ssa' | string;
  mode: 'quick' | 'extended' | string;
  sourceType: 'ocr-generated' | 'translated' | 'imported/external' | 'unknown' | string;
  selfReview: boolean;
  language?: string;
  revision: string;
  provider: string;
  model?: string;
  providerState: string;
  providerNotice?: string;
  totalCount: number;
  cleanCount: number;
  suggestedCount: number;
  reviewCount: number;
  unassessedCount: number;
  findings: SubtitleProofreadingFinding[];
};

export type SubtitleOCRQualityFinding = {
  cueId: number;
  originalText: string;
  suggestion?: string;
  aiSuggestion?: {
    cueId: number;
    originalHash: string;
    proposedText: string;
    reason: string;
    model: string;
    baselineRevision: string;
  };
  status: 'clean' | 'suspicious' | 'review' | string;
  reasons?: string[];
  evidence?: string[];
  candidateSource?: string;
  dictionaryRuleIds?: string[];
  dictionaryRuleVersions?: number[];
  baselineRevision: string;
  startSeconds: number;
  endSeconds: number;
  timingProvenance?: string;
  role?: string;
  geometry?: {
    canvasWidth: number;
    canvasHeight: number;
    bitmapX: number;
    bitmapY: number;
    bitmapWidth: number;
    bitmapHeight: number;
  };
};

export type PublishedSubtitleRevision = {
  artifactPath: string;
  artifactIdentity: string;
  sizeBytes: number;
  modifiedAtUnixNano: string;
  contentHash: string;
  logicalTransform?: string;
};

export type CanonicalOCRIdentity = {
  version: number;
  sourcePath: string;
  sourceSize: number;
  sourceModTimeUnixNano: string;
  sourceSnapshotVersion: string;
  streamIndex: number;
  streamId: string;
  streamCodec: string;
  requestedLanguage: string;
  ocrMode: string;
  preparationScope: string;
  segmentStart: string;
  segmentDuration: number;
  preparedStreamMapping: string;
  preparedRepresentation: string;
  preparedRepresentationHash?: string;
  pipelineProvenance: string;
  runtimeProvenance: string;
};

export type CanonicalOCRReviewTarget = {
  state: 'exact' | 'missing' | 'stale' | 'conflict' | string;
  message?: string;
  identity?: CanonicalOCRIdentity;
  logicalTransformKey?: string;
  format?: 'srt' | 'ass' | 'both' | string;
  artifacts?: Array<{
    ownerIdentity: string;
    path: string;
    format: 'srt' | 'ass' | string;
    logicalTransformKey?: string;
    revision: PublishedSubtitleRevision;
  }>;
  sidecarPaths?: string[];
  revisions?: PublishedSubtitleRevision[];
  baselineRevision?: string;
};

export type SubtitleOCRQualityReport = {
  artifactPath: string;
  format: 'srt' | 'ass' | string;
  logicalTransformKey?: string;
  revision: PublishedSubtitleRevision;
  revisionSet?: PublishedSubtitleRevision[];
  baselineRevision: string;
  analyzerVersion: string;
  repairVersion: string;
  dictionaryLanguage?: OCRDictionaryLanguage;
  dictionaryRevision?: string;
  cleanCount: number;
  suspiciousCount: number;
  reviewCount: number;
  legacyTextOnly?: boolean;
  canCorrect: boolean;
  findings: SubtitleOCRQualityFinding[];
  reviewTarget?: CanonicalOCRReviewTarget;
  ocrAI?: {
    enabled: boolean;
    state: string;
    model?: string;
    notice?: string;
    suggestedCount: number;
    unassessedCount: number;
  };
};

export type OCRDictionaryLanguage = 'eng' | 'spa' | 'jpn' | 'jpn_vert';

export type OCRDictionaryEntry = {
  id: string;
  language: OCRDictionaryLanguage;
  pattern: string;
  replacement: string;
  matchMode: 'literal';
  boundaryPolicy: 'word';
  leftContext?: string;
  rightContext?: string;
  casePolicy: 'exact' | 'insensitive';
  priority: number;
  enabled: boolean;
  source: 'builtin' | 'user';
  ruleVersion: number;
  createdAt: string;
  updatedAt: string;
};

export type OCRDictionaryRuleInput = {
  language: OCRDictionaryLanguage;
  pattern: string;
  replacement: string;
  matchMode: 'literal';
  boundaryPolicy: 'word';
  leftContext?: string;
  rightContext?: string;
  casePolicy: 'exact' | 'insensitive';
  priority: number;
  enabled?: boolean;
};

export type OCRDictionarySearchResponse = {
  entries: OCRDictionaryEntry[];
  page: number;
  pageSize: number;
  total: number;
};

export type OCRDictionaryTestMatchInput = {
  language: OCRDictionaryLanguage;
  sample: string;
  rule: OCRDictionaryRuleInput;
};

export type OCRDictionaryTestMatchResponse = {
  language: OCRDictionaryLanguage;
  matched: boolean;
  matchCount: number;
  result: string;
  ambiguous?: Array<{ start: number; end: number; ruleIds: string[] }>;
  dictionaryRevision: string;
};

export type OCRDictionaryReevaluationInput = {
  mediaPath?: string;
  sidecarPath?: string;
  sidecarPaths?: string[];
  format: 'srt' | 'ass' | 'both';
  logicalTransformKey?: string;
  dictionaryLanguage?: OCRDictionaryLanguage;
};

export type OCRDictionaryReevaluationChange = {
  cueId: number;
  before: string;
  after: string;
  ruleIds: string[];
  ruleVersions: number[];
  beforeTextHash: string;
  afterTextHash: string;
};

export type OCRDictionaryReevaluationPreview = {
  previewId: string;
  artifactRevision: string;
  revisions: NonNullable<SubtitleOCRQualityReport['revisionSet']>;
  dictionaryLanguage?: OCRDictionaryLanguage;
  dictionaryLanguageSource?: string;
  dictionaryRevision: string;
  cleanCount: number;
  suspiciousCount: number;
  reviewCount: number;
  requiresLanguageChoice: boolean;
  changes: OCRDictionaryReevaluationChange[];
};

export type OCRDictionaryReevaluationApplyInput = OCRDictionaryReevaluationInput & {
  previewId: string;
  artifactRevision: string;
  dictionaryRevision: string;
  acceptedCueIds: number[];
};

export type SubtitleExtractionOperationList = {
  operations: SubtitleExtractionOperation[];
};

export type SubtitleRestyleInput = {
  path: string;
  sourcePath: string;
  sourceRevision?: string;
  representation: 'srt' | 'ass' | 'ssa';
  language?: string;
  role?: SubtitleSemanticRole;
  mode: 'restyle' | 'srt_to_ass' | 'regenerate';
  replace?: boolean;
  overrideExistingASSStyles?: boolean;
  streamIndex?: number;
  ocrLanguage?: string;
  ocrMode?: string;
};

export type RemoteStorageMapping = {
  localRoot: string;
  remoteRoot: string;
};

export type RemoteExecutorConfig = {
  name: string;
  enabled: boolean;
  host: string;
  user: string;
  sshKeyPath: string;
  knownHostsPath: string;
  ffmpegPath: string;
  ffprobePath: string;
  storageMappings: RemoteStorageMapping[];
};

export type RemoteExecutorsSetting = {
  executors: RemoteExecutorConfig[];
};

export type RemoteExecutorProbe = {
  name: string;
  reachable: boolean;
  hostname?: string;
  ffmpegAvailable: boolean;
  ffmpegVersion?: string;
  ffprobeAvailable: boolean;
  ffprobeVersion?: string;
  videoToolbox: boolean;
  storageAccessible: boolean;
  error?: string;
};
