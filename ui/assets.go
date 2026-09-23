package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

type AssetKind int

const (
	AssetCSS AssetKind = iota
	AssetJS
)

type assetKey struct {
	name string
	kind AssetKind
}

type registeredAsset struct {
	content []byte
	hash    string
}

var assetRegistry = map[assetKey]*registeredAsset{}

// RegisterAsset is called from a component's init() (chunk 4). Safe without
// a mutex because all writes happen during Go's single-threaded startup
// phase, before any request is served.
func RegisterAsset(name string, kind AssetKind, content []byte) {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])[:8]
	assetRegistry[assetKey{name, kind}] = &registeredAsset{content: content, hash: hash}
}

func extFor(kind AssetKind) string {
	if kind == AssetJS {
		return "js"
	}
	return "css"
}

// AssetURL builds the fingerprinted URL for a registered asset. Panics on
// a missing registration.
func AssetURL(name string, kind AssetKind) string {
	a, ok := assetRegistry[assetKey{name, kind}]
	if !ok {
		panic(fmt.Sprintf("ui: asset not registered: %s (%s)", name, extFor(kind)))
	}
	return fmt.Sprintf("/assets/%s.%s.%s", name, a.hash, extFor(kind))
}

// AssetsHandler serves a registered asset by its fingerprinted URL, e.g.
// GET /assets/card.a1b2c3d4.css
func AssetsHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/assets/")
	parts := strings.Split(path, ".")
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	name, hash, ext := parts[0], parts[1], parts[2]

	var kind AssetKind
	switch ext {
	case "css":
		kind = AssetCSS
	case "js":
		kind = AssetJS
	default:
		http.NotFound(w, r)
		return
	}

	a, ok := assetRegistry[assetKey{name, kind}]
	if !ok || a.hash != hash {
		http.NotFound(w, r)
		return
	}

	if kind == AssetCSS {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(a.content)
}

// AssetRef is what a component reports it needs, NOT the raw content —
// resolving to a URL happens later, at the point something renders <head>.
type AssetRef struct {
	Name string
	Kind AssetKind
}

// AssetProvider is deliberately optional: Text and Nodes don't implement
// it, and CollectAssets treats that as "no assets" rather than an error.
type AssetProvider interface {
	Assets() []AssetRef
}

func CollectAssets(n Node) []AssetRef {
	if ap, ok := n.(AssetProvider); ok {
		return ap.Assets()
	}
	return nil
}

// AssetRef is a small comparable struct (string + int), so it can be used
// directly as a map key here without needing a separate key type.
func DedupeAssets(refs []AssetRef) []AssetRef {
	seen := map[AssetRef]bool{}
	var out []AssetRef
	for _, r := range refs {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}
