const state = { tracks: [], photos: [], selectedId: null, selectedPhotoId: null, mapReady: false, endpointMarkers: [], photoMarkers: [] };
const palette = ["#d7ff43", "#ff8a5b", "#55d8ff", "#f0bbff", "#72e6a1", "#ffd166"];
const elements = {
  fileInput: document.querySelector("#file-input"),
  photoInput: document.querySelector("#photo-input"),
  trackList: document.querySelector("#track-list"),
  emptyState: document.querySelector("#empty-state"),
  detailPanel: document.querySelector("#detail-panel"),
  photoPanel: document.querySelector("#photo-panel"),
  dropOverlay: document.querySelector("#drop-overlay"),
  toast: document.querySelector("#toast"),
};

lucide.createIcons();

const map = new maplibregl.Map({
  container: "map",
  style: "https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json",
  center: [8.2, 46.8],
  zoom: 4.4,
  attributionControl: false,
});
map.addControl(new maplibregl.NavigationControl({ showCompass: true }), "bottom-right");
map.addControl(new maplibregl.AttributionControl({ compact: true }), "bottom-left");

map.on("load", () => {
  map.addSource("hikes", { type: "geojson", data: featureCollection() });
  map.addLayer({
    id: "hike-casing",
    type: "line",
    source: "hikes",
    paint: { "line-color": "#070809", "line-width": ["interpolate", ["linear"], ["zoom"], 3, 3, 12, 9], "line-opacity": 0.75 },
  });
  map.addLayer({
    id: "hikes",
    type: "line",
    source: "hikes",
    paint: {
      "line-color": ["get", "color"],
      "line-width": ["interpolate", ["linear"], ["zoom"], 3, 1.5, 12, 4.5],
      "line-opacity": ["case", ["==", ["get", "id"], ""], 1, 0.9],
    },
  });
  state.mapReady = true;
  syncMap();
});

map.on("click", "hikes", (event) => {
  const id = event.features?.[0]?.properties?.id;
  if (id) selectTrack(id, true);
});
map.on("mouseenter", "hikes", () => { map.getCanvas().style.cursor = "pointer"; });
map.on("mouseleave", "hikes", () => { map.getCanvas().style.cursor = ""; });

function featureCollection() {
  return {
    type: "FeatureCollection",
    features: state.tracks.map((track, index) => ({
      type: "Feature",
      properties: { id: track.id, name: track.name, color: palette[index % palette.length] },
      geometry: { type: "MultiLineString", coordinates: track.coordinates },
    })),
  };
}

function syncMap() {
  if (!state.mapReady) return;
  map.getSource("hikes").setData(featureCollection());
  syncEndpointMarkers();
  syncPhotoMarkers();
}

function syncPhotoMarkers() {
  state.photoMarkers.forEach((marker) => marker.remove());
  state.photoMarkers = [];
  state.photos.forEach((photo) => {
    const element = document.createElement("button");
    element.className = `photo-marker ${photo.id === state.selectedPhotoId ? "active" : ""}`;
    element.type = "button";
    element.title = photo.name;
    element.style.backgroundImage = `url("${photo.url}")`;
    element.addEventListener("click", (event) => {
      event.stopPropagation();
      selectPhoto(photo.id);
    });
    const marker = new maplibregl.Marker({ element, anchor: "bottom" }).setLngLat([photo.longitude, photo.latitude]).addTo(map);
    state.photoMarkers.push(marker);
  });
}

function syncEndpointMarkers() {
  state.endpointMarkers.forEach((marker) => marker.remove());
  state.endpointMarkers = [];
  const track = state.tracks.find((candidate) => candidate.id === state.selectedId);
  if (!track?.start?.coordinates || !track?.end?.coordinates) return;
  [{ endpoint: track.start, label: "S", className: "" }, { endpoint: track.end, label: "F", className: "finish" }].forEach((item) => {
    const element = document.createElement("div");
    element.className = `endpoint-marker ${item.className}`;
    element.textContent = item.label;
    element.title = item.endpoint.name;
    const marker = new maplibregl.Marker({ element }).setLngLat(item.endpoint.coordinates).addTo(map);
    state.endpointMarkers.push(marker);
  });
}

