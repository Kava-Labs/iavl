package iavl

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	dbm "github.com/tendermint/tm-db"
)

// ImmutableTree contains the immutable tree at a given version. It is typically created by calling
// MutableTree.GetImmutable(), in which case the returned tree is safe for concurrent access as
// long as the version is not deleted via DeleteVersion() or the tree's pruning settings.
//
// Returned key/value byte slices must not be modified, since they may point to data located inside
// IAVL which would also be modified.
type ImmutableTree struct {
	root                   *Node
	Ndb                    *nodeDB
	version                int64
	skipFastStorageUpgrade bool
}

// NewImmutableTree creates both in-memory and persistent instances
func NewImmutableTree(db dbm.DB, cacheSize int, skipFastStorageUpgrade bool) *ImmutableTree {
	if db == nil {
		// In-memory Tree.
		return &ImmutableTree{}
	}
	return &ImmutableTree{
		// NodeDB-backed Tree.
		Ndb:                    newNodeDB(db, cacheSize, nil),
		skipFastStorageUpgrade: skipFastStorageUpgrade,
	}
}

// NewImmutableTreeWithOpts creates an ImmutableTree with the given options.
func NewImmutableTreeWithOpts(db dbm.DB, cacheSize int, opts *Options, skipFastStorageUpgrade bool) *ImmutableTree {
	return &ImmutableTree{
		// NodeDB-backed Tree.
		Ndb:                    newNodeDB(db, cacheSize, opts),
		skipFastStorageUpgrade: skipFastStorageUpgrade,
	}
}

// String returns a string representation of Tree.
func (t *ImmutableTree) String() string {
	leaves := []string{}
	t.Iterate(func(key []byte, val []byte) (stop bool) {
		leaves = append(leaves, fmt.Sprintf("%x: %x", key, val))
		return false
	})
	return "Tree{" + strings.Join(leaves, ", ") + "}"
}

// RenderShape provides a nested tree shape, ident is prepended in each level
// Returns an array of strings, one per line, to join with "\n" or display otherwise
func (t *ImmutableTree) RenderShape(indent string, encoder NodeEncoder) ([]string, error) {
	if encoder == nil {
		encoder = defaultNodeEncoder
	}
	return t.renderNode(t.root, indent, 0, encoder)
}

// NodeEncoder will take an id (hash, or key for leaf nodes), the depth of the node,
// and whether or not this is a leaf node.
// It returns the string we wish to print, for iaviwer
type NodeEncoder func(hash []byte, key []byte, depth int, isLeaf bool) string

// defaultNodeEncoder can encode any node unless the client overrides it
func defaultNodeEncoder(hash []byte, key []byte, depth int, isLeaf bool) string {
	prefix := "- "
	if isLeaf {
		prefix = "* "
	}
	if len(hash) == 0 {
		return fmt.Sprintf("%s<nil>", prefix)
	}
	return fmt.Sprintf("%s%X", prefix, hash)
}

func (t *ImmutableTree) renderNode(node *Node, indent string, depth int, encoder func([]byte, []byte, int, bool) string) ([]string, error) {
	prefix := strings.Repeat(indent, depth)
	// handle nil
	if node == nil {
		return []string{fmt.Sprintf("%s<nil>", prefix)}, nil
	}
	// handle leaf
	if node.isLeaf() {
		here := fmt.Sprintf("%s%s", prefix, encoder(node.hash, node.key, depth, true))
		return []string{here}, nil
	}

	// recurse on inner node
	here := fmt.Sprintf("%s%s", prefix, encoder(node.hash, node.key, depth, false))

	rightNode, err := node.getRightNode(t)
	if err != nil {
		return nil, err
	}

	leftNode, err := node.getLeftNode(t)
	if err != nil {
		return nil, err
	}

	right, err := t.renderNode(rightNode, indent, depth+1, encoder)
	if err != nil {
		return nil, err
	}

	result, err := t.renderNode(leftNode, indent, depth+1, encoder) // left
	if err != nil {
		return nil, err
	}

	// Left children, here, right children
	result = append(result, here)
	result = append(result, right...)
	return result, nil
}

