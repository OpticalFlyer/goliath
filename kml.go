package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type KML struct {
	XMLName   xml.Name   `xml:"kml"`
	Documents []Document `xml:"Document"`
	Folders   []Folder   `xml:"Folder"` // Folders without a document
}

type Document struct {
	XMLName    xml.Name    `xml:"Document"`
	Name       string      `xml:"name"`
	Folders    []Folder    `xml:"Folder"`
	Documents  []Document  `xml:"Document"`  // Handle nested documents
	Placemarks []Placemark `xml:"Placemark"` // Placemarks without a folder
	Styles     []Style     `xml:"Style"`
	StyleMaps  []StyleMap  `xml:"StyleMap"`
}

type Folder struct {
	XMLName    xml.Name    `xml:"Folder"`
	Name       string      `xml:"name"`
	Placemarks []Placemark `xml:"Placemark"`
	Folders    []Folder    `xml:"Folder"`   // Handle nested folders
	Documents  []Document  `xml:"Document"` // Handle nested documents
}

type Placemark struct {
	Name          string        `xml:"name"`
	StyleURL      string        `xml:"styleUrl"`
	Style         Style         `xml:"Style"`
	Point         KMLPoint      `xml:"Point"`
	LineString    KMLLineString `xml:"LineString"`
	Polygon       KMLPolygon    `xml:"Polygon"`
	MultiGeometry MultiGeometry `xml:"MultiGeometry"`
}

type KMLPolygon struct {
	OuterBoundaryIs OuterBoundaryIs `xml:"outerBoundaryIs"`
}

type OuterBoundaryIs struct {
	LinearRing LinearRing `xml:"LinearRing"`
}

type LinearRing struct {
	Coordinates string `xml:"coordinates"`
}

type MultiGeometry struct {
	LineStrings []KMLLineString `xml:"LineString"`
	Polygons    []KMLPolygon    `xml:"Polygon"`
}

type KMLPoint struct {
	Coordinates string `xml:"coordinates"`
}

type KMLLineString struct {
	Coordinates string `xml:"coordinates"`
}

type StyleMap struct {
	XMLName xml.Name `xml:"StyleMap"`
	ID      string   `xml:"id,attr"`
	Pairs   []Pair   `xml:"Pair"`
}

type Pair struct {
	Key      string `xml:"key"`
	StyleURL string `xml:"styleUrl"`
}

/* Sample Style from KML
<Style id="s_ylw-pushpin">
		<IconStyle>
			<color>ff0701fc</color>
			<scale>0.8</scale>
			<Icon>
				<href>http://maps.google.com/mapfiles/kml/pushpin/ylw-pushpin.png</href>
			</Icon>
			<hotSpot x="20" y="2" xunits="pixels" yunits="pixels"/>
		</IconStyle>
</Style>
*/

type Style struct {
	XMLName   xml.Name  `xml:"Style"`
	ID        string    `xml:"id,attr"`
	IconStyle IconStyle `xml:"IconStyle"`
	LineStyle LineStyle `xml:"LineStyle"`
}

type IconStyle struct {
	Color   string  `xml:"color"`
	Scale   float64 `xml:"scale"`
	Icon    Icon    `xml:"Icon"`
	HotSpot HotSpot `xml:"hotSpot"`
}

type Icon struct {
	Href string `xml:"href"`
}

type HotSpot struct {
	X      float64 `xml:"x,attr"`
	Y      float64 `xml:"y,attr"`
	XUnits string  `xml:"xunits,attr"`
	YUnits string  `xml:"yunits,attr"`
}

type LineStyle struct {
	Color string  `xml:"color"`
	Width float64 `xml:"width"`
}