function render() {
  const totalDistance = state.tracks.reduce((sum, track) => sum + track.distanceKm, 0);
  const totalAscent = state.tracks.reduce((sum, track) => sum + track.elevationGain, 0);
  const totalDescent = state.tracks.reduce((sum, track) => sum + track.elevationDescent, 0);
  document.querySelector("#hike-count").textContent = `${state.tracks.length} ${state.tracks.length === 1 ? "hike" : "hikes"}`;
  document.querySelector("#total-distance").textContent = formatDistance(totalDistance, false);
  document.querySelector("#total-ascent").textContent = Math.round(totalAscent).toLocaleString();
  document.querySelector("#total-descent").textContent = Math.round(totalDescent).toLocaleString();
  elements.emptyState.classList.toggle("hidden", state.tracks.length > 0);

  if (state.tracks.length === 0) {
    elements.trackList.innerHTML = '<div class="library-empty">Your imported hikes will appear here.</div>';
  } else {
    elements.trackList.innerHTML = state.tracks.map((track, index) => `
      <button class="track-item ${track.id === state.selectedId ? "active" : ""}" data-track-id="${track.id}" type="button">
        <span class="track-swatch" style="background:${palette[index % palette.length]}"></span>
        <span class="track-copy"><strong>${escapeHTML(track.name)}</strong><span>${formatDate(track.startedAt)}</span></span>
        <span class="track-distance">${formatDistance(track.distanceKm)}</span>
      </button>`).join("");
  }
  syncMap();
}

function selectTrack(id, focus = false) {
  const track = state.tracks.find((candidate) => candidate.id === id);
  if (!track) return;
  state.selectedId = id;
  state.selectedPhotoId = null;
  elements.photoPanel.classList.remove("open");
  document.querySelector("#detail-name").textContent = track.name;
  document.querySelector("#detail-date").textContent = formatDate(track.startedAt).toUpperCase();
  document.querySelector("#detail-distance").textContent = formatDistance(track.distanceKm);
  document.querySelector("#detail-ascent").textContent = `${Math.round(track.elevationGain).toLocaleString()} m`;
  document.querySelector("#detail-descent").textContent = `${Math.round(track.elevationDescent).toLocaleString()} m`;
  document.querySelector("#detail-highest").textContent = formatElevation(track.highestPoint);
  document.querySelector("#detail-lowest").textContent = formatElevation(track.lowestPoint);
  document.querySelector("#detail-duration").textContent = formatDuration(track.duration);
  document.querySelector("#detail-start").textContent = track.start?.name || "Start";
  document.querySelector("#detail-end").textContent = track.end?.name || "Finish";
  renderElevationProfile(track);
  elements.detailPanel.classList.add("open");
  render();
  if (focus) fitTracks([track]);
}

function selectPhoto(id) {
  const photo = state.photos.find((candidate) => candidate.id === id);
  if (!photo) return;
  state.selectedPhotoId = id;
  state.selectedId = null;
  elements.detailPanel.classList.remove("open");
  document.querySelector("#photo-preview").src = photo.url;
  document.querySelector("#photo-preview").alt = photo.name;
  document.querySelector("#photo-name").textContent = photo.name;
  document.querySelector("#photo-date").textContent = formatPhotoDate(photo.capturedAt);
  elements.photoPanel.classList.add("open");
  syncMap();
  map.easeTo({ center: [photo.longitude, photo.latitude], zoom: Math.max(map.getZoom(), 13), duration: 700 });
}

function renderElevationProfile(track) {
  const container = document.querySelector("#elevation-chart");
  const range = document.querySelector("#profile-range");
  const profile = track.elevationProfile || [];
  if (profile.length < 2 || track.highestPoint == null || track.lowestPoint == null) {
    range.textContent = "";
    container.innerHTML = '<div class="elevation-empty">No elevation data in this GPX file</div>';
    return;
  }

  const width = 472;
  const height = 96;
  const elevationSpan = Math.max(track.highestPoint - track.lowestPoint, 1);
  const distanceSpan = Math.max(track.distanceKm, 0.001);
  const points = profile.map((point) => {
    const x = (point.distanceKm / distanceSpan) * width;
    const y = height - ((point.elevation - track.lowestPoint) / elevationSpan) * (height - 8) - 4;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  }).join(" ");
  const area = `0,${height} ${points} ${width},${height}`;
  range.textContent = `${formatElevation(track.lowestPoint)} — ${formatElevation(track.highestPoint)}`;
  container.innerHTML = `<svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" role="img" aria-label="Elevation profile from ${Math.round(track.lowestPoint)} to ${Math.round(track.highestPoint)} meters">
    <line class="profile-guide" x1="0" y1="4" x2="${width}" y2="4"></line>
    <line class="profile-guide" x1="0" y1="${height - 1}" x2="${width}" y2="${height - 1}"></line>
    <polygon class="profile-area" points="${area}"></polygon>
    <polyline class="profile-line" points="${points}"></polyline>
  </svg>`;
}

