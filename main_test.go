package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fitdecoder "github.com/tormoder/fit"
)

func TestParseGPXCalculatesTrackDetails(t *testing.T) {
	contents := []byte(`<?xml version="1.0"?><gpx><trk><name>Ridge Walk</name><type>running</type><trkseg>
		<trkpt lat="46.0000" lon="7.0000"><ele>1000</ele><name>Valley Station</name><time>2026-08-20T08:00:00Z</time></trkpt>
		<trkpt lat="46.0090" lon="7.0000"><ele>1120</ele><time>2026-08-20T08:30:00Z</time></trkpt>
		<trkpt lat="46.0090" lon="7.0130"><ele>1100</ele><name>Lake Hut</name><time>2026-08-20T09:00:00Z</time></trkpt>
	</trkseg></trk></gpx>`)

	result, err := parseGPX(contents, "ridge.gpx")
	if err != nil {
		t.Fatalf("parseGPX() error = %v", err)
	}
	if result.Name != "Ridge Walk" {
		t.Fatalf("Name = %q, want Ridge Walk", result.Name)
	}
	if result.Activity != "Running" || result.ActivityType != "running" {
		t.Errorf("Activity = %q (%q), want Running (running)", result.Activity, result.ActivityType)
	}
	if math.Abs(result.DistanceKM-2.0) > 0.05 {
		t.Errorf("DistanceKM = %.3f, want approximately 2.0", result.DistanceKM)
	}
	if result.ElevationGain != 120 {
		t.Errorf("ElevationGain = %.0f, want 120", result.ElevationGain)
	}
	if result.ElevationDescent != 20 {
		t.Errorf("ElevationDescent = %.0f, want 20", result.ElevationDescent)
	}
	if result.HighestPoint == nil || *result.HighestPoint != 1120 {
		t.Errorf("HighestPoint = %v, want 1120", result.HighestPoint)
	}
	if result.LowestPoint == nil || *result.LowestPoint != 1000 {
		t.Errorf("LowestPoint = %v, want 1000", result.LowestPoint)
	}
	if len(result.ElevationProfile) != 3 || result.ElevationProfile[2].DistanceKM != result.DistanceKM {
		t.Errorf("ElevationProfile = %#v, want three cumulative points", result.ElevationProfile)
	}
	if result.Start.Name != "Valley Station" || result.End.Name != "Lake Hut" {
		t.Errorf("Endpoints = %q to %q, want Valley Station to Lake Hut", result.Start.Name, result.End.Name)
	}
	if result.Duration != 3600 {
		t.Errorf("Duration = %d, want 3600", result.Duration)
	}
	if len(result.Coordinates) != 1 || len(result.Coordinates[0]) != 3 {
		t.Errorf("Coordinates shape = %d segments", len(result.Coordinates))
	}
}

func TestParseGPXAcceptsStationaryActivity(t *testing.T) {
	result, err := parseGPX([]byte(`<gpx><trk><name>Gym session</name><type>bouldering</type><trkseg><trkpt lat="46" lon="7"><time>2026-08-20T08:00:00Z</time></trkpt></trkseg></trk></gpx>`), "gym.gpx")
	if err != nil {
		t.Fatalf("parseGPX() error = %v", err)
	}
	if result.DistanceKM != 0 || len(result.Coordinates) != 1 || len(result.Coordinates[0]) != 1 {
		t.Errorf("stationary activity = %#v, want one point and zero distance", result)
	}
	if result.Start.Coordinates[0] != 7 || result.Start.Coordinates[1] != 46 {
		t.Errorf("location = %#v, want [7, 46]", result.Start.Coordinates)
	}
}

func TestParseGPXRejectsTrackWithoutPoints(t *testing.T) {
	_, err := parseGPX([]byte(`<gpx><trk><trkseg></trkseg></trk></gpx>`), "empty.gpx")
	if err == nil {
		t.Fatal("parseGPX() expected an error")
	}
}

func TestParseGPXHandlesMissingElevation(t *testing.T) {
	result, err := parseGPX([]byte(`<gpx><trk><trkseg>
		<trkpt lat="46" lon="7" /><trkpt lat="46.01" lon="7.01" />
	</trkseg></trk></gpx>`), "flat.gpx")
	if err != nil {
		t.Fatalf("parseGPX() error = %v", err)
	}
	if result.HighestPoint != nil || result.LowestPoint != nil || len(result.ElevationProfile) != 0 {
		t.Errorf("missing elevation produced extrema or profile: %#v", result)
	}
	if len(result.Coordinates[0][0]) != 2 {
		t.Errorf("coordinate has %d values, want longitude and latitude only", len(result.Coordinates[0][0]))
	}
}

