// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// IDSource returns fresh identifiers, e.g. from uuid7.Generator.
type IDSource func() (string, error)

// InstantiateTemplate returns a deep copy of the template's subtree in which
// every node has a fresh identifier (SCH-031), and the mapping from old to
// new identifiers so callers can rewrite references to the copied nodes.
// References to other entities — components, translations, assets, graphs —
// are kept: they name things outside the subtree.
func InstantiateTemplate(t *TemplateDocument, newID IDSource) (Node, map[string]string, error) {
	data, err := json.Marshal(t.Root)
	if err != nil {
		return Node{}, nil, fmt.Errorf("schema.InstantiateTemplate %s: %w", t.Key, err)
	}
	var root Node
	if err := json.Unmarshal(data, &root); err != nil {
		return Node{}, nil, fmt.Errorf("schema.InstantiateTemplate %s: %w", t.Key, err)
	}
	ids := map[string]string{}
	if err := renumber(&root, newID, ids); err != nil {
		return Node{}, nil, fmt.Errorf("schema.InstantiateTemplate %s: %w", t.Key, err)
	}
	return root, ids, nil
}

// renumber gives n and its descendants fresh identifiers, in document order
// with slots visited by name, so the result is deterministic for a
// deterministic source.
func renumber(n *Node, newID IDSource, ids map[string]string) error {
	if _, dup := ids[n.ID]; dup {
		return fmt.Errorf("node %s appears twice in the template", n.ID)
	}
	id, err := newID()
	if err != nil {
		return fmt.Errorf("new identifier: %w", err)
	}
	ids[n.ID] = id
	n.ID = id
	if err := renumberChildren(n, newID, ids); err != nil {
		return err
	}
	if err := renumberVariants(n, newID, ids); err != nil {
		return err
	}
	return renumberSlots(n, newID, ids)
}

// renumberChildren renumbers the children and the control arm of n.
func renumberChildren(n *Node, newID IDSource, ids map[string]string) error {
	for i := range n.Children {
		if err := renumber(&n.Children[i], newID, ids); err != nil {
			return err
		}
	}
	if n.Control != nil {
		return renumber(n.Control, newID, ids)
	}
	return nil
}

// renumberVariants renumbers the variant arms of n, by name.
func renumberVariants(n *Node, newID IDSource, ids map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(n.Variants)) {
		arm := n.Variants[name]
		if err := renumber(&arm, newID, ids); err != nil {
			return err
		}
		n.Variants[name] = arm
	}
	return nil
}

// renumberSlots renumbers the nodes that fill the slots of n, by name.
func renumberSlots(n *Node, newID IDSource, ids map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(n.Slots)) {
		fill := n.Slots[name]
		if fill.One != nil {
			if err := renumber(fill.One, newID, ids); err != nil {
				return err
			}
		}
		for i := range fill.Many {
			if err := renumber(&fill.Many[i], newID, ids); err != nil {
				return err
			}
		}
	}
	return nil
}