function fitTracks(tracks = state.tracks) {
  const points = tracks.flatMap((track) => track.coordinates.flat());
  if (tracks === state.tracks) {
    points.push(...state.photos.map((photo) => [photo.longitude, photo.latitude]));
  }
  if (!points.length) return;
  const bounds = points.reduce((box, coordinate) => box.extend(coordinate), new maplibregl.LngLatBounds(points[0], points[0]));
  map.fitBounds(bounds, { padding: { top: 100, right: 80, bottom: 100, left: 80 }, maxZoom: 14, duration: 1100 });
}

async function loadTracks() {
  try {
    const response = await fetch("/api/tracks");
    if (!response.ok) throw new Error("Could not load hikes");
    state.tracks = await response.json();
    render();
    if (state.tracks.length) fitTracks();
  } catch (error) {
    showToast(error.message);
  }
}

async function loadPhotos() {
  try {
    const response = await fetch("/api/photos");
    if (!response.ok) throw new Error("Could not load photos");
    state.photos = await response.json();
    syncMap();
  } catch (error) {
    showToast(error.message);
  }
}

async function importFiles(fileList) {
  const files = [...fileList].filter((file) => file.name.toLowerCase().endsWith(".gpx"));
  if (!files.length) {
    showToast("Choose one or more GPX files");
    return;
  }
  const body = new FormData();
  files.forEach((file) => body.append("files", file));
  showToast(`Importing ${files.length} ${files.length === 1 ? "hike" : "hikes"}…`);
  try {
    const response = await fetch("/api/tracks", { method: "POST", body });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || "Import failed");
    const importedIds = new Set(result.map((track) => track.id));
    state.tracks = [...result, ...state.tracks.filter((track) => !importedIds.has(track.id))];
    render();
    fitTracks(result);
    showToast(`${result.length} ${result.length === 1 ? "hike" : "hikes"} added`);
  } catch (error) {
    showToast(error.message);
  } finally {
    elements.fileInput.value = "";
  }
}

async function importPhotos(fileList) {
  const files = [...fileList].filter((file) => /\.jpe?g$/i.test(file.name) || file.type === "image/jpeg");
  if (!files.length) {
    showToast("Choose one or more JPEG photos");
    return;
  }
  const body = new FormData();
  files.forEach((file) => body.append("files", file));
  showToast(`Checking GPS data in ${files.length} ${files.length === 1 ? "photo" : "photos"}…`);
  try {
    const response = await fetch("/api/photos", { method: "POST", body });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || "Photo import failed");
    const importedIds = new Set(result.imported.map((photo) => photo.id));
    state.photos = [...result.imported, ...state.photos.filter((photo) => !importedIds.has(photo.id))];
    syncMap();
    if (result.imported.length) fitTracks();
    if (result.rejected.length) {
	  const firstReason = result.rejected[0].reason || "missing GPS information";
      showToast(`${result.imported.length} added · ${result.rejected.length} skipped: ${firstReason}`);
    } else {
      showToast(`${result.imported.length} ${result.imported.length === 1 ? "photo" : "photos"} added`);
    }
  } catch (error) {
    showToast(error.message);
  } finally {
    elements.photoInput.value = "";
  }
}

async function deleteSelectedPhoto() {
  const photo = state.photos.find((candidate) => candidate.id === state.selectedPhotoId);
  if (!photo || !window.confirm(`Delete “${photo.name}”?`)) return;
  const response = await fetch(`/api/photos/${photo.id}`, { method: "DELETE" });
  if (!response.ok) {
    showToast("Could not delete this photo");
    return;
  }
  state.photos = state.photos.filter((candidate) => candidate.id !== photo.id);
  state.selectedPhotoId = null;
  elements.photoPanel.classList.remove("open");
  syncMap();
  showToast("Photo deleted");
}

