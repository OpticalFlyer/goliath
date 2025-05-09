package proj

import (
	"math"
	"testing"
)

func TestLatLonToTileCoords(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon float64
		zoom     int
		wantX    float64
		wantY    float64
	}{
		{
			name:  "Center of map at zoom 1",
			lat:   0,
			lon:   0,
			zoom:  1,
			wantX: 1.0,
			wantY: 1.0,
		},
		{
			name:  "Top-left corner at zoom 1",
			lat:   maxLat,
			lon:   -180,
			zoom:  1,
			wantX: 0.0,
			wantY: 0.0,
		},
		{
			name:  "Bottom-right corner at zoom 1",
			lat:   minLat,
			lon:   180,
			zoom:  1,
			wantX: 2.0,
			wantY: 2.0,
		},
		{
			name:  "Middle of tile (1,1) at zoom 1",
			lat:   0,
			lon:   90,
			zoom:  1,
			wantX: 1.5,
			wantY: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotX, gotY := LatLonToTileCoords(tt.lat, tt.lon, tt.zoom)
			if math.Abs(gotX-tt.wantX) > 1e-6 || math.Abs(gotY-tt.wantY) > 1e-6 {
				t.Errorf("got (%f, %f); want (%f, %f)",
					gotX, gotY, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestEPSG3857ToTileCoords(t *testing.T) {
	tests := []struct {
		name      string
		x, y      float64
		zoom      int
		wantX     float64
		wantY     float64
		tolerance float64
	}{
		{
			name:      "Center of map at zoom 1",
			x:         0,
			y:         0,
			zoom:      1,
			wantX:     1.0,
			wantY:     1.0,
			tolerance: 1e-6,
		},
		{
			name:      "Top-left corner at zoom 1",
			x:         -20037508.34,
			y:         20037508.34,
			zoom:      1,
			wantX:     0.0,
			wantY:     0.0,
			tolerance: 1e-6,
		},
		{
			name:      "Bottom-right corner at zoom 1",
			x:         20037508.34,
			y:         -20037508.34,
			zoom:      1,
			wantX:     2.0,
			wantY:     2.0,
			tolerance: 1e-6,
		},
		{
			name: "Portland, OR approximate location at zoom 12",
			// Coordinates verified with GDAL:
			// EPSG:3857 (-13656274, 5703158) = EPSG:4326 (-122.67640, 45.51621)
			x:         -13656274.0,
			y:         5703158.0,
			zoom:      12,
			wantX:     652.215,
			wantY:     1465.090,
			tolerance: 0.001, // Tile-level precision
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotX, gotY := EPSG3857ToTileCoords(tt.x, tt.y, tt.zoom)
			if math.Abs(gotX-tt.wantX) > tt.tolerance || math.Abs(gotY-tt.wantY) > tt.tolerance {
				t.Errorf("EPSG3857ToTileCoords(%f, %f, %d) = (%f, %f); want (%f, %f)",
					tt.x, tt.y, tt.zoom, gotX, gotY, tt.wantX, tt.wantY)
			}
		})
	}
}
