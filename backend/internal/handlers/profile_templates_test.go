package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/profiletemplates"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func profileTemplateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "profile-template-handler.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.ProfileAssignment{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestProfileTemplateListReturnsCanonicalRegistry(t *testing.T) {
	db := profileTemplateTestDB(t)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/profile-templates", nil)

	NewProfileHandler(db).ListTemplates(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var got []profiletemplates.Template
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(profiletemplates.All()) {
		t.Fatalf("template count = %d, want %d", len(got), len(profiletemplates.All()))
	}
	if got[0].Key == "" || got[0].Version == 0 || got[0].Definition.Container == "" {
		t.Fatalf("catalog omitted required template definition: %#v", got[0])
	}
	var count int64
	if err := db.Model(&models.Profile{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("listing templates persisted %d user profiles", count)
	}
}

func TestCloneProfileTemplateCreatesIndependentUserProfileWithProvenance(t *testing.T) {
	db := profileTemplateTestDB(t)
	template, ok := profiletemplates.Get("hevc-small-size")
	if !ok {
		t.Fatal("expected hevc-small-size template")
	}
	clone := func(name string) models.Profile {
		t.Helper()
		payload, err := json.Marshal(ProfileTemplateCloneInput{Name: name, Scope: "asset", TemplateVersion: template.Version})
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Params = gin.Params{{Key: "key", Value: template.Key}}
		context.Request = httptest.NewRequest(http.MethodPost, "/api/profile-templates/hevc-small-size/clone", bytes.NewReader(payload))
		context.Request.Header.Set("Content-Type", "application/json")
		NewProfileHandler(db).CloneTemplate(context)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("clone status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var profile models.Profile
		if err := json.Unmarshal(recorder.Body.Bytes(), &profile); err != nil {
			t.Fatal(err)
		}
		return profile
	}

	first := clone("Small copy one")
	if first.ID == 0 || first.TemplateKey != template.Key || first.TemplateVersion != template.Version {
		t.Fatalf("clone identity/provenance = %#v", first)
	}
	if first.Name != "Small copy one" || first.Scope != "asset" || first.VideoCodec != template.Definition.VideoCodec {
		t.Fatalf("clone did not copy the template definition and requested identity: %#v", first)
	}
	first.WorkerConfig["videoPreset"] = "superfast"
	if err := db.Save(&first).Error; err != nil {
		t.Fatal(err)
	}

	second := clone("Small copy two")
	if second.ID == first.ID || second.WorkerConfig["videoPreset"] != "slow" {
		t.Fatalf("clones were not independent: first=%#v second=%#v", first, second)
	}
	assignment, inherited, err := applyProfileAssignment(db, ProfileAssignmentInput{
		TargetType: "asset", TargetPath: "/raw/cloned.mkv", MediaType: "video", Selection: "profile", VideoProfileID: second.ID,
	})
	if err != nil || inherited || assignment.VideoProfileID != second.ID {
		t.Fatalf("cloned User Profile was not assignable: assignment=%#v inherited=%t err=%v", assignment, inherited, err)
	}
	unchanged, ok := profiletemplates.Get(template.Key)
	if !ok || unchanged.Definition.WorkerConfig["videoPreset"] != "slow" {
		t.Fatalf("cloning or editing a user profile changed the template: %#v", unchanged)
	}
}

func TestCloneProfileTemplateRejectsUnsupportedVersion(t *testing.T) {
	db := profileTemplateTestDB(t)
	payload, err := json.Marshal(ProfileTemplateCloneInput{Name: "Stale copy", TemplateVersion: 99})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "key", Value: "hevc-small-size"}}
	context.Request = httptest.NewRequest(http.MethodPost, "/api/profile-templates/hevc-small-size/clone", bytes.NewReader(payload))
	context.Request.Header.Set("Content-Type", "application/json")
	NewProfileHandler(db).CloneTemplate(context)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&models.Profile{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale clone created %d user profiles", count)
	}
}

func TestProfileTemplateCannotBeAssignedDirectly(t *testing.T) {
	db := profileTemplateTestDB(t)
	template, ok := profiletemplates.Get("hevc-small-size")
	if !ok {
		t.Fatal("expected hevc-small-size template")
	}
	payload, err := json.Marshal(ProfileAssignmentInput{
		TargetType: "asset", TargetPath: "/raw/movie.mkv", MediaType: "video", Selection: "profile", ProfileKey: template.Key,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/profile-assignments", bytes.NewReader(payload))
	context.Request.Header.Set("Content-Type", "application/json")
	NewProfileAssignmentHandler(db).Upsert(context)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("direct template assignment status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&models.ProfileAssignment{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected template assignment still created %d assignment rows", count)
	}
}
