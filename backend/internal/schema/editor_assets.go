// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package schema

import "strings"

// editorAssetFolders are the folders under assets/ that Studio fills for itself: screen thumbnails (STU-052), native
// slot previews (WGT-031) and translation context shots (STU-134). Nothing a project builds refers to them.
var editorAssetFolders = []string{"thumbnails/", "previews/", "l10n-context/"}

// EditorOnlyAsset reports whether an asset file is one Studio stores for its own use. Such a file stays in the draft
// for Studio, but no bundle carries it: a device has no use for pictures of the editor's screens, and shipping them
// would hand every user images of screens that are not released.
func EditorOnlyAsset(file string) bool {
	for _, folder := range editorAssetFolders {
		if strings.HasPrefix(file, folder) {
			return true
		}
	}
	return false
}
