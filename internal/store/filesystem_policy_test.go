package store

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCheckDataDirectoryFilesystemRejectsKnownRemote(t *testing.T) {
	setFilesystemInspector(t, func(string) (filesystemInfo, error) {
		return filesystemInfo{Type: "NFS", Support: filesystemRemote}, nil
	})

	err := checkDataDirectoryFilesystem(t.TempDir())
	var rejection *NetworkFilesystemError
	if !errors.As(err, &rejection) {
		t.Fatalf("checkDataDirectoryFilesystem error = %v, want NetworkFilesystemError", err)
	}
	if rejection.Filesystem != "NFS" {
		t.Errorf("rejection filesystem = %q, want NFS", rejection.Filesystem)
	}
	if !strings.Contains(err.Error(), "ENGRAM_DATA_DIR") {
		t.Errorf("rejection does not require ENGRAM_DATA_DIR: %v", err)
	}
}

func TestCheckDataDirectoryFilesystemPreservesUnknownCompatibility(t *testing.T) {
	setFilesystemInspector(t, func(string) (filesystemInfo, error) {
		return filesystemInfo{Type: "mysteryfs", Support: filesystemUnknown}, nil
	})

	if err := checkDataDirectoryFilesystem(t.TempDir()); err != nil {
		t.Fatalf("checkDataDirectoryFilesystem unknown filesystem: %v", err)
	}
}

func TestCheckDataDirectoryFilesystemAllowsExplicitNetworkOverride(t *testing.T) {
	setFilesystemInspector(t, func(string) (filesystemInfo, error) {
		return filesystemInfo{Type: "NFS", Support: filesystemRemote}, nil
	})
	t.Setenv(AllowUnsafeNetworkFilesystemEnv, "1")

	if err := checkDataDirectoryFilesystem(t.TempDir()); err != nil {
		t.Fatalf("checkDataDirectoryFilesystem with explicit override: %v", err)
	}
}

func TestCheckDataDirectoryFilesystemKeepsRejectingWithoutExplicitOverride(t *testing.T) {
	setFilesystemInspector(t, func(string) (filesystemInfo, error) {
		return filesystemInfo{Type: "NFS", Support: filesystemRemote}, nil
	})

	for _, value := range []string{"", "0", "true", "yes", "2"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv(AllowUnsafeNetworkFilesystemEnv, value)

			err := checkDataDirectoryFilesystem(t.TempDir())
			var rejection *NetworkFilesystemError
			if !errors.As(err, &rejection) {
				t.Fatalf("checkDataDirectoryFilesystem error = %v, want NetworkFilesystemError", err)
			}
		})
	}
}

func TestCheckDataDirectoryFilesystemRejectsWhenOverrideIsUnset(t *testing.T) {
	setFilesystemInspector(t, func(string) (filesystemInfo, error) {
		return filesystemInfo{Type: "NFS", Support: filesystemRemote}, nil
	})
	t.Setenv(AllowUnsafeNetworkFilesystemEnv, "placeholder")
	if err := os.Unsetenv(AllowUnsafeNetworkFilesystemEnv); err != nil {
		t.Fatalf("unset %s: %v", AllowUnsafeNetworkFilesystemEnv, err)
	}

	var rejection *NetworkFilesystemError
	if err := checkDataDirectoryFilesystem(t.TempDir()); !errors.As(err, &rejection) {
		t.Fatalf("checkDataDirectoryFilesystem error = %v, want NetworkFilesystemError", err)
	}
}

func setFilesystemInspector(t *testing.T, inspector func(string) (filesystemInfo, error)) {
	t.Helper()
	original := filesystemInspector
	filesystemInspector = inspector
	t.Cleanup(func() { filesystemInspector = original })
}
