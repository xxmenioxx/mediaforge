package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These types intentionally live in a _test.go file. They characterize a
// possible coordination protocol; production handlers do not use this lock.
type issue415PrototypeIdentity struct {
	key      string
	path     string
	resolved bool
}

func (i issue415PrototypeIdentity) sameBinding(other issue415PrototypeIdentity) bool {
	if i.resolved != other.resolved {
		return false
	}
	if i.resolved {
		return i.key == other.key && i.path == other.path
	}
	return i.path == other.path
}

type issue415PrototypeRequest struct {
	keys     []string
	fallback bool
	queued   chan struct{}
	granted  chan struct{}
	active   bool
}

type issue415PrototypeCoordinator struct {
	mu        sync.Mutex
	pending   []*issue415PrototypeRequest
	active    map[string]bool
	refs      map[string]int
	exclusive bool
}

type issue415PrototypeLease struct {
	coordinator *issue415PrototypeCoordinator
	request     *issue415PrototypeRequest
	once        sync.Once
}

func newIssue415PrototypeCoordinator() *issue415PrototypeCoordinator {
	return &issue415PrototypeCoordinator{active: make(map[string]bool), refs: make(map[string]int)}
}

func (c *issue415PrototypeCoordinator) enqueue(identities ...issue415PrototypeIdentity) *issue415PrototypeRequest {
	r := &issue415PrototypeRequest{queued: make(chan struct{}), granted: make(chan struct{})}
	if len(identities) == 0 {
		r.fallback = true
	}
	for _, identity := range identities {
		if !identity.resolved || identity.key == "" {
			r.fallback = true
			continue
		}
		r.keys = append(r.keys, identity.key)
	}
	if r.fallback {
		r.keys = nil
	} else {
		sort.Strings(r.keys)
		r.keys = uniqueIssue415Keys(r.keys)
	}

	c.mu.Lock()
	for _, key := range r.keys {
		c.refs[key]++
	}
	c.pending = append(c.pending, r)
	close(r.queued)
	c.pumpLocked()
	c.mu.Unlock()
	return r
}

func uniqueIssue415Keys(keys []string) []string {
	if len(keys) < 2 {
		return keys
	}
	unique := keys[:1]
	for _, key := range keys[1:] {
		if key != unique[len(unique)-1] {
			unique = append(unique, key)
		}
	}
	return unique
}

func (c *issue415PrototypeCoordinator) acquire(ctx context.Context, identities ...issue415PrototypeIdentity) (*issue415PrototypeLease, error) {
	return c.wait(ctx, c.enqueue(identities...))
}

func (c *issue415PrototypeCoordinator) wait(ctx context.Context, r *issue415PrototypeRequest) (*issue415PrototypeLease, error) {
	if err := ctx.Err(); err != nil {
		c.cancelRequest(r)
		return nil, err
	}
	select {
	case <-r.granted:
		if err := ctx.Err(); err != nil {
			c.cancelRequest(r)
			return nil, err
		}
		return &issue415PrototypeLease{coordinator: c, request: r}, nil
	case <-ctx.Done():
		c.cancelRequest(r)
		return nil, ctx.Err()
	}
}

func (c *issue415PrototypeCoordinator) cancelRequest(r *issue415PrototypeRequest) {
	c.mu.Lock()
	if r.active {
		c.releaseLocked(r)
	} else {
		for index, pending := range c.pending {
			if pending == r {
				c.pending = append(c.pending[:index], c.pending[index+1:]...)
				c.dropRefsLocked(r)
				break
			}
		}
	}
	c.pumpLocked()
	c.mu.Unlock()
}

func (l *issue415PrototypeLease) release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		c := l.coordinator
		c.mu.Lock()
		c.releaseLocked(l.request)
		c.pumpLocked()
		c.mu.Unlock()
	})
}

func (c *issue415PrototypeCoordinator) releaseLocked(r *issue415PrototypeRequest) {
	if !r.active {
		return
	}
	r.active = false
	if r.fallback {
		c.exclusive = false
		return
	}
	for _, key := range r.keys {
		delete(c.active, key)
	}
	c.dropRefsLocked(r)
}

func (c *issue415PrototypeCoordinator) dropRefsLocked(r *issue415PrototypeRequest) {
	for _, key := range r.keys {
		if c.refs[key] <= 1 {
			delete(c.refs, key)
		} else {
			c.refs[key]--
		}
	}
}

