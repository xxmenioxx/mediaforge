package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AssetHandler struct {
	db *gorm.DB
}

type AssetInventory struct {
	Unprocessed       []Asset      `json:"unprocessed"`
	Converted         []Asset      `json:"converted"`
	UnprocessedGroups []AssetGroup `json:"unprocessedGroups"`
	ConvertedGroups   []AssetGroup `json:"convertedGroups"`
}

type AssetReviewState struct {
	RequiresReview bool      `json:"requiresReview"`
	Reason         string    `json:"reason"`
	Source         string    `json:"source"`
	Tags           []string  `json:"tags"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type AssetMetadataState struct {
	Categories []string  `json:"categories"`
	Tags       []string  `json:"tags"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Asset struct {
	LibraryID    uint               `json:"libraryId"`
	LibraryName  string             `json:"libraryName"`
	Path         string             `json:"path"`
	RelativePath string             `json:"relativePath"`
	GroupPath    string             `json:"groupPath"`
	FileName     string             `json:"fileName"`
	Extension    string             `json:"extension"`
	SizeBytes    int64              `json:"sizeBytes"`
	ModifiedAt   time.Time          `json:"modifiedAt"`
	Status       string             `json:"status"`
	Review       AssetReviewState   `json:"review"`
	Metadata     AssetMetadataState `json:"metadata"`
}

type AssetGroup struct {
	ID           string             `json:"id"`
	LibraryID    uint               `json:"libraryId"`
	LibraryName  string             `json:"libraryName"`
	Path         string             `json:"path"`
	RelativePath string             `json:"relativePath"`
	Status       string             `json:"status"`
	FileCount    int                `json:"fileCount"`
	SizeBytes    int64              `json:"sizeBytes"`
	ModifiedAt   time.Time          `json:"modifiedAt"`
	Assets       []Asset            `json:"assets"`
	Review       AssetReviewState   `json:"review"`
	PathReview   AssetReviewState   `json:"pathReview"`
	Metadata     AssetMetadataState `json:"metadata"`
	PathMetadata AssetMetadataState `json:"pathMetadata"`
}

type AssetReviewUpdateInput struct {
	RequiresReview bool     `json:"requiresReview"`
	Reason         string   `json:"reason"`
	Source         string   `json:"source"`
	Tags           []string `json:"tags"`
}

type AssetMetadataUpdateInput struct {
	Categories []string `json:"categories"`
	Tags       []string `json:"tags"`
}

func NewAssetHandler(db *gorm.DB) AssetHandler {
	return AssetHandler{db: db}
}

func (h AssetHandler) List(c *gin.Context) {
	var libraries []models.Library
	if err := h.db.Order("name asc").Find(&libraries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	inventory := AssetInventory{
		Unprocessed:       []Asset{},
		Converted:         []Asset{},
		UnprocessedGroups: []AssetGroup{},
		ConvertedGroups:   []AssetGroup{},
	}
	seenDestinationPaths := map[string]struct{}{}

	for _, library := range libraries {
		destinationPath := strings.TrimSpace(library.DestinationPath)
		if destinationPath == "" {
			continue
		}
		if _, seen := seenDestinationPaths[destinationPath]; seen {
			continue
		}
		seenDestinationPaths[destinationPath] = struct{}{}

		convertedAssets := collectAssets(library, destinationPath, "converted")
		inventory.Converted = append(inventory.Converted, convertedAssets...)
	}

	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sourceLibrary := models.Library{Name: "Originals", SourcePath: rawRoot}
	sourceAssets := collectAssets(sourceLibrary, rawRoot, "unprocessed")
	convertedKeys := assetKeySet(inventory.Converted)
	for _, asset := range sourceAssets {
		if _, ok := convertedKeys[assetKey(asset.RelativePath)]; !ok {
			inventory.Unprocessed = append(inventory.Unprocessed, asset)
		}
	}

	reviews := assetReviewOverrides(h.db)
	metadata := assetMetadataOverrides(h.db)
	applyAssetReviews(inventory.Unprocessed, reviews)
	applyAssetReviews(inventory.Converted, reviews)
	applyAssetMetadata(inventory.Unprocessed, metadata)
	applyAssetMetadata(inventory.Converted, metadata)
	sortAssets(inventory.Unprocessed)
	sortAssets(inventory.Converted)
	inventory.UnprocessedGroups = groupAssets(inventory.Unprocessed, reviews, metadata)
	inventory.ConvertedGroups = groupAssets(inventory.Converted, reviews, metadata)
	sortAssetGroups(inventory.UnprocessedGroups)
	sortAssetGroups(inventory.ConvertedGroups)

	c.JSON(http.StatusOK, inventory)
}

func (h AssetHandler) UpdateMetadata(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	allowed, err := h.pathBelongsToLibrary(resolvedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	var input AssetMetadataUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entries := assetMetadataOverrides(h.db)
	cleanPath := filepath.Clean(resolvedPath)
	categories := normalizedTags(input.Categories)
	tags := normalizedTags(input.Tags)
	if len(categories) == 0 && len(tags) == 0 {
		delete(entries, cleanPath)
	} else {
		entries[cleanPath] = AssetMetadataState{
			Categories: categories,
			Tags:       tags,
			UpdatedAt:  time.Now(),
		}
	}

	if err := saveAssetMetadataOverrides(h.db, entries); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": cleanPath, "metadata": metadataForPath(cleanPath, entries)})
}

func (h AssetHandler) UpdateReview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	allowed, err := h.pathBelongsToLibrary(resolvedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	var input AssetReviewUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reviews := assetReviewOverrides(h.db)
	cleanPath := filepath.Clean(resolvedPath)
	if !input.RequiresReview && strings.TrimSpace(input.Reason) == "" && len(input.Tags) == 0 {
		delete(reviews, cleanPath)
	} else {
		source := strings.TrimSpace(input.Source)
		if source == "" {
			source = "manual"
		}
		reviews[cleanPath] = AssetReviewState{
			RequiresReview: input.RequiresReview,
			Reason:         strings.TrimSpace(input.Reason),
			Source:         source,
			Tags:           normalizedTags(input.Tags),
			UpdatedAt:      time.Now(),
		}
	}

	if err := saveAssetReviewOverrides(h.db, reviews); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": cleanPath, "review": reviews[cleanPath]})
}

func (h AssetHandler) Preview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	path = resolvedPath

	if !isMediaExtension(strings.ToLower(filepath.Ext(path))) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a supported media file"})
		return
	}

	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	file, err := os.Open(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "media path is not readable from the backend container"})
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media path must point to a file"})
		return
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Disposition", `inline; filename="`+filepath.Base(path)+`"`)
	http.ServeContent(c.Writer, c.Request, filepath.Base(path), info.ModTime(), file)
}

