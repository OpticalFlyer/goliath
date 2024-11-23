package main

import (
	"container/list"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jonas-p/go-shp"
)

func (g *Game) loadShapefile(path string) {
	fmt.Printf("Loading shapefile: %s\n", path)

	// Create new layer with name from filename
	filename := filepath.Base(path)
	layerName := strings.TrimSuffix(filename, filepath.Ext(filename))
	newLayer := NewLayer(layerName, g.ScreenWidth, g.ScreenHeight)

	// Add layer as child of current layer if one is selected, otherwise add as root
	if g.currentLayer != nil {
		g.currentLayer.AddChild(newLayer)
		g.currentLayer.Expanded = true // Auto-expand parent
	} else {
		g.layers = append(g.layers, newLayer)
	}

	// Update layer panel with new layer
	g.layerPanel.UpdateLayers(g.layers)
	g.layerPanel.visible = true // Show layer panel when adding new layer

	// Rest of loadShapefile remains the same...
	shapeFile, err := shp.Open(path)
	if err != nil {
		fmt.Printf("Error opening shapefile: %v\n", err)
		return
	}
	defer shapeFile.Close()

	if shapeFile.Next() {
		_, shape := shapeFile.Shape()

		shapeFile.Close()
		shapeFile, _ = shp.Open(path)

		switch shape := shape.(type) {
		case *shp.Point, *shp.PointZ:
			g.loadPointShapefile(shapeFile, newLayer)
		case *shp.PolyLine, *shp.PolyLineZ:
			g.loadLineShapefile(shapeFile, newLayer)
		case *shp.Polygon, *shp.PolygonZ:
			g.loadPolygonShapefile(shapeFile, newLayer)
		default:
			fmt.Printf("Unsupported shapefile type: %T\n", shape)
			return
		}
	}
}

func (g *Game) loadPointShapefile(shapeFile *shp.Reader, layer *Layer) {
	const numWorkers = 10
	jobs := make(chan shp.Shape, 1000)
	var wg sync.WaitGroup
	count := atomic.Int64{}

	var cacheClearMutex sync.Mutex
	lastCacheClear := atomic.Int64{}

	// Launch workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for shape := range jobs {
				var lat, lon float64
				switch pt := shape.(type) {
				case *shp.Point:
					lat, lon = pt.Y, pt.X
				case *shp.PointZ:
					lat, lon = pt.Y, pt.X
				}

				p := NewPoint(lat, lon)
				layer.PointLayer.Index.Insert(p, p.Bounds())

				newCount := count.Add(1)

				// Check if we need to clear cache
				if newCount/100000 > lastCacheClear.Load() {
					cacheClearMutex.Lock()
					if newCount/100000 > lastCacheClear.Load() {
						layer.PointTileCache.mu.Lock()
						layer.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
						layer.PointTileCache.lruList = list.New()
						layer.PointTileCache.lruMap = make(map[string]*list.Element)
						layer.PointTileCache.mu.Unlock()

						lastCacheClear.Store(newCount / 100000)
						g.needRedraw = true
						fmt.Printf("Cleared cache after %d points\n", newCount)
					}
					cacheClearMutex.Unlock()
				}

				if newCount%10000 == 0 {
					fmt.Printf("Loaded %d points...\n", newCount)
				}
			}
		}()
	}

	// Start sender
	go func() {
		for shapeFile.Next() {
			_, shape := shapeFile.Shape()
			jobs <- shape
		}
		close(jobs)
	}()

	wg.Wait()

	fmt.Printf("Loaded %d points from shapefile\n", count.Load())

	// Final cache clear
	layer.PointTileCache.mu.Lock()
	layer.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
	layer.PointTileCache.lruList = list.New()
	layer.PointTileCache.lruMap = make(map[string]*list.Element)
	layer.PointTileCache.mu.Unlock()

	g.needRedraw = true
}

