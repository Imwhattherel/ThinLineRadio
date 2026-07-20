// Copyright (C) 2025 Thinline Dynamic Solutions
//
// mapping_boundaries_import.go — download Census TIGER cartographic boundaries
// (counties, places, townships) for US-wide incident map overlays.

package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"rdio-scanner/server/mapping"
)

const (
	defaultBoundaryCacheDir = ".tiger-cache/boundaries"
	boundaryMaxDownloadBytes = 96 << 20
)

// ImportBoundariesOptions controls a boundary layer download/import job.
type ImportBoundariesOptions struct {
	StateFIPS       []string
	Layers          []mapping.BoundaryLayer
	ReplaceExisting bool
	CacheDir        string
	Year            int
}

func (ms *MappingStore) ImportBoundaries(opts ImportBoundariesOptions) (map[string]int, error) {
	states := normalizeStateFIPSList(opts.StateFIPS)
	if len(states) == 0 {
		return nil, fmt.Errorf("at least one state FIPS code required")
	}
	layers := normalizeBoundaryLayers(opts.Layers)
	if len(layers) == 0 {
		return nil, fmt.Errorf("at least one layer required (county, place, cousub)")
	}
	year := opts.Year
	if year <= 0 {
		year = mapping.TigerBoundaryYear
	}
	cacheRoot := strings.TrimSpace(opts.CacheDir)
	if cacheRoot == "" {
		cacheRoot = defaultBoundaryCacheDir
	}

	counts := map[string]int{"features": 0}
	for _, state := range states {
		for _, layer := range layers {
			if opts.ReplaceExisting {
				if _, err := ms.db.Sql.Exec(`DELETE FROM "mappingBoundaries" WHERE "stateFips" = $1 AND "layerType" = $2`, state, string(layer)); err != nil {
					return counts, err
				}
			}
			n, err := ms.importBoundaryStateLayer(cacheRoot, state, layer, year)
			if err != nil {
				return counts, fmt.Errorf("%s %s: %w", state, layer, err)
			}
			counts["features"] += n
			counts[string(layer)] += n
		}
	}
	return counts, nil
}