func (c *issue415PrototypeCoordinator) pumpLocked() {
	remaining := make([]*issue415PrototypeRequest, 0, len(c.pending))
	for _, request := range c.pending {
		if c.conflictsWithActiveLocked(request) || conflictsWithEarlierIssue415Request(remaining, request) {
			remaining = append(remaining, request)
			continue
		}
		request.active = true
		if request.fallback {
			c.exclusive = true
		} else {
			for _, key := range request.keys {
				c.active[key] = true
			}
		}
		close(request.granted)
	}
	c.pending = remaining
}

func (c *issue415PrototypeCoordinator) conflictsWithActiveLocked(request *issue415PrototypeRequest) bool {
	if c.exclusive {
		return true
	}
	if request.fallback {
		return len(c.active) != 0
	}
	for _, key := range request.keys {
		if c.active[key] {
			return true
		}
	}
	return false
}

func conflictsWithEarlierIssue415Request(earlier []*issue415PrototypeRequest, current *issue415PrototypeRequest) bool {
	for _, request := range earlier {
		if request.fallback || current.fallback {
			return true
		}
		left, right := 0, 0
		for left < len(request.keys) && right < len(current.keys) {
			switch strings.Compare(request.keys[left], current.keys[right]) {
			case -1:
				left++
			case 1:
				right++
			default:
				return true
			}
		}
	}
	return false
}

type issue415PrototypeState struct {
	ID            uint `gorm:"primaryKey"`
	AssetRecordID uint `gorm:"uniqueIndex"`
	Value         string
	FrozenValue   string
	ReservedBy    string
}

func (issue415PrototypeState) TableName() string { return "issue415_prototype_state" }

func newIssue415PrototypeDB(t testing.TB, name string) (*gorm.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), name+".sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.AssetRecord{}, &issue415PrototypeState{}); err != nil {
		t.Fatal(err)
	}
	absoluteDBPath, err := filepath.Abs(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return db, filepath.Clean(absoluteDBPath)
}

func createIssue415PrototypeAsset(t testing.TB, db *gorm.DB, path string, value string) models.AssetRecord {
	t.Helper()
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	asset := models.AssetRecord{Path: filepath.Clean(absolutePath), FileName: filepath.Base(path), Status: "library"}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&issue415PrototypeState{AssetRecordID: asset.ID, Value: value}).Error; err != nil {
		t.Fatal(err)
	}
	return asset
}

// The resolver is deliberately conservative and test-only. A path gets a key
// only when one current AssetRecord row names it and no other indexed path is
// an alias of the same filesystem object. Unknowns use the exclusive gate.
func resolveIssue415PrototypeIdentity(db *gorm.DB, dbNamespace string, path string) issue415PrototypeIdentity {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return issue415PrototypeIdentity{path: filepath.Clean(path)}
	}
	absolutePath = filepath.Clean(absolutePath)
	identity := issue415PrototypeIdentity{path: absolutePath}
	var record models.AssetRecord
	if err := db.Where("path = ?", absolutePath).First(&record).Error; err != nil || record.ID == 0 || record.Missing {
		return identity
	}
	requestedInfo, err := os.Stat(absolutePath)
	if err != nil {
		return identity
	}
	var indexed []models.AssetRecord
	if err := db.Find(&indexed).Error; err != nil {
		return identity
	}
	for _, other := range indexed {
		if other.ID == record.ID {
			continue
		}
		otherInfo, statErr := os.Stat(other.Path)
		if statErr != nil {
			return identity
		}
		if os.SameFile(requestedInfo, otherInfo) {
			return identity
		}
	}
	identity.key = fmt.Sprintf("%s\x00%d", dbNamespace, record.ID)
	identity.resolved = true
	return identity
}

func assertIssue415NotGranted(t *testing.T, request *issue415PrototypeRequest) {
	t.Helper()
	select {
	case <-request.granted:
		t.Fatal("request was granted while a conflicting lease was held")
	default:
	}
}

