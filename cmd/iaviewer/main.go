package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	dbm "github.com/tendermint/tm-db"

	"github.com/cosmos/iavl"
	ibytes "github.com/cosmos/iavl/internal/bytes"
)

// TODO: make this configurable?
const (
	DefaultCacheSize int = 10000
)

func isValidOption(opt string) bool {
	options := []string{"data", "shape", "versions", "latest", "get", "hash"}

	for _, o := range options {
		if o == opt {
			return true
		}
	}

	return false
}

func main() {
	args := os.Args[1:]

	fmt.Printf("len(args): %v\n", len(args))
	fmt.Printf("option: %v\n", args[0])

	if len(args) < 3 || !isValidOption(args[0]) {
		fmt.Fprintln(os.Stderr, "Usage: iaviewer <data|shape|versions> <leveldb dir> <prefix> [version number]")
		fmt.Fprintln(os.Stderr, "<prefix> is the prefix of db, and the iavl tree of different modules in cosmos-sdk uses ")
		fmt.Fprintln(os.Stderr, "different <prefix> to identify, just like \"s/k:gov/\" represents the prefix of gov module")
		os.Exit(1)
	}

	version := 0
	if len(args) >= 4 && args[0] != "latest" {
		var err error
		version, err = strconv.Atoi(args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid version number: %s\n", err)
			os.Exit(1)
		}
	}

	tree, err := ReadTree(args[1], version, []byte(args[2]))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading data: %s\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "data":
		PrintKeys(tree)
		hash, err := tree.Hash()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error hashing tree: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("Hash: %X\n", hash)
		fmt.Printf("Size: %X\n", tree.Size())
	case "shape":
		// PrintShapeAtNode(tree, "E8D1DEB84CCC8777F093A815B256D35F12DC1EF2362E0A597E56D8C2265C6590")
		PrintShape(tree)
	case "versions":
		PrintVersions(tree)
	case "hash":
		PrintHash(tree)
	case "latest":
		PrintLatestVersions(tree)
	case "get":
		if len(args) < 5 {
			fmt.Fprintln(os.Stderr, "Usage: iaviewer get <leveldb dir> <prefix> <version number> <key>")
			os.Exit(1)
		}

		keyStr := args[4]

		// Parse keyStr as hex to bytes
		key, err := hex.DecodeString(keyStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding key: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("Getting key: %X\n", key)

		value, err := tree.Get(key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting value: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("Value: %X\n", value)

		fmt.Printf("Iterating over range\n")

		// Start from one key before the key we're looking for
		start := key[:len(key)-1]
		// End at one key after the key we're looking for
		end := key
		end[len(end)-1]++

		tree.IterateRange(
			start,
			end,
			true,
			func(key, value, hash []byte, isLeaf bool) bool {
				if isLeaf {
					fmt.Printf("[LEAF] Key: %X, Value: %X, Hash: %X\n", key, value, hash)
				} else {
					fmt.Printf("[BRANCH] Key: %X, Value: %X, Hash: %X, \n", key, value, hash)
				}

				// Continue iterating
				return false
			},
		)

	}
}

func OpenDB(dir string) (dbm.DB, error) {
	switch {
	case strings.HasSuffix(dir, ".db"):
		dir = dir[:len(dir)-3]
	case strings.HasSuffix(dir, ".db/"):
		dir = dir[:len(dir)-4]
	default:
		return nil, fmt.Errorf("database directory must end with .db")
	}
	// TODO: doesn't work on windows!
	cut := strings.LastIndex(dir, "/")
	if cut == -1 {
		return nil, fmt.Errorf("cannot cut paths on %s", dir)
	}
	name := dir[cut+1:]
	// db, err := dbm.NewRocksDB(name, dir[:cut])
	db, err := dbm.NewGoLevelDB(name, dir[:cut])
	if err != nil {
		return nil, err
	}
	return db, nil
}

// nolint: deadcode
func PrintDBStats(db dbm.DB) {
	count := 0
	prefix := map[string]int{}
	itr, err := db.Iterator(nil, nil)
	if err != nil {
		panic(err)
	}

	defer itr.Close()
	for ; itr.Valid(); itr.Next() {
		key := ibytes.UnsafeBytesToStr(itr.Key()[:1])
		prefix[key]++
		count++
	}
	if err := itr.Error(); err != nil {
		panic(err)
	}
	fmt.Printf("DB contains %d entries\n", count)
	for k, v := range prefix {
		fmt.Printf("  %s: %d\n", k, v)
	}
}

// ReadTree loads an iavl tree from the directory
// If version is 0, load latest, otherwise, load named version
// The prefix represents which iavl tree you want to read. The iaviwer will always set a prefix.
func ReadTree(dir string, version int, prefix []byte) (*iavl.MutableTree, error) {
	fmt.Printf("Reading tree from %s\n", dir)
	db, err := OpenDB(dir)
	if err != nil {
		return nil, err
	}
	if len(prefix) != 0 {
		fmt.Printf("Setting prefix: %s\n", prefix)
		db = dbm.NewPrefixDB(db, prefix)
	}

	fmt.Printf("Creating new mutable tree\n")
	tree, err := iavl.NewMutableTree(db, DefaultCacheSize, false)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Loading version %d\n", version)
	ver, err := tree.LoadVersion(int64(version))
	fmt.Printf("Got version: %d\n", ver)
	return tree, err
}

func PrintKeys(tree *iavl.MutableTree) {
	fmt.Println("Printing all keys with hashed values (to detect diff)")
	tree.Iterate(func(key []byte, value []byte) bool {
		printKey := parseWeaveKey(key)
		digest := sha256.Sum256(value)
		fmt.Printf("  %s\n    %X\n", printKey, digest)
		return false
	})
}

// parseWeaveKey assumes a separating : where all in front should be ascii,
// and all afterwards may be ascii or binary
func parseWeaveKey(key []byte) string {
	cut := bytes.IndexRune(key, ':')
	if cut == -1 {
		return encodeID(key)
	}
	prefix := key[:cut]
	id := key[cut+1:]
	return fmt.Sprintf("%s:%s", encodeID(prefix), encodeID(id))
}

// casts to a string if it is printable ascii, hex-encodes otherwise
func encodeID(id []byte) string {
	for _, b := range id {
		if b < 0x20 || b >= 0x80 {
			return strings.ToUpper(hex.EncodeToString(id))
		}
	}
	return string(id)
}

func ReverseParseWeaveKey(keyStr string) ([]byte, error) {
	// Check if the string is hex-encoded
	if isHexEncoded(keyStr) {
		// If it is, decode it
		return hex.DecodeString(keyStr)
	}
	// If it's not hex-encoded, return the string as is
	return []byte(keyStr), nil
}

// Helper function to check if a string is hex-encoded
func isHexEncoded(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

func PrintShape(tree *iavl.MutableTree) {
	// shape := tree.RenderShape("  ", nil)
	//TODO: handle this error
	shape, _ := tree.RenderShape("  ", nodeEncoder)
	fmt.Println(strings.Join(shape, "\n"))
}

func PrintShapeAtNode(tree *iavl.MutableTree, targetHash string) {
	iterator, err := tree.Iterator(nil, nil, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting iterator: %s\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "size: %v\n", tree.Size())

	iter := iterator.(*iavl.Iterator)
	count := 0

	for ; iter.Valid(); iter.Next() {
		count++
		if count%1000 == 0 {
			fmt.Fprintf(os.Stderr, "\rOn %d", count)
		}

		if parseWeaveKey(iter.Hash()) == targetHash {
			break
		}

		fmt.Printf("Weave: Key: %v\t Hash: %v\n", parseWeaveKey(iter.Key()), parseWeaveKey(iter.Hash()))
		fmt.Printf("Raw: Key: %x\t Hash: %x\n", iter.Key(), iter.Hash())
	}

	shape, _ := tree.RenderShape("  ", nodeEncoder)
	fmt.Println(strings.Join(shape, "\n"))
}

func nodeEncoder(hash []byte, key []byte, depth int, isLeaf bool) string {
	prefix := fmt.Sprintf("-%d ", depth)
	if isLeaf {
		prefix = fmt.Sprintf("*%d ", depth)
	}
	if len(hash) == 0 {
		return fmt.Sprintf("%s<nil>", prefix)
	}

	if isLeaf {
		return fmt.Sprintf("%s%s", prefix, parseWeaveKey(key))
	}

	return fmt.Sprintf("%s%s (key: %s)", prefix, parseWeaveKey(hash), parseWeaveKey(key))
}

func PrintVersions(tree *iavl.MutableTree) {
	versions := tree.AvailableVersions()
	fmt.Println("Available versions:")
	for _, v := range versions {
		fmt.Printf("  %d\n", v)
	}
}

func PrintHash(tree *iavl.MutableTree) {
	hash, err := tree.Hash()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting hash of the latest saved version of the tree: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Hash: %X\n", hash)
}

func PrintLatestVersions(tree *iavl.MutableTree) {
	version, err := tree.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading latest version: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Latest Version: %v\n", version)
}
