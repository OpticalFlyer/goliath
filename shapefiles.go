package main

import (
	"container/list"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/jonas-p/go-shp"
)

func (g *Game) loadShapefile(path string) {
	fmt.Printf("Loading shapefile: %s\n", path)

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
		case *shp.Point:
			g.loadPointShapefile(shapeFile)
		case *shp.PolyLine:
			g.loadLineShapefile(shapeFile)
		case *shp.Polygon, *shp.PolygonZ:
			g.loadPolygonShapefile(shapeFile)
		default:
			fmt.Printf("Unsupported shapefile type: %T\n", shape)
			return
		}
	}
}

func (g *Game) loadPointShapefile(shapeFile *shp.Reader) {
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
			localCount := 0
			for shape := range jobs {
				point := shape.(*shp.Point)
				p := NewPoint(point.Y, point.X)

				// Insert directly in worker
				g.PointLayer.Index.Insert(p, p.Bounds())

				localCount++
				newCount := count.Add(1)

				// Check if we need to clear cache
				if newCount/100000 > lastCacheClear.Load() {
					cacheClearMutex.Lock()
					if newCount/100000 > lastCacheClear.Load() {
						// Clear point tile cache
						g.PointTileCache.mu.Lock()
						g.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
						g.PointTileCache.lruList = list.New()
						g.PointTileCache.lruMap = make(map[string]*list.Element)
						g.PointTileCache.mu.Unlock()

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

	// Wait for completion
	wg.Wait()

	fmt.Printf("Loaded %d points from shapefile\n", count.Load())

	// Clear point tile cache
	g.PointTileCache.mu.Lock()
	g.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
	g.PointTileCache.lruList = list.New()
	g.PointTileCache.lruMap = make(map[string]*list.Element)
	g.PointTileCache.mu.Unlock()

	g.needRedraw = true
}

func (g *Game) loadLineShapefile(shapeFile *shp.Reader) {
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
				polyline := shape.(*shp.PolyLine)
				points := make([]Point, len(polyline.Points))

				// Convert shapefile points to our Point type
				for i, pt := range polyline.Points {
					points[i] = Point{Lat: pt.Y, Lon: pt.X}
				}

				line := &LineString{Points: points}
				g.PolylineLayer.Index.Insert(line, line.Bounds())

				newCount := count.Add(1)

				// Clear cache periodically
				if newCount/1000 > lastCacheClear.Load() {
					cacheClearMutex.Lock()
					if newCount/1000 > lastCacheClear.Load() {
						g.LineTileCache.mu.Lock()
						g.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
						g.LineTileCache.lruList = list.New()
						g.LineTileCache.lruMap = make(map[string]*list.Element)
						g.LineTileCache.mu.Unlock()

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

	// Wait for completion
	wg.Wait()

	fmt.Printf("Loaded %d lines from shapefile\n", count.Load())

	// Final cache clear
	g.LineTileCache.mu.Lock()
	g.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
	g.LineTileCache.lruList = list.New()
	g.LineTileCache.lruMap = make(map[string]*list.Element)
	g.LineTileCache.mu.Unlock()

	g.needRedraw = true
}

func (g *Game) loadPolygonShapefile(shapeFile *shp.Reader) {
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

				// Handle parts properly using part indices
				for i := 0; i < len(points); i++ {
					polyPoints := make([]Point, len(points[i]))
					for j, pt := range points[i] {
						polyPoints[j] = Point{Lat: pt.Y, Lon: pt.X}
					}

					// Create and insert polygon
					polygon := &Polygon{Points: polyPoints}
					g.PolygonLayer.Index.Insert(polygon, polygon.Bounds())
				}

				newCount := count.Add(1)

				// Clear cache periodically
				if newCount/1000 > lastCacheClear.Load() {
					cacheClearMutex.Lock()
					if newCount/1000 > lastCacheClear.Load() {
						g.PolygonTileCache.mu.Lock()
						g.PolygonTileCache.cache = make(map[int]map[int]map[int]*PolygonTile)
						g.PolygonTileCache.lruList = list.New()
						g.PolygonTileCache.lruMap = make(map[string]*list.Element)
						g.PolygonTileCache.mu.Unlock()

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

	// Wait for completion
	wg.Wait()

	fmt.Printf("Loaded %d polygon features from shapefile\n", count.Load())

	// Final cache clear
	g.PolygonTileCache.mu.Lock()
	g.PolygonTileCache.cache = make(map[int]map[int]map[int]*PolygonTile)
	g.PolygonTileCache.lruList = list.New()
	g.PolygonTileCache.lruMap = make(map[string]*list.Element)
	g.PolygonTileCache.mu.Unlock()

	g.needRedraw = true
}
