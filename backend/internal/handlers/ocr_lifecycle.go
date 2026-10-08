package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	canonicalOCRPipelineProvenance = "canonical-subtitle-pipeline/ocr-evidence-v1;" + canonicalOCRLifecycleVersion + ";" + canonicalOCRParserVersion + ";" + canonicalOCRCleanupVersion + ";" + canonicalOCRGeometryVersion + ";" + canonicalOCRExtractionVersion + ";" + canonicalOCRScoringVersion
	canonicalOCRLifecycleVersion   = "lifecycle-v1"
	canonicalOCRParserVersion      = "parser-v1"
	canonicalOCRCleanupVersion     = "cleanup-v1"
	canonicalOCRGeometryVersion    = "geometry-v1"
	canonicalOCRExtractionVersion  = "extraction-v1"
	canonicalOCRScoringVersion     = "score-v2"
	canonicalOCRIdentityVersion    = 1
	canonicalOCRRuntimeVersion     = "ocr-runtime-v1"
	canonicalOCRCacheVersion       = 1
	canonicalOCRCacheMaxAge        = 7 * 24 * time.Hour
	canonicalOCRCacheMaxBytes      = 256 * 1024 * 1024
	canonicalOCRPreparedHashLimit  = 128 * 1024 * 1024
	canonicalOCRRuntimeCacheTTL    = 30 * time.Second
)

// canonicalOCRIdentity deliberately contains no requested output format. It
// identifies the recognition/cleanup evidence that SRT and ASS serializers
// consume after the shared flight has completed.
type canonicalOCRIdentity struct {
	Version                    int    `json:"version"`
	SourcePath                 string `json:"sourcePath"`
	SourceSize                 int64  `json:"sourceSize"`
	SourceModTimeUnixNano      int64  `json:"sourceModTimeUnixNano"`
	SourceSnapshotVersion      string `json:"sourceSnapshotVersion"`
	StreamIndex                int    `json:"streamIndex"`
	StreamID                   string `json:"streamId"`
	StreamCodec                string `json:"streamCodec"`
	RequestedLanguage          string `json:"requestedLanguage"`
	OCRMode                    string `json:"ocrMode"`
	PreparationScope           string `json:"preparationScope"`
	SegmentStart               string `json:"segmentStart"`
	SegmentDuration            int    `json:"segmentDuration"`
	PreparedStreamMapping      string `json:"preparedStreamMapping"`
	PreparedRepresentation     string `json:"preparedRepresentation"`
	PreparedRepresentationHash string `json:"preparedRepresentationHash,omitempty"`
	PipelineProvenance         string `json:"pipelineProvenance"`
	RuntimeProvenance          string `json:"runtimeProvenance"`
}

