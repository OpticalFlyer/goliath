# Goliath

Another experimental GIS/CAD application.

## Features

- Interactive map interface
- Support for multiple basemaps
- Command input textbox for various operations
- Tile caching for improved performance

## Installation

- Install [Git](https://git-scm.com/downloads)
- Install [Go](https://go.dev/dl/)
- `go install github.com/OpticalFlyer/goliath@main`

## Usage

### Drawing

- PO    Point
- PL    Polyline
- POL   Polygon
- DEL   Delete

### Selection

shift+click (select/deselect toggle current layer)  
ctrl+click (select/deselect toggle all layers)  
ctrl+shift+click (deselect all layers)  

drag box (select, any layer, exclusive) - next selection replaces this one  
shift+drag box (select current layer)  
ctrl+drag box (select all layers)  
ctrl+shift+drag box (deselect all layers)
