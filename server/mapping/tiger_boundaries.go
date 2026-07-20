// Copyright (C) 2025 Thinline Dynamic Solutions
//
// tiger_boundaries.go — parse Census cartographic boundary shapefiles (counties,
// places, county subdivisions) for US-wide map overlay layers.

package mapping

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/jonas-p/go-shp"
)

const TigerBoundaryYear = 2024

// BoundaryLayer identifies a Census cartographic boundary layer type.
type BoundaryLayer string

const (
	BoundaryLayerCounty BoundaryLayer = "county"
	BoundaryLayerPlace  BoundaryLayer = "place"
	BoundaryLayerCousub BoundaryLayer = "cousub"
)

func (l BoundaryLayer) Valid() bool {
	switch l {
	case BoundaryLayerCounty, BoundaryLayerPlace, BoundaryLayerCousub:
		return true
	default:
		return false
	}
}

func (l BoundaryLayer) censusSuffix() string {
	switch l {
	case BoundaryLayerCounty:
		return "county"
	case BoundaryLayerPlace:
		return "place"
	case BoundaryLayerCousub:
		return "cousub"
	default:
		return ""
	}
}

// BoundaryDownloadSpec describes where to fetch a layer and how to filter it.
type BoundaryDownloadSpec struct {
	URL             string
	ShapeBase       string
	FilterStateFIPS string
}

// TigerBoundaryDownloadSpec returns Census GENZ download info. County shapefiles
// are published as a single US-wide zip (per-state county zips 404 on GENZ2024).
func TigerBoundaryDownloadSpec(stateFIPS string, layer BoundaryLayer, year int) BoundaryDownloadSpec {
	if year <= 0 {
		year = TigerBoundaryYear
	}
	stateFIPS = strings.TrimSpace(stateFIPS)
	suffix := layer.censusSuffix()
	if layer == BoundaryLayerCounty {
		return BoundaryDownloadSpec{
			URL: fmt.Sprintf("https://www2.census.gov/geo/tiger/GENZ%d/shp/cb_%d_us_county_500k.zip",
				year, year),
			ShapeBase:       fmt.Sprintf("cb_%d_us_county_500k", year),
			FilterStateFIPS: stateFIPS,
		}
	}
	return BoundaryDownloadSpec{
		URL: fmt.Sprintf("https://www2.census.gov/geo/tiger/GENZ%d/shp/cb_%d_%s_%s_500k.zip",
			year, year, stateFIPS, suffix),
		ShapeBase:       fmt.Sprintf("cb_%d_%s_%s_500k", year, stateFIPS, suffix),
		FilterStateFIPS: stateFIPS,
	}
}

// BoundaryCacheDirName is the cache subdirectory for a download spec.
func (s BoundaryDownloadSpec) CacheDirName(layer BoundaryLayer) string {
	if layer == BoundaryLayerCounty {
		return "us/county"
	}
	if s.FilterStateFIPS != "" {
		return s.FilterStateFIPS + "/" + string(layer)
	}
	return "us/" + string(layer)
}

// BoundaryFeature is one parsed administrative polygon ready for DB insert.
type BoundaryFeature struct {
	GEOID      string
	Name       string
	StateFIPS  string
	Layer      BoundaryLayer
	Geometry   json.RawMessage
	MinLat     float64
	MinLon     float64
	MaxLat     float64
	MaxLon     float64
	CentroidLat float64
	CentroidLon float64
	ColorIndex int
}

