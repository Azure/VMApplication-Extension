// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type mvDirsPreference string

const (
	mvDirsPreferenceSrcDir  mvDirsPreference = "srcdir"
	mvDirsPreferenceDestDir mvDirsPreference = "destdir"
)

// moveDirsAll moves all immediate entries from srcDir into dstDir.
// On name clashes, srcdir preference replaces destination entries, while
// destdir preference keeps destination entries and skips source entries.
// Each entry is moved with os.Rename.
// removes source directory after moving all entries, if not error is encountered, regardless of preference.
func moveDirsAll(srcDir, dstDir string, preference mvDirsPreference) error {
	if preference != mvDirsPreferenceSrcDir && preference != mvDirsPreferenceDestDir {
		return fmt.Errorf("unsupported moveDirsAll preference '%s'", preference)
	}

	dstInfo, err := os.Lstat(dstDir)
	if err != nil {
		if os.IsNotExist(err) {
			return renameWithParent(srcDir, dstDir)
		}
		return err
	}

	if !dstInfo.IsDir() {
		if preference == mvDirsPreferenceDestDir {
			return os.RemoveAll(srcDir)
		}

		if err := os.RemoveAll(dstDir); err != nil {
			return err
		}

		return renameWithParent(srcDir, dstDir)
	}

	return mergeDirWithPreferenceWalk(srcDir, dstDir, preference)
}

func mergeDirWithPreferenceWalk(srcDir, dstDir string, preference mvDirsPreference) error {
	if err := filepath.WalkDir(srcDir, func(srcPath string, sourcePathInfo fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Build the destination path by applying the current relative source path to dstDir.
		rel, err := filepath.Rel(srcDir, srcPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dstPath := filepath.Join(dstDir, rel)

		if sourcePathInfo.IsDir() {
			dstInfo, err := os.Lstat(dstPath)
			if err != nil {
				if os.IsNotExist(err) {
					// Destination entry does not exist; move the source directory to destination.
					if err := renameWithParent(srcPath, dstPath); err != nil {
						return err
					}
					// The full source directory was moved, so skip walking the old subtree.
					return fs.SkipDir
				}
				return err
			}

			if dstInfo.IsDir() {
				// Both are directories; continue walking to merge contents.
				return nil
			}

			// Destination is not a directory while source is a directory; resolve by preference.
			if preference == mvDirsPreferenceDestDir {
				// Keep destination entry; skip source directory and all its children.
				return fs.SkipDir
			}

			// Source wins: remove destination entry, move full source directory, then skip old subtree.
			if err := os.RemoveAll(dstPath); err != nil {
				return err
			}
			if err := renameWithParent(srcPath, dstPath); err != nil {
				return err
			}
			return fs.SkipDir
		}

		// Source is not a directory. If destination exists, resolve by preference.
		_, err = os.Lstat(dstPath)
		if err == nil {
			if preference == mvDirsPreferenceDestDir {
				// Keep destination entry; skip source entry.
				return nil
			}
			// Source wins: remove destination entry, then move source entry.
			if err := os.RemoveAll(dstPath); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}

		// Move source entry to destination.
		return renameWithParent(srcPath, dstPath)
	}); err != nil {
		return err
	}

	return os.RemoveAll(srcDir)
}

func renameWithParent(srcPath, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		return fmt.Errorf("failed to rename '%s' to '%s': %w", srcPath, dstPath, err)
	}

	return nil
}