func processFoldersAndDocuments(folders []Folder, documents []Document, game *Game, parentLayer *Layer) error {
	// Process Folders
	for _, folder := range folders {
		// Create a new layer for the folder
		folderLayer := NewLayer(folder.Name, game.ScreenWidth, game.ScreenHeight)
		parentLayer.AddChild(folderLayer)

		// Process Placemarks in the folder
		err := processPlacemarks(folder.Placemarks, game, folderLayer)
		if err != nil {
			return err
		}

		// Recursively process nested folders and documents
		err = processFoldersAndDocuments(folder.Folders, folder.Documents, game, folderLayer)
		if err != nil {
			return err
		}
	}

	// Process Documents
	for _, document := range documents {
		// Use the name from the document's name tag
		documentName := document.Name
		if documentName == "" {
			documentName = "Document"
		}

		// Create a new layer for the document
		documentLayer := NewLayer(documentName, game.ScreenWidth, game.ScreenHeight)
		parentLayer.AddChild(documentLayer)

		// Update the StyleMap for each Document.StyleMaps
		convertedStyleMap := convertStyleMapsToMap(document.StyleMaps)
		for id, pairs := range convertedStyleMap {
			if _, exists := game.StyleMap[id]; !exists {
				game.StyleMap[id] = pairs
				log.Printf("Added StyleMap %s - normal: %s, highlight: %s\n", id, pairs["normal"], pairs["highlight"])
			} else {
				for k, v := range pairs {
					game.StyleMap[id][k] = v
				}
			}
		}

		// Update the convertedMap for each Document.Styles
		convertedStyle := convertStylesToMap(document.Styles)
		for id, styleEntry := range convertedStyle {
			if _, exists := game.Styles[id]; !exists {
				game.Styles[id] = styleEntry
				//log.Printf("Added Style %s - Color: %s, Width: %f\n", id, styleEntry.Color, styleEntry.Width)
			} else {
				game.Styles[id] = styleEntry
			}
		}

		// Update the IconStyles for each Document.Styles
		convertedIconStyles := convertIconStylesToMap(document.Styles)
		newHrefs := make(map[string]bool)
		for id, iconStyleEntry := range convertedIconStyles {
			if _, exists := game.IconStyles[id]; !exists {
				game.IconStyles[id] = iconStyleEntry
				log.Printf("Added IconStyle %s - Color: %s, Scale: %f, Hotspot (%.0f, %.0f), Href: %s\n", id, iconStyleEntry.Color, iconStyleEntry.Scale, iconStyleEntry.HotSpot.X, iconStyleEntry.HotSpot.Y, iconStyleEntry.Href)
				if len(iconStyleEntry.Href) > 0 {
					newHrefs[iconStyleEntry.Href] = true
				}
			}
		}

		// Download and process new IconStyle images
		err := downloadIconImages(game, newHrefs)
		if err != nil {
			return err
		}

		// Process Placemarks within the document with no folder
		err = processPlacemarks(document.Placemarks, game, documentLayer)
		if err != nil {
			return err
		}

		// Recursively process folders and documents in the document
		err = processFoldersAndDocuments(document.Folders, document.Documents, game, documentLayer)
		if err != nil {
			return err
		}
	}

	return nil
}

func convertIconStylesToMap(styles []Style) map[string]IconStyleData {
	convertedMap := make(map[string]IconStyleData)

	for _, style := range styles {
		convertedMap[style.ID] = IconStyleData{
			ID:      style.ID,
			Color:   style.IconStyle.Color,
			Scale:   style.IconStyle.Scale,
			Href:    style.IconStyle.Icon.Href,
			HotSpot: style.IconStyle.HotSpot,
		}
	}

	return convertedMap
}

func downloadIconImages(game *Game, hrefs map[string]bool) error {
	if game.IconImages == nil {
		game.IconImages = make(map[string]*ebiten.Image)
	}

	for href := range hrefs {
		if _, exists := game.IconImages[href]; !exists {
			img, err := downloadAndDecodeImage(href)
			if err != nil {
				return err
			}
			game.IconImages[href] = ebiten.NewImageFromImage(img)
			log.Printf("Downloaded image: %s\n", href)
			log.Printf("Image dimensions: width=%d, height=%d\n", img.Bounds().Dx(), img.Bounds().Dy())
		}
	}
	return nil
}