// Size returns the number of leaf nodes in the tree.
func (t *ImmutableTree) Size() int64 {
	if t.root == nil {
		return 0
	}
	return t.root.size
}

// Version returns the version of the tree.
func (t *ImmutableTree) Version() int64 {
	return t.version
}

// Height returns the height of the tree.
func (t *ImmutableTree) Height() int8 {
	if t.root == nil {
		return 0
	}
	return t.root.height
}

// Has returns whether or not a key exists.
func (t *ImmutableTree) Has(key []byte) (bool, error) {
	if t.root == nil {
		return false, nil
	}
	return t.root.has(t, key)
}

// Hash returns the root hash.
func (t *ImmutableTree) Hash() ([]byte, error) {
	hash, _, err := t.root.hashWithCount()
	return hash, err
}

// Export returns an iterator that exports tree nodes as ExportNodes. These nodes can be
// imported with MutableTree.Import() to recreate an identical tree.
func (t *ImmutableTree) Export() (*Exporter, error) {
	return newExporter(t)
}

// GetWithIndex returns the index and value of the specified key if it exists, or nil and the next index
// otherwise. The returned value must not be modified, since it may point to data stored within
// IAVL.
//
// The index is the index in the list of leaf nodes sorted lexicographically by key. The leftmost leaf has index 0.
// It's neighbor has index 1 and so on.
func (t *ImmutableTree) GetWithIndex(key []byte) (int64, []byte, error) {
	if t.root == nil {
		return 0, nil, nil
	}
	return t.root.get(t, key)
}

// Get returns the value of the specified key if it exists, or nil.
// The returned value must not be modified, since it may point to data stored within IAVL.
// Get potentially employs a more performant strategy than GetWithIndex for retrieving the value.
// If tree.skipFastStorageUpgrade is true, this will work almost the same as GetWithIndex.
func (t *ImmutableTree) Get(key []byte) ([]byte, error) {
	if t.root == nil {
		return nil, nil
	}

	if !t.skipFastStorageUpgrade {
		// attempt to get a FastNode directly from db/cache.
		// if call fails, fall back to the original IAVL logic in place.
		fastNode, err := t.Ndb.GetFastNode(key)
		if err != nil {
			_, result, err := t.root.get(t, key)
			return result, err
		}

		if fastNode == nil {
			// If the tree is of the latest version and fast node is not in the tree
			// then the regular node is not in the tree either because fast node
			// represents live state.
			if t.version == t.Ndb.latestVersion {
				return nil, nil
			}

			_, result, err := t.root.get(t, key)
			return result, err
		}

		if fastNode.versionLastUpdatedAt <= t.version {
			return fastNode.value, nil
		}
	}

	// otherwise skipFastStorageUpgrade is true or
	// the cached node was updated later than the current tree. In this case,
	// we need to use the regular stategy for reading from the current tree to avoid staleness.
	_, result, err := t.root.get(t, key)
	return result, err
}

// GetByIndex gets the key and value at the specified index.
func (t *ImmutableTree) GetByIndex(index int64) (key []byte, value []byte, err error) {
	if t.root == nil {
		return nil, nil, nil
	}

	return t.root.getByIndex(t, index)
}

// Iterate iterates over all keys of the tree. The keys and values must not be modified,
// since they may point to data stored within IAVL. Returns true if stopped by callback, false otherwise
func (t *ImmutableTree) Iterate(fn func(key []byte, value []byte) bool) (bool, error) {
	if t.root == nil {
		return false, nil
	}

	itr, err := t.Iterator(nil, nil, true)
	defer itr.Close()
	if err != nil {
		return false, err
	}
	for ; itr.Valid(); itr.Next() {
		if fn(itr.Key(), itr.Value()) {
			return true, nil
		}

	}
	return false, nil
}

// Iterator returns an iterator over the immutable tree.
func (t *ImmutableTree) Iterator(start, end []byte, ascending bool) (dbm.Iterator, error) {

	return NewIterator(start, end, ascending, t), nil
}