func (h AssetHandler) CompatiblePreview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	profileID := strings.TrimSpace(c.Query("profileId"))
	videoCodecOverride := strings.TrimSpace(c.Query("videoCodec"))
	qualityValueOverride := strings.TrimSpace(c.Query("qualityValue"))
	videoPresetOverride := strings.TrimSpace(c.Query("videoPreset"))
	pixFmtOverride := strings.TrimSpace(c.Query("pixFmt"))
	videoFiltersOverride := strings.TrimSpace(c.Query("videoFilters"))
	x265ParamsOverride := strings.TrimSpace(c.Query("x265Params"))
	start, ok := boundedPreviewStart(c.Query("start"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start must be HH:MM:SS or seconds"})
		return
	}
	seconds := boundedPreviewSeconds(c.Query("seconds"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	path = resolvedPath

	if !isMediaExtension(strings.ToLower(filepath.Ext(path))) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a supported media file"})
		return
	}

	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	if info, err := os.Stat(path); err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "media path is not readable from the backend container"})
		return
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-analyzeduration", "100M",
		"-probesize", "100M",
		"-ss", start,
		"-t", strconv.Itoa(seconds),
		"-i", path,
		"-map", "0:v:0?",
		"-map", "0:a:0?",
		"-vf", previewVideoFilterChain(videoFiltersOverride),
	}
	args = append(args, previewVideoCodecArgs(h.db, profileID, videoCodecOverride, qualityValueOverride, videoPresetOverride, pixFmtOverride, x265ParamsOverride)...)
	args = append(args,
		"-c:a", "aac",
		"-b:a", "160k",
		"-ac", "2",
		"-sn",
		"-dn",
		"-map_metadata", "-1",
		"-avoid_negative_ts", "make_zero",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	)

	cmd := exec.CommandContext(c.Request.Context(), "ffmpeg", args...)
	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": message})
		return
	}

	c.Header("Content-Type", "video/mp4")
	c.Header("Content-Disposition", `inline; filename="`+strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+`-preview.mp4"`)
	c.Data(http.StatusOK, "video/mp4", output)
}