async function deleteSelectedTrack() {
  const track = state.tracks.find((candidate) => candidate.id === state.selectedId);
  if (!track || !window.confirm(`Delete “${track.name}”?`)) return;
  const response = await fetch(`/api/tracks/${track.id}`, { method: "DELETE" });
  if (!response.ok) {
    showToast("Could not delete this hike");
    return;
  }
  state.tracks = state.tracks.filter((candidate) => candidate.id !== track.id);
  state.selectedId = null;
  elements.detailPanel.classList.remove("open");
  render();
  if (state.tracks.length) fitTracks();
  showToast("Hike deleted");
}

function formatDistance(distance, unit = true) {
  const value = distance >= 100 ? Math.round(distance).toLocaleString() : distance.toFixed(1);
  return unit ? `${value} km` : value;
}

function formatDuration(seconds) {
  if (!seconds) return "—";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.round((seconds % 3600) / 60);
  return hours ? `${hours}h ${minutes}m` : `${minutes}m`;
}

function formatElevation(elevation) {
  return elevation == null ? "—" : `${Math.round(elevation).toLocaleString()} m`;
}

function formatDate(date) {
  if (!date) return "Date unknown";
  return new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", year: "numeric" }).format(new Date(date));
}

function formatPhotoDate(date) {
  if (!date) return "Date unknown";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(date));
}

function escapeHTML(value) {
  const node = document.createElement("span");
  node.textContent = value;
  return node.innerHTML;
}

let toastTimer;
function showToast(message) {
  elements.toast.textContent = message;
  elements.toast.classList.add("visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => elements.toast.classList.remove("visible"), 2600);
}

document.querySelectorAll("#import-button, #import-map-button, #empty-import-button").forEach((button) => {
  button.addEventListener("click", () => elements.fileInput.click());
});
elements.fileInput.addEventListener("change", () => importFiles(elements.fileInput.files));
document.querySelector("#photo-import-button").addEventListener("click", () => elements.photoInput.click());
elements.photoInput.addEventListener("change", () => importPhotos(elements.photoInput.files));
elements.trackList.addEventListener("click", (event) => {
  const item = event.target.closest("[data-track-id]");
  if (item) selectTrack(item.dataset.trackId, true);
});
document.querySelector("#fit-button").addEventListener("click", () => fitTracks());
document.querySelector("#focus-track").addEventListener("click", () => selectTrack(state.selectedId, true));
document.querySelector("#delete-track").addEventListener("click", deleteSelectedTrack);
document.querySelector("#close-detail").addEventListener("click", () => {
  state.selectedId = null;
  elements.detailPanel.classList.remove("open");
  render();
});
document.querySelector("#close-photo").addEventListener("click", () => {
  state.selectedPhotoId = null;
  elements.photoPanel.classList.remove("open");
  syncMap();
});
document.querySelector("#delete-photo").addEventListener("click", deleteSelectedPhoto);

let dragDepth = 0;
window.addEventListener("dragenter", (event) => { event.preventDefault(); dragDepth += 1; elements.dropOverlay.classList.add("visible"); });
window.addEventListener("dragover", (event) => event.preventDefault());
window.addEventListener("dragleave", () => { dragDepth -= 1; if (dragDepth === 0) elements.dropOverlay.classList.remove("visible"); });
window.addEventListener("drop", (event) => {
  event.preventDefault();
  dragDepth = 0;
  elements.dropOverlay.classList.remove("visible");
  const files = [...event.dataTransfer.files];
  const gpxFiles = files.filter((file) => file.name.toLowerCase().endsWith(".gpx"));
  const photoFiles = files.filter((file) => /\.jpe?g$/i.test(file.name) || file.type === "image/jpeg");
  if (gpxFiles.length) importFiles(gpxFiles);
  if (photoFiles.length) importPhotos(photoFiles);
  if (!gpxFiles.length && !photoFiles.length) showToast("Choose GPX or JPEG files");
});

loadTracks();
loadPhotos();