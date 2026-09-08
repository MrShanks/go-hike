package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"image/jpeg"
	"io"
	"io/fs"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	fitdecoder "github.com/tormoder/fit"
)

const (
	maxUploadSize = 25 << 20
	maxPhotoBatch = 100 << 20
)

type gpxDocument struct {
	Tracks []gpxTrack `xml:"trk"`
}

type gpxTrack struct {
	Name     string       `xml:"name"`
	Type     string       `xml:"type"`
	Segments []gpxSegment `xml:"trkseg"`
}

type gpxSegment struct {
	Points []gpxPoint `xml:"trkpt"`
}

type gpxPoint struct {
	Latitude  float64   `xml:"lat,attr"`
	Longitude float64   `xml:"lon,attr"`
	Elevation *float64  `xml:"ele"`
	Name      string    `xml:"name"`
	Time      time.Time `xml:"time"`
}

type elevationPoint struct {
	DistanceKM float64 `json:"distanceKm"`
	Elevation  float64 `json:"elevation"`
}

type trackEndpoint struct {
	Name        string    `json:"name"`
	Coordinates []float64 `json:"coordinates"`
}

type track struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Activity         string           `json:"activity"`
	ActivityType     string           `json:"activityType"`
	FileName         string           `json:"fileName"`
	DistanceKM       float64          `json:"distanceKm"`
	ElevationGain    float64          `json:"elevationGain"`
	ElevationDescent float64          `json:"elevationDescent"`
	HighestPoint     *float64         `json:"highestPoint"`
	LowestPoint      *float64         `json:"lowestPoint"`
	ElevationProfile []elevationPoint `json:"elevationProfile"`
	Start            trackEndpoint    `json:"start"`
	End              trackEndpoint    `json:"end"`
	Duration         int64            `json:"duration"`
	StartedAt        *time.Time       `json:"startedAt"`
	Coordinates      [][][]float64    `json:"coordinates"`
}

type photo struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Latitude    float64    `json:"latitude"`
	Longitude   float64    `json:"longitude"`
	CapturedAt  *time.Time `json:"capturedAt"`
	URL         string     `json:"url"`
	ContentType string     `json:"-"`
}

type photoImportResult struct {
	Imported []photo          `json:"imported"`
	Rejected []photoRejection `json:"rejected"`
}

type photoRejection struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type trackImportResult struct {
	Imported []track          `json:"imported"`
	Rejected []trackRejection `json:"rejected"`
}

type trackRejection struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type server struct {
	dataDir string
	static  fs.FS
}

func main() {
	static, err := fs.Sub(assets, "web")
	if err != nil {
		log.Fatal(err)
	}

	app := &server{dataDir: "data", static: static}
	if err := os.MkdirAll(app.dataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	address := ":8080"
	log.Printf("Tracks is running at http://localhost%s", address)
	log.Fatal(http.ListenAndServe(address, app.routes()))
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tracks", s.listTracks)
	mux.HandleFunc("POST /api/tracks", s.importTracks)
	mux.HandleFunc("DELETE /api/tracks/{id}", s.deleteTrack)
	mux.HandleFunc("GET /api/photos", s.listPhotos)
	mux.HandleFunc("POST /api/photos", s.importPhotos)
	mux.HandleFunc("GET /api/photos/{id}/image", s.servePhoto)
	mux.HandleFunc("DELETE /api/photos/{id}", s.deletePhoto)
	mux.Handle("/", http.FileServer(http.FS(s.static)))
	return mux
}

func (s *server) listPhotos(w http.ResponseWriter, _ *http.Request) {
	photos, err := s.loadPhotos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load your photos")
		return
	}
	writeJSON(w, http.StatusOK, photos)
}

