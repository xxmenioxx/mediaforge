import type {
  Library,
  LibraryInput,
  LogFile,
  LogFileContent,
  MWPImportSummary,
  AnalysisBackfillResponse,
  AppSetting,
  AssetReviewUpdateInput,
  AssetMetadataUpdateInput,
  AssetConversionUpdateInput,
  AssetInventory,
  AssetSyncResult,
  AdvisorRequest,
  AdvisorResponse,
  ProfileSuggestion,
  ClaimJobInput,
  ExecuteJobInput,
  ExecutionPlan,
  ExternalSubtitle,
  JobArtifactsResponse,
  Profile,
  ProfileInput,
  PathBrowseResponse,
  PublishResult,
  QueueJob,
  QueueJobInput,
  QueueJobUpdateInput,
  RuntimeSnapshot,
  RuntimeProfilesResponse,
  ScanResult,
  CompatiblePreviewOptions,
  ProfileSampleEstimate,
  QualityRecommendationResponse,
  PreviewInspection,
  PreviewFrameMetrics,
  SoftwareVersions,
  UpdateLibraryInput,
  UpdateProfileInput,
  UpdateSettingInput,
  UpdateJobStatusInput,
  ValidationResult,
  WorkerNode,
  SchedulerRecoveryReport,
  HousekeepingReport,
  SubtitleExtractionOperation,
  SubtitleExtractionOperationList,
  SubtitleExtractionResult,
} from './types';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '';

export class ApiRequestError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
    this.name = 'ApiRequestError';
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
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
      }
    } catch {
      const text = await response.text().catch(() => '');
      if (text.trim()) {
        message = text.trim();
      }
    }
    throw new ApiRequestError(message, response.status);
  }

  const contentType = response.headers.get('content-type')?.toLowerCase() ?? '';
  if (!contentType.includes('application/json')) {
    throw new Error(
      `Expected a JSON API response but received ${contentType || 'an unknown content type'}. The MVForge frontend and backend may be running different versions.`,
    );
  }
  return response.json() as Promise<T>;
}

function compatiblePreviewPath({
  path,
  profileId = 0,
  profile,
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
  mode = 'quick',
  previewNormalization = 'normalize_bt709',
  subtitleStreamIndex,
}: CompatiblePreviewOptions) {
  return `/api/assets/preview/compatible?path=${encodeURIComponent(path)}&profileId=${profileId}&start=${encodeURIComponent(start)}&seconds=${seconds}&videoCodec=${encodeURIComponent(videoCodec)}&qualityValue=${qualityValue}&videoPreset=${encodeURIComponent(videoPreset)}&pixFmt=${encodeURIComponent(pixFmt)}&videoFilters=${encodeURIComponent(videoFilters)}&x265Params=${encodeURIComponent(x265Params)}&videoEncoder=${encodeURIComponent(videoEncoder)}&useHardwareIfAvailable=${useHardwareIfAvailable}&globalQuality=${globalQuality}&qsvRateControl=${encodeURIComponent(qsvRateControl)}&qsvLookAheadDepth=${qsvLookAheadDepth}&qsvExtendedBRC=${qsvExtendedBRC}&qsvAdaptiveI=${qsvAdaptiveI}&qsvAdaptiveB=${qsvAdaptiveB}&mode=${mode}&previewNormalization=${encodeURIComponent(previewNormalization)}${subtitleStreamIndex === undefined ? '' : `&subtitleStreamIndex=${subtitleStreamIndex}`}${profile === undefined ? '' : `&profile=${encodeURIComponent(JSON.stringify(profile))}`}`;
}

