# Trails

A private hiking atlas built with Go and MapLibre. Import GPX files, see every route on a dark interactive map, and click a trail for distance, elevation gain, date, and duration. Geotagged JPEG photos can also be imported and viewed at their GPS location; photos without embedded GPS information are skipped.

## Run

```sh
go run .
```

Open [http://localhost:8080](http://localhost:8080). Imported GPX files are stored in the local `data/` directory and are not sent to a third-party application server. The map background is loaded from CARTO/OpenStreetMap and requires an internet connection.

## Test

```sh
go test ./...
```