func previewVideoCodecArgs(db *gorm.DB, profileID string, videoCodecOverride string, qualityValueOverride string, videoPresetOverride string, pixFmtOverride string, x265ParamsOverride string) []string {
	videoCodec := strings.TrimSpace(videoCodecOverride)
	qualityValue := 24
	videoPreset := strings.TrimSpace(videoPresetOverride)
	pixFmt := strings.TrimSpace(pixFmtOverride)
	x265Params := strings.TrimSpace(x265ParamsOverride)

	if parsedQuality, err := strconv.Atoi(strings.TrimSpace(qualityValueOverride)); err == nil && parsedQuality > 0 {
		qualityValue = parsedQuality
	}

	if videoCodec == "" && profileID != "" {
		id, err := strconv.ParseUint(profileID, 10, 64)
		if err == nil {
			var profile models.Profile
			if db.First(&profile, uint(id)).Error == nil {
				videoCodec = profile.VideoCodec
				if profile.QualityValue > 0 {
					qualityValue = profile.QualityValue
				}
				if profile.WorkerConfig != nil {
					if videoPreset == "" {
						videoPreset = workerStringValue(profile.WorkerConfig["videoPreset"])
					}
					if pixFmt == "" {
						pixFmt = workerStringValue(profile.WorkerConfig["pixFmt"])
					}
					if x265Params == "" {
						x265Params = workerStringValue(profile.WorkerConfig["x265Params"])
					}
				}
			}
		}
	}

	if videoCodec == "" || videoCodec == "copy" {
		videoCodec = "x264"
	}

	args := []string{"-c:v", ffmpegCodecName(videoCodec)}
	if videoPreset == "" {
		videoPreset = "veryfast"
	}
	args = append(args, "-preset", videoPreset)
	if pixFmt == "" {
		pixFmt = "yuv420p"
		if isTenBitVideoCodec(videoCodec) {
			pixFmt = "yuv420p10le"
		}
	}
	args = append(args, "-pix_fmt", pixFmt)
	if x265Params != "" && strings.Contains(ffmpegCodecName(videoCodec), "265") {
		args = append(args, "-x265-params", x265Params)
	}
	args = append(args, "-crf", strconv.Itoa(qualityValue))
	return args
}

func previewVideoFilterChain(videoFilters string) string {
	scale := "scale='min(1280,iw)':-2"
	videoFilters = strings.TrimSpace(videoFilters)
	if videoFilters == "" {
		return scale
	}
	return videoFilters + "," + scale
}

func (h AssetHandler) AudioPreview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	profileKey := strings.TrimSpace(c.Query("profileKey"))
	filterOverride := strings.TrimSpace(c.Query("filters"))
	start, ok := boundedPreviewStart(c.Query("start"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start must be HH:MM:SS or seconds"})
		return
	}
	seconds := boundedPreviewSeconds(c.Query("seconds"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	path = resolvedPath

	if !isMediaExtension(strings.ToLower(filepath.Ext(path))) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a supported media file"})
		return
	}

	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	if info, err := os.Stat(path); err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "media path is not readable from the backend container"})
		return
	}

	filterChain := "anull"
	if filterOverride != "" {
		filterChain = filterOverride
	} else if profileKey != "" {
		audioProfile, err := lookupAudioProfile(h.db, profileKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if audioProfile != nil {
			filterChain = effectiveAudioFilters(*audioProfile)
		}
	}
	filterChain = sanitizeAudioFilterChain(filterChain)

	cmd := exec.CommandContext(
		c.Request.Context(),
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-ss", start,
		"-t", strconv.Itoa(seconds),
		"-i", path,
		"-map", "0:a:0?",
		"-vn",
		"-af", filterChain,
		"-c:a", "pcm_s16le",
		"-f", "wav",
		"pipe:1",
	)

	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": message})
		return
	}

	c.Header("Content-Type", "audio/wav")
	c.Header("Content-Disposition", `inline; filename="`+strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+`-audio-preview.wav"`)
	c.Data(http.StatusOK, "audio/wav", output)
}