func (s *server) importPhotos(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	log.Printf("photo upload started: remote=%q content_length=%d content_type=%q data_dir=%q", r.RemoteAddr, r.ContentLength, r.Header.Get("Content-Type"), s.dataDir)
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBatch)
	if err := r.ParseMultipartForm(maxPhotoBatch); err != nil {
		log.Printf("photo upload failed: could not parse multipart form: %v", err)
		writeError(w, http.StatusBadRequest, "Upload photos up to 100 MB per batch")
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		log.Printf("photo upload failed: multipart form contains no files field; fields=%v", multipartFieldNames(r.MultipartForm.File))
		writeError(w, http.StatusBadRequest, "Choose at least one photo")
		return
	}
	log.Printf("photo upload parsed: files=%d", len(files))

	result := photoImportResult{Imported: make([]photo, 0), Rejected: make([]photoRejection, 0)}
	for _, header := range files {
		log.Printf("photo upload processing: name=%q size=%d", header.Filename, header.Size)
		parsed, contents, err := readUploadedPhoto(header)
		if err != nil {
			log.Printf("photo upload rejected: name=%q size=%d error=%v", header.Filename, header.Size, err)
			result.Rejected = append(result.Rejected, photoRejection{Name: header.Filename, Reason: err.Error()})
			continue
		}
		if err := s.savePhoto(parsed, contents); err != nil {
			log.Printf("photo upload failed: name=%q id=%q size=%d data_dir=%q error=%v", header.Filename, parsed.ID, len(contents), s.dataDir, err)
			writeError(w, http.StatusInternalServerError, "Could not save your photos")
			return
		}
		log.Printf("photo upload saved: name=%q id=%q size=%d", header.Filename, parsed.ID, len(contents))
		result.Imported = append(result.Imported, parsed)
	}
	log.Printf("photo upload completed: imported=%d rejected=%d duration=%s", len(result.Imported), len(result.Rejected), time.Since(startedAt))
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) servePhoto(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		http.NotFound(w, r)
		return
	}
	_, err := s.readPhotoMetadata(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, filepath.Join(s.dataDir, "photos", id+".jpg"))
}

func (s *server) deletePhoto(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, "Invalid photo ID")
		return
	}
	if err := os.Remove(filepath.Join(s.dataDir, "photos", id+".jpg")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "Photo not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not delete photo")
		return
	}
	_ = os.Remove(filepath.Join(s.dataDir, "photos", id+".json"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) loadPhotos() ([]photo, error) {
	entries, err := filepath.Glob(filepath.Join(s.dataDir, "photos", "*.json"))
	if err != nil {
		return nil, err
	}
	photos := make([]photo, 0, len(entries))
	for _, path := range entries {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var item photo
		if err := json.Unmarshal(contents, &item); err != nil {
			log.Printf("skipping invalid photo metadata %s: %v", path, err)
			continue
		}
		photos = append(photos, item)
	}
	sort.Slice(photos, func(i, j int) bool {
		if photos[i].CapturedAt == nil {
			return false
		}
		if photos[j].CapturedAt == nil {
			return true
		}
		return photos[i].CapturedAt.After(*photos[j].CapturedAt)
	})
	return photos, nil
}

func (s *server) savePhoto(item photo, contents []byte) error {
	directory := filepath.Join(s.dataDir, "photos")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	metadata, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, item.ID+".jpg"), contents, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, item.ID+".json"), metadata, 0o644)
}

func (s *server) readPhotoMetadata(id string) (photo, error) {
	contents, err := os.ReadFile(filepath.Join(s.dataDir, "photos", id+".json"))
	if err != nil {
		return photo{}, err
	}
	var item photo
	err = json.Unmarshal(contents, &item)
	return item, err
}

func readUploadedPhoto(header *multipart.FileHeader) (photo, []byte, error) {
	file, err := header.Open()
	if err != nil {
		return photo{}, nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		return photo{}, nil, err
	}
	return parsePhoto(contents, header.Filename)
}