func downloadAndDecodeImage(url string) (image.Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func convertStyleMapsToMap(styleMaps []StyleMap) map[string]map[string]string {
	styleMapMap := make(map[string]map[string]string)

	for _, styleMap := range styleMaps {
		pairMap := make(map[string]string)
		for _, pair := range styleMap.Pairs {
			styleURL := pair.StyleURL
			if len(styleURL) > 0 && styleURL[0] == '#' {
				styleURL = styleURL[1:]
			}

			pairMap[pair.Key] = styleURL
		}
		styleMapMap[styleMap.ID] = pairMap
	}

	return styleMapMap
}

func convertStylesToMap(styles []Style) map[string]PolyLineStyle {
	convertedMap := make(map[string]PolyLineStyle)

	for _, style := range styles {
		convertedMap[style.ID] = PolyLineStyle{
			Color: style.LineStyle.Color,
			Width: float32(style.LineStyle.Width),
		}
	}

	return convertedMap
}

/*
Sometimes there is an embedded style in the placemark
<Style><LineStyle><color>FF00ffff</color><width>5</width></LineStyle></Style>
*/

func processPlacemarks(placemarks []Placemark, game *Game, layer *Layer) error {
	for _, placemark := range placemarks {
		var lineStrings []KMLLineString
		var points []KMLPoint
		var polygons []KMLPolygon

		// Determine geometry type
		if len(placemark.LineString.Coordinates) > 0 {
			lineStrings = append(lineStrings, placemark.LineString)
		} else if len(placemark.MultiGeometry.LineStrings) > 0 {
			lineStrings = append(lineStrings, placemark.MultiGeometry.LineStrings...)
		} else if len(placemark.Point.Coordinates) > 0 {
			points = append(points, placemark.Point)
		} else if len(placemark.Polygon.OuterBoundaryIs.LinearRing.Coordinates) > 0 {
			polygons = append(polygons, placemark.Polygon)
		} else if len(placemark.MultiGeometry.Polygons) > 0 {
			polygons = append(polygons, placemark.MultiGeometry.Polygons...)
		} else {
			continue
		}

		// Process lines
		for _, lineString := range lineStrings {
			rawLineString := strings.TrimSpace(lineString.Coordinates)
			coordinates := strings.Split(strings.TrimSpace(rawLineString), " ")

			// Handle styling
			styleURL := placemark.StyleURL
			var lineColor color.RGBA
			var lineWidth float32

			if len(styleURL) > 0 { // Either a StyleMap or Style link
				if styleURL[0] == '#' {
					styleURL = styleURL[1:] // Strip leading #
				}

				if _, exists := game.StyleMap[styleURL]; !exists { // Not a StyleMap link
					lineColor, _ = hexStringToColor(game.Styles[styleURL].Color)
					lineWidth = game.Styles[styleURL].Width
				} else { // StyleMap link
					lineColor, _ = hexStringToColor(game.Styles[game.StyleMap[styleURL]["normal"]].Color)
					lineWidth = game.Styles[game.StyleMap[styleURL]["normal"]].Width
				}
			} else { // Embedded style
				if len(placemark.Style.LineStyle.Color) > 0 {
					lineColor, _ = hexStringToColor(placemark.Style.LineStyle.Color)
				} else {
					lineColor = color.RGBA{0, 0, 255, 255} // Default blue
				}
				if placemark.Style.LineStyle.Width > 0 {
					lineWidth = float32(placemark.Style.LineStyle.Width)
				} else {
					lineWidth = 1.0
				}
			}

			// Make sure minimum line width is 1
			if lineWidth < 1 {
				lineWidth = 1
			}

			line := &LineString{
				Points: make([]Point, 0),
				Color:  lineColor, // Set the color
				Width:  lineWidth, // Set the width
			}

			// Parse coordinates
			for _, coordinate := range coordinates {
				latLon := strings.Split(strings.TrimSpace(coordinate), ",")
				if len(latLon) >= 2 {
					lat, err := strconv.ParseFloat(latLon[1], 64)
					if err != nil {
						return err
					}
					lon, err := strconv.ParseFloat(latLon[0], 64)
					if err != nil {
						return err
					}
					line.Points = append(line.Points, Point{Lat: lat, Lon: lon})
				}
			}

			// Add to layer
			layer.PolylineLayer.Index.Insert(line, line.Bounds())
			log.Printf("Added line with %d points to layer %s\n", len(line.Points), layer.Name)
		}

		// Process polygons
		for _, polygon := range polygons {
			rawCoordinates := strings.TrimSpace(polygon.OuterBoundaryIs.LinearRing.Coordinates)
			coordinates := strings.Split(strings.TrimSpace(rawCoordinates), " ")

			// Remove duplicate last point if present
			if len(coordinates) > 1 && coordinates[0] == coordinates[len(coordinates)-1] {
				coordinates = coordinates[:len(coordinates)-1]
			}

			poly := &Polygon{Points: make([]Point, 0)}

			// Parse coordinates
			for _, coordinate := range coordinates {
				latLon := strings.Split(strings.TrimSpace(coordinate), ",")
				if len(latLon) >= 2 {
					lat, err := strconv.ParseFloat(latLon[1], 64)
					if err != nil {
						return err
					}
					lon, err := strconv.ParseFloat(latLon[0], 64)
					if err != nil {
						return err
					}
					poly.Points = append(poly.Points, Point{Lat: lat, Lon: lon})
				}
			}

			// Add to layer
			layer.PolygonLayer.Index.Insert(poly, poly.Bounds())
			log.Printf("Added polygon with %d points to layer %s\n", len(poly.Points), layer.Name)
		}

		// Process points
		for _, point := range points {
			latLon := strings.Split(strings.TrimSpace(point.Coordinates), ",")
			if len(latLon) >= 2 {
				lat, err := strconv.ParseFloat(latLon[1], 64)
				if err != nil {
					return err
				}
				lon, err := strconv.ParseFloat(latLon[0], 64)
				if err != nil {
					return err
				}

				p := NewPoint(lat, lon)

				// Handle icon styles
				styleURL := placemark.StyleURL
				if len(styleURL) > 0 {
					if styleURL[0] == '#' {
						styleURL = styleURL[1:]
					}
					if _, exists := game.StyleMap[styleURL]; !exists {
						if style, exists := game.IconStyles[styleURL]; exists {
							p.IconImage = game.IconImages[style.Href]
							p.Scale = style.Scale
							p.HotSpot = style.HotSpot
						}
					} else {
						normalStyle := game.StyleMap[styleURL]["normal"]
						if style, exists := game.IconStyles[normalStyle]; exists {
							p.IconImage = game.IconImages[style.Href]
							p.Scale = style.Scale
							p.HotSpot = style.HotSpot
						}
					}
				}

				// Add to layer
				layer.PointLayer.Index.Insert(p, p.Bounds())
				log.Printf("Added point to layer %s\n", layer.Name)
			}
		}
	}

	log.Printf("Processed %d placemarks\n", len(placemarks))
	return nil
}

func hexStringToColor(hex string) (color.RGBA, error) {
	if len(hex) != 8 {
		return color.RGBA{}, fmt.Errorf("invalid color string")
	}

	a, err := strconv.ParseUint(hex[0:2], 16, 8)
	if err != nil {
		return color.RGBA{}, err
	}

	b, err := strconv.ParseUint(hex[2:4], 16, 8)
	if err != nil {
		return color.RGBA{}, err
	}

	g, err := strconv.ParseUint(hex[4:6], 16, 8)
	if err != nil {
		return color.RGBA{}, err
	}

	r, err := strconv.ParseUint(hex[6:8], 16, 8)
	if err != nil {
		return color.RGBA{}, err
	}

	return color.RGBA{
		R: uint8(r),
		G: uint8(g),
		B: uint8(b),
		A: uint8(a),
	}, nil
}

func LoadKMLFile(filename string, game *Game, layer *Layer) error {
	var kmlData []byte
	var err error

	if strings.HasSuffix(strings.ToLower(filename), ".kmz") {
		// Read KMZ file
		r, err := zip.OpenReader(filename)
		if err != nil {
			return err
		}
		defer r.Close()

		// Find the KML file inside the KMZ archive
		for _, f := range r.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".kml") {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				defer rc.Close()

				kmlData, err = io.ReadAll(rc)
				if err != nil {
					return err
				}

				break
			}
		}

		if kmlData == nil {
			return fmt.Errorf("no KML file found in the KMZ archive")
		}

	} else {
		// Read KML file
		kmlData, err = os.ReadFile(filename)
		if err != nil {
			return err
		}
	}

	err = LoadKML(kmlData, game, layer)
	if err != nil {
		return err
	}

	return nil
}