func (ms *MappingStore) importBoundaryStateLayer(cacheRoot, stateFIPS string, layer mapping.BoundaryLayer, year int) (int, error) {
	spec := mapping.TigerBoundaryDownloadSpec(stateFIPS, layer, year)
	dir := filepath.Join(cacheRoot, spec.CacheDirName(layer))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	shapePrefix, err := ensureBoundaryExtract(dir, spec)
	if err != nil {
		return 0, err
	}
	features, err := mapping.ParseTigerBoundaryShapefile(shapePrefix, layer, spec.FilterStateFIPS)
	if err != nil {
		return 0, err
	}
	if len(features) == 0 {
		return 0, fmt.Errorf("no features parsed")
	}

	now := time.Now().UnixMilli()
	tx, err := ms.db.Sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	inserted := 0
	for _, f := range features {
		_, err := tx.Exec(`INSERT INTO "mappingBoundaries"
			("geoid","name","stateFips","layerType","geometry","minLat","minLon","maxLat","maxLon","centroidLat","centroidLon","colorIndex","updatedAt")
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT ("layerType","geoid") DO UPDATE SET
			"name" = EXCLUDED."name", "stateFips" = EXCLUDED."stateFips", "geometry" = EXCLUDED."geometry",
			"minLat" = EXCLUDED."minLat", "minLon" = EXCLUDED."minLon", "maxLat" = EXCLUDED."maxLat", "maxLon" = EXCLUDED."maxLon",
			"centroidLat" = EXCLUDED."centroidLat", "centroidLon" = EXCLUDED."centroidLon", "colorIndex" = EXCLUDED."colorIndex", "updatedAt" = EXCLUDED."updatedAt"`,
			f.GEOID, f.Name, f.StateFIPS, string(f.Layer), string(f.Geometry),
			f.MinLat, f.MinLon, f.MaxLat, f.MaxLon, f.CentroidLat, f.CentroidLon, f.ColorIndex, now)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, tx.Commit()
}

func ensureBoundaryExtract(dir string, spec mapping.BoundaryDownloadSpec) (string, error) {
	shapePrefix := filepath.Join(dir, spec.ShapeBase)
	if _, err := os.Stat(shapePrefix + ".shp"); err == nil {
		return shapePrefix, nil
	}
	zipPath := filepath.Join(dir, spec.ShapeBase+".zip")
	if err := downloadTigerZipWithLimit(spec.URL, zipPath, boundaryMaxDownloadBytes); err != nil {
		return "", err
	}
	if err := unzipTigerFile(zipPath, dir); err != nil {
		return "", err
	}
	if _, err := os.Stat(shapePrefix + ".shp"); err != nil {
		return "", fmt.Errorf("shapefile missing after extract: %s.shp", spec.ShapeBase)
	}
	return shapePrefix, nil
}

// unzipTigerFile extracts every entry in a Census TIGER shapefile zip
// (.shp/.shx/.dbf/.prj) into destDir, flattening any internal directory
// structure. Shared by the boundary-layer importer above.
func unzipTigerFile(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		outPath := filepath.Join(destDir, name)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func downloadTigerZipWithLimit(url, dest string, maxBytes int64) error {
	resp, err := httpGet(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
}

func normalizeStateFIPSList(states []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range states {
		s = strings.TrimSpace(s)
		if len(s) == 1 {
			s = "0" + s
		}
		if len(s) != 2 || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func normalizeBoundaryLayers(layers []mapping.BoundaryLayer) []mapping.BoundaryLayer {
	seen := map[mapping.BoundaryLayer]bool{}
	var out []mapping.BoundaryLayer
	for _, l := range layers {
		if !l.Valid() || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

func (ms *MappingStore) BoundaryStats() map[string]any {
	stats := map[string]any{"total": 0, "byLayer": map[string]int{}, "byState": map[string]int{}}
	rows, err := ms.db.Sql.Query(`SELECT "layerType", "stateFips", COUNT(*) FROM "mappingBoundaries" GROUP BY "layerType", "stateFips"`)
	if err != nil {
		return stats
	}
	defer rows.Close()
	byLayer := map[string]int{}
	byState := map[string]int{}
	total := 0
	for rows.Next() {
		var layer, state string
		var n int
		if rows.Scan(&layer, &state, &n) != nil {
			continue
		}
		total += n
		byLayer[layer] += n
		byState[state] += n
	}
	stats["total"] = total
	stats["byLayer"] = byLayer
	stats["byState"] = byState
	return stats
}

func (ms *MappingStore) DeleteAllBoundaries() error {
	_, err := ms.db.Sql.Exec(`DELETE FROM "mappingBoundaries"`)
	return err
}

// spokenLocalityBoundaryMaxMi bounds how far from home coverage a spoken
// community may sit; mutual aid goes to neighbors, not across the state.
const spokenLocalityBoundaryMaxMi = 50.0

// BoundaryCentroidForLocality resolves a dispatch-spoken community name to the
// nearest imported place/township boundary centroid within
// spokenLocalityBoundaryMaxMi of the home coverage center. Returns the
// centroid plus a radius derived from the boundary's bounding box.
func (ms *MappingStore) BoundaryCentroidForLocality(name string, homeLat, homeLon float64) (lat, lon, radiusMi float64, ok bool) {
	n := strings.ToUpper(strings.TrimSpace(name))
	if n == "" || (homeLat == 0 && homeLon == 0) {
		return 0, 0, 0, false
	}
	rows, err := ms.db.Sql.Query(
		`SELECT "centroidLat", "centroidLon", "minLat", "minLon", "maxLat", "maxLon"
		 FROM "mappingBoundaries"
		 WHERE "layerType" IN ('place','cousub')
		   AND (UPPER("name") = $1 OR UPPER("name") IN ($1 || ' TOWNSHIP', $1 || ' VILLAGE', $1 || ' CITY'))`,
		n,
	)
	if err != nil {
		return 0, 0, 0, false
	}
	defer rows.Close()
	bestDist := spokenLocalityBoundaryMaxMi
	for rows.Next() {
		var cLat, cLon, minLat, minLon, maxLat, maxLon float64
		if rows.Scan(&cLat, &cLon, &minLat, &minLon, &maxLat, &maxLon) != nil {
			continue
		}
		if cLat == 0 && cLon == 0 {
			continue
		}
		d := milesBetween(homeLat, homeLon, cLat, cLon)
		if d >= bestDist {
			continue
		}
		bestDist = d
		lat, lon = cLat, cLon
		radiusMi = milesBetween(cLat, cLon, maxLat, maxLon)
		if radiusMi < 3 {
			radiusMi = 3
		}
		ok = true
	}
	return lat, lon, radiusMi, ok
}

func (ms *MappingStore) QueryBoundariesBBox(west, south, east, north float64, layers []string, limit int) ([]map[string]any, error) {
	if west > east {
		west, east = east, west
	}
	if south > north {
		south, north = north, south
	}
	layerFilter := normalizeBoundaryLayerStrings(layers)
	if len(layerFilter) == 0 {
		layerFilter = []string{"county", "place", "cousub"}
	}
	perLayerLimit := map[string]int{
		"county": 500,
		"cousub": 2500,
		"place":  2500,
	}
	var out []map[string]any
	for _, layer := range layerFilter {
		n := perLayerLimit[layer]
		if n <= 0 {
			n = 500
		}
		if limit > 0 && n > limit {
			n = limit
		}
		part, err := ms.queryBoundariesBBoxLayer(west, south, east, north, layer, n)
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func (ms *MappingStore) queryBoundariesBBoxLayer(west, south, east, north float64, layer string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 500
	}
	query := `SELECT "geoid", "name", "layerType", "colorIndex", "geometry"
		FROM "mappingBoundaries"
		WHERE "minLat" <= $2 AND "maxLat" >= $1 AND "minLon" <= $4 AND "maxLon" >= $3
		AND "layerType" = $5
		ORDER BY "name"
		LIMIT $6`
	rows, err := ms.db.Sql.Query(query, south, north, west, east, layer, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var geoid, name, layerType, geom string
		var colorIndex int
		if rows.Scan(&geoid, &name, &layerType, &colorIndex, &geom) != nil {
			continue
		}
		out = append(out, map[string]any{
			"type": "Feature",
			"properties": map[string]any{
				"geoid": geoid, "name": name, "layer": layerType, "colorIndex": colorIndex,
			},
			"geometry": parseBoundaryGeometryJSON(geom),
		})
	}
	return out, nil
}

func parseBoundaryGeometryJSON(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return map[string]any{}
	}
	return v
}

func httpGet(url string) (*http.Response, error) {
	return http.Get(url)
}

// boundaryImportState tracks background boundary import progress for admin UI.
type boundaryImportState struct {
	Active    bool           `json:"active"`
	Phase     string         `json:"phase"`
	Message   string         `json:"message"`
	Total     int            `json:"total"`
	Completed int            `json:"completed"`
	Percent   int            `json:"percent"`
	Done      bool           `json:"done"`
	Error     string         `json:"error"`
	Counts    map[string]int `json:"counts"`
}

type boundaryImportManager struct {
	mu    sync.Mutex
	state boundaryImportState
}

var globalBoundaryImport = &boundaryImportManager{}

func (m *boundaryImportManager) snapshot() boundaryImportState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *boundaryImportManager) start(store *MappingStore, controller *Controller, opts ImportBoundariesOptions) error {
	m.mu.Lock()
	if m.state.Active {
		m.mu.Unlock()
		return fmt.Errorf("a boundary import is already in progress")
	}
	states := normalizeStateFIPSList(opts.StateFIPS)
	layers := normalizeBoundaryLayers(opts.Layers)
	total := len(states) * len(layers)
	m.state = boundaryImportState{
		Active: true, Phase: "download", Total: total, Message: "Starting…",
	}
	m.mu.Unlock()

	go func() {
		counts := map[string]int{"features": 0}
		done := 0
		for _, state := range states {
			for _, layer := range layers {
				m.mu.Lock()
				m.state.Phase = "download"
				m.state.Message = fmt.Sprintf("Downloading %s %s…", stateNameForFIPS(state), layerLabel(layer))
				m.mu.Unlock()

				if opts.ReplaceExisting && done == 0 {
					// Only bulk-delete matching rows once at start per state/layer inside ImportBoundaries
				}

				n, err := func() (int, error) {
					m.mu.Lock()
					m.state.Phase = "import"
					m.state.Message = fmt.Sprintf("Importing %s %s…", stateNameForFIPS(state), layerLabel(layer))
					m.mu.Unlock()

					subOpts := ImportBoundariesOptions{
						StateFIPS:       []string{state},
						Layers:          []mapping.BoundaryLayer{layer},
						ReplaceExisting: opts.ReplaceExisting,
						CacheDir:        opts.CacheDir,
						Year:            opts.Year,
					}
					c, err := store.ImportBoundaries(subOpts)
					if err != nil {
						return 0, err
					}
					for k, v := range c {
						counts[k] += v
					}
					return c["features"], nil
				}()
				done++
				m.mu.Lock()
				m.state.Completed = done
				if m.state.Total > 0 {
					m.state.Percent = done * 100 / m.state.Total
				}
				m.mu.Unlock()
				if err != nil {
					m.mu.Lock()
					m.state.Active = false
					m.state.Done = true
					m.state.Error = err.Error()
					m.mu.Unlock()
					return
				}
				_ = n
			}
		}
		m.mu.Lock()
		m.state.Active = false
		m.state.Done = true
		m.state.Phase = "done"
		m.state.Percent = 100
		m.state.Message = fmt.Sprintf("Imported %d boundaries", counts["features"])
		m.state.Counts = counts
		m.mu.Unlock()
		if counts["features"] > 0 {
			enableMapBoundariesAfterImport(controller)
		}
	}()
	return nil
}

func enableMapBoundariesAfterImport(controller *Controller) {
	if controller == nil {
		return
	}
	controller.Options.mutex.Lock()
	mi := &controller.Options.MappingIntegration
	mi.MapBoundariesEnabled = true
	if len(mi.MapBoundaryLayers) == 0 {
		mi.MapBoundaryLayers = []string{"county"}
	}
	controller.Options.mutex.Unlock()
	_ = controller.Options.Write(controller.Database)
}

func normalizeBoundaryLayerStrings(layers []string) []string {
	var out []mapping.BoundaryLayer
	for _, s := range layers {
		l := mapping.BoundaryLayer(strings.ToLower(strings.TrimSpace(s)))
		if l.Valid() {
			out = append(out, l)
		}
	}
	return layerStrings(out)
}

func layerStrings(layers []mapping.BoundaryLayer) []string {
	var out []string
	for _, l := range layers {
		out = append(out, string(l))
	}
	return out
}

func layerLabel(l mapping.BoundaryLayer) string {
	switch l {
	case mapping.BoundaryLayerCounty:
		return "counties"
	case mapping.BoundaryLayerPlace:
		return "cities & villages"
	case mapping.BoundaryLayerCousub:
		return "townships"
	default:
		return string(l)
	}
}

func stateNameForFIPS(fips string) string {
	if name, ok := usStateFIPSNames[fips]; ok {
		return name
	}
	return fips
}

var usStateFIPSNames = map[string]string{
	"01": "Alabama", "02": "Alaska", "04": "Arizona", "05": "Arkansas", "06": "California",
	"08": "Colorado", "09": "Connecticut", "10": "Delaware", "11": "District of Columbia",
	"12": "Florida", "13": "Georgia", "15": "Hawaii", "16": "Idaho", "17": "Illinois",
	"18": "Indiana", "19": "Iowa", "20": "Kansas", "21": "Kentucky", "22": "Louisiana",
	"23": "Maine", "24": "Maryland", "25": "Massachusetts", "26": "Michigan", "27": "Minnesota",
	"28": "Mississippi", "29": "Missouri", "30": "Montana", "31": "Nebraska", "32": "Nevada",
	"33": "New Hampshire", "34": "New Jersey", "35": "New Mexico", "36": "New York",
	"37": "North Carolina", "38": "North Dakota", "39": "Ohio", "40": "Oklahoma",
	"41": "Oregon", "42": "Pennsylvania", "44": "Rhode Island", "45": "South Carolina",
	"46": "South Dakota", "47": "Tennessee", "48": "Texas", "49": "Utah", "50": "Vermont",
	"51": "Virginia", "53": "Washington", "54": "West Virginia", "55": "Wisconsin", "56": "Wyoming",
}