func parsePhoto(contents []byte, fileName string) (photo, []byte, error) {
	if _, err := jpeg.DecodeConfig(bytes.NewReader(contents)); err != nil {
		return photo{}, nil, errors.New("only JPEG photos are supported")
	}
	metadata, err := exif.Decode(bytes.NewReader(contents))
	if err != nil {
		return photo{}, nil, errors.New("photo has no readable EXIF data")
	}
	latitude, longitude, err := metadata.LatLong()
	if err != nil {
		return photo{}, nil, errors.New("photo has no GPS information")
	}
	if !validCoordinates(latitude, longitude) {
		return photo{}, nil, errors.New("photo GPS coordinates are missing or invalid")
	}
	hash := sha256.Sum256(contents)
	id := hex.EncodeToString(hash[:8])
	result := photo{
		ID:          id,
		Name:        filepath.Base(fileName),
		Latitude:    latitude,
		Longitude:   longitude,
		URL:         "/api/photos/" + id + "/image",
		ContentType: "image/jpeg",
	}
	if capturedAt, err := metadata.DateTime(); err == nil {
		result.CapturedAt = &capturedAt
	}
	return result, contents, nil
}

func validID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func validCoordinates(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsNaN(longitude) &&
		!math.IsInf(latitude, 0) && !math.IsInf(longitude, 0) &&
		latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

func (s *server) listTracks(w http.ResponseWriter, _ *http.Request) {
	tracks, err := s.loadTracks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load your activities")
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *server) importTracks(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	log.Printf("track upload started: remote=%q content_length=%d content_type=%q data_dir=%q", r.RemoteAddr, r.ContentLength, r.Header.Get("Content-Type"), s.dataDir)
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		log.Printf("track upload failed: could not parse multipart form: %v", err)
		writeError(w, http.StatusBadRequest, "Upload GPX or FIT files up to 25 MB")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		log.Printf("track upload failed: multipart form contains no files field; fields=%v", multipartFieldNames(r.MultipartForm.File))
		writeError(w, http.StatusBadRequest, "Choose at least one GPX or FIT file")
		return
	}
	log.Printf("track upload parsed: files=%d", len(files))

	result := trackImportResult{Imported: make([]track, 0, len(files)), Rejected: make([]trackRejection, 0)}
	for _, header := range files {
		log.Printf("track upload processing: name=%q size=%d", header.Filename, header.Size)
		parsed, contents, err := readUploadedTrack(header)
		if err != nil {
			log.Printf("track upload rejected: name=%q size=%d error=%v", header.Filename, header.Size, err)
			result.Rejected = append(result.Rejected, trackRejection{Name: header.Filename, Reason: err.Error()})
			continue
		}
		destination := filepath.Join(s.dataDir, parsed.ID+".gpx")
		if err := os.WriteFile(destination, contents, 0o644); err != nil {
			log.Printf("track upload failed: name=%q id=%q size=%d destination=%q error=%v", header.Filename, parsed.ID, len(contents), destination, err)
			writeError(w, http.StatusInternalServerError, "Could not save your activity")
			return
		}
		log.Printf("track upload saved: name=%q id=%q size=%d destination=%q", header.Filename, parsed.ID, len(contents), destination)
		result.Imported = append(result.Imported, parsed)
	}

	log.Printf("track upload completed: imported=%d rejected=%d duration=%s", len(result.Imported), len(result.Rejected), time.Since(startedAt))
	writeJSON(w, http.StatusCreated, result)
}

func multipartFieldNames(files map[string][]*multipart.FileHeader) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *server) deleteTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, "Invalid track ID")
		return
	}
	if err := os.Remove(filepath.Join(s.dataDir, id+".gpx")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "Activity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not delete your activity")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) loadTracks() ([]track, error) {
	entries, err := filepath.Glob(filepath.Join(s.dataDir, "*.gpx"))
	if err != nil {
		return nil, err
	}

	tracks := make([]track, 0, len(entries))
	for _, path := range entries {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		parsed, err := parseGPX(contents, filepath.Base(path))
		if err != nil {
			log.Printf("skipping invalid GPX file %s: %v", path, err)
			continue
		}
		tracks = append(tracks, parsed)
	}
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].StartedAt == nil {
			return false
		}
		if tracks[j].StartedAt == nil {
			return true
		}
		return tracks[i].StartedAt.After(*tracks[j].StartedAt)
	})
	return tracks, nil
}

