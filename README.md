# Tracks

A private activity atlas built with Go and MapLibre. Import GPX files for hiking, running, cycling, swimming, and other activities; see every mappable route on a dark interactive map; and click a track for distance, elevation, date, and duration. Geotagged JPEG photos can also be imported and viewed at their GPS location; photos without embedded GPS information are skipped.

## Run

```sh
go run .
```

Open [http://localhost:8080](http://localhost:8080). Imported activities and photos are stored in the repository's local `data/` directory and are not sent to a third-party application server. The map background is loaded from CARTO/OpenStreetMap and requires an internet connection.

## Test

```sh
go test ./...
```