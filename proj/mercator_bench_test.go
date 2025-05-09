package proj

import "testing"

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

func BenchmarkEPSG3857ToTileCoords(b *testing.B) {
	coords := [][3]float64{
		{0, 0, 1},
		{20037508.34, 20037508.34, 10},
		{-20037508.34, -20037508.34, 15},
		{-13656274.0, 5703158.0, 12}, // Portland, OR
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, c := range coords {
			EPSG3857ToTileCoords(c[0], c[1], int(c[2]))
		}
	}
}