func readUploadedTrack(header *multipart.FileHeader) (track, []byte, error) {
	file, err := header.Open()
	if err != nil {
		return track{}, nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		return track{}, nil, err
	}
	switch strings.ToLower(filepath.Ext(header.Filename)) {
	case ".gpx":
	case ".fit":
		contents, err = fitToGPX(contents, header.Filename)
		if err != nil {
			return track{}, nil, err
		}
	default:
		return track{}, nil, errors.New("only .gpx and .fit files are supported")
	}
	parsed, err := parseGPX(contents, header.Filename)
	return parsed, contents, err
}

func fitToGPX(contents []byte, fileName string) ([]byte, error) {
	fitFile, err := fitdecoder.Decode(bytes.NewReader(contents))
	if err != nil {
		return nil, errors.New("invalid FIT document")
	}
	activity, err := fitFile.Activity()
	if err != nil || activity == nil {
		return nil, errors.New("FIT file does not contain an activity")
	}

	segment := gpxSegment{Points: make([]gpxPoint, 0, len(activity.Records))}
	for _, record := range activity.Records {
		if record == nil || record.PositionLat.Invalid() || record.PositionLong.Invalid() {
			continue
		}
		point := gpxPoint{
			Latitude:  record.PositionLat.Degrees(),
			Longitude: record.PositionLong.Degrees(),
		}
		if !record.Timestamp.IsZero() {
			point.Time = record.Timestamp
		}
		if record.EnhancedAltitude != ^uint32(0) {
			elevation := float64(record.EnhancedAltitude)/1000 - 500
			point.Elevation = &elevation
		} else if record.Altitude != ^uint16(0) {
			elevation := float64(record.Altitude)/5 - 500
			point.Elevation = &elevation
		}
		segment.Points = append(segment.Points, point)
	}
	if len(segment.Points) == 0 {
		return nil, errors.New("FIT activity has no mappable GPS track points")
	}

	activityType := ""
	if len(activity.Sessions) > 0 && activity.Sessions[0] != nil {
		activityType = strings.ToLower(activity.Sessions[0].Sport.String())
	}
	document := struct {
		XMLName xml.Name   `xml:"gpx"`
		Version string     `xml:"version,attr"`
		Creator string     `xml:"creator,attr"`
		Tracks  []gpxTrack `xml:"trk"`
	}{
		Version: "1.1",
		Creator: "Tracks",
		Tracks: []gpxTrack{{
			Name:     strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)),
			Type:     activityType,
			Segments: []gpxSegment{segment},
		}},
	}
	converted, err := xml.Marshal(document)
	if err != nil {
		return nil, errors.New("could not convert FIT activity to GPX")
	}
	return append([]byte(xml.Header), converted...), nil
}

