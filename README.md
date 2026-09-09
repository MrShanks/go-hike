# Tracks

A private activity atlas built with Go and MapLibre. Import GPX or FIT files for hiking, running, cycling, swimming, and other activities; FIT activities are converted to GPX automatically. See every mappable route on a dark interactive map, inspect elevation profiles, and switch between flat and 3D terrain. Geotagged JPEG photos are matched to nearby same-day activities, while photos without GPS data can be added directly to an activity gallery.

## Run

```sh
go run .
```

Open [http://localhost:8080](http://localhost:8080). Imported activities and photos are stored in the repository's local `data/` directory and are not sent to a third-party application server. The map background is loaded from CARTO/OpenStreetMap and requires an internet connection.

## Docker Compose

Build and start Tracks in the background:

```sh
docker compose up --build -d
```

Open [http://localhost:8080](http://localhost:8080). The Compose configuration mounts the repository's `data/` directory into the container, so imports remain available after the container is restarted or replaced.

If port 8080 is already in use, select another host port and open that port instead:

```sh
TRACKS_PORT=18080 docker compose up --build -d
```

View logs or stop the application with:

```sh
docker compose ps
docker compose logs -f
docker compose down
```

## Docker

To build and run the image without Compose:

```sh
docker build -t tracks:local .
docker run --rm -p 8080:8080 \
	-v "$(pwd)/data:/app/data" \
	tracks:local
```

The bind mount keeps imported GPX and photo files in this repository's `data/` directory. The directory is excluded from the image build context so private activity data is not included in the image.

## Project layout

- `main.go` contains application startup, route registration, and shared HTTP helpers.
- `tracks.go` owns GPX/FIT parsing, track persistence, and track handlers.
- `photos.go` owns photo parsing, persistence, matching, and photo handlers.
- `web/app-core.js` contains shared UI state, DOM references, formatting, and notifications.
- `web/app-map.js` owns MapLibre setup, terrain, map layers, and markers.
- `web/app.js` owns activity and photo workflows and rendering.
- `web/app-events.js` owns event registration, drag and drop, and application bootstrap.
- `web/index.html` and `web/styles.css` define the embedded interface and visual system.
- `main_test.go` covers parsing, matching, imports, persistence, and route contracts.

## Test

```sh
go test ./...
```