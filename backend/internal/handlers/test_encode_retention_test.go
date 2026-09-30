package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func TestDeleteTestEncodeRequiresExplicitConfirmationForKeep(t *testing.T) {
	db := testEncodeTestDB(t, "delete-kept-test-encode")
	root := t.TempDir()
	library := models.Library{Name: "TV", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "test.mkv")
	if err := os.WriteFile(output, []byte("test output"), 0o600); err != nil {
		t.Fatal(err)
	}
	testEncode := models.TestEncode{
		SourcePath: "/raw/source.mkv", LibraryID: library.ID, ConfigurationSource: "global",
		ConfigurationHash: "test", Status: testEncodeReady, Phase: "ready", OutputPath: output, Keep: true,
	}
	if err := db.Create(&testEncode).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TestEncodeArtifact{TestEncodeID: testEncode.ID, LibraryID: library.ID, Path: output, Type: models.TestEncodeArtifactMediaOutput, Expected: true, State: "owned"}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.DELETE("/api/test-encodes/:id", (AssetHandler{db: db}).DeleteTestEncode)
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodDelete, "/api/test-encodes/"+testEncodeIDString(testEncode.ID), nil))
	if first.Code != http.StatusConflict {
		t.Fatalf("unconfirmed Keep deletion status=%d body=%s", first.Code, first.Body.String())
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("unconfirmed deletion changed output: %v", err)
	}
	confirmed := httptest.NewRecorder()
	router.ServeHTTP(confirmed, httptest.NewRequest(http.MethodDelete, "/api/test-encodes/"+testEncodeIDString(testEncode.ID)+"?confirmKeep=true", nil))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirmed Keep deletion status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("confirmed Keep output remains: %v", err)
	}
	var persisted models.TestEncode
	if err := db.Unscoped().First(&persisted, testEncode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.DeletedAt == nil || persisted.Status != testEncodeDeleted {
		t.Fatalf("confirmed deletion did not tombstone run: %#v", persisted)
	}
}

func TestOrphanCleanupRequiresPersistedOwnershipAndLeavesUnownedFiles(t *testing.T) {
	db := testEncodeTestDB(t, "delete-test-encode-orphan")
	root := t.TempDir()
	library := models.Library{Name: "TV", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	ownedPath := filepath.Join(root, "proven-test-output.mkv")
	unownedPath := filepath.Join(root, "ambiguous.mkv")
	for _, path := range []string{ownedPath, unownedPath} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	deletedAt := time.Now().UTC()
	owner := models.TestEncode{
		SourcePath: "/raw/source.mkv", LibraryID: library.ID, ConfigurationSource: "global",
		ConfigurationHash: "test", Status: testEncodeDeleted, Phase: "deleted", DeletedAt: &deletedAt,
	}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	artifact := models.TestEncodeArtifact{
		TestEncodeID: owner.ID, LibraryID: library.ID, Path: ownedPath,
		Type: models.TestEncodeArtifactMediaOutput, Expected: true, State: "owned",
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler := AssetHandler{db: db}
	router.GET("/api/test-encodes/orphans", handler.ListOrphanedTestEncodeArtifacts)
	router.DELETE("/api/test-encodes/orphans/:artifactId", handler.DeleteOrphanedTestEncodeArtifact)
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/test-encodes/orphans", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("orphan listing status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Artifacts []testEncodeOrphanArtifact `json:"artifacts"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Artifacts) != 1 || listed.Artifacts[0].ID != artifact.ID || !listed.Artifacts[0].Exists {
		t.Fatalf("unexpected proven orphan inventory: %#v", listed.Artifacts)
	}
	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/api/test-encodes/orphans/"+testEncodeIDString(artifact.ID), nil))
	if deleted.Code != http.StatusOK {
		t.Fatalf("orphan delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(ownedPath); !os.IsNotExist(err) {
		t.Fatalf("owned orphan remains: %v", err)
	}
	if _, err := os.Stat(unownedPath); err != nil {
		t.Fatalf("unowned ambiguous file was changed: %v", err)
	}
}

func TestDeleteTestEncodeAllowsExplicitStaleRecordCleanup(t *testing.T) {
	db := testEncodeTestDB(t, "delete-stale-test-encode")
	root := t.TempDir()
	library := models.Library{Name: "TV", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "interrupted-test.mkv")
	if err := os.WriteFile(output, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := models.TestEncode{
		SourcePath: "/raw/source.mkv", LibraryID: library.ID, ConfigurationSource: "global",
		ConfigurationHash: "test", Status: testEncodeRunning, Phase: "encoding", OutputPath: output,
		ReconciliationStatus: "stale",
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TestEncodeArtifact{TestEncodeID: stale.ID, LibraryID: library.ID, Path: output, Type: models.TestEncodeArtifactMediaOutput, Expected: true, State: "owned"}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.DELETE("/api/test-encodes/:id", (AssetHandler{db: db}).DeleteTestEncode)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/test-encodes/"+testEncodeIDString(stale.ID), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("stale Test Encode cleanup status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("stale owned artifact remains: %v", err)
	}
}

func testEncodeIDString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
