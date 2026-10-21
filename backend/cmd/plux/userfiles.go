// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readNamedFile reads the file the developer named on the command line. The
// read goes through an os.Root on the file's directory, so the name cannot
// step out of that directory; a symbolic link that leaves it is refused.
func readNamedFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Dir(path), err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// writeNamedFile writes the file the developer named on the command line,
// confined to the file's directory like readNamedFile.
func writeNamedFile(path string, data []byte, perm os.FileMode) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Dir(path), err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(filepath.Base(path), data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// openDirectory creates a directory and its parents when they are missing,
// and opens it as an os.Root, so that nothing written through it can leave
// it.
func openDirectory(path string) (*os.Root, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}
	top := filepath.VolumeName(abs) + string(filepath.Separator)
	fsRoot, err := os.OpenRoot(top)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", top, err)
	}
	defer func() { _ = fsRoot.Close() }()
	rel := strings.TrimPrefix(abs, top)
	if rel == "" {
		rel = "."
	}
	if err := fsRoot.MkdirAll(rel, 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", abs, err)
	}
	dir, err := fsRoot.OpenRoot(rel)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", abs, err)
	}
	return dir, nil
}
