package handlers

import "testing"

func TestRemoteExecutorMapPath(t *testing.T) {
	executor := RemoteExecutor{
		StorageMappings: []RemoteStorageMapping{
			{
				LocalRoot:  "/media/raw",
				RemoteRoot: "/Volumes/docker/nas-media-stack/work/mediaforge/raw",
			},
			{
				LocalRoot:  "/media/staging",
				RemoteRoot: "/Volumes/docker/nas-media-stack/work/mediaforge/staging",
			},
		},
	}

	got, err := executor.MapPath(
		"/media/raw/movies/Django Unchained/movie.mkv",
	)
	if err != nil {
		t.Fatal(err)
	}

	want := "/Volumes/docker/nas-media-stack/work/mediaforge/raw/movies/Django Unchained/movie.mkv"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRemoteExecutorMapPathRejectsOutsideStorage(t *testing.T) {
	executor := RemoteExecutor{
		StorageMappings: []RemoteStorageMapping{
			{
				LocalRoot:  "/media/raw",
				RemoteRoot: "/Volumes/docker/nas-media-stack/work/mediaforge/raw",
			},
		},
	}

	_, err := executor.MapPath("/etc/passwd")
	if err == nil {
		t.Fatal("expected path mapping to fail")
	}
}

func TestRemoteExecutorMapPathPreservesSpaces(t *testing.T) {
	executor := RemoteExecutor{
		StorageMappings: []RemoteStorageMapping{
			{
				LocalRoot:  "/media/raw",
				RemoteRoot: "/Volumes/docker/nas-media-stack/work/mediaforge/raw",
			},
		},
	}

	got, err := executor.MapPath(
		"/media/raw/movies/Django Unchained/movie title.mkv",
	)
	if err != nil {
		t.Fatal(err)
	}

	want := "/Volumes/docker/nas-media-stack/work/mediaforge/raw/movies/Django Unchained/movie title.mkv"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRemoteExecutorMapPathRoot(t *testing.T) {
	executor := RemoteExecutor{
		StorageMappings: []RemoteStorageMapping{
			{
				LocalRoot:  "/media/staging",
				RemoteRoot: "/Volumes/docker/nas-media-stack/work/mediaforge/staging",
			},
		},
	}

	got, err := executor.MapPath("/media/staging")
	if err != nil {
		t.Fatal(err)
	}

	want := "/Volumes/docker/nas-media-stack/work/mediaforge/staging"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestShellQuote(t *testing.T) {
	got := shellQuote("Django's Movie.mkv")
	want := `'Django'"'"'s Movie.mkv'`

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRemoteExecutorMapPathRejectsSimilarPrefix(t *testing.T) {
	executor := RemoteExecutor{
		StorageMappings: []RemoteStorageMapping{
			{
				LocalRoot:  "/media/raw",
				RemoteRoot: "/remote/raw",
			},
		},
	}

	_, err := executor.MapPath("/media/raw-old/movie.mkv")
	if err == nil {
		t.Fatal("expected similar path prefix to be rejected")
	}
}
