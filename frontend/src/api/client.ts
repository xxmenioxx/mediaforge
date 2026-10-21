import type {
  Library,
  LibraryInput,
  LibraryRenameApplyResponse,
  LibraryRenamePlan,
  LogFile,
  LogFileContent,
  MWPImportSummary,
  AnalysisBackfillResponse,
  AppSetting,
  Asset,
  AssetReviewUpdateInput,
  AssetMetadataUpdateInput,
  AssetConversionUpdateInput,
  AssetTracksUpdateInput,
  AssetTracksUpdateResponse,
  AssetInventory,
  ArchivedOriginalMutationInput,
  AssetEditInput,
  AssetEditResponse,
  AssetSyncResult,
  AdvisorRequest,
  AdvisorResponse,
  ProfileSuggestion,
  ClaimJobInput,
  ExecuteJobInput,
  ExecutionPlan,
  ExternalSubtitle,
  JobArtifactsResponse,
  JobLogResponse,
  Profile,
  ProfileInput,
  ProfileTemplate,
  CloneProfileTemplateInput,
  ProfileAssignment,
  ProfileAssignmentInput,
  PathBrowseResponse,
  PublishResult,
  PublisherPublicationSummary,
  PublisherEligibilityResponse,
  QueueJob,
  QueueJobInput,
  QueueJobUpdateInput,
  QueueReorderInput,
  QueueBatchInput,
  QueueBatchResponse,
  QueueBatchPreflightInput,
  QueueBatchPreflightResponse,
  QueueSelectedAssetsInput,
  QueueSelectedAssetsResponse,
  ValidationOverride,
  ValidationRunDetail,
  ValidationResult,
  ValidationSummaryPage,
  ValidationHistoryPage,
  RuntimeState,
  RuntimeProfilesResponse,
  ScanResult,
  SnapshotOperation,
  CompatiblePreviewOptions,
  CompatiblePreviewRequest,
  CompatiblePreviewLifecycleResponse,
  ProfileSampleEstimate,
  ProfileSampleEstimateOperation,
  LabFidelityOperation,
  QSVTemporalExperimentRequest,
  QualityRecommendationResponse,
  PreviewInspection,
  PreviewFrameMetrics,
  SoftwareVersions,
  UpdateLibraryInput,
  UpdateProfileInput,
  UpdateSettingInput,
  UpdateSettingsBatchInput,
  UpdateJobStatusInput,
  WorkerNode,
  SchedulerRecoveryReport,
  HousekeepingReport,
  SubtitleExtractionOperation,
  SubtitleExtractionOperationList,
  SubtitleRestyleInput,
  SubtitleCompositionInput,
  SubtitleEligibilityResponse,
  SubtitleExtractionResult,
  OCRDictionaryEntry,
  OCRDictionaryLanguage,
  OCRDictionaryReevaluationApplyInput,
  OCRDictionaryReevaluationInput,
  OCRDictionaryReevaluationPreview,
  OCRDictionaryRuleInput,
  OCRDictionarySearchResponse,
  OCRDictionaryTestMatchResponse,
  OCRDictionaryTestMatchInput,
  CanonicalOCRReviewTarget,
  SubtitleTranslationCapability,
  SubtitleTranslationCapabilityInput,
  SubtitleTranslationReadiness,
  StartSubtitleTranslationInput,
  RemoteExecutorConfig,
  RemoteExecutorProbe,
  TrackProfileResolutionPreview,
  TestEncode,
  TestEncodeInput,
  TestEncodeInventorySummary,
  TestEncodeDeletePreview,
  TestEncodeDeleteSelectionResult,
  TestEncodeReconcileResponse,
  TestEncodeOrphanArtifact,
  LabResolutionRequired,
} from './types';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '';

export class ApiRequestError extends Error {
  readonly code?: string;
  readonly state?: string;
  readonly reason?: string;
  readonly retryable?: boolean;
  readonly recreate?: boolean;
  readonly resolution?: LabResolutionRequired;

  constructor(message: string, readonly status: number, details?: {
    code?: string;
    state?: string;
    reason?: string;
    retryable?: boolean;
    recreate?: boolean;
    resolution?: LabResolutionRequired;
  }) {
    super(message);
    this.name = 'ApiRequestError';
    this.code = details?.code;
    this.state = details?.state;
    this.reason = details?.reason;
    this.retryable = details?.retryable;
    this.recreate = details?.recreate;
    this.resolution = details?.resolution;
  }
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    credentials: 'omit',
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });

  if (!response.ok) {
    let message = `Request failed with ${response.status}`;
    try {
      const payload = await response.clone().json();
      if (payload && typeof payload.error === 'string' && payload.error.trim()) {
        message = payload.error;
      } else if (payload && typeof payload.reason === 'string' && payload.reason.trim()) {
        message = payload.reason;
      }
    } catch {
      const text = await response.text().catch(() => '');
      if (text.trim()) {
        message = text.trim();
      }
    }
    const payload = await response.clone().json().catch(() => null) as {
      code?: unknown;
      state?: unknown;
      reason?: unknown;
      retryable?: unknown;
      recreate?: unknown;
      resolution?: unknown;
    } | null;
    throw new ApiRequestError(message, response.status, {
      code: typeof payload?.code === 'string' ? payload.code : undefined,
      state: typeof payload?.state === 'string' ? payload.state : undefined,
      reason: typeof payload?.reason === 'string' ? payload.reason : undefined,
      retryable: typeof payload?.retryable === 'boolean' ? payload.retryable : undefined,
      recreate: typeof payload?.recreate === 'boolean' ? payload.recreate : undefined,
      resolution: payload?.resolution && typeof payload.resolution === 'object' ? payload.resolution as LabResolutionRequired : undefined,
    });
  }

  const contentType = response.headers.get('content-type')?.toLowerCase() ?? '';
  if (!contentType.includes('application/json')) {
    throw new Error(
      `Expected a JSON API response but received ${contentType || 'an unknown content type'}. The MVForge frontend and backend may be running different versions.`,
    );
  }
  return response.json() as Promise<T>;
}