func (identity canonicalOCRIdentity) key() string {
	encoded, _ := json.Marshal(identity)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

type canonicalOCRRequest struct {
	Identity                 canonicalOCRIdentity
	MediaPath                string
	Stream                   FFProbeStream
	Language                 string
	DictionaryLanguage       string
	DictionaryLanguageSource string
	Mode                     string
	PreparedLocal            bool
	PreparationStart         float64
}

type canonicalOCRRuntimeSnapshot struct {
	cacheKey     string
	provenance   string
	capabilities map[string]struct{}
	message      string
	createdAt    time.Time
}

var canonicalOCRRuntimeCache = struct {
	sync.Mutex
	items map[string]canonicalOCRRuntimeSnapshot
}{items: map[string]canonicalOCRRuntimeSnapshot{}}

type canonicalOCRFailure struct {
	cause       error
	diagnostics string
}

func (failure *canonicalOCRFailure) Error() string {
	if strings.TrimSpace(failure.diagnostics) == "" {
		return failure.cause.Error()
	}
	return fmt.Sprintf("%v: %s", failure.cause, conciseCommandOutput(failure.diagnostics, 1200))
}

func (failure *canonicalOCRFailure) Unwrap() error {
	return failure.cause
}

type canonicalOCREvidence struct {
	Identity                 canonicalOCRIdentity               `json:"identity"`
	RequestedLanguage        string                             `json:"requestedLanguage"`
	EffectiveLanguage        string                             `json:"effectiveLanguage"`
	DictionaryLanguage       string                             `json:"dictionaryLanguage,omitempty"`
	DictionaryLanguageSource string                             `json:"dictionaryLanguageSource,omitempty"`
	OCRMode                  string                             `json:"ocrMode"`
	Message                  string                             `json:"message"`
	Complete                 bool                               `json:"complete"`
	Integrity                string                             `json:"integrity"`
	Cues                     []canonicalBitmapSubtitleCue       `json:"cues"`
	Geometry                 []bitmapCueGeometry                `json:"geometry"`
	CorrectionProvenance     []canonicalOCRCorrectionProvenance `json:"correctionProvenance,omitempty"`
}

type canonicalOCRCorrectionProvenance struct {
	CueID                  int      `json:"cueId"`
	Action                 string   `json:"action"`
	CandidateSource        string   `json:"candidateSource,omitempty"`
	TextHash               string   `json:"textHash,omitempty"`
	BeforeTextHash         string   `json:"beforeTextHash,omitempty"`
	AfterTextHash          string   `json:"afterTextHash,omitempty"`
	DictionaryLanguage     string   `json:"dictionaryLanguage,omitempty"`
	DictionaryRevision     string   `json:"dictionaryRevision,omitempty"`
	DictionaryRuleIDs      []string `json:"dictionaryRuleIds,omitempty"`
	DictionaryRuleVersions []int    `json:"dictionaryRuleVersions,omitempty"`
	OperationID            string   `json:"operationId,omitempty"`
	Validation             string   `json:"validation"`
	SelectedBy             string   `json:"selectedBy"`
}

type canonicalOCREvidenceCacheRecord struct {
	Version   int                  `json:"version"`
	Key       string               `json:"key"`
	CreatedAt time.Time            `json:"createdAt"`
	Payload   canonicalOCREvidence `json:"payload"`
	Checksum  string               `json:"checksum"`
}

type canonicalOCRFlightProgress struct {
	processed int
	total     int
}

type canonicalOCRFlight struct {
	mu          sync.Mutex
	key         string
	request     canonicalOCRRequest
	ctx         context.Context
	cancel      context.CancelFunc
	workspace   string
	done        chan struct{}
	subscribers map[*canonicalOCRSubscription]OCRProgressFunc
	progress    canonicalOCRFlightProgress
	result      *canonicalOCREvidence
	err         error
	finished    bool
}

type canonicalOCRSubscription struct {
	flight     *canonicalOCRFlight
	onProgress OCRProgressFunc
	once       sync.Once
}

var canonicalOCRFlights = struct {
	sync.Mutex
	items map[string]*canonicalOCRFlight
}{items: map[string]*canonicalOCRFlight{}}

func canonicalOCRRequestFor(
	mediaPath string,
	stream FFProbeStream,
	language string,
	mode string,
	segmentStart float64,
	segmentDuration int,
) canonicalOCRRequest {
	cleanPath, _ := filepath.Abs(filepath.Clean(mediaPath))
	if cleanPath == "" {
		cleanPath = filepath.Clean(mediaPath)
	}
	statSize := int64(-1)
	statMTime := int64(-1)
	if info, err := os.Stat(cleanPath); err == nil {
		statSize = info.Size()
		statMTime = info.ModTime().UnixNano()
	}
	preparedLocal := segmentDuration > 0
	scope := "full-asset"
	if preparedLocal {
		scope = "segmented-test-encode"
	}
	mode = normalizedOCRMode(mode)
	dictionaryLanguage := ""
	dictionaryLanguageSource := ""
	if strings.TrimSpace(language) != "" {
		if canonical, ok := strictOCRDictionaryLanguage(language); ok {
			dictionaryLanguage = canonical
			dictionaryLanguageSource = "request"
		}
	} else if canonical, ok := strictOCRDictionaryLanguage(stream.Tags["language"]); ok {
		dictionaryLanguage = canonical
		dictionaryLanguageSource = "stream-tag"
	}
	language = normalizedOCRLanguage(language, stream.Tags["language"])
	return canonicalOCRRequest{
		Identity: canonicalOCRIdentity{
			Version:                canonicalOCRIdentityVersion,
			SourcePath:             cleanPath,
			SourceSize:             statSize,
			SourceModTimeUnixNano:  statMTime,
			SourceSnapshotVersion:  "stat-v1",
			StreamIndex:            stream.Index,
			StreamID:               fmt.Sprint(stream.ID),
			StreamCodec:            strings.ToLower(strings.TrimSpace(stream.CodecName)),
			RequestedLanguage:      language,
			OCRMode:                mode,
			PreparationScope:       scope,
			SegmentStart:           strconv.FormatFloat(segmentStart, 'f', 6, 64),
			SegmentDuration:        segmentDuration,
			PreparedStreamMapping:  fmt.Sprintf("subtitle-stream:%d;codec:%s", stream.Index, strings.ToLower(strings.TrimSpace(stream.CodecName))),
			PreparedRepresentation: "prepared-subtitle-v1",
			PipelineProvenance:     canonicalOCRPipelineProvenance,
		},
		MediaPath:                cleanPath,
		Stream:                   stream,
		Language:                 language,
		DictionaryLanguage:       dictionaryLanguage,
		DictionaryLanguageSource: dictionaryLanguageSource,
		Mode:                     mode,
		PreparedLocal:            preparedLocal,
		PreparationStart:         segmentStart,
	}
}

func canonicalOCRRequestWithRuntime(ctx context.Context, request canonicalOCRRequest) (canonicalOCRRequest, error) {
	snapshot, err := resolveCanonicalOCRRuntimeProvenance(ctx)
	if err != nil {
		return request, err
	}
	request.Identity.RuntimeProvenance = snapshot.provenance
	return request, nil
}

func resolveCanonicalOCRRuntimeProvenance(ctx context.Context) (canonicalOCRRuntimeSnapshot, error) {
	seconvPath, err := exec.LookPath("seconv")
	if err != nil {
		return canonicalOCRRuntimeSnapshot{}, fmt.Errorf("OCR is not available in this backend image (seconv was not found)")
	}
	tesseractPath, err := exec.LookPath("tesseract")
	if err != nil {
		return canonicalOCRRuntimeSnapshot{}, fmt.Errorf("OCR is not available in this backend image (Tesseract was not found)")
	}
	seconvProvenance, err := ocrToolProvenance(seconvPath)
	if err != nil {
		return canonicalOCRRuntimeSnapshot{}, fmt.Errorf("cannot fingerprint SeConv runtime: %w", err)
	}
	tesseractProvenance, err := ocrToolProvenance(tesseractPath)
	if err != nil {
		return canonicalOCRRuntimeSnapshot{}, fmt.Errorf("cannot fingerprint Tesseract runtime: %w", err)
	}
	cacheKey := seconvProvenance + "\x00" + tesseractProvenance
	canonicalOCRRuntimeCache.Lock()
	if cached, ok := canonicalOCRRuntimeCache.items[cacheKey]; ok && time.Since(cached.createdAt) < canonicalOCRRuntimeCacheTTL {
		canonicalOCRRuntimeCache.Unlock()
		return cloneCanonicalOCRRuntimeSnapshot(cached), nil
	}
	canonicalOCRRuntimeCache.Unlock()

	capabilities, message, err := probeTesseractLanguagesUncached(ctx)
	if err != nil {
		return canonicalOCRRuntimeSnapshot{}, err
	}
	languages := sortedOCRLanguages(capabilities)
	languageBytes := sha256.Sum256([]byte(strings.Join(languages, "\n")))
	payload, err := json.Marshal(struct {
		Version             string   `json:"version"`
		SeConvProvenance    string   `json:"seconv"`
		TesseractProvenance string   `json:"tesseract"`
		Languages           []string `json:"languages"`
		LanguageFingerprint string   `json:"languageFingerprint"`
		JapaneseHorizontal  bool     `json:"jpn"`
		JapaneseVertical    bool     `json:"jpnVert"`
		PipelineProvenance  string   `json:"pipeline"`
	}{
		Version:             canonicalOCRRuntimeVersion,
		SeConvProvenance:    seconvProvenance,
		TesseractProvenance: tesseractProvenance,
		Languages:           languages,
		LanguageFingerprint: hex.EncodeToString(languageBytes[:]),
		JapaneseHorizontal:  hasOCRLanguage(capabilities, "jpn"),
		JapaneseVertical:    hasOCRLanguage(capabilities, "jpn_vert"),
		PipelineProvenance:  canonicalOCRPipelineProvenance,
	})
	if err != nil {
		return canonicalOCRRuntimeSnapshot{}, fmt.Errorf("cannot serialize OCR runtime provenance: %w", err)
	}
	snapshot := canonicalOCRRuntimeSnapshot{
		cacheKey:     cacheKey,
		provenance:   string(payload),
		capabilities: cloneOCRCapabilities(capabilities),
		message:      message,
		createdAt:    time.Now(),
	}
	canonicalOCRRuntimeCache.Lock()
	canonicalOCRRuntimeCache.items[cacheKey] = snapshot
	canonicalOCRRuntimeCache.Unlock()
	return cloneCanonicalOCRRuntimeSnapshot(snapshot), nil
}

func ocrToolProvenance(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	return fmt.Sprintf("path=%s;size=%d;mtime=%d;mode=%o", path, info.Size(), info.ModTime().UnixNano(), info.Mode().Perm()), nil
}

func hasOCRLanguage(capabilities map[string]struct{}, language string) bool {
	_, ok := capabilities[language]
	return ok
}

func cloneOCRCapabilities(capabilities map[string]struct{}) map[string]struct{} {
	clone := make(map[string]struct{}, len(capabilities))
	for language := range capabilities {
		clone[language] = struct{}{}
	}
	return clone
}

func cloneCanonicalOCRRuntimeSnapshot(snapshot canonicalOCRRuntimeSnapshot) canonicalOCRRuntimeSnapshot {
	snapshot.capabilities = cloneOCRCapabilities(snapshot.capabilities)
	return snapshot
}

func canonicalOCREvidenceForRequest(
	ctx context.Context,
	request canonicalOCRRequest,
	onProgress OCRProgressFunc,
) (*canonicalOCREvidence, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := canonicalOCRRequestWithRuntime(ctx, request)
	if err != nil {
		return nil, err
	}
	key := request.Identity.key()
	if cached, ok := loadCanonicalOCREvidence(ctx, request, key); ok {
		cached.Message = strings.TrimSpace("OCR evidence cache hit; reusing canonical evidence\n" + cached.Message)
		if onProgress != nil {
			onProgress(1, 1)
		}
		return canonicalOCREvidenceWithDictionaryAuthority(cached, request), nil
	}

	root := canonicalOCRWorkspaceRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create internal OCR workspace: %w", err)
	}

	canonicalOCRFlights.Lock()
	flight := canonicalOCRFlights.items[key]
	if flight != nil {
		flight.mu.Lock()
		reusable := !flight.finished && flight.ctx.Err() == nil
		if !reusable {
			flight.mu.Unlock()
			// Terminal or cancelled flights are never reusable. A fresh request
			// must rerun OCR after cache invalidation instead of receiving stale
			// in-memory evidence while registry cleanup is still completing.
			delete(canonicalOCRFlights.items, key)
			flight = nil
		} else {
			subscription := &canonicalOCRSubscription{flight: flight, onProgress: onProgress}
			flight.subscribers[subscription] = onProgress
			progress := flight.progress
			flight.mu.Unlock()
			canonicalOCRFlights.Unlock()
			if progress.total > 0 && onProgress != nil {
				onProgress(progress.processed, progress.total)
			}
			evidence, err := subscription.wait(ctx)
			return canonicalOCREvidenceWithDictionaryAuthority(evidence, request), err
		}
	}
	flightCtx, cancel := context.WithCancel(context.Background())
	workspace, err := os.MkdirTemp(root, ".mvforge-ocr-flight-*")
	if err != nil {
		canonicalOCRFlights.Unlock()
		cancel()
		return nil, fmt.Errorf("cannot create shared OCR workspace: %w", err)
	}
	flight = &canonicalOCRFlight{
		key:         key,
		request:     request,
		ctx:         flightCtx,
		cancel:      cancel,
		workspace:   workspace,
		done:        make(chan struct{}),
		subscribers: map[*canonicalOCRSubscription]OCRProgressFunc{},
	}
	canonicalOCRFlights.items[key] = flight
	subscription := &canonicalOCRSubscription{flight: flight, onProgress: onProgress}
	flight.mu.Lock()
	flight.subscribers[subscription] = onProgress
	progress := flight.progress
	flight.mu.Unlock()
	canonicalOCRFlights.Unlock()

	if progress.total > 0 && onProgress != nil {
		onProgress(progress.processed, progress.total)
	}
	go flight.execute()
	evidence, err := subscription.wait(ctx)
	return canonicalOCREvidenceWithDictionaryAuthority(evidence, request), err
}