func parseGPX(contents []byte, fileName string) (track, error) {
	var document gpxDocument
	if err := xml.Unmarshal(contents, &document); err != nil {
		return track{}, errors.New("invalid GPX document")
	}
	if len(document.Tracks) == 0 {
		return track{}, errors.New("no tracks found")
	}

	hash := sha256.Sum256(contents)
	result := track{
		ID:               hex.EncodeToString(hash[:8]),
		Name:             strings.TrimSpace(document.Tracks[0].Name),
		Activity:         activityName(document.Tracks[0].Type),
		ActivityType:     normalizeActivityType(document.Tracks[0].Type),
		FileName:         filepath.Base(fileName),
		ElevationProfile: make([]elevationPoint, 0),
		Coordinates:      make([][][]float64, 0),
	}
	if result.Name == "" {
		result.Name = strings.TrimSuffix(result.FileName, filepath.Ext(result.FileName))
	}

	var firstTime, lastTime time.Time
	var firstPoint, lastPoint *gpxPoint
	for _, sourceTrack := range document.Tracks {
		for _, segment := range sourceTrack.Segments {
			if len(segment.Points) == 0 {
				continue
			}
			coordinates := make([][]float64, 0, len(segment.Points))
			for index, point := range segment.Points {
				coordinate := []float64{point.Longitude, point.Latitude}
				if point.Elevation != nil {
					coordinate = append(coordinate, *point.Elevation)
				}
				coordinates = append(coordinates, coordinate)
				if firstPoint == nil {
					pointCopy := point
					firstPoint = &pointCopy
				}
				pointCopy := point
				lastPoint = &pointCopy
				if !point.Time.IsZero() {
					if firstTime.IsZero() || point.Time.Before(firstTime) {
						firstTime = point.Time
					}
					if lastTime.IsZero() || point.Time.After(lastTime) {
						lastTime = point.Time
					}
				}
				if index > 0 {
					previous := segment.Points[index-1]
					result.DistanceKM += haversine(previous.Latitude, previous.Longitude, point.Latitude, point.Longitude)
					if previous.Elevation != nil && point.Elevation != nil {
						change := *point.Elevation - *previous.Elevation
						if change > 0 {
							result.ElevationGain += change
						} else {
							result.ElevationDescent -= change
						}
					}
				}
				if point.Elevation != nil {
					result.ElevationProfile = append(result.ElevationProfile, elevationPoint{DistanceKM: result.DistanceKM, Elevation: *point.Elevation})
					if result.HighestPoint == nil || *point.Elevation > *result.HighestPoint {
						elevation := *point.Elevation
						result.HighestPoint = &elevation
					}
					if result.LowestPoint == nil || *point.Elevation < *result.LowestPoint {
						elevation := *point.Elevation
						result.LowestPoint = &elevation
					}
				}
			}
			result.Coordinates = append(result.Coordinates, coordinates)
		}
	}
	if len(result.Coordinates) == 0 {
		return track{}, errors.New("activity has no mappable GPS track points")
	}
	result.Start = endpointFromPoint(firstPoint, "Start")
	result.End = endpointFromPoint(lastPoint, "Finish")
	if !firstTime.IsZero() {
		result.StartedAt = &firstTime
		if lastTime.After(firstTime) {
			result.Duration = int64(lastTime.Sub(firstTime).Seconds())
		}
	}
	return result, nil
}

func normalizeActivityType(activityType string) string {
	return strings.ToLower(strings.TrimSpace(activityType))
}

func activityName(activityType string) string {
	switch normalizeActivityType(activityType) {
	case "hiking", "hikingtourtrail", "walking", "mountaineering":
		return "Hiking"
	case "running", "trail_running":
		return "Running"
	case "lap_swimming", "open_water_swimming", "swimming":
		return "Swimming"
	case "cycling", "road_cycling":
		return "Cycling"
	case "gravel_cycling":
		return "Gravel cycling"
	case "bouldering", "rock_climbing", "climbing":
		return "Bouldering"
	case "":
		return "Activity"
	default:
		words := strings.Fields(strings.ReplaceAll(normalizeActivityType(activityType), "_", " "))
		for index := range words {
			words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
		}
		return strings.Join(words, " ")
	}
}

func endpointFromPoint(point *gpxPoint, fallbackName string) trackEndpoint {
	name := strings.TrimSpace(point.Name)
	if name == "" {
		name = fallbackName
	}
	return trackEndpoint{Name: name, Coordinates: []float64{point.Longitude, point.Latitude}}
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371.0088
	latitudeDelta := (lat2 - lat1) * math.Pi / 180
	longitudeDelta := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