func LoadKMLDroppedFiles(droppedFiles fs.FS, game *Game, layer *Layer) error {
	var kmlData []byte
	var wg sync.WaitGroup
	var mu sync.Mutex
	var loadErrors []error

	files, _ := fs.ReadDir(droppedFiles, ".")
	for _, fileEntry := range files {
		if !fileEntry.IsDir() {
			fileInfo, err := fileEntry.Info()
			if err != nil {
				log.Println("Error getting file info:", err)
				continue
			}
			fileSize := fileInfo.Size()

			file, err := droppedFiles.Open(fileEntry.Name())
			if err != nil {
				log.Println("Error opening file:", err)
				continue
			}
			defer file.Close()

			if strings.HasSuffix(strings.ToLower(fileEntry.Name()), ".kmz") {
				// Read KMZ file
				content, err := io.ReadAll(file)
				if err != nil {
					return err
				}
				contentReader := bytes.NewReader(content)

				r, err := zip.NewReader(contentReader, fileSize)
				if err != nil {
					return err
				}

				// Find the KML file and icons inside the KMZ archive
				for _, f := range r.File {
					if strings.HasSuffix(strings.ToLower(f.Name), ".kml") {
						rc, err := f.Open()
						if err != nil {
							return err
						}
						defer rc.Close()

						kmlData, err = io.ReadAll(rc)
						if err != nil {
							return err
						}
					} else if strings.HasSuffix(strings.ToLower(f.Name), ".png") || strings.HasSuffix(strings.ToLower(f.Name), ".jpg") {
						wg.Add(1)
						go func(f *zip.File) {
							defer wg.Done()
							rc, err := f.Open()
							if err != nil {
								mu.Lock()
								loadErrors = append(loadErrors, err)
								mu.Unlock()
								return
							}
							defer rc.Close()

							img, _, err := image.Decode(rc)
							if err != nil {
								mu.Lock()
								loadErrors = append(loadErrors, err)
								mu.Unlock()
								return
							}
							mu.Lock()
							game.IconImages[f.Name] = ebiten.NewImageFromImage(img)
							mu.Unlock()
						}(f)
					}
				}

				if kmlData == nil {
					return fmt.Errorf("no KML file found in the KMZ archive")
				}

			} else {
				// Read KML file
				kmlData, err = io.ReadAll(file)
				if err != nil {
					return err
				}
			}
		}
	}

	// Wait for all image processing goroutines to complete
	wg.Wait()

	if len(loadErrors) > 0 {
		return fmt.Errorf("errors occurred while loading KMZ images: %v", loadErrors)
	}

	// Load the KML data into the provided layer
	err := LoadKML(kmlData, game, layer)
	if err != nil {
		return err
	}

	return nil
}