func canonicalOCREvidenceWithDictionaryAuthority(evidence *canonicalOCREvidence, request canonicalOCRRequest) *canonicalOCREvidence {
	if evidence == nil {
		return nil
	}
	copy := cloneCanonicalOCREvidence(evidence)
	copy.DictionaryLanguage = request.DictionaryLanguage
	copy.DictionaryLanguageSource = request.DictionaryLanguageSource
	return copy
}

func (subscription *canonicalOCRSubscription) wait(ctx context.Context) (*canonicalOCREvidence, error) {
	flight := subscription.flight
	select {
	case <-flight.done:
		subscription.detach()
		return flight.snapshotResult()
	default:
	}
	select {
	case <-flight.done:
		subscription.detach()
		return flight.snapshotResult()
	case <-ctx.Done():
		subscription.detach()
		return nil, ctx.Err()
	}
}

func (subscription *canonicalOCRSubscription) detach() {
	subscription.once.Do(func() {
		flight := subscription.flight
		flight.mu.Lock()
		delete(flight.subscribers, subscription)
		last := len(flight.subscribers) == 0 && !flight.finished
		flight.mu.Unlock()
		if last {
			flight.cancel()
		}
	})
}

func (flight *canonicalOCRFlight) snapshotResult() (*canonicalOCREvidence, error) {
	flight.mu.Lock()
	defer flight.mu.Unlock()
	if flight.result != nil {
		return cloneCanonicalOCREvidence(flight.result), nil
	}
	return nil, flight.err
}

