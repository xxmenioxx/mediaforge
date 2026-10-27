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

// TestAssetPathLifecycleOwnerGuardAndCallInventory characterizes path and
// lifecycle handlers that mutate assets without directly acquiring
// assetMutationMu. These routes currently use assetOverrideMutationMu for
// their filesystem/DB transition, which does not coordinate with Queue,
// scanner, or maintenance admission. This test records the current call and
// guard ownership; it does not assert that a future coordinator or fallback
// exists.
func TestAssetPathLifecycleOwnerGuardAndCallInventory(t *testing.T) {
	type ownerExpectation struct {
		function    string
		beforeLock  []string
		afterLock   []string
		hasConflict bool
	}
	want := []ownerExpectation{
		{
			function:    "assets.go:PublishAsIs",
			beforeLock:  []string{"Count"},
			afterLock:   []string{"mergeAssetDirectory", "applyDirectPublicationEpisodeNames", "saveDirectPublication"},
			hasConflict: true,
		},
		{
			function:    "assets.go:ApplyLibraryPathRename",
			beforeLock:  []string{"validateLibraryRenameHasNoActiveWork"},
			afterLock:   []string{"applyLibraryRenamePlan", "reconcileLibraryRenameState"},
			hasConflict: true,
		},
		{
			function:    "assets.go:MigratePath",
			beforeLock:  []string{"assetHasActiveWork"},
			afterLock:   []string{"moveAssetDirectory", "migrateAssetPathOverrides"},
			hasConflict: true,
		},
		{
			function:    "assets.go:Rename",
			afterLock:   []string{"assetHasActiveWork", "activeAssetMaintenance", "applyLibraryRenamePlan"},
			hasConflict: true,
		},
	}

	functions := make(map[string]*ast.FuncDecl)
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			functions[filepath.Base(name)+":"+function.Name.Name] = function
		}
	}

	for _, expected := range want {
		t.Run(expected.function, func(t *testing.T) {
			function := functions[expected.function]
			if function == nil {
				t.Fatalf("path/lifecycle owner %s was not found", expected.function)
			}
			calls := make(map[string][]token.Pos)
			selectors := make(map[string]bool)
			lifecycleLock := token.NoPos
			assetOverrideLock := token.NoPos
			assetOverrideUnlock := false
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.SelectorExpr:
					if receiver, ok := value.X.(*ast.Ident); ok {
						selectors[receiver.Name+"."+value.Sel.Name] = true
					}
				case *ast.CallExpr:
					name := ""
					switch called := value.Fun.(type) {
					case *ast.Ident:
						name = called.Name
					case *ast.SelectorExpr:
						name = called.Sel.Name
						if receiver, ok := called.X.(*ast.Ident); ok && receiver.Name == "assetMutationMu" && name == "Lock" {
							lifecycleLock = value.Pos()
						}
						if receiver, ok := called.X.(*ast.Ident); ok && receiver.Name == "assetOverrideMutationMu" {
							if name == "Lock" {
								assetOverrideLock = value.Pos()
							}
							if name == "Unlock" {
								assetOverrideUnlock = true
							}
						}
					}
					if name != "" {
						calls[name] = append(calls[name], value.Pos())
					}
				}
				return true
			})

			if lifecycleLock.IsValid() {
				t.Fatalf("%s now directly acquires assetMutationMu; update the characterization and gate disposition", expected.function)
			}
			if !assetOverrideLock.IsValid() || !assetOverrideUnlock {
				t.Fatalf("%s no longer records its assetOverrideMutationMu guard", expected.function)
			}
			if expected.hasConflict && !selectors["http.StatusConflict"] {
				t.Fatalf("%s no longer exposes an HTTP conflict outcome", expected.function)
			}
			for _, call := range expected.beforeLock {
				positions := calls[call]
				if len(positions) == 0 || positions[0] >= assetOverrideLock {
					t.Fatalf("%s must retain its pre-guard %s check", expected.function, call)
				}
			}
			for _, call := range expected.afterLock {
				positions := calls[call]
				if len(positions) == 0 || positions[0] <= assetOverrideLock {
					t.Fatalf("%s must retain its guarded %s mutation/call", expected.function, call)
				}
			}
		})
	}
}
