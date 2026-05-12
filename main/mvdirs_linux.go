// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"io"
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
// Each entry is moved with os.Rename first, then falls back to copy+remove
// when rename cannot be used (for example, cross-device boundaries).
// removes source directory after moving all entries, if not error is encountered, regardless of preference.
func moveDirsAll(srcDir, dstDir string, preference mvDirsPreference) error {
	if preference != mvDirsPreferenceSrcDir && preference != mvDirsPreferenceDestDir {
		return fmt.Errorf("unsupported moveDirsAll preference '%s'", preference)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if err := movePathWithPreference(srcPath, dstPath, preference); err != nil {
			return err
		}
	}

	return os.RemoveAll(srcDir)
}

func movePathWithPreference(srcPath, dstPath string, preference mvDirsPreference) error {
	_, err := os.Lstat(dstPath)
	destinationExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if destinationExists {
		if preference == mvDirsPreferenceDestDir {
			return nil
		}

		if err := os.RemoveAll(dstPath); err != nil {
			return err
		}
	}

	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	}

	info, err := os.Lstat(srcPath)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(srcPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(target, dstPath); err != nil {
			return err
		}
		return os.RemoveAll(srcPath)
	}

	if info.IsDir() {
		if err := copyDirOverwrite(srcPath, dstPath); err != nil {
			return err
		}
		return os.RemoveAll(srcPath)
	}

	if err := copyFileOverwrite(srcPath, dstPath, info.Mode().Perm()); err != nil {
		return err
	}

	return os.RemoveAll(srcPath)
}

// copyDirOverwrite copies src into dst recursively and overwrites collisions.
// Existing files are replaced; existing directories are reused.
func copyDirOverwrite(srcDir, dstDir string) error {
	srcDir = filepath.Clean(srcDir)
	dstDir = filepath.Clean(dstDir)

	return filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcDir, srcPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dstDir, 0o755)
		}

		dstPath := filepath.Join(dstDir, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}

		// Handle symlinks explicitly.
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			_ = os.RemoveAll(dstPath)
			return os.Symlink(target, dstPath)
		}

		if d.IsDir() {
			// If a non-dir exists at dstPath, remove it so dir can be created.
			if dstInfo, err := os.Lstat(dstPath); err == nil && !dstInfo.IsDir() {
				if err := os.RemoveAll(dstPath); err != nil {
					return err
				}
			}
			return os.MkdirAll(dstPath, info.Mode().Perm())
		}

		return copyFileOverwrite(srcPath, dstPath, info.Mode().Perm())
	})
}

func copyFileOverwrite(srcPath, dstPath string, mode os.FileMode) error {
	// If a directory exists where file should go, remove it first.
	if dstInfo, err := os.Lstat(dstPath); err == nil && dstInfo.IsDir() {
		if err := os.RemoveAll(dstPath); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