func (flight *canonicalOCRFlight) broadcastProgress(processed, total int) {
	flight.mu.Lock()
	flight.progress = canonicalOCRFlightProgress{processed: processed, total: total}
	callbacks := make([]OCRProgressFunc, 0, len(flight.subscribers))
	for _, callback := range flight.subscribers {
		if callback != nil {
			callbacks = append(callbacks, callback)
		}
	}
	flight.mu.Unlock()
	for _, callback := range callbacks {
		callback(processed, total)
	}
}

func (flight *canonicalOCRFlight) hasSubscribers() bool {
	flight.mu.Lock()
	defer flight.mu.Unlock()
	return len(flight.subscribers) > 0
}

func (flight *canonicalOCRFlight) execute() {
	canonicalPath := filepath.Join(flight.workspace, "canonical.srt")
	rawPath := filepath.Join(flight.workspace, "canonical.raw.srt")
	ocrMediaPath, ocrStream, prepareErr := prepareCanonicalOCRInput(flight.ctx, flight.request, flight.workspace)
	if prepareErr != nil {
		_ = cleanupSubtitleOperationWorkspace(flight.workspace)
		flight.finish(nil, prepareErr)
		return
	}
	result, err := runBitmapOCRWithTiming(
		flight.ctx,
		ocrMediaPath,
		ocrStream,
		"srt",
		flight.request.Language,
		flight.request.Mode,
		canonicalPath,
		rawPath,
		flight.broadcastProgress,
		flight.request.PreparedLocal,
		flight.request.PreparationStart,
	)
	if err != nil && !errors.Is(err, context.Canceled) && strings.TrimSpace(result.Message) != "" {
		err = &canonicalOCRFailure{cause: err, diagnostics: result.Message}
	}

	var evidence *canonicalOCREvidence
	if err == nil && flight.ctx.Err() == nil && flight.hasSubscribers() {
		identity := flight.request.Identity
		preparedRequest := flight.request
		preparedRequest.Stream = ocrStream
		if preparedHash := preparedOCRRepresentationHash(preparedRequest, flight.workspace); preparedHash != "" {
			identity.PreparedRepresentationHash = preparedHash
		}
		evidence = &canonicalOCREvidence{
			Identity:                 identity,
			RequestedLanguage:        flight.request.Language,
			EffectiveLanguage:        result.EffectiveLanguage,
			DictionaryLanguage:       flight.request.DictionaryLanguage,
			DictionaryLanguageSource: flight.request.DictionaryLanguageSource,
			OCRMode:                  flight.request.Mode,
			Message:                  result.Message,
			Complete:                 true,
			Integrity:                "canonical-cues-validated-v1",
			Cues:                     cloneCanonicalCues(result.Cues),
			Geometry:                 append([]bitmapCueGeometry(nil), result.Geometry...),
		}
		commitCanonicalOCREvidence(flight.request, flight.key, evidence)
	} else if err == nil && flight.ctx.Err() != nil {
		err = flight.ctx.Err()
	}

	// The flight workspace is not owned by any subscriber. It is removed only
	// after the shared execution has terminated, using the independent bounded
	// cleanup path used by subtitle operations.
	_ = cleanupSubtitleOperationWorkspace(flight.workspace)
	flight.finish(evidence, err)
}

