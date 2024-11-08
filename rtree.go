// rtree.go
package main

import (
	"math"
	"sort"
)

const (
	maxEntries = 9
	minEntries = 4
)

// Bounds represents a bounding box
type Bounds struct {
	MinX, MinY, MaxX, MaxY float64
}

// Entry represents a geometry entry in the R-tree
type Entry struct {
	Bounds   Bounds
	Geometry interface{}
	IsLeaf   bool
	Children []*Entry
	Parent   *Entry
}

// RTree represents the root of an R-tree spatial index
type RTree struct {
	Root *Entry
	Size int
}

// NewRTree creates a new R-tree
func NewRTree() *RTree {
	return &RTree{
		Root: &Entry{IsLeaf: true},
	}
}

// Insert adds a geometry to the R-tree
func (rt *RTree) Insert(geometry interface{}, bounds Bounds) {
	rt.Size++
	entry := &Entry{
		Bounds:   bounds,
		Geometry: geometry,
		IsLeaf:   true,
	}

	if rt.Root == nil || len(rt.Root.Children) == 0 {
		if rt.Root == nil {
			rt.Root = &Entry{IsLeaf: true}
		}
		rt.Root.Children = append(rt.Root.Children, entry)
		entry.Parent = rt.Root
		rt.adjustTree(rt.Root)
		return
	}

	leaf := rt.chooseLeaf(rt.Root, entry)
	leaf.Children = append(leaf.Children, entry)
	entry.Parent = leaf
	rt.adjustTree(leaf)

	if len(leaf.Children) > maxEntries {
		rt.splitNode(leaf)
	}

	// Mark layer as dirty
	if layer, ok := geometry.(*GeometryLayer); ok {
		layer.dirty = true
	}
}

// Search finds all geometries that intersect with the given bounds
func (rt *RTree) Search(bounds Bounds) []interface{} {
	if rt.Root == nil {
		return nil
	}

	var results []interface{}
	rt.searchNode(rt.Root, bounds, &results)
	return results
}

// searchNode recursively searches for intersecting geometries
func (rt *RTree) searchNode(node *Entry, bounds Bounds, results *[]interface{}) {
	// Skip if node doesn't intersect with search bounds
	if !rt.intersects(node.Bounds, bounds) {
		return
	}

	if node.IsLeaf {
		// For leaf nodes, check each child entry
		for _, child := range node.Children {
			if rt.intersects(child.Bounds, bounds) {
				*results = append(*results, child.Geometry)
			}
		}
	} else {
		// For non-leaf nodes, recursively search all children
		for _, child := range node.Children {
			rt.searchNode(child, bounds, results)
		}
	}
}

// chooseLeaf selects the best leaf node for inserting a new entry
func (rt *RTree) chooseLeaf(node *Entry, entry *Entry) *Entry {
	if node.IsLeaf {
		return node
	}

	var minEnlargement float64 = math.MaxFloat64
	var chosen *Entry

	for _, child := range node.Children {
		enlargement := rt.enlargementNeeded(child.Bounds, entry.Bounds)
		if enlargement < minEnlargement {
			minEnlargement = enlargement
			chosen = child
		}
	}

	return rt.chooseLeaf(chosen, entry)
}

// adjustTree updates the bounding boxes and propagates splits up the tree
func (rt *RTree) adjustTree(node *Entry) {
	current := node
	for current != nil {
		if len(current.Children) > 0 {
			// Recalculate bounds from children
			bounds := current.Children[0].Bounds
			for _, child := range current.Children[1:] {
				bounds.MinX = math.Min(bounds.MinX, child.Bounds.MinX)
				bounds.MinY = math.Min(bounds.MinY, child.Bounds.MinY)
				bounds.MaxX = math.Max(bounds.MaxX, child.Bounds.MaxX)
				bounds.MaxY = math.Max(bounds.MaxY, child.Bounds.MaxY)
			}
			current.Bounds = bounds
		}
		current = current.Parent
	}
}

// splitNode handles node splitting when max entries is exceeded
func (rt *RTree) splitNode(node *Entry) {
	if len(node.Children) <= maxEntries {
		return
	}

	// Sort children by x-coordinate of center
	sort.Slice(node.Children, func(i, j int) bool {
		xi := (node.Children[i].Bounds.MinX + node.Children[i].Bounds.MaxX) / 2
		xj := (node.Children[j].Bounds.MinX + node.Children[j].Bounds.MaxX) / 2
		return xi < xj
	})

	// Split point
	splitAt := len(node.Children) / 2

	// Create new node for right half
	newNode := &Entry{
		IsLeaf:   node.IsLeaf,
		Children: make([]*Entry, len(node.Children)-splitAt),
		Parent:   node.Parent,
	}

	// Distribute children
	copy(newNode.Children, node.Children[splitAt:])
	node.Children = node.Children[:splitAt]

	// Update parent references
	for _, child := range newNode.Children {
		child.Parent = newNode
	}

	// If root was split, create new root
	if node.Parent == nil {
		rt.Root = &Entry{
			IsLeaf:   false,
			Children: []*Entry{node, newNode},
		}
		node.Parent = rt.Root
		newNode.Parent = rt.Root
	} else {
		node.Parent.Children = append(node.Parent.Children, newNode)
		if len(node.Parent.Children) > maxEntries {
			rt.splitNode(node.Parent)
		}
	}

	// Adjust bounds
	rt.adjustTree(node)
	rt.adjustTree(newNode)
}

// Helper functions

func (rt *RTree) intersects(b1, b2 Bounds) bool {
	return !(b1.MinX > b2.MaxX || b1.MaxX < b2.MinX ||
		b1.MinY > b2.MaxY || b1.MaxY < b2.MinY)
}

func (rt *RTree) enlargementNeeded(original, new Bounds) float64 {
	origArea := (original.MaxX - original.MinX) * (original.MaxY - original.MinY)
	minX := math.Min(original.MinX, new.MinX)
	minY := math.Min(original.MinY, new.MinY)
	maxX := math.Max(original.MaxX, new.MaxX)
	maxY := math.Max(original.MaxY, new.MaxY)
	newArea := (maxX - minX) * (maxY - minY)
	return newArea - origArea
}
