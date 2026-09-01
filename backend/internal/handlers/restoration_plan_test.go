package handlers

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
)

func exactRestorationProfile() models.Profile {
	return models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoFilters": "setfield=prog,cas=strength=0.16,exposure=exposure=0.12,eq=brightness=0:contrast=1:saturation=0.96:gamma=0.94,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024,hqdn3d=4:3:6:4.5,chromanr=thres=25:sizew=3:sizeh=3,crop=704:448:8:16,deblock=filter=strong:block=8,bwdif=mode=send_frame:parity=bff:deint=all,zscale=matrix=bt709",
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{
			RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p,
			SourceWidth: 704, SourceHeight: 448, SourceSAR: "40:33", SourceDAR: "4:3",
			TargetWidth: 960, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true,
			SharpenMode: UpscaleSharpenCustom, SharpenStrength: .16, Confidence: UpscaleConfidenceHigh,
		},
		"effectiveOutputProgressive": true,
		"effectiveOutputFrameRate":   "30000/1001",
	}}
}

func TestResolvedRestorationPlanRendersExactStructuredChainInCanonicalOrder(t *testing.T) {
	profile := resolveRestorationPlan(exactRestorationProfile(), &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "8:9", DisplayAspectRatio: "4:3"})
	plan, ok := resolvedRestorationPlanFromProfile(profile)
	if !ok {
		t.Fatal("resolved restoration plan missing")
	}
	want := "bwdif=mode=send_frame:parity=bff:deint=all,deblock=filter=strong:block=8,crop=704:448:8:16,chromanr=thres=25:sizew=3:sizeh=3,hqdn3d=4:3:6:4.5,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024,exposure=exposure=0.12,eq=brightness=0:contrast=1:saturation=0.96:gamma=0.94,zscale=w=960:h=720:filter=lanczos:matrix=bt709,setsar=1,cas=strength=0.16,setfield=prog"
	if plan.ResolvedFilterChain != want {
		t.Fatalf("resolved chain:\n%s\nwant:\n%s", plan.ResolvedFilterChain, want)
	}
	if got := argumentValue(videoWorkerArgsForSource(profile, &MediaStream{Width: 720, Height: 480}), "-vf"); got != want {
		t.Fatalf("worker did not consume resolved plan: %s", got)
	}
	wantStages := []string{"motion", "deblock", "crop", "chroma_cleanup", "denoise", "deband", "image_adjustments", "image_adjustments", "smart_upscale", "sar_normalization", "final_sharpen", "field_metadata"}
	gotStages := make([]string, 0, len(plan.Stages))
	for _, stage := range plan.Stages {
		gotStages = append(gotStages, stage.Stage)
	}
	if !reflect.DeepEqual(gotStages, wantStages) {
		t.Fatalf("stages=%v want=%v", gotStages, wantStages)
	}
}

func TestQueueSnapshotFreezesRestorationPlanAndWorkerConsumesIt(t *testing.T) {
	profile := exactRestorationProfile()
	profile.WorkerConfig[restorationProvenanceSnapshotKey] = models.JSONMap{"version": 1, "appliedRecommendations": models.JSONList{models.JSONMap{"id": "upscale", "confidence": "high"}}}
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "queue_create")
	if err != nil {
		t.Fatal(err)
	}
	scan := models.ScanResult{
		Width: 720, Height: 480, VideoCodec: "mpeg2video",
		VideoStreams:        models.JSONList{models.JSONMap{"width": 720, "height": 480, "sampleAspectRatio": "8:9", "displayAspectRatio": "4:3"}},
		InterlaceAnalysis:   models.JSONMap{"version": interlaceAnalysisVersion, "status": "interlaced", "confidence": .99, "recommendedAction": "deinterlace", "fieldOrder": "bff"},
		RestorationAnalysis: structToJSONMap(restorationEvidenceUnavailable()),
	}
	QueueHandler{}.freezeResolvedRestorationSnapshot(snapshot, scan)
	frozenProfile, err := scheduler.RestoreProfileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	frozen, ok := resolvedRestorationPlanFromProfile(frozenProfile)
	if !ok || !strings.Contains(frozen.ResolvedFilterChain, "hqdn3d=4:3:6:4.5") || !strings.Contains(frozen.ResolvedFilterChain, "cas=strength=0.16") || frozen.RecommendationProvenance == nil {
		t.Fatalf("Queue restoration snapshot incomplete: %#v", frozen)
	}

	profile.WorkerConfig["videoFilters"] = "hqdn3d=1:1:1:1"
	profile.WorkerConfig["upscaleSharpenCustomStrength"] = .30
	consumed := resolveRestorationPlan(frozenProfile, &MediaStream{Width: 1920, Height: 1080, SampleAspectRatio: "1:1"})
	stillFrozen, _ := resolvedRestorationPlanFromProfile(consumed)
	if !reflect.DeepEqual(frozen, stillFrozen) {
		t.Fatalf("Worker re-resolved frozen restoration plan: before=%#v after=%#v", frozen, stillFrozen)
	}
	if got := argumentValue(videoWorkerArgsForSource(consumed, nil), "-vf"); got != frozen.ResolvedFilterChain {
		t.Fatalf("Worker command did not use frozen restoration chain: %s", got)
	}
}

func TestRestorationPlanVideoCopyIsExplicitlyBlockedAndCommandSafe(t *testing.T) {
	profile := exactRestorationProfile()
	profile.VideoCodec = "copy"
	profile = resolveRestorationPlan(profile, &MediaStream{Width: 720, Height: 480})
	plan, _ := resolvedRestorationPlanFromProfile(profile)
	if plan.Executable || !plan.RequiresVideoEncode || len(plan.Warnings) != 1 {
		t.Fatalf("Copy restoration safety was not explicit: %#v", plan)
	}
	args := videoWorkerArgsForSource(profile, &MediaStream{Width: 720, Height: 480})
	if argumentValue(args, "-vf") != "" || strings.Contains(strings.Join(args, " "), "cas=") {
		t.Fatalf("Copy command contains restoration filters: %v", args)
	}
}

func TestValidateRestorationOutputUsesFrozenGeometryCadenceAndFrameStructure(t *testing.T) {
	profile := resolveRestorationPlan(exactRestorationProfile(), &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "8:9"})
	output := models.JSONMap{"width": 960, "height": 720, "sampleAspectRatio": "1/1", "displayAspectRatio": "4:3", "frameRate": "30000/1001", "realFrameRate": "30000/1001", "fieldOrder": "progressive"}
	report := validateRestorationOutput(profile, models.JSONMap{"width": 720, "height": 480, "sampleAspectRatio": "8:9"}, output, validateSmartUpscaleOutput(profile, nil, output))
	if report["status"] != "passed" || report["validationResult"] != "passed" {
		t.Fatalf("restoration output should pass: %#v", report)
	}
	bad := models.JSONMap{"width": 1280, "height": 720, "sampleAspectRatio": "1:1", "frameRate": "30000/1001", "fieldOrder": "interlaced"}
	report = validateRestorationOutput(profile, nil, bad, validateSmartUpscaleOutput(profile, nil, bad))
	if report["status"] != "mismatch" {
		t.Fatalf("stretched/interlaced output passed restoration validation: %#v", report)
	}
}