func TestParseGPXBuildsNameFromTypeAndDate(t *testing.T) {
	contents := []byte(`<gpx><trk><type>hiking</type><trkseg>
		<trkpt lat="46" lon="7"><time>2026-08-20T08:00:00Z</time></trkpt>
	</trkseg></trk></gpx>`)

	result, err := parseGPX(contents, "activity.gpx")
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "Hiking · 20 Aug 2026" {
		t.Errorf("Name = %q, want Hiking · 20 Aug 2026", result.Name)
	}
}

func TestParseGPXNameFallsBackToFileName(t *testing.T) {
	contents := []byte(`<gpx><trk><type>hiking</type><trkseg>
		<trkpt lat="46" lon="7" />
	</trkseg></trk></gpx>`)

	result, err := parseGPX(contents, "fallback-name.gpx")
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "fallback-name" {
		t.Errorf("Name = %q, want fallback-name", result.Name)
	}
}

func TestParsePhotoRejectsJPEGWithoutGPS(t *testing.T) {
	imageBuffer := new(bytes.Buffer)
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageData.Set(0, 0, color.White)
	if err := jpeg.Encode(imageBuffer, imageData, nil); err != nil {
		t.Fatal(err)
	}
	_, _, err := parsePhoto(imageBuffer.Bytes(), "ordinary.jpg", false)
	if err == nil {
		t.Fatal("parsePhoto() accepted a JPEG without GPS information")
	}
}