func TestIssue415PrototypeIdentityUsesSQLiteBindingAndExclusiveFallbackForAliases(t *testing.T) {
	db, namespace := newIssue415PrototypeDB(t, "identity")
	root := t.TempDir()
	canonicalPath := filepath.Join(root, "canonical.mkv")
	aliasPath := filepath.Join(root, "alias.mkv")
	if err := os.WriteFile(canonicalPath, []byte("synthetic media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonicalPath, aliasPath); err != nil {
		t.Fatal(err)
	}
	createIssue415PrototypeAsset(t, db, canonicalPath, "a")
	createIssue415PrototypeAsset(t, db, aliasPath, "b")
	canonical := resolveIssue415PrototypeIdentity(db, namespace, canonicalPath)
	alias := resolveIssue415PrototypeIdentity(db, namespace, aliasPath)
	if canonical.resolved || alias.resolved {
		t.Fatalf("two indexed aliases for one filesystem object must fail closed: canonical=%+v alias=%+v", canonical, alias)
	}
	if canonical.path == alias.path {
		t.Fatal("distinct alias paths must retain separate diagnostic paths while sharing exclusive fallback")
	}

	unindexed := filepath.Join(root, "unindexed.mkv")
	if err := os.WriteFile(unindexed, []byte("unindexed"), 0o600); err != nil {
		t.Fatal(err)
	}
	unindexedIdentity := resolveIssue415PrototypeIdentity(db, namespace, unindexed)
	if unindexedIdentity.resolved {
		t.Fatalf("unindexed file unexpectedly received a canonical identity: %+v", unindexedIdentity)
	}

	coordinator := newIssue415PrototypeCoordinator()
	keyed := issue415PrototypeIdentity{key: "db\x001", path: "/known.mkv", resolved: true}
	keyedLease, err := coordinator.acquire(context.Background(), keyed)
	if err != nil {
		t.Fatal(err)
	}
	fallbackRequest := coordinator.enqueue(canonical)
	assertIssue415NotGranted(t, fallbackRequest)
	keyedLease.release()
	fallbackLease, err := coordinator.wait(context.Background(), fallbackRequest)
	if err != nil {
		t.Fatal(err)
	}
	blockedKeyedRequest := coordinator.enqueue(keyed)
	assertIssue415NotGranted(t, blockedKeyedRequest)
	fallbackLease.release()
	keyedLease, err = coordinator.wait(context.Background(), blockedKeyedRequest)
	if err != nil {
		t.Fatal(err)
	}
	keyedLease.release()
}

func TestIssue415PrototypeSameAssetSerializationAndIndependentOverlap(t *testing.T) {
	coordinator := newIssue415PrototypeCoordinator()
	a := issue415PrototypeIdentity{key: "db\x001", path: "/a", resolved: true}
	b := issue415PrototypeIdentity{key: "db\x002", path: "/b", resolved: true}
	firstA, err := coordinator.acquire(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	queuedA := coordinator.enqueue(a)
	assertIssue415NotGranted(t, queuedA)
	leaseB, err := coordinator.acquire(context.Background(), b)
	if err != nil {
		t.Fatalf("independent asset was serialized behind asset A: %v", err)
	}
	firstA.release()
	secondA, err := coordinator.wait(context.Background(), queuedA)
	if err != nil {
		t.Fatal(err)
	}
	secondA.release()
	leaseB.release()
}

func TestIssue415PrototypeCancellationRemovesQueuedRequestAndReleasesLease(t *testing.T) {
	coordinator := newIssue415PrototypeCoordinator()
	identity := issue415PrototypeIdentity{key: "db\x001", path: "/a", resolved: true}
	first, err := coordinator.acquire(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	request := coordinator.enqueue(identity)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.wait(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error=%v, want context.Canceled", err)
	}
	first.release()
	coordinator.mu.Lock()
	refsAfterRelease := len(coordinator.refs)
	coordinator.mu.Unlock()
	if refsAfterRelease != 0 {
		t.Fatalf("canceled and released requests left keyed references: %+v", coordinator.refs)
	}
	next, err := coordinator.acquire(context.Background(), identity)
	if err != nil {
		t.Fatalf("canceled request left a stale lock reference: %v", err)
	}
	next.release()
}

func TestIssue415PrototypeMultiAssetSortsAtomicallyAndRollsBack(t *testing.T) {
	coordinator := newIssue415PrototypeCoordinator()
	a := issue415PrototypeIdentity{key: "db\x001", path: "/a", resolved: true}
	b := issue415PrototypeIdentity{key: "db\x002", path: "/b", resolved: true}
	blockB, err := coordinator.acquire(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	multi := coordinator.enqueue(b, a)
	if got := strings.Join(multi.keys, ","); got != "db\x001,db\x002" {
		t.Fatalf("multi-asset acquisition order=%q, want sorted keys", got)
	}
	assertIssue415NotGranted(t, multi)
	// If acquisition were incremental, A would be held while B is unavailable.
	coordinator.mu.Lock()
	partiallyReservedA := coordinator.active[a.key]
	coordinator.mu.Unlock()
	if partiallyReservedA {
		t.Fatal("multi-asset request partially reserved A before B was available")
	}
	blockB.release()
	multiLease, err := coordinator.wait(context.Background(), multi)
	if err != nil {
		t.Fatal(err)
	}

	db, _ := newIssue415PrototypeDB(t, "rollback")
	if err := db.Create(&issue415PrototypeState{AssetRecordID: 1, Value: "before-a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&issue415PrototypeState{AssetRecordID: 2, Value: "before-b"}).Error; err != nil {
		t.Fatal(err)
	}
	rollbackErr := errors.New("injected synthetic transaction failure")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", 1).Update("value", "partial-a").Error; err != nil {
			return err
		}
		if err := tx.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", 2).Update("value", "partial-b").Error; err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("transaction error=%v, want injected rollback", err)
	}
	var rows []issue415PrototypeState
	if err := db.Order("asset_record_id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Value != "before-a" || rows[1].Value != "before-b" {
		t.Fatalf("failed multi-asset transaction left partial rows: %+v", rows)
	}
	multiLease.release()

	firstOrder, err := coordinator.acquire(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	reversed := coordinator.enqueue(b, a)
	assertIssue415NotGranted(t, reversed)
	firstOrder.release()
	reversedLease, err := coordinator.wait(context.Background(), reversed)
	if err != nil {
		t.Fatalf("reversed acquisition deadlocked after sorted release: %v", err)
	}
	reversedLease.release()
}

func TestIssue415PrototypeQueueFreezeAndSaveTracksHaveOneOrder(t *testing.T) {
	db, namespace := newIssue415PrototypeDB(t, "queue-freeze")
	root := t.TempDir()
	path := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	asset := createIssue415PrototypeAsset(t, db, path, "before")
	identity := resolveIssue415PrototypeIdentity(db, namespace, path)
	if !identity.resolved {
		t.Fatalf("single indexed fixture should resolve: %+v", identity)
	}
	coordinator := newIssue415PrototypeCoordinator()

	queueLease, err := coordinator.acquire(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", asset.ID).Update("frozen_value", "before").Error; err != nil {
		t.Fatal(err)
	}
	saveRequest := coordinator.enqueue(identity)
	assertIssue415NotGranted(t, saveRequest)
	queueLease.release()
	saveLease, err := coordinator.wait(context.Background(), saveRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", asset.ID).Update("value", "after").Error; err != nil {
		t.Fatal(err)
	}
	saveLease.release()
	var state issue415PrototypeState
	if err := db.Where("asset_record_id = ?", asset.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.FrozenValue != "before" || state.Value != "after" {
		t.Fatalf("Queue snapshot mixed Save Tracks state: frozen=%q current=%q", state.FrozenValue, state.Value)
	}

	// The opposite lock order yields a snapshot of the committed Save Tracks value.
	if err := db.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", asset.ID).Updates(map[string]any{"value": "saved-first", "frozen_value": ""}).Error; err != nil {
		t.Fatal(err)
	}
	saveLease, err = coordinator.acquire(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", asset.ID).Update("value", "saved-first-committed").Error; err != nil {
		t.Fatal(err)
	}
	saveLease.release()
	queueLease, err = coordinator.acquire(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", asset.ID).Update("frozen_value", "saved-first-committed").Error; err != nil {
		t.Fatal(err)
	}
	queueLease.release()
	if err := db.Where("asset_record_id = ?", asset.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.FrozenValue != "saved-first-committed" {
		t.Fatalf("Queue freeze failed to observe committed Save Tracks state: %+v", state)
	}
}

func TestIssue415PrototypeScannerAndMaintenanceReservationSerializeWithSaveTracks(t *testing.T) {
	for _, owner := range []string{"scanner", "maintenance"} {
		t.Run(owner, func(t *testing.T) {
			db, namespace := newIssue415PrototypeDB(t, owner)
			path := filepath.Join(t.TempDir(), "asset.mkv")
			if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
				t.Fatal(err)
			}
			asset := createIssue415PrototypeAsset(t, db, path, "before")
			identity := resolveIssue415PrototypeIdentity(db, namespace, path)
			if !identity.resolved {
				t.Fatalf("indexed fixture should resolve: %+v", identity)
			}
			coordinator := newIssue415PrototypeCoordinator()
			reservationLease, err := coordinator.acquire(context.Background(), identity)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", asset.ID).Update("reserved_by", owner).Error; err != nil {
				t.Fatal(err)
			}
			reservationLease.release()
			tracksLease, err := coordinator.acquire(context.Background(), identity)
			if err != nil {
				t.Fatal(err)
			}
			var state issue415PrototypeState
			if err := db.Where("asset_record_id = ?", asset.ID).First(&state).Error; err != nil {
				t.Fatal(err)
			}
			if state.ReservedBy != owner {
				t.Fatalf("reservation marker lost: got %q, want %q", state.ReservedBy, owner)
			}
			saveApplied := false
			if state.ReservedBy == "" {
				if err := db.Model(&issue415PrototypeState{}).Where("asset_record_id = ?", asset.ID).Update("value", "after").Error; err != nil {
					t.Fatal(err)
				}
				saveApplied = true
			}
			if saveApplied {
				t.Fatalf("Save Tracks applied during active %s reservation", owner)
			}
			tracksLease.release()
			if err := db.Where("asset_record_id = ?", asset.ID).First(&state).Error; err != nil {
				t.Fatal(err)
			}
			if state.Value != "before" {
				t.Fatalf("Save Tracks crossed active %s reservation: value=%q", owner, state.Value)
			}
		})
	}
}

func TestIssue415PrototypeRenameAndMigrationUseExclusiveIdentityTransition(t *testing.T) {
	root := t.TempDir()
	dbA, namespaceA := newIssue415PrototypeDB(t, "source")
	dbB, namespaceB := newIssue415PrototypeDB(t, "destination")
	oldPath := filepath.Join(root, "old.mkv")
	newPath := filepath.Join(root, "new.mkv")
	if err := os.WriteFile(oldPath, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	assetA := createIssue415PrototypeAsset(t, dbA, oldPath, "source")
	oldIdentity := resolveIssue415PrototypeIdentity(dbA, namespaceA, oldPath)
	coordinator := newIssue415PrototypeCoordinator()
	transition, err := coordinator.acquire(context.Background(), issue415PrototypeIdentity{path: oldPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if err := dbA.Model(&models.AssetRecord{}).Where("id = ?", assetA.ID).Update("path", newPath).Error; err != nil {
		t.Fatal(err)
	}
	newIdentity := resolveIssue415PrototypeIdentity(dbA, namespaceA, newPath)
	if !oldIdentity.resolved || !newIdentity.resolved || oldIdentity.key != newIdentity.key {
		t.Fatalf("same-database rename must retain validated AssetRecord identity: old=%+v new=%+v", oldIdentity, newIdentity)
	}
	oldPathAfterRename := resolveIssue415PrototypeIdentity(dbA, namespaceA, oldPath)
	if oldIdentity.sameBinding(oldPathAfterRename) || oldIdentity.sameBinding(newIdentity) {
		t.Fatalf("rename must invalidate a path-bound identity snapshot: before=%+v oldPathNow=%+v newPath=%+v", oldIdentity, oldPathAfterRename, newIdentity)
	}
	newPathRequest := coordinator.enqueue(newIdentity)
	assertIssue415NotGranted(t, newPathRequest)
	transition.release()
	newPathLease, err := coordinator.wait(context.Background(), newPathRequest)
	if err != nil {
		t.Fatal(err)
	}
	newPathLease.release()

	// Migration to a different DB namespace runs under the same exclusive
	// transition gate; a reused numeric ID is not treated as identity continuity.
	migration, err := coordinator.acquire(context.Background(), issue415PrototypeIdentity{path: newPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := dbA.Delete(&models.AssetRecord{}, assetA.ID).Error; err != nil {
		t.Fatal(err)
	}
	assetB := createIssue415PrototypeAsset(t, dbB, newPath, "destination")
	newDBIdentity := resolveIssue415PrototypeIdentity(dbB, namespaceB, newPath)
	if !newDBIdentity.resolved {
		t.Fatalf("destination database fixture should resolve: %+v", newDBIdentity)
	}
	if oldIdentity.key == newDBIdentity.key || assetA.ID != assetB.ID {
		t.Fatalf("database namespace must distinguish reused IDs: sourceID=%d destinationID=%d sourceKey=%q destinationKey=%q", assetA.ID, assetB.ID, oldIdentity.key, newDBIdentity.key)
	}
	sourceAfterMigration := resolveIssue415PrototypeIdentity(dbA, namespaceA, newPath)
	if sourceAfterMigration.resolved || oldIdentity.sameBinding(sourceAfterMigration) {
		t.Fatalf("source DB binding remained authorized after migration: old=%+v source-now=%+v", oldIdentity, sourceAfterMigration)
	}
	if oldIdentity.sameBinding(newDBIdentity) {
		t.Fatal("migration across database namespaces must not inherit a source identity binding")
	}
	pendingDestination := coordinator.enqueue(newDBIdentity)
	assertIssue415NotGranted(t, pendingDestination)
	migration.release()
	destinationLease, err := coordinator.wait(context.Background(), pendingDestination)
	if err != nil {
		t.Fatal(err)
	}
	destinationLease.release()
}
