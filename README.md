# Tracks

A private activity atlas built with Go and MapLibre. Import GPX or FIT files for hiking, running, cycling, swimming, and other activities; FIT activities are converted to GPX automatically. See every mappable route on a dark interactive map and click a track for distance, elevation, date, and duration. Geotagged JPEG photos can also be imported and viewed at their GPS location; photos without embedded GPS information are skipped.

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

## Test

```sh
go test ./...
```