var afftdnNoiseFloorPattern = regexp.MustCompile(`afftdn=([^,]*\bnf=)(-?\d+(?:\.\d+)?)`)

func sanitizeAudioFilterChain(filterChain string) string {
	filters := []string{}
	for _, rawFilter := range strings.Split(filterChain, ",") {
		filter := strings.TrimSpace(rawFilter)
		if filter == "" {
			continue
		}
		if arnndnModelMissing(filter) {
			continue
		}
		filters = append(filters, sanitizeAfftdnFilter(filter))
	}
	if len(filters) == 0 {
		return "anull"
	}
	return strings.Join(filters, ",")
}

func sanitizeAfftdnFilter(filter string) string {
	return afftdnNoiseFloorPattern.ReplaceAllStringFunc(filter, func(match string) string {
		parts := afftdnNoiseFloorPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		value, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return match
		}
		if value > -20 {
			value = -20
		}
		if value < -80 {
			value = -80
		}
		return "afftdn=" + parts[1] + trimFloat(value)
	})
}

func arnndnModelMissing(filter string) bool {
	if !strings.HasPrefix(filter, "arnndn=") {
		return false
	}
	modelPath := ""
	for _, part := range strings.Split(strings.TrimPrefix(filter, "arnndn="), ":") {
		if strings.HasPrefix(part, "m=") {
			modelPath = strings.TrimPrefix(part, "m=")
			break
		}
	}
	if strings.TrimSpace(modelPath) == "" {
		return false
	}
	if info, err := os.Stat(modelPath); err == nil && !info.IsDir() {
		return false
	}
	return true
}

func trimFloat(value float64) string {
	if value == float64(int(value)) {
		return strconv.Itoa(int(value))
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func boundedPreviewSeconds(value string) int {
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 20
	}
	if seconds < 5 {
		return 5
	}
	if seconds > 120 {
		return 120
	}
	return seconds
}

func boundedPreviewStart(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "00:00:00", true
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return "", false
		}
		if seconds > 24*60*60 {
			seconds = 24 * 60 * 60
		}
		return secondsToTimestamp(seconds), true
	}

	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return "", false
	}

	hours, errHours := strconv.Atoi(parts[0])
	minutes, errMinutes := strconv.Atoi(parts[1])
	seconds, errSeconds := strconv.Atoi(parts[2])
	if errHours != nil || errMinutes != nil || errSeconds != nil {
		return "", false
	}
	if hours < 0 || minutes < 0 || minutes > 59 || seconds < 0 || seconds > 59 {
		return "", false
	}
	if hours > 24 {
		hours = 24
		minutes = 0
		seconds = 0
	}

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds), true
}

func secondsToTimestamp(totalSeconds int) string {
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func (h AssetHandler) resolveMediaPath(mediaPath string) (string, error) {
	mediaPath = strings.TrimSpace(mediaPath)
	if filepath.IsAbs(mediaPath) {
		return filepath.Clean(mediaPath), nil
	}

	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(rawRoot, mediaPath)), nil
}

func (h AssetHandler) pathBelongsToLibrary(mediaPath string) (bool, error) {
	mediaAbs, err := filepath.Abs(mediaPath)
	if err != nil {
		return false, err
	}

	var libraries []models.Library
	if err := h.db.Find(&libraries).Error; err != nil {
		return false, err
	}

	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		return false, err
	}

	allowedRoots := []string{rawRoot}
	for _, library := range libraries {
		allowedRoots = append(allowedRoots, library.DestinationPath)
		if strings.TrimSpace(library.SourcePath) != "" {
			allowedRoots = append(allowedRoots, library.SourcePath)
		}
	}

	for _, root := range allowedRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}

		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}

		relative, err := filepath.Rel(rootAbs, mediaAbs)
		if err != nil {
			continue
		}

		if relative == "." || (!strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && relative != "..") {
			return true, nil
		}
	}

	return false, nil
}

