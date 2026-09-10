package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	fitdecoder "github.com/tormoder/fit"
)

const (
	maxTrackFileSize = 50 << 20
	maxTrackBatch    = 200 << 20
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
	NameSource       string           `json:"-"`
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

type trackImportResult struct {
	Imported []track          `json:"imported"`
	Rejected []trackRejection `json:"rejected"`
}

type trackRejection struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type trackMetadata struct {
	Name string `json:"name"`
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
	r.Body = http.MaxBytesReader(w, r.Body, maxTrackBatch)
	if err := r.ParseMultipartForm(maxTrackFileSize); err != nil {
		log.Printf("track upload failed: could not parse multipart form: %v", err)
		writeError(w, http.StatusBadRequest, "Upload up to 200 MiB per batch")
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
		log.Printf("track upload processing: filename=%q size=%d", header.Filename, header.Size)
		parsed, contents, err := readUploadedTrack(header)
		if err != nil {
			log.Printf("track upload rejected: filename=%q size=%d error=%v", header.Filename, header.Size, err)
			result.Rejected = append(result.Rejected, trackRejection{Name: header.Filename, Reason: err.Error()})
			continue
		}
		log.Printf("track upload resolved: filename=%q track_name=%q name_source=%q activity_type=%q started_at=%v", header.Filename, parsed.Name, parsed.NameSource, parsed.ActivityType, parsed.StartedAt)
		destination := filepath.Join(s.dataDir, parsed.ID+".gpx")
		if err := os.WriteFile(destination, contents, 0o644); err != nil {
			log.Printf("track upload failed: filename=%q track_name=%q id=%q size=%d destination=%q error=%v", header.Filename, parsed.Name, parsed.ID, len(contents), destination, err)
			writeError(w, http.StatusInternalServerError, "Could not save your activity")
			return
		}
		log.Printf("track upload saved: filename=%q track_name=%q name_source=%q id=%q size=%d destination=%q", header.Filename, parsed.Name, parsed.NameSource, parsed.ID, len(contents), destination)
		result.Imported = append(result.Imported, parsed)
	}

	log.Printf("track upload completed: imported=%d rejected=%d duration=%s", len(result.Imported), len(result.Rejected), time.Since(startedAt))
	writeJSON(w, http.StatusCreated, result)
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
	_ = os.Remove(filepath.Join(s.dataDir, id+".json"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) renameTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, "Invalid track ID")
		return
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, id+".gpx")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "Activity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not rename your activity")
		return
	}

	var request trackMetadata
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "Provide a valid activity name")
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len([]rune(request.Name)) > 200 {
		writeError(w, http.StatusBadRequest, "Activity name must be between 1 and 200 characters")
		return
	}

	contents, err := json.Marshal(request)
	if err != nil || os.WriteFile(filepath.Join(s.dataDir, id+".json"), contents, 0o644) != nil {
		writeError(w, http.StatusInternalServerError, "Could not rename your activity")
		return
	}
	log.Printf("track renamed: id=%q track_name=%q", id, request.Name)
	writeJSON(w, http.StatusOK, request)
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
		metadataContents, err := os.ReadFile(filepath.Join(s.dataDir, parsed.ID+".json"))
		if err == nil {
			var metadata trackMetadata
			if json.Unmarshal(metadataContents, &metadata) == nil && strings.TrimSpace(metadata.Name) != "" {
				parsed.Name = strings.TrimSpace(metadata.Name)
				parsed.NameSource = "user"
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
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
	if header.Size > maxTrackFileSize {
		return track{}, nil, errors.New("file exceeds the 50 MiB limit")
	}
	file, err := header.Open()
	if err != nil {
		return track{}, nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxTrackFileSize+1))
	if err != nil {
		return track{}, nil, err
	}
	if len(contents) > maxTrackFileSize {
		return track{}, nil, errors.New("file exceeds the 50 MiB limit")
	}
	nameSource := ""
	switch strings.ToLower(filepath.Ext(header.Filename)) {
	case ".gpx":
	case ".fit":
		contents, nameSource, err = fitToGPX(contents, header.Filename)
		if err != nil {
			return track{}, nil, err
		}
	default:
		return track{}, nil, errors.New("only .gpx and .fit files are supported")
	}
	parsed, err := parseGPX(contents, header.Filename)
	if nameSource != "" {
		parsed.NameSource = nameSource
	}
	return parsed, contents, err
}

func fitToGPX(contents []byte, fileName string) ([]byte, string, error) {
	fitFile, err := fitdecoder.Decode(bytes.NewReader(contents))
	if err != nil {
		return nil, "", errors.New("invalid FIT document")
	}
	activity, err := fitFile.Activity()
	if err != nil || activity == nil {
		return nil, "", errors.New("FIT file does not contain an activity")
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
			elevation := float64(record.EnhancedAltitude)/5 - 500
			point.Elevation = &elevation
		} else if record.Altitude != ^uint16(0) {
			elevation := float64(record.Altitude)/5 - 500
			point.Elevation = &elevation
		}
		segment.Points = append(segment.Points, point)
	}
	if len(segment.Points) == 0 {
		return nil, "", errors.New("FIT activity has no mappable GPS track points")
	}

	activityType := ""
	var startedAt time.Time
	if len(activity.Sessions) > 0 && activity.Sessions[0] != nil {
		session := activity.Sessions[0]
		activityType = normalizeFITEnum(session.Sport.String())
		subSport := normalizeFITEnum(session.SubSport.String())
		if subSport != "" && subSport != "generic" && subSport != "invalid" {
			if activityType == "running" && subSport == "trail" {
				subSport = "trail_running"
			}
			activityType = subSport
		}
		startedAt = session.StartTime
	}
	trackName := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	nameSource := "filename"
	if activityType != "" && !startedAt.IsZero() {
		trackName = activityName(activityType) + " · " + startedAt.Format("2 Jan 2006")
		nameSource = "activity_and_date"
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
			Name:     trackName,
			Type:     activityType,
			Segments: []gpxSegment{segment},
		}},
	}
	converted, err := xml.Marshal(document)
	if err != nil {
		return nil, "", errors.New("could not convert FIT activity to GPX")
	}
	return append([]byte(xml.Header), converted...), nameSource, nil
}

func normalizeFITEnum(value string) string {
	var normalized strings.Builder
	for index, character := range strings.TrimSpace(value) {
		if index > 0 && character >= 'A' && character <= 'Z' {
			normalized.WriteByte('_')
		}
		normalized.WriteRune(character)
	}
	return strings.ToLower(normalized.String())
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
		NameSource:       "gpx_metadata",
		Activity:         activityName(document.Tracks[0].Type),
		ActivityType:     normalizeActivityType(document.Tracks[0].Type),
		FileName:         filepath.Base(fileName),
		ElevationProfile: make([]elevationPoint, 0),
		Coordinates:      make([][][]float64, 0),
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
	if result.Name == "" {
		result.Name = strings.TrimSuffix(result.FileName, filepath.Ext(result.FileName))
		result.NameSource = "filename"
		if result.ActivityType != "" && !firstTime.IsZero() {
			result.Name = result.Activity + " · " + firstTime.Format("2 Jan 2006")
			result.NameSource = "activity_and_date"
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
