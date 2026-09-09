package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/jpeg"
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

	"github.com/rwcarlsen/goexif/exif"
)

const maxPhotoBatch = 100 << 20

type photo struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	TrackID          string     `json:"trackId,omitempty"`
	ManualAssignment bool       `json:"manualAssignment,omitempty"`
	HasLocation      bool       `json:"hasLocation"`
	Latitude         float64    `json:"latitude,omitempty"`
	Longitude        float64    `json:"longitude,omitempty"`
	CapturedAt       *time.Time `json:"capturedAt"`
	URL              string     `json:"url"`
	ContentType      string     `json:"-"`
}

type photoImportResult struct {
	Imported []photo          `json:"imported"`
	Rejected []photoRejection `json:"rejected"`
}

type photoRejection struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func (s *server) listPhotos(w http.ResponseWriter, _ *http.Request) {
	photos, err := s.loadPhotos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load your photos")
		return
	}
	tracks, err := s.loadTracks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not match photos to activities")
		return
	}
	for index := range photos {
		if photos[index].ManualAssignment {
			continue
		}
		if assignPhotoToNearestTrack(&photos[index], tracks) {
			if err := s.savePhotoMetadata(photos[index]); err != nil {
				writeError(w, http.StatusInternalServerError, "Could not match photos to activities")
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, photos)
}

func (s *server) importPhotos(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	targetTrackID := strings.TrimSpace(r.PathValue("id"))
	log.Printf("photo upload started: remote=%q target_track_id=%q content_length=%d content_type=%q data_dir=%q", r.RemoteAddr, targetTrackID, r.ContentLength, r.Header.Get("Content-Type"), s.dataDir)
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
	tracks, err := s.loadTracks()
	if err != nil {
		log.Printf("photo upload failed: could not load activities: %v", err)
		writeError(w, http.StatusInternalServerError, "Could not match photos to activities")
		return
	}
	if targetTrackID != "" {
		if !validID(targetTrackID) || !trackExists(tracks, targetTrackID) {
			writeError(w, http.StatusBadRequest, "Choose a valid activity for these photos")
			return
		}
	}

	result := photoImportResult{Imported: make([]photo, 0), Rejected: make([]photoRejection, 0)}
	for _, header := range files {
		log.Printf("photo upload processing: name=%q size=%d", header.Filename, header.Size)
		parsed, contents, err := readUploadedPhoto(header, targetTrackID != "")
		if err != nil {
			log.Printf("photo upload rejected: name=%q size=%d error=%v", header.Filename, header.Size, err)
			result.Rejected = append(result.Rejected, photoRejection{Name: header.Filename, Reason: err.Error()})
			continue
		}
		if targetTrackID != "" {
			parsed.TrackID = targetTrackID
			parsed.ManualAssignment = true
		} else {
			assignPhotoToNearestTrack(&parsed, tracks)
		}
		if err := s.savePhoto(parsed, contents); err != nil {
			log.Printf("photo upload failed: name=%q id=%q size=%d data_dir=%q error=%v", header.Filename, parsed.ID, len(contents), s.dataDir, err)
			writeError(w, http.StatusInternalServerError, "Could not save your photos")
			return
		}
		log.Printf("photo upload saved: name=%q id=%q track_id=%q size=%d", header.Filename, parsed.ID, parsed.TrackID, len(contents))
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
		if !item.HasLocation && (item.Latitude != 0 || item.Longitude != 0) {
			item.HasLocation = validCoordinates(item.Latitude, item.Longitude)
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
	if err := os.WriteFile(filepath.Join(directory, item.ID+".jpg"), contents, 0o644); err != nil {
		return err
	}
	return s.savePhotoMetadata(item)
}

func (s *server) savePhotoMetadata(item photo) error {
	metadata, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, "photos", item.ID+".json"), metadata, 0o644)
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

func readUploadedPhoto(header *multipart.FileHeader, allowMissingGPS bool) (photo, []byte, error) {
	file, err := header.Open()
	if err != nil {
		return photo{}, nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		return photo{}, nil, err
	}
	return parsePhoto(contents, header.Filename, allowMissingGPS)
}

func parsePhoto(contents []byte, fileName string, allowMissingGPS bool) (photo, []byte, error) {
	if _, err := jpeg.DecodeConfig(bytes.NewReader(contents)); err != nil {
		return photo{}, nil, errors.New("only JPEG photos are supported")
	}
	hash := sha256.Sum256(contents)
	id := hex.EncodeToString(hash[:8])
	result := photo{
		ID:          id,
		Name:        filepath.Base(fileName),
		URL:         "/api/photos/" + id + "/image",
		ContentType: "image/jpeg",
	}
	metadata, err := exif.Decode(bytes.NewReader(contents))
	if err != nil {
		if allowMissingGPS {
			return result, contents, nil
		}
		return photo{}, nil, errors.New("photo has no readable EXIF data")
	}
	if capturedAt, err := metadata.DateTime(); err == nil {
		result.CapturedAt = &capturedAt
	}
	latitude, longitude, err := metadata.LatLong()
	if err != nil {
		if allowMissingGPS {
			return result, contents, nil
		}
		return photo{}, nil, errors.New("photo has no GPS information")
	}
	if !validCoordinates(latitude, longitude) {
		if allowMissingGPS {
			return result, contents, nil
		}
		return photo{}, nil, errors.New("photo GPS coordinates are missing or invalid")
	}
	result.HasLocation = true
	result.Latitude = latitude
	result.Longitude = longitude
	return result, contents, nil
}

func trackExists(tracks []track, id string) bool {
	for _, item := range tracks {
		if item.ID == id {
			return true
		}
	}
	return false
}

func validCoordinates(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsNaN(longitude) &&
		!math.IsInf(latitude, 0) && !math.IsInf(longitude, 0) &&
		latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

func assignPhotoToNearestTrack(item *photo, tracks []track) bool {
	previousTrackID := item.TrackID
	item.TrackID = ""
	if item.CapturedAt == nil || !item.HasLocation {
		return previousTrackID != ""
	}

	closestDistance := math.Inf(1)
	for _, candidate := range tracks {
		if candidate.StartedAt == nil || !sameLocalDate(*candidate.StartedAt, *item.CapturedAt) {
			continue
		}
		for _, segment := range candidate.Coordinates {
			for _, coordinate := range segment {
				if len(coordinate) < 2 {
					continue
				}
				distance := haversine(item.Latitude, item.Longitude, coordinate[1], coordinate[0])
				if distance < closestDistance {
					closestDistance = distance
					item.TrackID = candidate.ID
				}
			}
		}
	}
	return item.TrackID != previousTrackID
}

func sameLocalDate(activityTime, photoTime time.Time) bool {
	activityYear, activityMonth, activityDay := activityTime.In(photoTime.Location()).Date()
	photoYear, photoMonth, photoDay := photoTime.Date()
	return activityYear == photoYear && activityMonth == photoMonth && activityDay == photoDay
}