func settingPath(db *gorm.DB, key string, fallback string) (string, error) {
	paths := models.JSONMap{}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "paths").Error; err == nil && setting.Value != nil {
		paths = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	return stringSetting(paths, key, fallback), nil
}

func collectAssets(library models.Library, root string, status string) []Asset {
	if strings.TrimSpace(root) == "" {
		return []Asset{}
	}

	assets := []Asset{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}

		extension := strings.ToLower(filepath.Ext(path))
		if !isMediaExtension(extension) {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}

		relativePath := relativeAssetPath(root, path)
		groupPath := logicalAssetGroupPath(relativePath)

		assets = append(assets, Asset{
			LibraryID:    library.ID,
			LibraryName:  library.Name,
			Path:         path,
			RelativePath: relativePath,
			GroupPath:    groupPath,
			FileName:     filepath.Base(path),
			Extension:    extension,
			SizeBytes:    info.Size(),
			ModifiedAt:   info.ModTime(),
			Status:       status,
		})

		return nil
	})

	return assets
}

func groupAssets(assets []Asset, reviews map[string]AssetReviewState, metadata map[string]AssetMetadataState) []AssetGroup {
	groupsByKey := map[string]*AssetGroup{}
	order := []string{}

	for _, asset := range assets {
		key := assetGroupKey(asset)
		group, exists := groupsByKey[key]
		if !exists {
			groupPath := strings.Trim(asset.GroupPath, string(os.PathSeparator))
			absolutePath := absoluteAssetGroupPath(asset)
			group = &AssetGroup{
				ID:           key,
				LibraryID:    asset.LibraryID,
				LibraryName:  asset.LibraryName,
				Path:         absolutePath,
				RelativePath: groupPath,
				Status:       asset.Status,
				Assets:       []Asset{},
				PathReview:   reviewForPath(absolutePath, reviews),
				PathMetadata: metadataForPath(absolutePath, metadata),
			}
			group.Review = group.PathReview
			group.Metadata = group.PathMetadata
			groupsByKey[key] = group
			order = append(order, key)
		}

		group.FileCount++
		group.SizeBytes += asset.SizeBytes
		if asset.ModifiedAt.After(group.ModifiedAt) {
			group.ModifiedAt = asset.ModifiedAt
		}
		group.Review = mergeAssetReviewState(group.Review, asset.Review)
		group.Metadata = mergeAssetMetadataState(group.Metadata, asset.Metadata)
		group.Assets = append(group.Assets, asset)
	}

	groups := make([]AssetGroup, 0, len(order))
	for _, key := range order {
		group := groupsByKey[key]
		sortAssets(group.Assets)
		groups = append(groups, *group)
	}

	return groups
}

func assetMetadataOverrides(db *gorm.DB) map[string]AssetMetadataState {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "assetMetadataOverrides").Error; err != nil || setting.Value == nil {
		return map[string]AssetMetadataState{}
	}

	rawEntries, ok := setting.Value["entries"].(map[string]interface{})
	if !ok {
		return map[string]AssetMetadataState{}
	}

	entries := map[string]AssetMetadataState{}
	for rawPath, rawValue := range rawEntries {
		entry, ok := rawValue.(map[string]interface{})
		if !ok {
			continue
		}
		metadata := AssetMetadataState{
			Categories: stringSliceFromUnknown(entry["categories"]),
			Tags:       stringSliceFromUnknown(entry["tags"]),
		}
		if updatedAt, err := time.Parse(time.RFC3339, stringFromUnknown(entry["updatedAt"])); err == nil {
			metadata.UpdatedAt = updatedAt
		}
		entries[filepath.Clean(rawPath)] = metadata
	}
	return entries
}

func saveAssetMetadataOverrides(db *gorm.DB, metadata map[string]AssetMetadataState) error {
	entries := map[string]interface{}{}
	for path, item := range metadata {
		entry := map[string]interface{}{
			"categories": item.Categories,
			"tags":       item.Tags,
			"updatedAt":  item.UpdatedAt.Format(time.RFC3339),
		}
		entries[filepath.Clean(path)] = entry
	}

	setting := models.AppSetting{}
	value := models.JSONMap{"entries": entries}
	if err := db.First(&setting, "key = ?", "assetMetadataOverrides").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.Create(&models.AppSetting{Key: "assetMetadataOverrides", Value: value}).Error
		}
		return err
	}
	setting.Value = value
	return db.Save(&setting).Error
}