func LoadKML(kmlData []byte, game *Game, layer *Layer) error {
	var err error

	// Check if the data is UTF-16 encoded and convert it to UTF-8 if necessary
	if kmlData[0] == 0xFF && kmlData[1] == 0xFE {
		log.Println("Found UTF-16 little Endian")
		decoder := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder()
		kmlData, err = io.ReadAll(transform.NewReader(bytes.NewReader(kmlData), decoder))
		if err != nil {
			return err
		}
	} else if kmlData[0] == 0xFE && kmlData[1] == 0xFF {
		log.Println("Found UTF-16 Big Endian")
		decoder := unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder()
		kmlData, err = io.ReadAll(transform.NewReader(bytes.NewReader(kmlData), decoder))
		if err != nil {
			return err
		}
	}

	kmlString := string(kmlData)
	// Update the encoding in the XML declaration
	kmlString = strings.Replace(kmlString, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
	// Remove the 'kml:' prefix from the KML data that some files seem to have..
	kmlString = strings.Replace(kmlString, "<kml:", "<", -1)
	kmlString = strings.Replace(kmlString, "</kml:", "</", -1)
	kmlData = []byte(kmlString)

	var kml KML
	err = xml.Unmarshal(kmlData, &kml)
	if err != nil {
		return err
	}

	// Process the Folders at the KML level
	err = processFoldersAndDocuments(kml.Folders, nil, game, layer)
	if err != nil {
		return err
	}

	// Process the Documents at the KML level
	err = processFoldersAndDocuments(nil, kml.Documents, game, layer)
	if err != nil {
		return err
	}

	// Force a redraw after loading
	game.needRedraw = true

	return nil
}