function compatiblePreviewPath(options: CompatiblePreviewOptions) {
  if (options.profile !== undefined) {
    throw new Error('Structured compatible preview profiles must be registered with createCompatiblePreviewRequest().');
  }
  const {
  path,
  profileId = 0,
  start = '00:00:00',
  seconds = 20,
  videoCodec = '',
  qualityValue = 0,
  videoPreset = '',
  pixFmt = '',
  videoFilters = '',
  x265Params = '',
  videoEncoder = 'auto',
  useHardwareIfAvailable = false,
  globalQuality = 25,
  qsvRateControl = 'icq',
  qsvLookAheadDepth = 40,
  qsvExtendedBRC = false,
  qsvAdaptiveI = false,
  qsvAdaptiveB = false,
  qsvPStrategy = 0,
  mode = 'quick',
  previewNormalization = 'normalize_bt709',
  subtitleStreamIndex,
  ephemeral = false,
  } = options;
  return `/api/assets/preview/compatible?path=${encodeURIComponent(path)}&profileId=${profileId}&start=${encodeURIComponent(start)}&seconds=${seconds}&videoCodec=${encodeURIComponent(videoCodec)}&qualityValue=${qualityValue}&videoPreset=${encodeURIComponent(videoPreset)}&pixFmt=${encodeURIComponent(pixFmt)}&videoFilters=${encodeURIComponent(videoFilters)}&x265Params=${encodeURIComponent(x265Params)}&videoEncoder=${encodeURIComponent(videoEncoder)}&useHardwareIfAvailable=${useHardwareIfAvailable}&globalQuality=${globalQuality}&qsvRateControl=${encodeURIComponent(qsvRateControl)}&qsvLookAheadDepth=${qsvLookAheadDepth}&qsvExtendedBRC=${qsvExtendedBRC}&qsvAdaptiveI=${qsvAdaptiveI}&qsvAdaptiveB=${qsvAdaptiveB}&qsvPStrategy=${qsvPStrategy}&mode=${mode}&previewNormalization=${encodeURIComponent(previewNormalization)}&ephemeral=${ephemeral}${subtitleStreamIndex === undefined ? '' : `&subtitleStreamIndex=${subtitleStreamIndex}`}`;
}

function compatiblePreviewRequestPath(requestId: string, resource: 'video' | 'frame' | 'inspect' | 'metrics' | 'start' | 'status' | 'cancel') {
  if (typeof requestId !== 'string' || !requestId.trim()) {
    throw new Error('Compatible Preview requestId is required before requesting a resource');
  }
  return `/api/assets/preview/compatible/requests/${encodeURIComponent(requestId.trim())}/${resource}`;
}