func (flight *canonicalOCRFlight) finish(result *canonicalOCREvidence, err error) {
	flight.mu.Lock()
	flight.result = result
	flight.err = err
	flight.finished = true
	close(flight.done)
	flight.mu.Unlock()
	canonicalOCRFlights.Lock()
	if canonicalOCRFlights.items[flight.key] == flight {
		delete(canonicalOCRFlights.items, flight.key)
	}
	canonicalOCRFlights.Unlock()
}

func cloneCanonicalCues(cues []canonicalBitmapSubtitleCue) []canonicalBitmapSubtitleCue {
	cloned := make([]canonicalBitmapSubtitleCue, len(cues))
	copy(cloned, cues)
	for index := range cloned {
		if cues[index].Geometry != nil {
			geometry := *cues[index].Geometry
			cloned[index].Geometry = &geometry
		}
	}
	return cloned
}

func cloneCanonicalOCREvidence(evidence *canonicalOCREvidence) *canonicalOCREvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	clone.Cues = cloneCanonicalCues(evidence.Cues)
	clone.Geometry = append([]bitmapCueGeometry(nil), evidence.Geometry...)
	return &clone
}

func canonicalOCRWorkspaceRoot() string {
	if root := strings.TrimSpace(os.Getenv("MVFORGE_OCR_LIFECYCLE_ROOT")); root != "" {
		return root
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		candidate := filepath.Join(cacheDir, "mvforge", "ocr")
		if err := os.MkdirAll(candidate, 0o700); err == nil {
			return candidate
		}
	}
	return filepath.Join(os.TempDir(), "mvforge-ocr")
}