func (g *Game) loadLineShapefile(shapeFile *shp.Reader, layer *Layer) {
	const numWorkers = 10
	jobs := make(chan shp.Shape, 1000)
	var wg sync.WaitGroup
	count := atomic.Int64{}

	var cacheClearMutex sync.Mutex
	lastCacheClear := atomic.Int64{}

	// Launch workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for shape := range jobs {
				var points []Point

				switch polyline := shape.(type) {
				case *shp.PolyLine:
					points = make([]Point, len(polyline.Points))
					for i, pt := range polyline.Points {
						points[i] = Point{Lat: pt.Y, Lon: pt.X}
					}
				case *shp.PolyLineZ:
					points = make([]Point, len(polyline.Points))
					for i, pt := range polyline.Points {
						points[i] = Point{Lat: pt.Y, Lon: pt.X}
					}
				}

				line := &LineString{Points: points}
				layer.PolylineLayer.Index.Insert(line, line.Bounds())

				newCount := count.Add(1)

				if newCount/1000 > lastCacheClear.Load() {
					cacheClearMutex.Lock()
					if newCount/1000 > lastCacheClear.Load() {
						layer.LineTileCache.mu.Lock()
						layer.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
						layer.LineTileCache.lruList = list.New()
						layer.LineTileCache.lruMap = make(map[string]*list.Element)
						layer.LineTileCache.mu.Unlock()

						lastCacheClear.Store(newCount / 1000)
						g.needRedraw = true
						fmt.Printf("Cleared cache after %d lines\n", newCount)
					}
					cacheClearMutex.Unlock()
				}

				if newCount%100 == 0 {
					fmt.Printf("Loaded %d lines...\n", newCount)
				}
			}
		}()
	}

	// Start sender
	go func() {
		for shapeFile.Next() {
			_, shape := shapeFile.Shape()
			jobs <- shape
		}
		close(jobs)
	}()

	wg.Wait()

	fmt.Printf("Loaded %d lines from shapefile\n", count.Load())

	// Final cache clear
	layer.LineTileCache.mu.Lock()
	layer.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
	layer.LineTileCache.lruList = list.New()
	layer.LineTileCache.lruMap = make(map[string]*list.Element)
	layer.LineTileCache.mu.Unlock()

	g.needRedraw = true
}

func (g *Game) loadPolygonShapefile(shapeFile *shp.Reader, layer *Layer) {
	const numWorkers = 10
	jobs := make(chan shp.Shape, 1000)
	var wg sync.WaitGroup
	count := atomic.Int64{}

	var cacheClearMutex sync.Mutex
	lastCacheClear := atomic.Int64{}

	// Launch workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for shape := range jobs {
				var points [][]shp.Point

				switch poly := shape.(type) {
				case *shp.Polygon:
					points = [][]shp.Point{poly.Points}
				case *shp.PolygonZ:
					points = [][]shp.Point{poly.Points}
				}

				for i := 0; i < len(points); i++ {
					polyPoints := make([]Point, len(points[i]))
					for j, pt := range points[i] {
						polyPoints[j] = Point{Lat: pt.Y, Lon: pt.X}
					}

					polygon := &Polygon{Points: polyPoints}
					layer.PolygonLayer.Index.Insert(polygon, polygon.Bounds())
				}

				newCount := count.Add(1)

				if newCount/1000 > lastCacheClear.Load() {
					cacheClearMutex.Lock()
					if newCount/1000 > lastCacheClear.Load() {
						layer.PolygonTileCache.mu.Lock()
						layer.PolygonTileCache.cache = make(map[int]map[int]map[int]*PolygonTile)
						layer.PolygonTileCache.lruList = list.New()
						layer.PolygonTileCache.lruMap = make(map[string]*list.Element)
						layer.PolygonTileCache.mu.Unlock()

						lastCacheClear.Store(newCount / 1000)
						g.needRedraw = true
						fmt.Printf("Cleared cache after %d polygons\n", newCount)
					}
					cacheClearMutex.Unlock()
				}

				if newCount%100 == 0 {
					fmt.Printf("Loaded %d polygons...\n", newCount)
				}
			}
		}()
	}

	// Start sender
	go func() {
		for shapeFile.Next() {
			_, shape := shapeFile.Shape()
			jobs <- shape
		}
		close(jobs)
	}()

	wg.Wait()

	fmt.Printf("Loaded %d polygon features from shapefile\n", count.Load())

	// Final cache clear
	layer.PolygonTileCache.mu.Lock()
	layer.PolygonTileCache.cache = make(map[int]map[int]map[int]*PolygonTile)
	layer.PolygonTileCache.lruList = list.New()
	layer.PolygonTileCache.lruMap = make(map[string]*list.Element)
	layer.PolygonTileCache.mu.Unlock()

	g.needRedraw = true
}