export const api = {
	trackMaintenanceInventory: (path: string) => request<import('./types').TrackMaintenanceInventory>(`/api/assets/track-maintenance/inventory?path=${encodeURIComponent(path)}`),
	startTrackRemoval: (input: { path: string; streamIndexes: number[]; expectedFingerprint: string; confirmed: boolean }) =>
		request<import('./types').AssetMaintenanceOperation>('/api/assets/track-maintenance/remove', { method: 'POST', body: JSON.stringify(input) }),
	editTrack: (input: { path: string; streamIndex: number; expectedFingerprint: string; title: string; language: string; default: boolean; forced: boolean }) =>
		request<import('./types').AssetMaintenanceOperation>('/api/assets/track-maintenance/edit', { method: 'POST', body: JSON.stringify(input) }),
	addAACTrack: (input: { path: string; sourceStreamIndex: number; expectedFingerprint: string; bitrateKbps: number; channels: 'source' | 'stereo'; title: string; language: string; default: boolean }) =>
		request<import('./types').AssetMaintenanceOperation>('/api/assets/track-maintenance/add-aac', { method: 'POST', body: JSON.stringify(input) }),
	maintenanceOperation: (id: string) => request<import('./types').AssetMaintenanceOperation>(`/api/assets/track-maintenance/operations/${encodeURIComponent(id)}`),
  libraries: () => request<Library[]>('/api/libraries'),
  logFiles: () => request<LogFile[]>('/api/logs/files'),
  logFile: (name: string) => request<LogFileContent>(`/api/logs/files/${encodeURIComponent(name)}`),
  browsePaths: (root: 'raw' | 'library' | 'staging') =>
    request<PathBrowseResponse>(`/api/paths/browse?root=${encodeURIComponent(root)}`),
  assets: () => request<AssetInventory>('/api/assets'),
	assetScopeConfigurations: () => request<import('./types').AssetScopeConfiguration[]>('/api/asset-scope-configurations'),
	effectiveAssetConfiguration: (path: string) => request<import('./types').EffectiveAssetConfiguration>(`/api/assets/effective-configuration?path=${encodeURIComponent(path)}`),
	effectiveAssetConfigurations: (assetIds: number[]) => request<import('./types').EffectiveAssetConfigurationBatchResponse>('/api/assets/effective-configurations', { method: 'POST', body: JSON.stringify({ assetIds }) }),
	updateAssetScopeConfiguration: (input: import('./types').AssetScopeConfigurationInput) =>
		request<import('./types').AssetScopeConfiguration>('/api/asset-scope-configurations', { method: 'POST', body: JSON.stringify(input) }),
	configureLogicalGroupsBatch: (input: import('./types').ConfigureLogicalGroupsBatchInput) =>
		request<import('./types').ConfigureLogicalGroupsBatchResponse>('/api/asset-scope-configurations/logical-groups/batch', { method: 'POST', body: JSON.stringify(input) }),
  syncAssets: () =>
    request<AssetSyncResult>('/api/assets/sync', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  removeMissingAsset: (path: string) =>
    request<{ status: string; path: string; message: string }>(`/api/assets/missing?path=${encodeURIComponent(path)}`, {
      method: 'DELETE',
    }),
  removeAllMissingAssets: () =>
    request<{ status: string; removed: number; preserved: number; message: string }>('/api/assets/missing/all', {
      method: 'DELETE',
    }),
  recoverAsset: (input: ArchivedOriginalMutationInput) =>
    request<{ status: string; sourcePath: string; recoveredPath: string; message: string }>(`/api/assets/recover?path=${encodeURIComponent(input.path)}`, {
      method: 'POST',
      body: JSON.stringify({ archiveAssetRecordId: input.archiveAssetRecordId, libraryAssetRecordId: input.libraryAssetRecordId }),
    }),
  deleteArchiveAsset: (input: ArchivedOriginalMutationInput) =>
    request<{ status: string; path: string; message: string }>(`/api/assets/archive?path=${encodeURIComponent(input.path)}`, {
      method: 'DELETE',
      body: JSON.stringify({ confirmed: true, archiveAssetRecordId: input.archiveAssetRecordId, libraryAssetRecordId: input.libraryAssetRecordId }),
    }),
  deleteArchiveAssets: (paths: string[]) =>
    request<{ status: string; completed: number; failures: Array<{ path: string; status: number; error: string }> }>('/api/assets/archive/batch', {
      method: 'DELETE',
      body: JSON.stringify({ paths, confirmed: true }),
    }),
  deleteConvertedAsset: (path: string) =>
    request<{ status: string; convertedPath: string; archivedOriginalPath: string; restoredPath: string; jobId: number; message: string }>(`/api/assets/delete-converted?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  returnPublishedAsIsAsset: (path: string) =>
    request<{ status: string; publishedPath: string; restoredPath: string; message: string }>(`/api/assets/return-published-as-is?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  acceptAssetAsIs: (path: string) =>
    request<Asset>(`/api/assets/accept-as-is?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  reconsiderAsset: (path: string) =>
    request<Asset>(`/api/assets/reconsider?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  extractAssetSubtitles: (
    input:
      | string
      | {
          path: string;
          streamIndex?: number;
          format?: 'original' | 'srt' | 'ass' | 'both';
          ocrLanguage?: string;
          ocrMode?: 'raw' | 'clean' | 'accurate';
        },
  ) => {
    const value = typeof input === 'string' ? { path: input } : input;

    return request<
      | SubtitleExtractionResult
      | {
          operationId: string;
          status: string;
          phase: string;
          progress: number;
          streamIndex: number;
          format: 'original' | 'srt' | 'ass' | 'both';
        }
    >(`/api/assets/extract-subtitles?path=${encodeURIComponent(value.path)}`, {
      method: 'POST',
      body: JSON.stringify({
        streamIndex: value.streamIndex,
        format: value.format,
        ocrLanguage: value.ocrLanguage,
        ocrMode: value.ocrMode,
      }),
    });
  },
  subtitleExtractionOperation: (operationId: string) =>
    request<SubtitleExtractionOperation>(
      `/api/assets/extract-subtitles/${encodeURIComponent(operationId)}`,
    ),

  cancelSubtitleExtractionOperation: (operationId: string) =>
    request<SubtitleExtractionOperation>(
      `/api/assets/extract-subtitles/${encodeURIComponent(operationId)}/cancel`,
      { method: 'POST' },
    ),

  subtitleExtractionOperations: (path: string) =>
    request<SubtitleExtractionOperationList>(
      `/api/assets/extract-subtitles?path=${encodeURIComponent(path)}`,
    ),

  subtitleOperation: (operationId: string) =>
    request<SubtitleExtractionOperation>(
      `/api/assets/subtitle-operations/${encodeURIComponent(operationId)}`,
    ),

  subtitleOperations: (path: string) =>
    request<SubtitleExtractionOperationList>(
      `/api/assets/subtitle-operations?path=${encodeURIComponent(path)}`,
    ),

  restyleSubtitle: (input: SubtitleRestyleInput) =>
    request<{ operationId: string; status: 'running'; phase: string; progress: number; format: string }>(
      '/api/assets/subtitle-restyle',
      { method: 'POST', body: JSON.stringify(input) },
    ),

  composeSubtitles: (input: SubtitleCompositionInput) =>
    request<{ operationId: string; status: 'running'; kind: 'compose'; phase: SubtitleExtractionOperation['phase']; progress: number; outputIdentity: string }>(
      '/api/assets/subtitle-compose',
      { method: 'POST', body: JSON.stringify(input) },
    ),

  cancelSubtitleOperation: (operationId: string) =>
    request<SubtitleExtractionOperation>(
      `/api/assets/subtitle-operations/${encodeURIComponent(operationId)}/cancel`,
      { method: 'POST' },
    ),

  retrySubtitleTranslation: (operationId: string) =>
    request<{ operation: SubtitleExtractionOperation }>(
      `/api/assets/subtitle-operations/${encodeURIComponent(operationId)}/retry`,
      { method: 'POST' },
    ),

  retrySubtitleOperation: (operationId: string) =>
    request<{ operation: SubtitleExtractionOperation }>(
      `/api/assets/subtitle-operations/${encodeURIComponent(operationId)}/retry`,
      { method: 'POST' },
    ),

  subtitleEligibility: (path: string) =>
    request<SubtitleEligibilityResponse>(
      `/api/assets/subtitle-eligibility?path=${encodeURIComponent(path)}`,
    ),

  subtitleTranslationCapability: (input: SubtitleTranslationCapabilityInput) =>
    request<SubtitleTranslationCapability>('/api/assets/subtitle-translation/capability', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  testSubtitleTranslationConnection: () =>
    request<SubtitleTranslationReadiness>('/api/settings/subtitle-translation/test-connection', {
      method: 'POST',
    }),

  enableSubtitleTranslation: () =>
    request<SubtitleTranslationReadiness>('/api/settings/subtitle-translation/enable', {
      method: 'POST',
    }),

  startSubtitleTranslation: (input: StartSubtitleTranslationInput) =>
    request<{ operation: SubtitleExtractionOperation }>('/api/assets/subtitle-translation', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  analyzeSubtitleOCR: (input: {
    mediaPath?: string;
    sidecarPath?: string;
    sidecarPaths?: string[];
    format: 'srt' | 'ass' | 'both';
    logicalTransformKey?: string;
    reviewTarget?: CanonicalOCRReviewTarget;
  }) =>
    request<{ operationId: string; status: string; phase: string; progress: number }>(
      '/api/assets/subtitle-ocr/analyze',
      { method: 'POST', body: JSON.stringify(input) },
    ),

  searchOCRDictionary: (input: {
    language?: OCRDictionaryLanguage;
    search?: string;
    source?: 'builtin' | 'user';
    enabled?: boolean;
    page?: number;
    pageSize?: number;
  } = {}) => {
    const query = new URLSearchParams();
    if (input.language) query.set('language', input.language);
    if (input.search?.trim()) query.set('search', input.search.trim());
    if (input.source) query.set('source', input.source);
    if (input.enabled !== undefined) query.set('enabled', String(input.enabled));
    if (input.page !== undefined) query.set('page', String(input.page));
    if (input.pageSize !== undefined) query.set('pageSize', String(input.pageSize));
    return request<OCRDictionarySearchResponse>(`/api/assets/ocr-dictionary?${query.toString()}`);
  },

  createOCRDictionaryEntry: (input: OCRDictionaryRuleInput) =>
    request<OCRDictionaryEntry>('/api/assets/ocr-dictionary', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  updateOCRDictionaryEntry: (id: string, input: OCRDictionaryRuleInput) =>
    request<OCRDictionaryEntry>(`/api/assets/ocr-dictionary/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),

  setOCRDictionaryEntryEnabled: (id: string, enabled: boolean) =>
    request<OCRDictionaryEntry>(`/api/assets/ocr-dictionary/${encodeURIComponent(id)}/enabled`, {
      method: 'PATCH',
      body: JSON.stringify({ enabled }),
    }),

  deleteOCRDictionaryEntry: (id: string) =>
    request<{ message?: string }>(`/api/assets/ocr-dictionary/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  testOCRDictionaryMatch: (input: OCRDictionaryTestMatchInput) =>
    request<OCRDictionaryTestMatchResponse>('/api/assets/ocr-dictionary/test-match', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  previewOCRDictionaryReevaluation: (input: OCRDictionaryReevaluationInput) =>
    request<OCRDictionaryReevaluationPreview>('/api/assets/subtitle-ocr/reevaluate/preview', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  applyOCRDictionaryReevaluation: (input: OCRDictionaryReevaluationApplyInput) =>
    request<{ operationId: string; status: string; phase: string; progress: number }>(
      '/api/assets/subtitle-ocr/reevaluate/apply',
      { method: 'POST', body: JSON.stringify(input) },
    ),

  correctSubtitleOCR: (input: {
    mediaPath?: string;
    sidecarPath?: string;
    sidecarPaths?: string[];
    format: 'srt' | 'ass' | 'both';
    logicalTransformKey?: string;
    baselineRevision: string;
    dictionaryLanguage?: OCRDictionaryLanguage;
    dictionaryRevision?: string;
    revisions?: Array<{
      artifactPath: string;
      artifactIdentity: string;
      sizeBytes: number;
      modifiedAtUnixNano: string;
      contentHash: string;
      logicalTransform?: string;
    }>;
    reviewTarget?: CanonicalOCRReviewTarget;
    corrections: Array<{
      cueId: number;
      action: 'keep' | 'pending' | 'approve' | 'apply' | 'edit' | 'reject';
      text?: string;
      suggestion?: string;
    }>;
  }) =>
    request<{ operationId: string; status: string; phase: string; progress: number }>(
      '/api/assets/subtitle-ocr/correct',
      { method: 'POST', body: JSON.stringify(input) },
    ),

  probeRemoteExecutor: (executor: RemoteExecutorConfig) =>
    request<RemoteExecutorProbe>('/api/remote-executors/probe', {
      method: 'POST',
      body: JSON.stringify(executor),
    }),

  externalAssetSubtitles: (path: string) =>
    request<ExternalSubtitle[]>(
      `/api/assets/external-subtitles?path=${encodeURIComponent(path)}`,
    ),
  assetMetadataSidecars: (path: string) =>
    request<import('./types').AssetMetadataSidecar[]>(
      `/api/assets/metadata-sidecars?path=${encodeURIComponent(path)}`,
    ),
  externalAssetSubtitleContent: (input: { path: string; subtitlePath: string }) =>
    request<{ path: string; content: string }>(`/api/assets/external-subtitles/content?path=${encodeURIComponent(input.path)}&subtitlePath=${encodeURIComponent(input.subtitlePath)}`),
  updateExternalAssetSubtitle: (input: { path: string; subtitlePath: string; content: string }) =>
    request<{ path: string; message: string }>(`/api/assets/external-subtitles?path=${encodeURIComponent(input.path)}`, {
      method: 'PUT',
      body: JSON.stringify({ subtitlePath: input.subtitlePath, content: input.content }),
    }),
  renameExternalAssetSubtitle: (input: { path: string; subtitlePath: string; fileName: string }) =>
    request<{ path: string; message: string }>(`/api/assets/external-subtitles/rename?path=${encodeURIComponent(input.path)}`, {
      method: 'POST',
      body: JSON.stringify({ subtitlePath: input.subtitlePath, fileName: input.fileName }),
    }),
  deleteExternalAssetSubtitle: (input: { path: string; subtitlePath: string }) =>
    request<{ path: string; message: string }>(`/api/assets/external-subtitles?path=${encodeURIComponent(input.path)}`, {
      method: 'DELETE',
      body: JSON.stringify({ subtitlePath: input.subtitlePath }),
    }),
  migrateAssetPath: (input: { sourcePath: string; destinationLibraryId: number }) =>
    request<{ status: string; sourcePath: string; destinationPath: string; sourceLibraryId: number; destinationLibraryId: number; assetsMoved: number }>('/api/assets/migrate-path', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  previewLibraryPathRename: (input: { libraryId: number; path: string }) =>
    request<LibraryRenamePlan>('/api/assets/library-rename/preview', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  applyLibraryPathRename: (input: { libraryId: number; path: string; planHash: string }) =>
    request<LibraryRenameApplyResponse>('/api/assets/library-rename/apply', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  publishAssetsAsIs: (input: { sourcePath: string; destinationLibraryId: number }) =>
    request<{ status: string; sourcePath: string; destinationPath: string; destinationLibraryId: number; assetsPublished: number; message: string }>('/api/assets/publish-as-is', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  confirmPublicationReconciliation: (input: { jobId: number; path: string }) =>
    request<{ status: string; jobId: number; oldPath: string; publishedPath: string }>('/api/assets/reconcile-publication', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateAssetReview: ({ path, ...review }: AssetReviewUpdateInput) =>
    request<{ path: string; review: unknown }>(`/api/assets/review?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify(review),
    }),
  updateAssetMetadata: ({ path, ...metadata }: AssetMetadataUpdateInput) =>
    request<{ path: string; metadata: unknown }>(`/api/assets/metadata?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify(metadata),
    }),
  updateAssetEdit: ({ path, ...edit }: AssetEditInput) =>
    request<AssetEditResponse>(`/api/assets/edit?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify(edit),
    }),
  renameAsset: (input: { path: string; fileName: string }) =>
    request<{ oldPath: string; path: string; fileName: string }>('/api/assets/rename', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateAssetConversion: ({ path, ...conversion }: AssetConversionUpdateInput) =>
    request<{ path: string; conversion: unknown }>(`/api/assets/conversion?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify(conversion),
    }),
  updateAssetTracks: ({ path, ...tracks }: AssetTracksUpdateInput) =>
    request<AssetTracksUpdateResponse>(`/api/assets/tracks?path=${encodeURIComponent(path)}`, {
      method: 'PATCH',
      body: JSON.stringify(tracks),
    }),
  testEncodes: (sourcePath?: string) => request<TestEncode[]>(`/api/test-encodes${sourcePath ? `?sourcePath=${encodeURIComponent(sourcePath)}` : ''}`),
  testEncodeInventory: (filters: Record<string, string | number | undefined> = {}) => {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value !== undefined && String(value).trim() !== '') query.set(key, String(value));
    }
    return request<TestEncode[]>(`/api/test-encodes${query.size ? `?${query.toString()}` : ''}`);
  },
  testEncodeInventorySummary: () => request<TestEncodeInventorySummary>('/api/test-encodes/summary'),
  reconcileTestEncodes: (ids: number[] = []) => request<TestEncodeReconcileResponse>('/api/test-encodes/reconcile', { method: 'POST', body: JSON.stringify({ ids }) }),
  orphanedTestEncodeArtifacts: () => request<{ artifacts: TestEncodeOrphanArtifact[]; limit: number }>('/api/test-encodes/orphans'),
  deleteOrphanedTestEncodeArtifact: (artifactId: number) => request<{ id: number; deleted: boolean }>(`/api/test-encodes/orphans/${artifactId}`, { method: 'DELETE' }),
  previewTestEncodeDelete: (ids: number[]) => request<TestEncodeDeletePreview>('/api/test-encodes/delete-preview', { method: 'POST', body: JSON.stringify({ ids }) }),
  deleteTestEncodes: (ids: number[], confirmKeep = false) => request<TestEncodeDeleteSelectionResult>('/api/test-encodes/delete', { method: 'POST', body: JSON.stringify({ ids, confirmKeep }) }),
  testEncode: (id: number) => request<TestEncode>(`/api/test-encodes/${id}`),
  evaluateTestEncodeProfileQualification: (id: number) => request<NonNullable<TestEncode['profileQualification']>>(`/api/test-encodes/${id}/profile-qualification/evaluate`, { method: 'POST', body: JSON.stringify({}) }),
  createTestEncode: (input: TestEncodeInput) => request<TestEncode>('/api/test-encodes', { method: 'POST', body: JSON.stringify(input) }),
  cancelTestEncode: (id: number) => request<{ id: number; status: string }>(`/api/test-encodes/${id}/cancel`, { method: 'POST', body: JSON.stringify({}) }),
  keepTestEncode: (id: number, keep: boolean) => request<TestEncode>(`/api/test-encodes/${id}/keep`, { method: 'POST', body: JSON.stringify({ keep }) }),
  deleteTestEncode: (id: number, confirmKeep = false) => request<{ id: number; status: string }>(`/api/test-encodes/${id}${confirmKeep ? '?confirmKeep=true' : ''}`, { method: 'DELETE' }),
  evaluateAdvisor: (advisorRequest: AdvisorRequest) =>
    request<AdvisorResponse>('/api/advisor/evaluate', {
      method: 'POST',
      body: JSON.stringify(advisorRequest),
    }),
  suggestProfile: (mediaPath: string) =>
    request<ProfileSuggestion>('/api/advisor/suggest', {
      method: 'POST',
      body: JSON.stringify({ mediaPath }),
    }),
  createLibrary: (library: LibraryInput) =>
    request<Library>('/api/libraries', {
      method: 'POST',
      body: JSON.stringify(library),
    }),
  updateLibrary: ({ id, ...library }: UpdateLibraryInput) =>
    request<Library>(`/api/libraries/${id}`, {
      method: 'POST',
      body: JSON.stringify(library),
    }),
  profiles: () => request<Profile[]>('/api/profiles'),
  profilesAdmin: () => request<Profile[]>('/api/profiles?includeDisabled=true&includeDeleted=true'),
  profileTemplates: () => request<ProfileTemplate[]>('/api/profile-templates'),
  cloneProfileTemplate: ({ key, ...input }: CloneProfileTemplateInput) =>
    request<Profile>(`/api/profile-templates/${encodeURIComponent(key)}/clone`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  createProfile: (profile: ProfileInput) =>
    request<Profile>('/api/profiles', {
      method: 'POST',
      body: JSON.stringify(profile),
    }),
  updateProfile: ({ id, ...profile }: UpdateProfileInput) =>
    request<Profile>(`/api/profiles/${id}`, {
      method: 'POST',
      body: JSON.stringify(profile),
    }),
  setProfileDisabled: ({ id, disabled }: { id: number; disabled: boolean }) =>
    request<Profile>(`/api/profiles/${id}/disabled`, {
      method: 'POST',
      body: JSON.stringify({ disabled }),
    }),
  deleteProfile: (id: number) =>
    request<{ status: string; id: number }>(`/api/profiles/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({}),
    }),
  profileAssignments: () => request<ProfileAssignment[]>('/api/profile-assignments'),
  updateProfileAssignment: (assignment: ProfileAssignmentInput) =>
    request<ProfileAssignment | { status: string }>('/api/profile-assignments', {
      method: 'POST',
      body: JSON.stringify(assignment),
    }),
  resolveTrackProfilePreview: (input: { assetPath: string; profile: Record<string, unknown> }) =>
    request<TrackProfileResolutionPreview>('/api/track-profiles/resolve-preview', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  queueJobs: () => request<QueueJob[]>('/api/queue/jobs'),
  publisherPublications: () => request<{ items: PublisherPublicationSummary[]; limit: number }>('/api/publisher/publications'),
  publisherEligibility: (limit = 100) => request<PublisherEligibilityResponse>(`/api/publisher/eligibility?limit=${limit}`),
  publisherAction: ({ publicationId, action, reason }: { publicationId: string; action: 'hold' | 'release' | 'retry'; reason: string }) =>
    request<PublishResult | { publicationId: string; state: string }>(`/api/publisher/publications/${encodeURIComponent(publicationId)}/actions`, {
      method: 'POST',
      body: JSON.stringify({ action, reason }),
    }),
  createQueueJob: (job: QueueJobInput) =>
    request<QueueJob>('/api/queue/jobs', {
      method: 'POST',
      body: JSON.stringify(job),
    }),
  createQueueBatch: (batch: QueueBatchInput) =>
  request<QueueBatchResponse>('/api/queue/batches', {
    method: 'POST',
    body: JSON.stringify(batch),
  }),
  createQueueBatchPreflight: (input: QueueBatchPreflightInput) =>
    request<QueueBatchPreflightResponse>('/api/queue/preflights', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  preflightSelectedAssets: (assetIds: number[]) =>
    request<QueueBatchPreflightResponse>('/api/queue/preflights/selected-assets', {
      method: 'POST',
      body: JSON.stringify({ assetIds }),
    }),
  getQueueBatchPreflight: (id: string) =>
    request<QueueBatchPreflightResponse>(`/api/queue/preflights/${encodeURIComponent(id)}`),
  editQueueBatchPreflight: (id: string, revision: number) =>
    request<QueueBatchPreflightResponse>(`/api/queue/preflights/${encodeURIComponent(id)}/edit`, {
      method: 'POST',
      body: JSON.stringify({ revision }),
    }),
  updateQueueBatchPreflight: (id: string, input: QueueBatchPreflightInput) =>
    request<QueueBatchPreflightResponse>(`/api/queue/preflights/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  acceptQueueBatchPreflight: (id: string, revision: number, assetIds: string[]) =>
    request<QueueBatchPreflightResponse>(`/api/queue/preflights/${encodeURIComponent(id)}/accept`, {
      method: 'POST',
      body: JSON.stringify({ revision, assetIds }),
    }),
  removeQueueBatchPreflightAsset: (id: string, revision: number, assetId: string) =>
    request<QueueBatchPreflightResponse>(`/api/queue/preflights/${encodeURIComponent(id)}/remove`, {
      method: 'POST',
      body: JSON.stringify({ revision, assetId }),
    }),
  discardQueueBatchPreflight: (id: string, revision: number) =>
    request<QueueBatchPreflightResponse>(`/api/queue/preflights/${encodeURIComponent(id)}/discard`, {
      method: 'POST',
      body: JSON.stringify({ revision }),
    }),
  queueSelectedAssets: (input: QueueSelectedAssetsInput) =>
    request<QueueSelectedAssetsResponse>('/api/queue/selected-assets', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  queueRemainingPreview: (assetIds: number[]) =>
    request<QueueSelectedAssetsResponse>('/api/assets/queue-remaining/preview', {
      method: 'POST',
      body: JSON.stringify({ assetIds, commit: false }),
    }),
  queueRemainingCommit: (assetIds: number[]) =>
    request<QueueSelectedAssetsResponse>('/api/assets/queue-remaining/commit', {
      method: 'POST',
      body: JSON.stringify({ assetIds, commit: true }),
    }),
  updateQueueJob: ({ jobId, ...job }: QueueJobUpdateInput) =>
    request<QueueJob>(`/api/queue/jobs/${jobId}`, {
      method: 'POST',
      body: JSON.stringify(job),
    }),
  reorderQueueJob: (input: QueueReorderInput) =>
    request<{ jobs: QueueJob[] }>('/api/queue/jobs/reorder', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  dismissQueueJob: (jobId: number) =>
    request<QueueJob>(`/api/queue/jobs/${jobId}`, {
      method: 'DELETE',
    }),
  dismissQueueBatch: (batchId: string) =>
    request<{ batchId: string; dismissedJobs: number; cleanedExecutionPlans: number }>(`/api/queue/batches/${encodeURIComponent(batchId)}`, {
      method: 'DELETE',
    }),
  jobArtifacts: (jobId: number) => request<JobArtifactsResponse>(`/api/queue/jobs/${jobId}/artifacts`),
  jobLog: (jobId: number, tail = 1000) => request<JobLogResponse>(`/api/queue/jobs/${jobId}/log?tail=${tail}`),
  executionPlans: (jobId: number) => request<ExecutionPlan[]>(`/api/queue/jobs/${jobId}/execution-plans`),
  reviewExecutionPlan: ({ jobId, planId, approve }: { jobId: number; planId: number; approve: boolean }) =>
    request<ExecutionPlan>(`/api/queue/jobs/${jobId}/execution-plans/${planId}/${approve ? 'approve' : 'reject'}`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  backfillAnalysisAsIsReports: () =>
    request<AnalysisBackfillResponse>('/api/analysis/backfill-as-is', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  claimQueueJob: (claim: ClaimJobInput) =>
    request<QueueJob>('/api/workers/claim', {
      method: 'POST',
      body: JSON.stringify(claim),
    }),
  workerNodes: () => request<WorkerNode[]>('/api/workers/nodes'),
  schedulerRecovery: () => request<SchedulerRecoveryReport>('/api/system/scheduler-recovery'),
  runSchedulerRecovery: () => request<SchedulerRecoveryReport>('/api/system/scheduler-recovery/run', { method: 'POST', body: JSON.stringify({}) }),
  previewHousekeeping: () => request<HousekeepingReport>('/api/system/housekeeping/preview'),
  runHousekeeping: (input: { previewId: string }) => request<HousekeepingReport>('/api/system/housekeeping/run', { method: 'POST', body: JSON.stringify(input) }),
  updateQueueJobStatus: ({ jobId, ...statusUpdate }: UpdateJobStatusInput) =>
    request<QueueJob>(`/api/workers/jobs/${jobId}/status`, {
      method: 'POST',
      body: JSON.stringify(statusUpdate),
    }),
  dryRunQueueJob: (jobId: number) =>
    request<QueueJob>(`/api/workers/jobs/${jobId}/dry-run`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  executeQueueJob: ({ jobId, overwrite = false }: ExecuteJobInput) =>
    request<QueueJob>(`/api/workers/jobs/${jobId}/execute`, {
      method: 'POST',
      body: JSON.stringify({ overwrite }),
    }),
  validateJob: (jobId: number) =>
    request<ValidationResult>(`/api/validation/jobs/${jobId}`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  validationRun: ({ jobId, runId }: { jobId: number; runId: string }) =>
    request<ValidationRunDetail>(`/api/validation/jobs/${jobId}/runs/${encodeURIComponent(runId)}`),
  createValidationOverride: ({ jobId, scope, reason, ruleKeys, requestedHandoff }: { jobId: number; scope: 'rule' | 'all_overrideable'; reason?: string; ruleKeys: string[]; requestedHandoff: 'publisher' | 'automatic' }) =>
    request<ValidationOverride>(`/api/validation/jobs/${jobId}/overrides`, {
      method: 'POST',
      body: JSON.stringify({ scope, reason: reason ?? '', ruleKeys, requestedHandoff }),
    }),
  validationReview: ({ view, limit, offset }: { view: 'exceptions' | 'pending' | 'all'; limit: number; offset: number }) =>
    request<ValidationSummaryPage>(`/api/validation/review?view=${view}&limit=${limit}&offset=${offset}`),
  validationHistory: ({ q, sort, limit, offset }: { q: string; sort: 'asc' | 'desc'; limit: number; offset: number }) =>
    request<ValidationHistoryPage>(`/api/validation/history?q=${encodeURIComponent(q)}&sort=${sort}&limit=${limit}&offset=${offset}`),
  publishJob: ({ jobId, reason }: { jobId: number; reason?: string }) =>
    request<PublishResult>(`/api/publisher/jobs/${jobId}/publish`, {
      method: 'POST',
      body: JSON.stringify({ reason }),
    }),
  discardPublisherJob: (jobId: number) =>
    request<{
      jobId: number;
      discarded: boolean;
      originalRecovery: string;
    }>(`/api/publisher/jobs/${jobId}`, {
      method: 'DELETE',
    }),
  settings: () => request<AppSetting[]>('/api/settings'),
  testJellyfinConnection: () => request<import('./types').JellyfinConnectionResult>('/api/settings/jellyfin/test-connection', { method: 'POST', body: JSON.stringify({}) }),
  jellyfinIntegrationState: () => request<import('./types').JellyfinIntegrationState>('/api/settings/jellyfin/state'),
  jellyfinOutcomes: () => request<import('./types').JellyfinIntegrationOutcome[]>('/api/jellyfin/outcomes'),
  manualJellyfinRefresh: (libraryId: number) => request<import('./types').JellyfinIntegrationOutcome>('/api/jellyfin/manual-refresh', { method: 'POST', body: JSON.stringify({ libraryId }) }),
  retryJellyfinOutcome: (id: number) => request<import('./types').JellyfinIntegrationOutcome>(`/api/jellyfin/outcomes/${id}/retry`, { method: 'POST', body: JSON.stringify({}) }),
  softwareVersions: () => request<SoftwareVersions>('/api/system/versions'),
  runtimeState: () => request<RuntimeState>('/api/system/runtime'),
  /** @deprecated Use runtimeState for the live current-state endpoint. */
  runtimeSnapshot: () => request<RuntimeState>('/api/system/runtime'),
  runtimeProfiles: () => request<RuntimeProfilesResponse>('/api/system/runtime/profiles'),
  refreshRuntime: () => request<RuntimeState>('/api/system/runtime/refresh', { method: 'POST', body: JSON.stringify({}) }),
  /** @deprecated Use refreshRuntime for the live current-state endpoint. */
  refreshRuntimeSnapshot: () => request<RuntimeState>('/api/system/runtime/refresh', { method: 'POST', body: JSON.stringify({}) }),
  updateSetting: ({ key, value }: UpdateSettingInput) =>
    request<AppSetting>(`/api/settings/${key}`, {
      method: 'POST',
      body: JSON.stringify({ value }),
    }),
  updateSettingsBatch: ({ settings }: UpdateSettingsBatchInput) =>
    request<AppSetting[]>('/api/settings/batch', {
      method: 'POST',
      body: JSON.stringify({ settings }),
    }),
  importMWP: () =>
    request<MWPImportSummary>('/api/import/mwp', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  scan: ({ path, force = false, analysisSeconds = 20 }: { path: string; force?: boolean; analysisSeconds?: 10 | 20 }) =>
    request<ScanResult>('/api/scan', {
      method: 'POST',
      body: JSON.stringify({ path, force, analysisSeconds }),
  }),
  latestSnapshot: (path: string) =>
    request<{
      found: boolean;
      snapshot: ScanResult | null;
      status: 'current' | 'missing' | 'changed' | 'stale' | 'legacy' | 'unavailable';
      requiresAnalysis: boolean;
      staleComponents: string[];
      analysisEvidence?: import('./types').AssetAnalysisEvidence;
    }>(`/api/scan?path=${encodeURIComponent(path)}`),
  startSnapshotOperation: ({ path, force = false, analysisSeconds = 20, deepCadence = false }: { path: string; force?: boolean; analysisSeconds?: 10 | 20; deepCadence?: boolean }) =>
    request<SnapshotOperation>('/api/scan/operations', {
      method: 'POST',
      body: JSON.stringify({ path, force, analysisSeconds, deepCadence }),
    }),
  snapshotOperation: (operationId: string) =>
    request<SnapshotOperation>(`/api/scan/operations/${encodeURIComponent(operationId)}`),
  cancelSnapshotOperation: (operationId: string) =>
    request<SnapshotOperation>(`/api/scan/operations/${encodeURIComponent(operationId)}/cancel`, { method: 'POST', body: JSON.stringify({}) }),
  snapshotOperations: (path: string) =>
    request<{ operations: SnapshotOperation[] }>(`/api/scan/operations?path=${encodeURIComponent(path)}`),
  assetPreviewUrl: (path: string) => `${API_BASE_URL}/api/assets/preview?path=${encodeURIComponent(path)}`,
  assetThumbnailUrl: (assetId: number) => `${API_BASE_URL}/api/assets/${encodeURIComponent(assetId)}/thumbnail`,
  createCompatiblePreviewRequest: async (options: CompatiblePreviewOptions, signal?: AbortSignal) => {
    const created = await request<CompatiblePreviewRequest>('/api/assets/preview/compatible/requests', {
      method: 'POST',
      body: JSON.stringify(options),
      signal,
    });
    if (typeof created.requestId !== 'string' || !created.requestId.trim()) {
      throw new Error('Compatible Preview request creation returned an empty requestId');
    }
    return { ...created, requestId: created.requestId.trim() };
  },
  startCompatiblePreview: (requestId: string, ownerId: string, leaseToken?: string, signal?: AbortSignal) =>
    request<CompatiblePreviewLifecycleResponse>(compatiblePreviewRequestPath(requestId, 'start'), {
      method: 'POST',
      body: JSON.stringify({ ownerId, ...(leaseToken ? { leaseToken } : {}) }),
      signal,
    }),
  compatiblePreviewStatus: (requestId: string, signal?: AbortSignal) =>
    request<CompatiblePreviewLifecycleResponse>(compatiblePreviewRequestPath(requestId, 'status'), { signal }),
  cancelCompatiblePreview: (requestId: string, ownerId: string, leaseToken: string, signal?: AbortSignal) =>
    request<CompatiblePreviewLifecycleResponse>(compatiblePreviewRequestPath(requestId, 'cancel'), {
      method: 'POST',
      body: JSON.stringify({ ownerId, leaseToken }),
      signal,
    }),
  compatibleAssetPreviewUrl: (request: string | CompatiblePreviewOptions) =>
    `${API_BASE_URL}${typeof request === 'string' ? compatiblePreviewRequestPath(request, 'video') : compatiblePreviewPath(request)}`,
  compatibleAssetFrameUrl: (requestId: string, frame: 'source' | 'output') =>
    `${API_BASE_URL}${compatiblePreviewRequestPath(requestId, 'frame')}?frame=${frame}`,
  compatibleAssetFrameMetrics: (requestId: string, signal?: AbortSignal) =>
    request<PreviewFrameMetrics>(compatiblePreviewRequestPath(requestId, 'metrics'), { signal }),
  inspectCompatibleAssetPreview: (requestId: string, signal?: AbortSignal) =>
    request<PreviewInspection>(compatiblePreviewRequestPath(requestId, 'inspect'), { signal }),
  estimateCompatibleAssetProfile: (input: { path: string; profileId?: number; profile: ProfileInput; seconds?: number }, signal?: AbortSignal) =>
    request<ProfileSampleEstimate>('/api/assets/preview/estimate', { method: 'POST', body: JSON.stringify(input), signal }),
  startProfileSampleEstimateOperation: (input: { path: string; profileId?: number; profile: ProfileInput; seconds?: number }) =>
    request<ProfileSampleEstimateOperation>('/api/assets/preview/estimate/operations', { method: 'POST', body: JSON.stringify(input) }),
  profileSampleEstimateOperation: (operationId: string) =>
    request<ProfileSampleEstimateOperation>(`/api/assets/preview/estimate/operations/${encodeURIComponent(operationId)}`),
  cancelProfileSampleEstimateOperation: (operationId: string) =>
    request<ProfileSampleEstimateOperation>(`/api/assets/preview/estimate/operations/${encodeURIComponent(operationId)}/cancel`, { method: 'POST', body: JSON.stringify({}) }),
  startLabFidelityOperation: (input: { primaryRequestId: string; referenceRequestId?: string; durationSeconds?: number; clientCorrelationSignature?: string; materialSignature?: string; draftSignature?: string; qsvTemporalExperiment?: QSVTemporalExperimentRequest }, signal?: AbortSignal) =>
    request<LabFidelityOperation>('/api/lab/fidelity/operations', { method: 'POST', body: JSON.stringify(input), signal }),
  labFidelityOperation: (operationId: string, signal?: AbortSignal) =>
    request<LabFidelityOperation>(`/api/lab/fidelity/operations/${encodeURIComponent(operationId)}`, { signal }),
  cancelLabFidelityOperation: (operationId: string, signal?: AbortSignal) =>
    request<LabFidelityOperation>(`/api/lab/fidelity/operations/${encodeURIComponent(operationId)}/cancel`, { method: 'POST', body: JSON.stringify({}), signal }),
  recommendEncoderQuality: ({ signal, ...input }: { path?: string; profile: ProfileInput; signal?: AbortSignal }) =>
    request<QualityRecommendationResponse>('/api/assets/quality-recommendation', { method: 'POST', body: JSON.stringify(input), signal }),
  audioPreviewUrl: ({
    path,
    profileKey = '',
    start = '00:00:00',
    seconds = 20,
    filters = '',
    compatibility = false,
    streamIndex,
  }: {
    path: string;
    profileKey?: string;
    start?: string;
    seconds?: number;
    filters?: string;
    compatibility?: boolean;
    streamIndex?: number;
  }) =>
    `${API_BASE_URL}/api/assets/preview/audio?path=${encodeURIComponent(path)}&profileKey=${encodeURIComponent(profileKey)}&start=${encodeURIComponent(start)}&seconds=${seconds}&filters=${encodeURIComponent(filters)}&compatibility=${compatibility}${streamIndex === undefined ? '' : `&streamIndex=${streamIndex}`}`,
};

export function createCompatiblePreviewOwnerId() {
  const randomUUID = globalThis.crypto?.randomUUID;
  if (typeof randomUUID === 'function') return randomUUID.call(globalThis.crypto);
  return `preview-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function abortableCompatiblePreviewDelay(milliseconds: number, signal?: AbortSignal) {
  if (signal?.aborted) return Promise.reject(new DOMException('The operation was aborted.', 'AbortError'));
  return new Promise<void>((resolve, reject) => {
    const timer = globalThis.setTimeout(resolve, milliseconds);
    const onAbort = () => {
      globalThis.clearTimeout(timer);
      reject(new DOMException('The operation was aborted.', 'AbortError'));
    };
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}

export async function waitForCompatiblePreviewReady(requestId: string, signal?: AbortSignal, ownerId = createCompatiblePreviewOwnerId()) {
  let leaseToken = '';
  let response: CompatiblePreviewLifecycleResponse;
  try {
    let startDelay = 1_000;
    for (let attempt = 0; ; attempt += 1) {
      try {
        response = await api.startCompatiblePreview(requestId, ownerId, undefined, signal);
        break;
      } catch (error) {
        if (signal?.aborted || error instanceof ApiRequestError || attempt >= 3) throw error;
        await abortableCompatiblePreviewDelay(startDelay, signal);
        startDelay = Math.min(startDelay * 2, 15_000);
      }
    }
    leaseToken = response.leaseToken ?? '';
    if (response.state === 'ready') return response;
    let delay = 1_000;
    let nextRenewal = Date.now() + 15_000;
    for (;;) {
      const status = await api.compatiblePreviewStatus(requestId, signal).catch(async (error: unknown) => {
        if (error instanceof ApiRequestError && ['failed', 'canceled', 'interrupted', 'stale_artifact', 'expired', 'unknown_request'].includes(error.code ?? error.state ?? '')) throw error;
        await abortableCompatiblePreviewDelay(Math.min(delay, 15_000), signal);
        return null;
      });
      if (status) {
        response = status;
        if (status.state === 'ready') return status;
        if (['failed', 'canceled', 'interrupted', 'stale', 'expired', 'unknown'].includes(status.state)) {
          throw new ApiRequestError(status.reason ?? `Compatible Preview is ${status.state}`, status.state === 'expired' ? 410 : status.state === 'unknown' ? 404 : 409, {
            code: status.code,
            state: status.state,
            reason: status.reason,
            retryable: status.retryable,
            recreate: status.recreate,
          });
        }
      }
      if (leaseToken && Date.now() >= nextRenewal) {
        try {
          const renewed = await api.startCompatiblePreview(requestId, ownerId, leaseToken, signal);
          leaseToken = renewed.leaseToken ?? leaseToken;
          nextRenewal = Date.now() + 15_000;
        } catch (error) {
          if (error instanceof ApiRequestError && ['invalid_lease', 'lease_expired'].includes(error.code ?? error.state ?? '')) {
            const afterRenewal = await api.compatiblePreviewStatus(requestId, signal).catch(() => undefined);
            if (afterRenewal?.state === 'ready') return afterRenewal;
            if (afterRenewal && ['failed', 'canceled', 'interrupted', 'stale', 'expired', 'unknown'].includes(afterRenewal.state)) {
              throw new ApiRequestError(afterRenewal.reason ?? `Compatible Preview is ${afterRenewal.state}`, afterRenewal.state === 'expired' ? 410 : afterRenewal.state === 'unknown' ? 404 : 409, {
                code: afterRenewal.code,
                state: afterRenewal.state,
                reason: afterRenewal.reason,
                retryable: afterRenewal.retryable,
                recreate: afterRenewal.recreate,
              });
            }
            throw error;
          }
          if (error instanceof ApiRequestError && ['expired', 'unknown_request'].includes(error.code ?? error.state ?? '')) throw error;
          nextRenewal = Date.now() + Math.min(delay, 15_000);
        }
      }
      await abortableCompatiblePreviewDelay(delay, signal);
      delay = Math.min(delay * 2, 5_000);
    }
  } catch (error) {
    if (leaseToken && (signal?.aborted || error instanceof DOMException && error.name === 'AbortError')) {
      await api.cancelCompatiblePreview(requestId, ownerId, leaseToken).catch(() => undefined);
    }
    throw error;
  }
}