// ParseTigerBoundaryShapefile reads a cb_* cartographic boundary shapefile.
// filterStateFIPS, when set, keeps only features whose STATEFP matches.
func ParseTigerBoundaryShapefile(shapePathPrefix string, layer BoundaryLayer, filterStateFIPS string) ([]BoundaryFeature, error) {
	filterStateFIPS = normalizeStateFIPS(filterStateFIPS)
	openPath := shapePathPrefix
	if !strings.HasSuffix(strings.ToLower(openPath), ".shp") {
		openPath += ".shp"
	}
	reader, err := shp.Open(openPath)
	if err != nil {
		return nil, fmt.Errorf("open shapefile %s: %w", shapePathPrefix, err)
	}
	defer reader.Close()

	fields := reader.Fields()
	idx := map[string]int{}
	for i, f := range fields {
		idx[strings.ToUpper(f.String())] = i
	}

	var out []BoundaryFeature
	for reader.Next() {
		recIdx, raw := reader.Shape()
		get := func(name string) string {
			i, ok := idx[name]
			if !ok {
				return ""
			}
			return strings.TrimSpace(reader.ReadAttribute(recIdx, i))
		}
		geoid := get("GEOID")
		if geoid == "" {
			geoid = get("GEOID20")
		}
		if geoid == "" {
			continue
		}
		name := get("NAMELSAD")
		if name == "" {
			name = get("NAME")
		}
		stateFIPS := normalizeStateFIPS(get("STATEFP"))
		if filterStateFIPS != "" && stateFIPS != filterStateFIPS {
			continue
		}
		geom, minLat, minLon, maxLat, maxLon, clat, clon, ok := shapeToGeoJSON(raw)
		if !ok {
			continue
		}
		out = append(out, BoundaryFeature{
			GEOID:       geoid,
			Name:        name,
			StateFIPS:   stateFIPS,
			Layer:       layer,
			Geometry:    geom,
			MinLat:      minLat,
			MinLon:      minLon,
			MaxLat:      maxLat,
			MaxLon:      maxLon,
			CentroidLat: clat,
			CentroidLon: clon,
		})
	}
	AssignBoundaryColors(out)
	return out, nil
}

func shapeToGeoJSON(raw shp.Shape) (json.RawMessage, float64, float64, float64, float64, float64, float64, bool) {
	switch s := raw.(type) {
	case *shp.Polygon:
		return ringsToGeoJSON(extractPolygonRings(s.Parts, s.Points))
	case *shp.PolygonM:
		return ringsToGeoJSON(extractPolygonRings(s.Parts, s.Points))
	case *shp.PolygonZ:
		return ringsToGeoJSON(extractPolygonRings(s.Parts, s.Points))
	default:
		return nil, 0, 0, 0, 0, 0, 0, false
	}
}

func extractPolygonRings(parts []int32, points []shp.Point) [][][]float64 {
	if len(parts) == 0 {
		coords := closeRing(points)
		if len(coords) < 4 {
			return nil
		}
		return [][][]float64{coords}
	}
	var rings [][][]float64
	for i := 0; i < len(parts); i++ {
		start := int(parts[i])
		end := len(points)
		if i+1 < len(parts) {
			end = int(parts[i+1])
		}
		if end-start < 3 {
			continue
		}
		coords := closeRing(points[start:end])
		if len(coords) >= 4 {
			rings = append(rings, coords)
		}
	}
	return rings
}

func closeRing(points []shp.Point) [][]float64 {
	coords := make([][]float64, 0, len(points)+1)
	for _, p := range points {
		coords = append(coords, []float64{p.X, p.Y})
	}
	if len(coords) > 0 {
		first := coords[0]
		last := coords[len(coords)-1]
		if first[0] != last[0] || first[1] != last[1] {
			coords = append(coords, []float64{first[0], first[1]})
		}
	}
	return coords
}

func ringsToGeoJSON(rings [][][]float64) (json.RawMessage, float64, float64, float64, float64, float64, float64, bool) {
	if len(rings) == 0 {
		return nil, 0, 0, 0, 0, 0, 0, false
	}
	// Use the largest ring when a record has multiple disjoint parts.
	best := rings[0]
	for _, r := range rings[1:] {
		if len(r) > len(best) {
			best = r
		}
	}
	geom := map[string]any{"type": "Polygon", "coordinates": [][][]float64{best}}
	b, err := json.Marshal(geom)
	if err != nil {
		return nil, 0, 0, 0, 0, 0, 0, false
	}
	minLat, minLon, maxLat, maxLon := boundsFromRing(best)
	clat := (minLat + maxLat) / 2
	clon := (minLon + maxLon) / 2
	return b, minLat, minLon, maxLat, maxLon, clat, clon, true
}