export const api = {
  libraries: () => request<Library[]>('/api/libraries'),
  logFiles: () => request<LogFile[]>('/api/logs/files'),
  logFile: (name: string) => request<LogFileContent>(`/api/logs/files/${encodeURIComponent(name)}`),
  browsePaths: (root: 'raw' | 'library' | 'staging') =>
    request<PathBrowseResponse>(`/api/paths/browse?root=${encodeURIComponent(root)}`),
  assets: () => request<AssetInventory>('/api/assets'),
  syncAssets: () =>
    request<AssetSyncResult>('/api/assets/sync', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  recoverAsset: (path: string) =>
    request<{ status: string; sourcePath: string; recoveredPath: string; message: string }>(`/api/assets/recover?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify({}),
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
  extractAssetSubtitles: (
    input:
      | string
      | {
          path: string;
          streamIndex?: number;
          format?: 'srt' | 'ass';
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
          format: 'srt' | 'ass';
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

  subtitleExtractionOperations: (path: string) =>
    request<SubtitleExtractionOperationList>(
      `/api/assets/extract-subtitles?path=${encodeURIComponent(path)}`,
    ),

  externalAssetSubtitles: (path: string) =>
    request<ExternalSubtitle[]>(
      `/api/assets/external-subtitles?path=${encodeURIComponent(path)}`,
    ),
  externalAssetSubtitleContent: (input: { path: string; subtitlePath: string }) =>
    request<{ path: string; content: string }>(`/api/assets/external-subtitles/content?path=${encodeURIComponent(input.path)}&subtitlePath=${encodeURIComponent(input.subtitlePath)}`),
  updateExternalAssetSubtitle: (input: { path: string; subtitlePath: string; content: string }) =>
    request<{ path: string; message: string }>(`/api/assets/external-subtitles?path=${encodeURIComponent(input.path)}`, {
      method: 'PUT',
      body: JSON.stringify({ subtitlePath: input.subtitlePath, content: input.content }),
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
  updateAssetConversion: ({ path, ...conversion }: AssetConversionUpdateInput) =>
    request<{ path: string; conversion: unknown }>(`/api/assets/conversion?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      body: JSON.stringify(conversion),
    }),
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
  queueJobs: () => request<QueueJob[]>('/api/queue/jobs'),
  createQueueJob: (job: QueueJobInput) =>
    request<QueueJob>('/api/queue/jobs', {
      method: 'POST',
      body: JSON.stringify(job),
    }),
  updateQueueJob: ({ jobId, ...job }: QueueJobUpdateInput) =>
    request<QueueJob>(`/api/queue/jobs/${jobId}`, {
      method: 'POST',
      body: JSON.stringify(job),
    }),
  dismissQueueJob: (jobId: number) =>
    request<QueueJob>(`/api/queue/jobs/${jobId}`, {
      method: 'DELETE',
    }),
  dismissQueueBatch: (batchId: string) =>
    request<{ batchId: string; removedPlaceholders: number; dismissedJobs: number; preservedCompleted: number }>(`/api/queue/batches/${encodeURIComponent(batchId)}`, {
      method: 'DELETE',
    }),
  jobArtifacts: (jobId: number) => request<JobArtifactsResponse>(`/api/queue/jobs/${jobId}/artifacts`),
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
  runHousekeeping: () => request<HousekeepingReport>('/api/system/housekeeping/run', { method: 'POST', body: JSON.stringify({}) }),
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
  publishJob: ({ jobId, overwrite = false }: { jobId: number; overwrite?: boolean }) =>
    request<PublishResult>(`/api/publisher/jobs/${jobId}/publish`, {
      method: 'POST',
      body: JSON.stringify({ overwrite }),
    }),
  settings: () => request<AppSetting[]>('/api/settings'),
  softwareVersions: () => request<SoftwareVersions>('/api/system/versions'),
  runtimeSnapshot: () => request<RuntimeSnapshot>('/api/system/runtime'),
  runtimeProfiles: () => request<RuntimeProfilesResponse>('/api/system/runtime/profiles'),
  refreshRuntimeSnapshot: () => request<RuntimeSnapshot>('/api/system/runtime/refresh', { method: 'POST', body: JSON.stringify({}) }),
  updateSetting: ({ key, value }: UpdateSettingInput) =>
    request<AppSetting>(`/api/settings/${key}`, {
      method: 'POST',
      body: JSON.stringify({ value }),
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
  assetPreviewUrl: (path: string) => `${API_BASE_URL}/api/assets/preview?path=${encodeURIComponent(path)}`,
  compatibleAssetPreviewUrl: (options: CompatiblePreviewOptions) =>
    `${API_BASE_URL}${compatiblePreviewPath(options)}`,
  compatibleAssetFrameUrl: (options: CompatiblePreviewOptions, frame: 'source' | 'output') =>
    `${API_BASE_URL}${compatiblePreviewPath(options)}&frame=${frame}`,
  compatibleAssetFrameMetrics: (options: CompatiblePreviewOptions) =>
    request<PreviewFrameMetrics>(compatiblePreviewPath(options).replace('/assets/preview/compatible', '/assets/preview/metrics')),
  inspectCompatibleAssetPreview: (options: CompatiblePreviewOptions) =>
    request<PreviewInspection>(compatiblePreviewPath(options).replace('/assets/preview/compatible', '/assets/preview/inspect')),
  estimateCompatibleAssetProfile: (input: { path: string; profileId?: number; profile: ProfileInput; seconds?: number }) =>
    request<ProfileSampleEstimate>('/api/assets/preview/estimate', { method: 'POST', body: JSON.stringify(input) }),
  recommendEncoderQuality: (input: { path?: string; profile: ProfileInput }) =>
    request<QualityRecommendationResponse>('/api/assets/quality-recommendation', { method: 'POST', body: JSON.stringify(input) }),
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