func assetReviewOverrides(db *gorm.DB) map[string]AssetReviewState {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "assetReviewOverrides").Error; err != nil || setting.Value == nil {
		return map[string]AssetReviewState{}
	}

	rawEntries, ok := setting.Value["entries"].(map[string]interface{})
	if !ok {
		return map[string]AssetReviewState{}
	}

	reviews := map[string]AssetReviewState{}
	for rawPath, rawValue := range rawEntries {
		entry, ok := rawValue.(map[string]interface{})
		if !ok {
			continue
		}
		review := AssetReviewState{
			RequiresReview: boolSetting(entry["requiresReview"], false),
			Reason:         stringFromUnknown(entry["reason"]),
			Source:         stringFromUnknown(entry["source"]),
			Tags:           stringSliceFromUnknown(entry["tags"]),
		}
		if updatedAt, err := time.Parse(time.RFC3339, stringFromUnknown(entry["updatedAt"])); err == nil {
			review.UpdatedAt = updatedAt
		}
		reviews[filepath.Clean(rawPath)] = review
	}
	return reviews
}

func saveAssetReviewOverrides(db *gorm.DB, reviews map[string]AssetReviewState) error {
	entries := map[string]interface{}{}
	for path, review := range reviews {
		entry := map[string]interface{}{
			"requiresReview": review.RequiresReview,
			"reason":         review.Reason,
			"source":         review.Source,
			"tags":           review.Tags,
			"updatedAt":      review.UpdatedAt.Format(time.RFC3339),
		}
		entries[filepath.Clean(path)] = entry
	}

	setting := models.AppSetting{}
	value := models.JSONMap{"entries": entries}
	if err := db.First(&setting, "key = ?", "assetReviewOverrides").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.Create(&models.AppSetting{Key: "assetReviewOverrides", Value: value}).Error
		}
		return err
	}
	setting.Value = value
	return db.Save(&setting).Error
}

func applyAssetReviews(assets []Asset, reviews map[string]AssetReviewState) {
	for index := range assets {
		assets[index].Review = reviewForPath(assets[index].Path, reviews)
	}
}

func applyAssetMetadata(assets []Asset, metadata map[string]AssetMetadataState) {
	for index := range assets {
		assets[index].Metadata = metadataForPath(assets[index].Path, metadata)
	}
}

func metadataForPath(path string, metadata map[string]AssetMetadataState) AssetMetadataState {
	cleanPath := filepath.Clean(path)
	item, ok := metadata[cleanPath]
	if !ok {
		return AssetMetadataState{Categories: []string{}, Tags: []string{}}
	}
	if item.Categories == nil {
		item.Categories = []string{}
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	return item
}

func reviewForPath(path string, reviews map[string]AssetReviewState) AssetReviewState {
	cleanPath := filepath.Clean(path)
	review, ok := reviews[cleanPath]
	if !ok {
		return AssetReviewState{Tags: []string{}}
	}
	if review.Tags == nil {
		review.Tags = []string{}
	}
	return review
}

func mergeAssetReviewState(current AssetReviewState, next AssetReviewState) AssetReviewState {
	if !next.RequiresReview && len(next.Tags) == 0 && next.Reason == "" {
		return current
	}
	if !current.RequiresReview && len(current.Tags) == 0 && current.Reason == "" {
		return next
	}
	merged := current
	merged.RequiresReview = current.RequiresReview || next.RequiresReview
	if merged.Reason == "" {
		merged.Reason = next.Reason
	}
	if merged.Source == "" {
		merged.Source = next.Source
	}
	if next.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = next.UpdatedAt
	}
	seenTags := map[string]struct{}{}
	tags := []string{}
	for _, tag := range append(current.Tags, next.Tags...) {
		cleanTag := strings.TrimSpace(tag)
		if cleanTag == "" {
			continue
		}
		if _, seen := seenTags[cleanTag]; seen {
			continue
		}
		seenTags[cleanTag] = struct{}{}
		tags = append(tags, cleanTag)
	}
	merged.Tags = tags
	return merged
}

