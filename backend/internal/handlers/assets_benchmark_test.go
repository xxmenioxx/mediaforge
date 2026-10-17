package handlers

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func BenchmarkArchiveDeleteIdentityResolution(b *testing.B) {
	for _, catalogSize := range []int{10, 1000} {
		b.Run(fmt.Sprintf("catalog_%d", catalogSize), func(b *testing.B) {
			db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(b, fmt.Sprintf("delete-archive-benchmark-%d", catalogSize))
			archivePath := filepath.Join(archiveRoot, "show", "episode.mkv")
			writeTestFile(b, archivePath, "archived original")
			_, libraryAsset, _, _ := seedContextualArchivedOriginal(b, db, rawRoot, libraryRoot, archiveRoot, "show/episode.mkv", archivePath)

			records := make([]models.AssetRecord, catalogSize)
			for index := range records {
				path := filepath.Join(rawRoot, "unrelated", fmt.Sprintf("asset-%05d.mkv", index))
				records[index] = models.AssetRecord{Path: path, RootPath: rawRoot, RelativePath: filepath.ToSlash(filepath.Join("unrelated", filepath.Base(path))), FileName: filepath.Base(path), Status: "unprocessed"}
			}
			if err := db.CreateInBatches(&records, 250).Error; err != nil {
				b.Fatal(err)
			}
			jobs := make([]models.QueueJob, len(records))
			for index := range records {
				sourceID := records[index].ID
				jobs[index] = models.QueueJob{MediaPath: records[index].Path, SourceAssetRecordID: &sourceID, Status: JobStatusFailed}
			}
			if err := db.CreateInBatches(&jobs, 250).Error; err != nil {
				b.Fatal(err)
			}

			handler := AssetHandler{db: db}
			b.Run("global_catalog_projection", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					var allRecords []models.AssetRecord
					if err := db.Find(&allRecords).Error; err != nil {
						b.Fatal(err)
					}
					if _, err := handler.assetLifecycleProjections(allRecords); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("scoped_current_asset_projection", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if _, _, err := handler.assetLifecycleProjectionForArchiveDelete(libraryAsset); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
