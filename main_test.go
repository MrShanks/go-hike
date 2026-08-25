package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
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

func TestParsePhotoRejectsJPEGWithoutGPS(t *testing.T) {
	imageBuffer := new(bytes.Buffer)
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageData.Set(0, 0, color.White)
	if err := jpeg.Encode(imageBuffer, imageData, nil); err != nil {
		t.Fatal(err)
	}
	_, _, err := parsePhoto(imageBuffer.Bytes(), "ordinary.jpg")
	if err == nil {
		t.Fatal("parsePhoto() accepted a JPEG without GPS information")
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