func mergeAssetMetadataState(current AssetMetadataState, next AssetMetadataState) AssetMetadataState {
	if len(next.Categories) == 0 && len(next.Tags) == 0 {
		return current
	}
	merged := current
	if next.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = next.UpdatedAt
	}
	merged.Categories = mergeStringLists(current.Categories, next.Categories)
	merged.Tags = mergeStringLists(current.Tags, next.Tags)
	return merged
}

func mergeStringLists(left []string, right []string) []string {
	seen := map[string]struct{}{}
	values := []string{}
	for _, value := range append(left, right...) {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, clean)
	}
	return values
}

func normalizedTags(tags []string) []string {
	seen := map[string]struct{}{}
	normalized := []string{}
	for _, tag := range tags {
		cleanTag := strings.TrimSpace(tag)
		if cleanTag == "" {
			continue
		}
		if _, ok := seen[cleanTag]; ok {
			continue
		}
		seen[cleanTag] = struct{}{}
		normalized = append(normalized, cleanTag)
	}
	return normalized
}

func boolSetting(value interface{}, fallback bool) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return fallback
}

func stringFromUnknown(value interface{}) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func stringSliceFromUnknown(value interface{}) []string {
	rawValues, ok := value.([]interface{})
	if !ok {
		return []string{}
	}
	values := []string{}
	for _, rawValue := range rawValues {
		if value, ok := rawValue.(string); ok {
			values = append(values, value)
		}
	}
	return normalizedTags(values)
}

func assetGroupKey(asset Asset) string {
	return fmt.Sprintf("%s:%d:%s", strings.TrimSpace(asset.Status), asset.LibraryID, strings.TrimSpace(asset.GroupPath))
}

func logicalAssetGroupPath(relativePath string) string {
	cleanPath := filepath.Clean(relativePath)
	if cleanPath == "." || cleanPath == string(os.PathSeparator) {
		return ""
	}
	parts := strings.Split(cleanPath, string(os.PathSeparator))
	if len(parts) <= 1 {
		return ""
	}
	if len(parts) == 2 {
		return parts[0]
	}
	return filepath.Join(parts[0], parts[1])
}

func absoluteAssetGroupPath(asset Asset) string {
	groupPath := strings.TrimSpace(asset.GroupPath)
	if groupPath == "" {
		return filepath.Dir(asset.Path)
	}

	relativePath := filepath.Clean(asset.RelativePath)
	assetPath := filepath.Clean(asset.Path)
	if relativePath == "." || !strings.HasSuffix(assetPath, relativePath) {
		return filepath.Dir(asset.Path)
	}

	root := strings.TrimSuffix(assetPath, relativePath)
	root = strings.TrimRight(root, string(os.PathSeparator))
	if root == "" {
		return filepath.Dir(asset.Path)
	}
	return filepath.Join(root, groupPath)
}

func relativeAssetPath(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return relative
}

func assetKeySet(assets []Asset) map[string]struct{} {
	keys := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		keys[assetKey(asset.RelativePath)] = struct{}{}
	}
	return keys
}

func assetKey(fileName string) string {
	return strings.ToLower(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
}

func isMediaExtension(extension string) bool {
	switch extension {
	case ".avi", ".flac", ".m4v", ".mka", ".mkv", ".mov", ".mp3", ".mp4", ".mpeg", ".mpg", ".ogg", ".opus", ".ts", ".wav", ".webm":
		return true
	default:
		return false
	}
}

func sortAssets(assets []Asset) {
	sort.SliceStable(assets, func(i, j int) bool {
		if assets[i].LibraryName == assets[j].LibraryName {
			return assets[i].RelativePath < assets[j].RelativePath
		}
		return assets[i].LibraryName < assets[j].LibraryName
	})
}

func sortAssetGroups(groups []AssetGroup) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].LibraryName == groups[j].LibraryName {
			return groups[i].RelativePath < groups[j].RelativePath
		}
		return groups[i].LibraryName < groups[j].LibraryName
	})
}
