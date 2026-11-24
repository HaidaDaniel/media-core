package artwork

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Cache bounds. A NAS with a small disk can still hold hundreds of covers;
// older files are evicted only when they are not referenced by a saved row.
const (
	DefaultMaxCacheBytes int64 = 64 << 20
	DefaultMaxImageBytes int64 = 2 << 20
	MaxImageDimension          = 3000
	cacheDirName               = "artwork-cache"
	cacheImageExt              = "img"
)

var (
	ErrCacheRef    = errors.New("artwork: invalid cache reference")
	ErrCacheSize   = errors.New("artwork: image exceeds size limit")
	ErrCacheFormat = errors.New("artwork: unsupported image format")
)

var cacheRefPattern = regexp.MustCompile(`^[a-f0-9]{64}\.img$`)

// Cache stores validated image bytes in the state directory. It is a cache,
// not user data: deleting it never removes a selection from SQLite.
type Cache struct {
	dir      string
	maxBytes int64
	maxImage int64
}

func NewCache(stateDir string, maxBytes, maxImage int64) (*Cache, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, fmt.Errorf("artwork: state dir is required")
	}
	if maxBytes <= 0 || maxImage <= 0 || maxImage > maxBytes {
		return nil, fmt.Errorf("artwork: invalid cache limits")
	}
	return &Cache{dir: filepath.Join(stateDir, cacheDirName), maxBytes: maxBytes, maxImage: maxImage}, nil
}

// Ref is a stable, opaque name derived from the provider's source URL plus
// the provider namespace. It contains no user input path components.
func Ref(provider, sourceURL string) string {
	sum := sha256.Sum256([]byte(provider + "\x00" + sourceURL))
	return hex.EncodeToString(sum[:]) + "." + cacheImageExt
}

func (c *Cache) path(ref string) (string, error) {
	if !cacheRefPattern.MatchString(ref) {
		return "", ErrCacheRef
	}
	return filepath.Join(c.dir, ref), nil
}

// validateImage checks the declared MIME and image header before any bytes are
// written. It decodes only the header (no full image processing) and refuses
// dimensions that could be used as a decompression bomb.
func (c *Cache) validateImage(data []byte, mime string) (string, error) {
	if int64(len(data)) == 0 || int64(len(data)) > c.maxImage {
		return "", ErrCacheSize
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCacheFormat, err)
	}
	if format != "jpeg" && format != "png" && format != "gif" {
		return "", fmt.Errorf("%w: %s", ErrCacheFormat, format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageDimension || cfg.Height > MaxImageDimension {
		return "", ErrCacheSize
	}
	if int64(cfg.Width)*int64(cfg.Height) > 9_000_000 {
		return "", ErrCacheSize
	}
	expected := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	if mime != "" && !strings.Contains(strings.ToLower(mime), expected) {
		return "", fmt.Errorf("%w: %s is not %s", ErrCacheFormat, mime, expected)
	}
	return expected, nil
}

// Put atomically stores a validated image and evicts old unreferenced files
// until the cache is within its byte budget.
func (c *Cache) Put(ref string, data []byte, mime string, protected map[string]bool) (int64, error) {
	path, err := c.path(ref)
	if err != nil {
		return 0, err
	}
	cleanMIME, err := c.validateImage(data, mime)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return 0, fmt.Errorf("artwork: create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return 0, fmt.Errorf("artwork: create temp image: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return 0, err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return 0, fmt.Errorf("artwork: write image: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return 0, fmt.Errorf("artwork: publish image: %w", err)
	}
	if err := c.evict(protected); err != nil {
		return 0, err
	}
	_ = cleanMIME
	return int64(len(data)), nil
}

// Open returns the cached bytes for a stored reference.
func (c *Cache) Open(ref string) ([]byte, string, error) {
	path, err := c.path(ref)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", fmt.Errorf("artwork: read cached image: %w", err)
	}
	mime, err := c.validateImage(data, "")
	if err != nil {
		return nil, "", err
	}
	return data, mime, nil
}

type cacheEntry struct {
	name string
	info os.FileInfo
}

// evict removes the oldest unreferenced files until the cache fits. The
// protected set comes from the database, so a saved selection is never
// evicted by a newer search.
func (c *Cache) evict(protected map[string]bool) error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("artwork: list cache: %w", err)
	}
	list := make([]cacheEntry, 0, len(entries))
	var total int64
	for _, e := range entries {
		if e.IsDir() || !cacheRefPattern.MatchString(e.Name()) || protected[e.Name()] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		list = append(list, cacheEntry{name: e.Name(), info: info})
		total += info.Size()
	}
	if total <= c.maxBytes {
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].info.ModTime().Before(list[j].info.ModTime()) })
	for _, e := range list {
		if total <= c.maxBytes {
			break
		}
		if err := os.Remove(filepath.Join(c.dir, e.name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("artwork: evict cache file: %w", err)
		}
		total -= e.info.Size()
	}
	return nil
}

// Clear deletes every cached image. Selection rows and provenance in SQLite
// remain intact; a later explicit selection rebuilds the cache.
func (c *Cache) Clear() (int, int64, error) {
	entries, err := os.ReadDir(c.dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("artwork: list cache for clear: %w", err)
	}
	count, bytes := 0, int64(0)
	for _, e := range entries {
		if e.IsDir() || !cacheRefPattern.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return count, bytes, err
		}
		if err := os.Remove(filepath.Join(c.dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return count, bytes, fmt.Errorf("artwork: remove cache file: %w", err)
		}
		count++
		bytes += info.Size()
	}
	return count, bytes, nil
}

// Stats reports current cache usage, ignoring temporary files.
func (c *Cache) Stats() (int, int64, error) {
	entries, err := os.ReadDir(c.dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	count, bytes := 0, int64(0)
	for _, e := range entries {
		if e.IsDir() || !cacheRefPattern.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return count, bytes, err
		}
		count++
		bytes += info.Size()
	}
	return count, bytes, nil
}

// Bounds exposes the configured limits for operator diagnostics.
func (c *Cache) Bounds() (int64, int64) { return c.maxBytes, c.maxImage }