func canonicalOCRCachePath(root, key string) string {
	return filepath.Join(root, "evidence-"+key+".json")
}

func loadCanonicalOCREvidence(ctx context.Context, request canonicalOCRRequest, key string) (*canonicalOCREvidence, bool) {
	if request.Identity.SourceSize < 0 || request.Identity.SourceModTimeUnixNano < 0 {
		return nil, false
	}
	path := canonicalOCRCachePath(canonicalOCRWorkspaceRoot(), key)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var record canonicalOCREvidenceCacheRecord
	if json.Unmarshal(content, &record) != nil || record.Version != canonicalOCRCacheVersion || record.Key != key {
		_ = os.Remove(path)
		return nil, false
	}
	if time.Since(record.CreatedAt) > canonicalOCRCacheMaxAge {
		_ = os.Remove(path)
		return nil, false
	}
	payload, _ := json.Marshal(record.Payload)
	sum := sha256.Sum256(payload)
	if record.Checksum != hex.EncodeToString(sum[:]) || !record.Payload.Complete || record.Payload.Integrity != "canonical-cues-validated-v1" {
		return nil, false
	}
	preparedHash, preparedOK := validateCachedPreparedRepresentation(ctx, request, record.Payload.Identity.PreparedRepresentationHash)
	if !preparedOK {
		return nil, false
	}
	request.Identity.PreparedRepresentationHash = preparedHash
	if !canonicalOCRIdentityMatches(request.Identity, record.Payload.Identity) {
		return nil, false
	}
	return cloneCanonicalOCREvidence(&record.Payload), true
}