func boundsFromRing(ring [][]float64) (minLat, minLon, maxLat, maxLon float64) {
	minLat, minLon = math.MaxFloat64, math.MaxFloat64
	maxLat, maxLon = -math.MaxFloat64, -math.MaxFloat64
	for _, pt := range ring {
		if len(pt) < 2 {
			continue
		}
		lon, lat := pt[0], pt[1]
		if lat < minLat {
			minLat = lat
		}
		if lat > maxLat {
			maxLat = lat
		}
		if lon < minLon {
			minLon = lon
		}
		if lon > maxLon {
			maxLon = lon
		}
	}
	return minLat, minLon, maxLat, maxLon
}

// BoundaryFillColors is the palette used on the incident map (adjacent regions
// are assigned different indices via AssignBoundaryColors).
var BoundaryFillColors = []string{
	"#6BA3D0", "#7BC47F", "#E8A87C", "#C39BD3", "#F7DC6F",
}

// AssignBoundaryColors greedy-colors features so adjacent polygons use
// different palette indices.
func AssignBoundaryColors(features []BoundaryFeature) {
	if len(features) == 0 {
		return
	}
	neighbors := buildBoundaryNeighbors(features)
	for i := range features {
		used := map[int]bool{}
		for _, n := range neighbors[i] {
			if n >= 0 && n < len(features) {
				used[features[n].ColorIndex] = true
			}
		}
		for c := 0; c < len(BoundaryFillColors); c++ {
			if !used[c] {
				features[i].ColorIndex = c
				break
			}
		}
	}
}

func buildBoundaryNeighbors(features []BoundaryFeature) [][]int {
	out := make([][]int, len(features))
	for i := 0; i < len(features); i++ {
		for j := i + 1; j < len(features); j++ {
			if !boundaryBBoxesTouch(features[i], features[j]) {
				continue
			}
			if !boundaryPolygonsTouch(features[i].Geometry, features[j].Geometry) {
				continue
			}
			out[i] = append(out[i], j)
			out[j] = append(out[j], i)
		}
	}
	return out
}

func boundaryBBoxesTouch(a, b BoundaryFeature) bool {
	const eps = 0.0005
	if a.MaxLat+eps < b.MinLat || b.MaxLat+eps < a.MinLat {
		return false
	}
	if a.MaxLon+eps < b.MinLon || b.MaxLon+eps < a.MinLon {
		return false
	}
	return true
}

func boundaryPolygonsTouch(ga, gb json.RawMessage) bool {
	ptsA := sampleGeometryVertices(ga, 48)
	ptsB := sampleGeometryVertices(gb, 48)
	if len(ptsA) == 0 || len(ptsB) == 0 {
		return false
	}
	const eps = 0.0008
	for _, a := range ptsA {
		for _, b := range ptsB {
			if math.Abs(a[0]-b[0]) <= eps && math.Abs(a[1]-b[1]) <= eps {
				return true
			}
		}
	}
	return false
}

func sampleGeometryVertices(raw json.RawMessage, maxPts int) [][2]float64 {
	var geom struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if json.Unmarshal(raw, &geom) != nil {
		return nil
	}
	switch geom.Type {
	case "Polygon":
		var rings [][][]float64
		if json.Unmarshal(geom.Coordinates, &rings) != nil {
			return nil
		}
		return sampleRingPoints(rings, maxPts)
	default:
		return nil
	}
}

func sampleRingPoints(rings [][][]float64, maxPts int) [][2]float64 {
	if len(rings) == 0 || len(rings[0]) == 0 {
		return nil
	}
	ring := rings[0]
	step := 1
	if len(ring) > maxPts {
		step = len(ring)/maxPts + 1
	}
	var out [][2]float64
	for i := 0; i < len(ring); i += step {
		pt := ring[i]
		if len(pt) >= 2 {
			out = append(out, [2]float64{pt[0], pt[1]})
		}
	}
	return out
}

func normalizeStateFIPS(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
