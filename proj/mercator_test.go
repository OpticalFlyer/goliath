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

func BenchmarkLatLonToTileCoords(b *testing.B) {
	coords := [][3]float64{
		{0, 0, 1},
		{maxLat, 180, 10},
		{minLat, -180, 15},
		{45.12345, -122.67890, 12},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, c := range coords {
			LatLonToTileCoords(c[0], c[1], int(c[2]))
		}
	}
}
