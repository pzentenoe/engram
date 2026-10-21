package store

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// AllowUnsafeNetworkFilesystemEnv is the opt-in escape hatch for setups where
// the data directory can only live on a network filesystem, such as an
// institutional cluster whose home is NFS and whose account has no local
// scratch. It is off unless it is set to exactly "1": a known remote
// filesystem still fails fast by default.
const AllowUnsafeNetworkFilesystemEnv = "ENGRAM_ALLOW_UNSAFE_NFS"

// unsafeNetworkFilesystemAllowed reports whether the operator explicitly
// accepted the risk of running SQLite WAL on a network filesystem. Any other
// value, including "0" and an empty value, keeps the rejection in place.
func unsafeNetworkFilesystemAllowed() bool {
	return os.Getenv(AllowUnsafeNetworkFilesystemEnv) == "1"
}

// filesystemSupport describes what Engram can establish about a filesystem.
// Unknown is deliberately not treated as local: it preserves compatibility
// when an operating system cannot classify a filesystem type.
type filesystemSupport string

const (
	filesystemLocal   filesystemSupport = "local"
	filesystemRemote  filesystemSupport = "remote"
	filesystemUnknown filesystemSupport = "unknown"
)

type filesystemInfo struct {
	Type    string
	Support filesystemSupport
}

// NetworkFilesystemError rejects persistent SQLite WAL storage on a known
// remote filesystem. Callers must propagate it unchanged so CLI, server, and
// MCP users receive the same actionable diagnosis.
type NetworkFilesystemError struct {
	DataDir    string
	Filesystem string
}

func (e *NetworkFilesystemError) Error() string {
	return fmt.Sprintf("engram: data directory %q is on %s; persistent SQLite WAL is unsafe on network filesystems — set ENGRAM_DATA_DIR to a local directory", e.DataDir, e.Filesystem)
}

// filesystemInspector is an OS-specific read-only adapter. Tests replace it
// to prove rejection happens before any data-directory or SQLite mutation.
var filesystemInspector = detectFilesystem

// checkDataDirectoryFilesystem rejects only filesystem types the platform
// positively identifies as remote. Detection failures and unknown types remain
// compatible rather than claiming they are local.
//
// The operator can accept the risk of a network filesystem explicitly. The
// warning is emitted on every check so it reaches the CLI, the server and the
// MCP child alike, because each of them opens the store independently.
func checkDataDirectoryFilesystem(dataDir string) error {
	probePath, err := existingFilesystemPath(dataDir)
	if err != nil {
		return nil
	}
	info, err := filesystemInspector(probePath)
	if err != nil || info.Support != filesystemRemote {
		return nil
	}
	if unsafeNetworkFilesystemAllowed() {
		log.Printf("[store] WARNING: data directory %q is on %s and %s=1 is set; persistent SQLite WAL on a network filesystem risks corruption and data loss",
			dataDir, info.Type, AllowUnsafeNetworkFilesystemEnv)
		return nil
	}
	return &NetworkFilesystemError{DataDir: dataDir, Filesystem: info.Type}
}

// existingFilesystemPath finds an existing ancestor without creating the data
// directory. A missing data directory inherits its parent filesystem policy.
func existingFilesystemPath(dataDir string) (string, error) {
	path := dataDir
	for {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", os.ErrNotExist
		}
		path = parent
	}
}