// IterateRange makes a callback for all nodes with key between start and end non-inclusive.
// If either are nil, then it is open on that side (nil, nil is the same as Iterate). The keys and
// values must not be modified, since they may point to data stored within IAVL.
func (t *ImmutableTree) IterateRange(start, end []byte, ascending bool, fn func(key []byte, value []byte, hash []byte, isLeaf bool) bool) (stopped bool) {
	if t.root == nil {
		return false
	}
	return t.root.traverseInRange(t, start, end, ascending, false, false, func(node *Node) bool {
		targetHash1, err := hex.DecodeString("C7FC20871082A1A1C32BB5854E0EEDEAFB4DC8F0D018AB8499899DCB871A0EAA")
		if err != nil {
			panic(err)
		}

		targetHash2, err := hex.DecodeString("56F0D956C4DD26A11D6BA8B27B4F006F5D9904F4BBCDD7B5977ED47143FE3E12")
		if err != nil {
			panic(err)
		}

		keysEqual := bytes.Equal(node.key, targetHash1) || bytes.Equal(node.key, targetHash2)
		hashesEqual := bytes.Equal(node.hash, targetHash1) || bytes.Equal(node.hash, targetHash2)

		if keysEqual || hashesEqual {
			fmt.Printf("\n\nFOUND %X!\n", node.hash)

			node.traverse(t, true, func(n *Node) bool {
				fmt.Printf("[isLeaf: %v] Node: %x -> %x\n", n.isLeaf(), n.key, n.value)
				fmt.Printf("Hash: %x, Version: %d\n", n.hash, n.version)
				return false
			})

			fmt.Print("===\n\n")
		}

		if node.height == 0 {
			return fn(node.key, node.value, node.hash, true)
		} else {
			return fn(node.key, node.value, node.hash, false)
		}

		return false
	})
}

// IterateRangeInclusive makes a callback for all nodes with key between start and end inclusive.
// If either are nil, then it is open on that side (nil, nil is the same as Iterate). The keys and
// values must not be modified, since they may point to data stored within IAVL.
func (t *ImmutableTree) IterateRangeInclusive(start, end []byte, ascending bool, fn func(key, value []byte, version int64) bool) (stopped bool) {
	if t.root == nil {
		return false
	}
	return t.root.traverseInRange(t, start, end, ascending, true, false, func(node *Node) bool {
		if node.height == 0 {
			return fn(node.key, node.value, node.version)
		}
		return false
	})
}

// IsFastCacheEnabled returns true if fast cache is enabled, false otherwise.
// For fast cache to be enabled, the following 2 conditions must be met:
// 1. The tree is of the latest version.
// 2. The underlying storage has been upgraded to fast cache
func (t *ImmutableTree) IsFastCacheEnabled() (bool, error) {
	isLatestTreeVersion, err := t.isLatestTreeVersion()
	if err != nil {
		return false, err
	}
	return isLatestTreeVersion && t.Ndb.hasUpgradedToFastStorage(), nil
}

func (t *ImmutableTree) isLatestTreeVersion() (bool, error) {
	latestVersion, err := t.Ndb.getLatestVersion()
	if err != nil {
		return false, err
	}
	return t.version == latestVersion, nil
}

// Clone creates a clone of the tree.
// Used internally by MutableTree.
func (t *ImmutableTree) clone() *ImmutableTree {
	return &ImmutableTree{
		root:                   t.root,
		Ndb:                    t.Ndb,
		version:                t.version,
		skipFastStorageUpgrade: t.skipFastStorageUpgrade,
	}
}

// nodeSize is like Size, but includes inner nodes too.
//
//nolint:unused
func (t *ImmutableTree) nodeSize() int {
	size := 0
	t.root.traverse(t, true, func(n *Node) bool {
		size++
		return false
	})
	return size
}

// TraverseStateChanges iterate the range of versions, compare each version to it's predecessor to extract the state changes of it.
// endVersion is exclusive.
func (t *ImmutableTree) TraverseStateChanges(startVersion, endVersion int64, fn func(version int64, changeSet *ChangeSet) error) error {
	return t.Ndb.traverseStateChanges(startVersion, endVersion, fn)
}