func TestParsePhotoAllowsMissingGPSForExplicitTrack(t *testing.T) {
	imageBuffer := new(bytes.Buffer)
	if err := jpeg.Encode(imageBuffer, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	result, _, err := parsePhoto(imageBuffer.Bytes(), "ordinary.jpg", true)
	if err != nil {
		t.Fatalf("parsePhoto() error = %v", err)
	}
	if result.HasLocation || result.Name != "ordinary.jpg" {
		t.Errorf("photo = %#v, want named photo without location", result)
	}
}

func TestImportPhotoWithoutGPSToExplicitTrack(t *testing.T) {
	trackContents := []byte(`<gpx><trk><name>Ridge walk</name><trkseg><trkpt lat="46" lon="7"/></trkseg></trk></gpx>`)
	parsedTrack, err := parseGPX(trackContents, "ridge.gpx")
	if err != nil {
		t.Fatal(err)
	}
	app := &server{dataDir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(app.dataDir, parsedTrack.ID+".gpx"), trackContents, 0o644); err != nil {
		t.Fatal(err)
	}

	imageBuffer := new(bytes.Buffer)
	if err := jpeg.Encode(imageBuffer, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "ordinary.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(imageBuffer.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/tracks/"+parsedTrack.ID+"/photos", body)
	request.SetPathValue("id", parsedTrack.ID)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	app.importPhotos(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var result photoImportResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 || result.Imported[0].TrackID != parsedTrack.ID || !result.Imported[0].ManualAssignment || result.Imported[0].HasLocation {
		t.Fatalf("Imported = %#v, want assigned photo without location", result.Imported)
	}
	photos, err := app.loadPhotos()
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 1 || photos[0].TrackID != parsedTrack.ID {
		t.Fatalf("photos = %#v, want persisted track assignment", photos)
	}
}

func TestAssignPhotoToNearestTrackOnSameLocalDay(t *testing.T) {
	photoTime := time.Date(2026, 8, 23, 0, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	nearTime := time.Date(2026, 8, 22, 22, 15, 0, 0, time.UTC)
	farTime := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	item := photo{HasLocation: true, Latitude: 46.8, Longitude: 8.0, CapturedAt: &photoTime}
	tracks := []track{
		{ID: "near", StartedAt: &nearTime, Coordinates: [][][]float64{{{8.01, 46.8}}}},
		{ID: "far", StartedAt: &farTime, Coordinates: [][][]float64{{{9.0, 47.0}}}},
	}

	if !assignPhotoToNearestTrack(&item, tracks) {
		t.Fatal("assignPhotoToNearestTrack() did not report a changed assignment")
	}
	if item.TrackID != "near" {
		t.Errorf("TrackID = %q, want near", item.TrackID)
	}
}

func TestAssignPhotoToNearestTrackRequiresSameDay(t *testing.T) {
	photoTime := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	trackTime := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	item := photo{TrackID: "old", HasLocation: true, Latitude: 46.8, Longitude: 8.0, CapturedAt: &photoTime}

	if !assignPhotoToNearestTrack(&item, []track{{ID: "other", StartedAt: &trackTime, Coordinates: [][][]float64{{{8.0, 46.8}}}}}) {
		t.Fatal("assignPhotoToNearestTrack() did not clear a stale assignment")
	}
	if item.TrackID != "" {
		t.Errorf("TrackID = %q, want no assignment", item.TrackID)
	}
}

func TestValidID(t *testing.T) {
	if !validID("00e3a2b6c989c2d6") {
		t.Error("validID() rejected a hexadecimal ID")
	}
	if validID("../../etc/passwd") {
		t.Error("validID() accepted a path-like ID")
	}
}

func TestValidCoordinatesRejectsNonFiniteValues(t *testing.T) {
	if validCoordinates(math.NaN(), 7) || validCoordinates(46, math.Inf(1)) {
		t.Error("validCoordinates() accepted non-finite GPS coordinates")
	}
	if !validCoordinates(0, 0) {
		t.Error("validCoordinates() rejected the valid coordinate 0, 0")
	}
}

func TestActivityName(t *testing.T) {
	tests := map[string]string{
		"lap_swimming":    "Swimming",
		"gravel_cycling":  "Gravel cycling",
		"hiking":          "Hiking",
		"hikingTourTrail": "Hiking",
		"bouldering":      "Bouldering",
		"":                "Activity",
	}
	for activityType, want := range tests {
		if got := activityName(activityType); got != want {
			t.Errorf("activityName(%q) = %q, want %q", activityType, got, want)
		}
	}
}

func TestImportTracksKeepsValidFilesWhenAnotherIsRejected(t *testing.T) {
	valid := []byte(`<gpx><trk><name>Run</name><type>running</type><trkseg>
		<trkpt lat="46" lon="7"/><trkpt lat="46.01" lon="7.01"/>
	</trkseg></trk></gpx>`)
	invalid := []byte(`<gpx><trk><name>Pool swim</name><type>lap_swimming</type><trkseg/></trk></gpx>`)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	for name, contents := range map[string][]byte{"run.gpx": valid, "swim.gpx": invalid} {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/tracks", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	app := &server{dataDir: t.TempDir()}
	app.importTracks(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var result trackImportResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 || result.Imported[0].Activity != "Running" {
		t.Errorf("Imported = %#v, want one running activity", result.Imported)
	}
	if len(result.Rejected) != 1 || result.Rejected[0].Name != "swim.gpx" || result.Rejected[0].Reason == "" {
		t.Errorf("Rejected = %#v, want swim.gpx with a reason", result.Rejected)
	}
	if matches, _ := filepath.Glob(filepath.Join(app.dataDir, "*.gpx")); len(matches) != 1 {
		t.Errorf("saved files = %d, want 1", len(matches))
	}
}

func TestRenameTrackPersistsNameOverride(t *testing.T) {
	contents := []byte(`<gpx><trk><name>Original name</name><type>hiking</type><trkseg>
		<trkpt lat="46" lon="7"><time>2026-08-20T08:00:00Z</time></trkpt>
	</trkseg></trk></gpx>`)
	parsed, err := parseGPX(contents, "original.gpx")
	if err != nil {
		t.Fatal(err)
	}
	app := &server{dataDir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(app.dataDir, parsed.ID+".gpx"), contents, 0o644); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPatch, "/api/tracks/"+parsed.ID, strings.NewReader(`{"name":"  Evening ridge walk  "}`))
	request.SetPathValue("id", parsed.ID)
	recorder := httptest.NewRecorder()
	app.renameTrack(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	tracks, err := app.loadTracks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].Name != "Evening ridge walk" || tracks[0].NameSource != "user" {
		t.Fatalf("tracks = %#v, want persisted user name", tracks)
	}
}

func TestDeleteResourceRoutesRemoveFiles(t *testing.T) {
	const id = "0123456789abcdef"
	tests := []struct {
		name  string
		path  string
		files []string
	}{
		{name: "track", path: "/api/tracks/" + id, files: []string{id + ".gpx", id + ".json"}},
		{name: "photo", path: "/api/photos/" + id, files: []string{filepath.Join("photos", id+".jpg"), filepath.Join("photos", id+".json")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			app := &server{dataDir: dataDir}
			for _, name := range test.files {
				path := filepath.Join(dataDir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			recorder := httptest.NewRecorder()
			app.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, test.path, nil))
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
			}
			for _, name := range test.files {
				if _, err := os.Stat(filepath.Join(dataDir, name)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s still exists after deletion", name)
				}
			}
		})
	}
}

func TestDeleteResourceRoutesRejectInvalidIDs(t *testing.T) {
	app := &server{dataDir: t.TempDir()}
	for _, path := range []string{"/api/tracks/not-an-id", "/api/photos/not-an-id"} {
		recorder := httptest.NewRecorder()
		app.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("DELETE %s status = %d, want %d", path, recorder.Code, http.StatusBadRequest)
		}
	}
}

func TestImportFITConvertsAndSavesGPX(t *testing.T) {
	header := fitdecoder.NewHeader(fitdecoder.V20, false)
	fitFile, err := fitdecoder.NewFile(fitdecoder.FileTypeActivity, header)
	if err != nil {
		t.Fatal(err)
	}
	activity, err := fitFile.Activity()
	if err != nil {
		t.Fatal(err)
	}
	session := fitdecoder.NewSessionMsg()
	session.Sport = fitdecoder.SportRunning
	session.SubSport = fitdecoder.SubSportTrail
	session.StartTime = time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	activity.Sessions = append(activity.Sessions, session)
	for index, coordinates := range [][2]float64{{46, 7}, {46.01, 7.01}} {
		record := fitdecoder.NewRecordMsg()
		record.Timestamp = time.Date(2026, 8, 20, 8, index*30, 0, 0, time.UTC)
		record.PositionLat = fitdecoder.NewLatitudeDegrees(coordinates[0])
		record.PositionLong = fitdecoder.NewLongitudeDegrees(coordinates[1])
		activity.Records = append(activity.Records, record)
	}

	fitContents := new(bytes.Buffer)
	if err := fitdecoder.Encode(fitContents, fitFile, binary.LittleEndian); err != nil {
		t.Fatal(err)
	}
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "morning-run.fit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(fitContents.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/tracks", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	app := &server{dataDir: t.TempDir()}
	app.importTracks(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var result trackImportResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 || result.Imported[0].Name != "Running · 20 Aug 2026" || result.Imported[0].Activity != "Running" || result.Imported[0].ActivityType != "trail_running" || result.Imported[0].FileName != "morning-run.fit" {
		t.Fatalf("Imported = %#v, want one converted running activity", result.Imported)
	}
	matches, err := filepath.Glob(filepath.Join(app.dataDir, "*.gpx"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("saved GPX files = %v, error = %v", matches, err)
	}
	converted, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(converted), "<gpx") || strings.Contains(string(converted), ".FIT") {
		t.Errorf("saved file is not converted GPX: %q", converted)
	}
}

func TestFITTrackNameFallsBackToFileName(t *testing.T) {
	header := fitdecoder.NewHeader(fitdecoder.V20, false)
	fitFile, err := fitdecoder.NewFile(fitdecoder.FileTypeActivity, header)
	if err != nil {
		t.Fatal(err)
	}
	activity, err := fitFile.Activity()
	if err != nil {
		t.Fatal(err)
	}
	record := fitdecoder.NewRecordMsg()
	record.PositionLat = fitdecoder.NewLatitudeDegrees(46)
	record.PositionLong = fitdecoder.NewLongitudeDegrees(7)
	activity.Records = append(activity.Records, record)

	fitContents := new(bytes.Buffer)
	if err := fitdecoder.Encode(fitContents, fitFile, binary.LittleEndian); err != nil {
		t.Fatal(err)
	}
	converted, nameSource, err := fitToGPX(fitContents.Bytes(), "fallback-name.fit")
	if err != nil {
		t.Fatal(err)
	}
	if nameSource != "filename" {
		t.Errorf("nameSource = %q, want filename", nameSource)
	}
	parsed, err := parseGPX(converted, "fallback-name.fit")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "fallback-name" {
		t.Errorf("Name = %q, want fallback-name", parsed.Name)
	}
}
