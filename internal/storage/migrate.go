package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const breadcrumbPrefix = "MOVED_TO_"

// MigrateLegacy moves the contents of legacyDir into targetDir, leaving a
// breadcrumb file behind so the user can trace where the data went.
//
// Returns (moved, error):
//   - moved=false, err=nil: nothing to do (legacy absent, or contains only breadcrumbs)
//   - moved=true,  err=nil: migration succeeded
//   - moved=false, err!=nil: a problem prevented migration (e.g. conflict)
//
// Behavior:
//   - legacyDir absent → noop
//   - legacyDir present but contains only previous breadcrumb files → noop
//   - legacyDir present + targetDir already has content → error (conflict)
//   - legacyDir present + targetDir absent or empty → rename; on cross-fs
//     failure fall back to copy-then-remove
func MigrateLegacy(legacyDir, targetDir string) (bool, error) {
	info, err := os.Stat(legacyDir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat legacy dir: %w", err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("legacy path %s exists but is not a directory", legacyDir)
	}

	hasReal, err := hasNonBreadcrumbEntries(legacyDir)
	if err != nil {
		return false, err
	}
	if !hasReal {
		return false, nil
	}

	if targetInfo, terr := os.Stat(targetDir); terr == nil {
		if !targetInfo.IsDir() {
			return false, fmt.Errorf("target path %s exists but is not a directory", targetDir)
		}
		hasContent, herr := dirHasAnyEntry(targetDir)
		if herr != nil {
			return false, herr
		}
		if hasContent {
			return false, fmt.Errorf("both legacy %s and centralized %s exist with content; refusing to migrate automatically", legacyDir, targetDir)
		}
		// Empty target — remove it so rename can take its place.
		if err := os.Remove(targetDir); err != nil {
			return false, fmt.Errorf("remove empty target dir: %w", err)
		}
	} else if !errors.Is(terr, fs.ErrNotExist) {
		return false, fmt.Errorf("stat target dir: %w", terr)
	}

	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return false, fmt.Errorf("create parent of target: %w", err)
	}

	if rerr := os.Rename(legacyDir, targetDir); rerr != nil {
		// Same-filesystem rename failed (likely EXDEV across mount points);
		// fall back to recursive copy followed by source removal.
		if cerr := copyTree(legacyDir, targetDir); cerr != nil {
			return false, fmt.Errorf("rename %s → %s failed: %w; copy fallback also failed: %v", legacyDir, targetDir, rerr, cerr)
		}
		if remErr := os.RemoveAll(legacyDir); remErr != nil {
			return false, fmt.Errorf("copy succeeded but removing source failed: %w", remErr)
		}
	}

	// Recreate empty legacy dir and drop a breadcrumb so the user can trace
	// where the data went. Failures here are non-fatal — the migration itself
	// already succeeded.
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		slog.Warn("storage: could not recreate legacy dir for breadcrumb", "error", err)
		return true, nil
	}
	breadcrumb := filepath.Join(legacyDir, breadcrumbPrefix+filepath.Base(targetDir)+".txt")
	content := fmt.Sprintf("tgconn data moved to: %s\nat: %s\n", targetDir, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(breadcrumb, []byte(content), 0644); err != nil {
		slog.Warn("storage: could not write breadcrumb", "path", breadcrumb, "error", err)
	}
	return true, nil
}

// hasNonBreadcrumbEntries reports whether dir contains anything other than
// MOVED_TO_*.txt breadcrumb files. Used to detect "already migrated" state.
func hasNonBreadcrumbEntries(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read legacy dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return true, nil
		}
		if !strings.HasPrefix(e.Name(), breadcrumbPrefix) {
			return true, nil
		}
	}
	return false, nil
}

func dirHasAnyEntry(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read target dir: %w", err)
	}
	return len(entries) > 0, nil
}

// copyTree recursively copies src into dst. dst must not yet exist.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target)
		}
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
