package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestAssetMutationMuDirectOwnerInventory is a source-level migration tripwire.
// It records the current direct users of the process-wide lifecycle mutex so a
// future coordinator change must review every existing owner and any new owner.
// It does not assert that a future keyed coordinator exists or prove runtime
// lock behavior by itself.
func TestAssetMutationMuDirectOwnerInventory(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	actual := make([]string, 0)
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			function, ok := node.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				return true
			}

			ownsLifecycleLock := false
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Lock" {
					return true
				}
				lock, ok := selector.X.(*ast.Ident)
				if ok && lock.Name == "assetMutationMu" {
					ownsLifecycleLock = true
				}
				return true
			})
			if ownsLifecycleLock {
				actual = append(actual, filepath.Base(name)+":"+function.Name.Name)
			}
			return false
		})
	}

	want := []string{
		"asset_tracks.go:UpdateTracks",
		"assets.go:ReturnPublishedAsIs",
		"queue.go:Create",
		"queue.go:queueSelectedAssets",
		"queue_preflight.go:AcceptBatchPreflight",
		"queue_preflight.go:CreateBatchPreflight",
		"queue_preflight.go:DiscardBatchPreflight",
		"queue_preflight.go:EditBatchPreflight",
		"queue_preflight.go:PreflightSelectedAssets",
		"queue_preflight.go:RemoveBatchPreflightAsset",
		"queue_preflight.go:UpdateBatchPreflight",
		"scanner.go:Scan",
		"scanner.go:StartSnapshotOperation",
		"track_maintenance.go:StartAddAACTrack",
		"track_maintenance.go:StartTrackEdit",
		"track_maintenance.go:StartTrackRemoval",
	}
	sort.Strings(actual)
	sort.Strings(want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("direct assetMutationMu owner inventory changed\n got: %v\nwant: %v", actual, want)
	}
}
