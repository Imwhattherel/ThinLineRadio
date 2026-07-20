// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	mapBaseTileCacheTTL  = 7 * 24 * time.Hour
	mapRadarTileCacheTTL = 5 * time.Minute
	mapTileFetchTimeout  = 20 * time.Second
)

var radarFrameIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,48}$`)

var (
	mapTileHTTPClient = &http.Client{
		Timeout: mapTileFetchTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        128,
			MaxIdleConnsPerHost: 32,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	mapTileFlight singleflight.Group
)

type mapTileRequest struct {
	Style   string
	FrameID string
	Z       int
	X       int
	Y       int
}

func mapTileCacheRoot() string {
	return filepath.Join(".", ".tile-cache", "map")
}

func parseMapTilePath(path string) (mapTileRequest, bool) {
	rest := strings.TrimPrefix(path, "/api/map/tiles/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return mapTileRequest{}, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 4 {
		return mapTileRequest{}, false
	}

	parseY := func(raw string) (int, bool) {
		raw = strings.TrimSuffix(strings.ToLower(raw), ".png")
		y, err := strconv.Atoi(raw)
		return y, err == nil
	}

	style := strings.ToLower(strings.TrimSpace(parts[0]))
	switch style {
	case "voyager", "dark", "satellite":
		if len(parts) != 4 {
			return mapTileRequest{}, false
		}
		z, err1 := strconv.Atoi(parts[1])
		x, err2 := strconv.Atoi(parts[2])
		y, ok := parseY(parts[3])
		if err1 != nil || err2 != nil || !ok {
			return mapTileRequest{}, false
		}
		return mapTileRequest{Style: style, Z: z, X: x, Y: y}, true
	case "radar":
		if len(parts) != 5 {
			return mapTileRequest{}, false
		}
		frameID := parts[1]
		if !radarFrameIDRe.MatchString(frameID) {
			return mapTileRequest{}, false
		}
		z, err1 := strconv.Atoi(parts[2])
		x, err2 := strconv.Atoi(parts[3])
		y, ok := parseY(parts[4])
		if err1 != nil || err2 != nil || !ok {
			return mapTileRequest{}, false
		}
		return mapTileRequest{Style: style, FrameID: frameID, Z: z, X: x, Y: y}, true
	default:
		return mapTileRequest{}, false
	}
}

func validMapTileCoords(style string, z, x, y int) bool {
	maxZoom := 18
	if style == "satellite" {
		maxZoom = 19
	}
	if z < 0 || z > maxZoom {
		return false
	}
	limit := 1 << uint(z)
	return x >= 0 && x < limit && y >= 0 && y < limit
}

func mapTileUpstreamURL(req mapTileRequest) (string, bool) {
	switch req.Style {
	case "voyager":
		return fmt.Sprintf("https://a.basemaps.cartocdn.com/rastertiles/voyager/%d/%d/%d.png", req.Z, req.X, req.Y), true
	case "dark":
		return fmt.Sprintf("https://a.basemaps.cartocdn.com/dark_all/%d/%d/%d.png", req.Z, req.X, req.Y), true
	case "satellite":
		return fmt.Sprintf("https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/%d/%d/%d", req.Z, req.Y, req.X), true
	case "radar":
		return fmt.Sprintf("https://mesonet.agron.iastate.edu/cache/tile.py/1.0.0/nexrad-n0q-%s/%d/%d/%d.png", req.FrameID, req.Z, req.X, req.Y), true
	default:
		return "", false
	}
}

func mapTileCachePath(req mapTileRequest) string {
	if req.Style == "radar" {
		return filepath.Join(mapTileCacheRoot(), "radar", req.FrameID, strconv.Itoa(req.Z), strconv.Itoa(req.X), fmt.Sprintf("%d.png", req.Y))
	}
	return filepath.Join(mapTileCacheRoot(), req.Style, strconv.Itoa(req.Z), strconv.Itoa(req.X), fmt.Sprintf("%d.png", req.Y))
}

func mapTileCacheTTL(style string) time.Duration {
	if style == "radar" {
		return mapRadarTileCacheTTL
	}
	return mapBaseTileCacheTTL
}

func mapTileCacheFresh(path string, ttl time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return time.Since(info.ModTime()) <= ttl
}

func serveCachedMapTile(w http.ResponseWriter, r *http.Request, path string, cacheStatus string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-TLR-Tile-Cache", cacheStatus)
	http.ServeFile(w, r, path)
	return true
}

func writeCachedMapTile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type mapTileFetchResult struct {
	body        []byte
	contentType string
}

func fetchMapTileUpstream(url string) (mapTileFetchResult, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return mapTileFetchResult{}, err
	}
	req.Header.Set("User-Agent", "ThinlineRadio/1.0 (map-tile-proxy)")
	resp, err := mapTileHTTPClient.Do(req)
	if err != nil {
		return mapTileFetchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return mapTileFetchResult{}, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	const maxTileBytes = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTileBytes))
	if err != nil {
		return mapTileFetchResult{}, err
	}
	if len(body) == 0 {
		return mapTileFetchResult{}, fmt.Errorf("empty tile")
	}
	ctype := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if ctype == "" {
		ctype = "image/png"
	}
	return mapTileFetchResult{body: body, contentType: ctype}, nil
}

func fetchMapTileDeduped(cacheKey, upstream string) (mapTileFetchResult, error) {
	v, err, _ := mapTileFlight.Do(cacheKey, func() (any, error) {
		return fetchMapTileUpstream(upstream)
	})
	if err != nil {
		return mapTileFetchResult{}, err
	}
	return v.(mapTileFetchResult), nil
}

// MapTilesHandler proxies basemap and radar tiles with on-disk caching.
// GET /api/map/tiles/{style}/{z}/{x}/{y}.png
// GET /api/map/tiles/radar/{frameId}/{z}/{x}/{y}.png
func (api *Api) MapTilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.exitWithError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	req, ok := parseMapTilePath(r.URL.Path)
	if !ok || !validMapTileCoords(req.Style, req.Z, req.X, req.Y) {
		http.NotFound(w, r)
		return
	}
	upstream, ok := mapTileUpstreamURL(req)
	if !ok {
		http.NotFound(w, r)
		return
	}

	cachePath := mapTileCachePath(req)
	ttl := mapTileCacheTTL(req.Style)
	if mapTileCacheFresh(cachePath, ttl) {
		if serveCachedMapTile(w, r, cachePath, "HIT") {
			return
		}
	}

	result, err := fetchMapTileDeduped(cachePath, upstream)
	if err != nil {
		if serveCachedMapTile(w, r, cachePath, "STALE") {
			return
		}
		api.exitWithError(w, http.StatusBadGateway, "tile fetch failed")
		return
	}

	body := append([]byte(nil), result.body...)
	go func(path string, data []byte) {
		_ = writeCachedMapTile(path, data)
	}(cachePath, body)

	w.Header().Set("Content-Type", result.contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-TLR-Tile-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
