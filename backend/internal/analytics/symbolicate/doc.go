// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package symbolicate turns the node path of a runtime error into the place
// in the source documents that produced the node (ANL-040, CMP-041).
//
// The runtime reports a node as "plugin/pageKey/index": the plugin, the key
// of the page that was rendering, and the node's index in that page's flat
// node array (BND-015). A release bundle holds no source map, so the server
// keeps the map the compiler returned next to the bundle (SRV-051). The
// bundle's meta section lists the pages by key and section identifier, and
// the source map lists, for each section and node index, the document file
// and JSON pointer the node was compiled from. Resolve joins the two.
//
// Both inputs are content-addressed and verified by the caller before they
// are passed in; the package neither reads storage nor parses a bundle
// beyond its directory and meta section. A path that cannot be resolved,
// because the page or node is not in the map, is reported as not found,
// never as a guess.
//
// Nodes nested in a component instance continue the path with further
// indices. The map of a plugin names the instance node in the page, so the
// extra segments are ignored and the instance is what is found: the node a
// developer opens in Studio is the instance that contains the error.
package symbolicate