func canonicalOCRIdentityMatches(expected, actual canonicalOCRIdentity) bool {
	return reflect.DeepEqual(expected, actual)
}

func validateCachedPreparedRepresentation(ctx context.Context, request canonicalOCRRequest, storedHash string) (string, bool) {
	if storedHash == "" {
		if preparedOCRRepresentationRequired(request) {
			return "", false
		}
		return "", true
	}
	root := canonicalOCRWorkspaceRoot()
	workspace, err := os.MkdirTemp(root, ".mvforge-ocr-cache-validate-*")
	if err != nil {
		return "", false
	}
	defer cleanupSubtitleOperationWorkspace(workspace)

	codec := strings.ToLower(strings.TrimSpace(request.Stream.CodecName))
	var preparedPath string
	switch codec {
	case "hdmv_pgs_subtitle", "pgssub":
		mediaPath, stream, prepareErr := prepareCanonicalOCRInput(ctx, request, workspace)
		if prepareErr != nil {
			return "", false
		}
		preparedPath = filepath.Join(workspace, fmt.Sprintf("prepared-stream-%d.sup", stream.Index))
		if _, extractErr := extractPGSSubtitleForOCR(ctx, mediaPath, stream, preparedPath); extractErr != nil {
			return "", false
		}
	case "dvd_subtitle", "dvb_subtitle":
		preparedPath = preparedOCRRepresentationPath(request, workspace)
	default:
		return "", false
	}
	actualHash := boundedOCRFileHash(preparedPath)
	if actualHash == "" || actualHash != storedHash {
		return "", false
	}
	return actualHash, true
}

func preparedOCRRepresentationRequired(request canonicalOCRRequest) bool {
	codec := strings.ToLower(strings.TrimSpace(request.Stream.CodecName))
	if codec == "hdmv_pgs_subtitle" || codec == "pgssub" {
		return true
	}
	if codec != "dvd_subtitle" && codec != "dvb_subtitle" {
		return false
	}
	extension := strings.ToLower(filepath.Ext(request.MediaPath))
	return extension == ".idx" || extension == ".sub"
}

func prepareCanonicalOCRInput(ctx context.Context, request canonicalOCRRequest, workspace string) (string, FFProbeStream, error) {
	if !request.PreparedLocal {
		return request.MediaPath, request.Stream, nil
	}
	segmentPath := filepath.Join(workspace, "subtitle-segment.mkv")
	args := bitmapSubtitleSegmentArgs(
		request.MediaPath,
		request.Stream.Index,
		segmentPath,
		request.PreparationStart,
		request.Identity.SegmentDuration,
	)
	var output bytes.Buffer
	if err := runSubtitleCommand(ctx, "ffmpeg", args, &output, &output); err != nil {
		return "", FFProbeStream{}, fmt.Errorf("cannot isolate subtitle test window for stream %d: %s", request.Stream.Index, fallback(strings.TrimSpace(output.String()), err.Error()))
	}
	streams, err := probeSubtitleStreams(ctx, segmentPath)
	if err != nil || len(streams) == 0 {
		return "", FFProbeStream{}, fmt.Errorf("subtitle test window for stream %d has no readable subtitle track", request.Stream.Index)
	}
	return segmentPath, streams[0], nil
}

func commitCanonicalOCREvidence(request canonicalOCRRequest, key string, evidence *canonicalOCREvidence) {
	if evidence == nil || request.Identity.SourceSize < 0 || request.Identity.SourceModTimeUnixNano < 0 {
		return
	}
	root := canonicalOCRWorkspaceRoot()
	if os.MkdirAll(root, 0o700) != nil {
		return
	}
	payload := cloneCanonicalOCREvidence(evidence)
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return
	}
	sum := sha256.Sum256(encodedPayload)
	record := canonicalOCREvidenceCacheRecord{
		Version:   canonicalOCRCacheVersion,
		Key:       key,
		CreatedAt: time.Now().UTC(),
		Payload:   *payload,
		Checksum:  hex.EncodeToString(sum[:]),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}
	destination := canonicalOCRCachePath(root, key)
	temporary, err := os.CreateTemp(root, ".ocr-evidence-*")
	if err != nil {
		return
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(encoded); err == nil {
		err = temporary.Chmod(0o600)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		_ = os.Rename(temporaryPath, destination)
	}
	pruneCanonicalOCREvidenceCache(root)
}

func pruneCanonicalOCREvidenceCache(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type cacheEntry struct {
		path string
		size int64
		when time.Time
	}
	entriesToKeep := make([]cacheEntry, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "evidence-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		if time.Since(info.ModTime()) > canonicalOCRCacheMaxAge {
			_ = os.Remove(filepath.Join(root, entry.Name()))
			continue
		}
		entriesToKeep = append(entriesToKeep, cacheEntry{path: filepath.Join(root, entry.Name()), size: info.Size(), when: info.ModTime()})
		total += info.Size()
	}
	if total <= canonicalOCRCacheMaxBytes {
		return
	}
	sort.Slice(entriesToKeep, func(i, j int) bool { return entriesToKeep[i].when.Before(entriesToKeep[j].when) })
	for _, entry := range entriesToKeep {
		if total <= canonicalOCRCacheMaxBytes {
			break
		}
		_ = os.Remove(entry.path)
		total -= entry.size
	}
}

func preparedOCRRepresentationHash(request canonicalOCRRequest, workspace string) string {
	path := preparedOCRRepresentationPath(request, workspace)
	if path == "" {
		return ""
	}
	return boundedOCRFileHash(path)
}

func preparedOCRRepresentationPath(request canonicalOCRRequest, workspace string) string {
	path := ""
	switch strings.ToLower(strings.TrimSpace(request.Stream.CodecName)) {
	case "hdmv_pgs_subtitle", "pgssub":
		path = filepath.Join(workspace, fmt.Sprintf("ocr-stream-%d.sup", request.Stream.Index))
	case "dvd_subtitle", "dvb_subtitle":
		extension := strings.ToLower(filepath.Ext(request.MediaPath))
		if extension == ".idx" {
			path = strings.TrimSuffix(request.MediaPath, extension) + ".sub"
		} else if extension == ".sub" {
			path = request.MediaPath
		}
	}
	if path == "" {
		return ""
	}
	return path
}

func boundedOCRFileHash(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() < 0 || info.Size() > canonicalOCRPreparedHashLimit {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}
