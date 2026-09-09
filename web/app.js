const state = { tracks: [], photos: [], selectedId: null, selectedPhotoId: null, galleryTrackId: null, gallerySelectionMode: false, selectedGalleryPhotoIds: new Set(), category: "", dateFrom: "", dateTo: "", sortBy: "startedAt", sortDirection: "desc", mapReady: false, endpointMarkers: [], activityMarkers: [], photoMarkers: [] };
const palette = ["#d7ff43", "#ff8a5b", "#55d8ff", "#f0bbff", "#72e6a1", "#ffd166"];
const elements = {
  fileInput: document.querySelector("#file-input"),
  photoInput: document.querySelector("#photo-input"),
	trackPhotoInput: document.querySelector("#track-photo-input"),
  trackList: document.querySelector("#track-list"),
  emptyState: document.querySelector("#empty-state"),
  detailPanel: document.querySelector("#detail-panel"),
  galleryPanel: document.querySelector("#gallery-panel"),
  galleryGrid: document.querySelector("#gallery-grid"),
  photoPanel: document.querySelector("#photo-panel"),
	lightboxPanel: document.querySelector("#lightbox-panel"),
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
  map.addSource("activities", { type: "geojson", data: featureCollection() });
  map.addLayer({
    id: "activity-casing",
    type: "line",
    source: "activities",
    paint: { "line-color": "#070809", "line-width": ["interpolate", ["linear"], ["zoom"], 3, 3, 12, 9], "line-opacity": 0.75 },
  });
  map.addLayer({
    id: "activities",
    type: "line",
    source: "activities",
    paint: {
      "line-color": ["get", "color"],
      "line-width": ["interpolate", ["linear"], ["zoom"], 3, 1.5, 12, 4.5],
      "line-opacity": ["case", ["==", ["get", "id"], ""], 1, 0.9],
    },
  });
  state.mapReady = true;
  syncMap();
});

map.on("click", "activities", (event) => {
  const id = event.features?.[0]?.properties?.id;
  if (id) selectTrack(id, true);
});
map.on("mouseenter", "activities", () => { map.getCanvas().style.cursor = "pointer"; });
map.on("mouseleave", "activities", () => { map.getCanvas().style.cursor = ""; });

function featureCollection() {
  return {
    type: "FeatureCollection",
    features: filteredTracks().filter((track) => !isStationary(track)).map((track) => ({
      type: "Feature",
	  properties: { id: track.id, name: track.name, color: trackColor(track) },
      geometry: { type: "MultiLineString", coordinates: track.coordinates },
    })),
  };
}

function filteredTracks() {
  const from = state.dateFrom ? new Date(`${state.dateFrom}T00:00:00`) : null;
  const to = state.dateTo ? new Date(`${state.dateTo}T23:59:59.999`) : null;
  return state.tracks.filter((track) => {
    if (state.category && (track.activity || "Activity") !== state.category) return false;
    if (!from && !to) return true;
    if (!track.startedAt) return false;
    const startedAt = new Date(track.startedAt);
    return (!from || startedAt >= from) && (!to || startedAt <= to);
  });
}

function sortedTracks(tracks) {
  const direction = state.sortDirection === "asc" ? 1 : -1;
  return [...tracks].sort((first, second) => {
    const firstValue = sortValue(first, state.sortBy);
    const secondValue = sortValue(second, state.sortBy);
    const firstMissing = firstValue == null || firstValue === "" || Number.isNaN(firstValue);
    const secondMissing = secondValue == null || secondValue === "" || Number.isNaN(secondValue);
    if (firstMissing !== secondMissing) return firstMissing ? 1 : -1;
    if (firstMissing) return first.name.localeCompare(second.name);
    if (typeof firstValue === "string") return firstValue.localeCompare(secondValue) * direction;
    return (firstValue - secondValue) * direction || first.name.localeCompare(second.name);
  });
}

function sortValue(track, field) {
  if (field === "startedAt") return track.startedAt ? new Date(track.startedAt).getTime() : null;
  if (field === "name" || field === "activity") return (track[field] || "").toLocaleLowerCase();
  return track[field];
}

function trackColor(track) {
  const index = state.tracks.findIndex((candidate) => candidate.id === track.id);
  return palette[Math.max(index, 0) % palette.length];
}

function isStationary(track) {
  return track.coordinates.flat().length === 1;
}

function syncMap() {
  if (!state.mapReady) return;
  map.getSource("activities").setData(featureCollection());
  syncActivityMarkers();
  syncEndpointMarkers();
  syncPhotoMarkers();
}

function syncActivityMarkers() {
  state.activityMarkers.forEach((marker) => marker.remove());
  state.activityMarkers = [];
  filteredTracks().filter(isStationary).forEach((track) => {
    const element = document.createElement("button");
    element.className = `stationary-marker ${track.id === state.selectedId ? "active" : ""}`;
    element.type = "button";
    element.title = `${track.activity || "Activity"}: ${track.name}`;
    element.style.setProperty("--marker-color", trackColor(track));
    element.innerHTML = `<i data-lucide="${activityIcon(track.activityType)}"></i>`;
    element.addEventListener("click", (event) => {
      event.stopPropagation();
      selectTrack(track.id, true);
    });
    const marker = new maplibregl.Marker({ element, anchor: "bottom" }).setLngLat(track.coordinates[0][0]).addTo(map);
    state.activityMarkers.push(marker);
  });
  lucide.createIcons();
}

function syncPhotoMarkers() {
  state.photoMarkers.forEach((marker) => marker.remove());
  state.photoMarkers = [];
	state.photos.filter((photo) => photo.hasLocation).forEach((photo) => {
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
  if (!track?.start?.coordinates || !track?.end?.coordinates || isStationary(track)) return;
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
	const visibleTracks = filteredTracks();
  const orderedTracks = sortedTracks(visibleTracks);
  const totalDistance = visibleTracks.reduce((sum, track) => sum + track.distanceKm, 0);
  const totalAscent = visibleTracks.reduce((sum, track) => sum + track.elevationGain, 0);
  const totalDescent = visibleTracks.reduce((sum, track) => sum + track.elevationDescent, 0);
	const totalTime = visibleTracks.reduce((sum, track) => sum + track.duration, 0);
  document.querySelector("#activity-count").textContent = `${visibleTracks.length} ${visibleTracks.length === 1 ? "activity" : "activities"}`;
  document.querySelector("#total-distance").textContent = formatDistance(totalDistance, false);
  document.querySelector("#total-ascent").textContent = Math.round(totalAscent).toLocaleString();
  document.querySelector("#total-descent").textContent = Math.round(totalDescent).toLocaleString();
	document.querySelector("#total-time").textContent = formatDuration(totalTime);
  elements.emptyState.classList.toggle("hidden", state.tracks.length > 0);

	if (state.tracks.length === 0) {
    elements.trackList.innerHTML = '<div class="library-empty">Your imported activities will appear here.</div>';
	} else if (visibleTracks.length === 0) {
	  elements.trackList.innerHTML = '<div class="library-empty">No activities match these filters.</div>';
  } else {
    elements.trackList.innerHTML = orderedTracks.map((track) => `
    <div class="track-item ${track.id === state.selectedId ? "active" : ""}" data-track-id="${track.id}">
		<span class="track-swatch" style="background:${trackColor(track)}"></span>
		<span class="activity-icon" title="${escapeHTML(track.activity || "Activity")}"><i data-lucide="${activityIcon(track.activityType)}"></i></span>
    <span class="track-copy"><button class="track-name" type="button" aria-label="Rename ${escapeHTML(track.name)}" title="Rename activity">${escapeHTML(track.name)}</button><span>${escapeHTML(track.activity || "Activity")} · ${formatDate(track.startedAt)}</span></span>
        <span class="track-distance">${formatDistance(track.distanceKm)}</span>
    </div>`).join("");
  }
  lucide.createIcons();
  syncMap();
}

function renderCategoryOptions() {
  const select = document.querySelector("#category-filter");
  const categories = [...new Set(state.tracks.map((track) => track.activity || "Activity"))].sort();
  select.innerHTML = '<option value="">All categories</option>' + categories.map((category) => `<option value="${escapeHTML(category)}">${escapeHTML(category)}</option>`).join("");
  select.value = state.category;
}

function activityIcon(activityType = "") {
  const type = activityType.toLowerCase();
  if (["hiking", "hikingtourtrail", "walking", "mountaineering"].includes(type)) return "mountain";
  if (["running", "trail_running"].includes(type)) return "footprints";
  if (["lap_swimming", "open_water_swimming", "swimming"].includes(type)) return "waves";
  if (["cycling", "road_cycling", "gravel_cycling"].includes(type)) return "bike";
  if (["bouldering", "rock_climbing", "climbing"].includes(type)) return "dumbbell";
  return "activity";
}

function selectTrack(id, focus = false) {
  const track = state.tracks.find((candidate) => candidate.id === id);
  if (!track) return;
  state.selectedId = id;
  state.selectedPhotoId = null;
  state.galleryTrackId = null;
  elements.galleryPanel.classList.remove("open");
  elements.photoPanel.classList.remove("open");
	elements.lightboxPanel.classList.remove("open");
  document.querySelector("#detail-name").textContent = track.name;
	document.querySelector("#detail-activity").textContent = (track.activity || "Activity").toUpperCase();
  document.querySelector("#detail-date").textContent = formatDate(track.startedAt).toUpperCase();
  document.querySelector("#detail-distance").textContent = formatDistance(track.distanceKm);
  document.querySelector("#detail-ascent").textContent = `${Math.round(track.elevationGain).toLocaleString()} m`;
  document.querySelector("#detail-descent").textContent = `${Math.round(track.elevationDescent).toLocaleString()} m`;
  document.querySelector("#detail-highest").textContent = formatElevation(track.highestPoint);
  document.querySelector("#detail-lowest").textContent = formatElevation(track.lowestPoint);
  document.querySelector("#detail-duration").textContent = formatDuration(track.duration);
  document.querySelector("#detail-start").textContent = track.start?.name || "Start";
  document.querySelector("#detail-end").textContent = track.end?.name || "Finish";
  refreshTrackPhotoCount();
  renderElevationProfile(track);
  elements.detailPanel.classList.add("open");
  render();
  if (focus) fitTracks([track]);
}

function selectPhoto(id, preserveTrack = false) {
  const photo = state.photos.find((candidate) => candidate.id === id);
  if (!photo) return;
  state.selectedPhotoId = id;
  if (!preserveTrack) {
    state.selectedId = null;
    state.galleryTrackId = null;
    elements.galleryPanel.classList.remove("open");
    elements.detailPanel.classList.remove("open");
  }
  if (preserveTrack) {
    document.querySelector("#lightbox-preview").src = photo.url;
    document.querySelector("#lightbox-preview").alt = photo.name;
    document.querySelector("#lightbox-name").textContent = photo.name;
    document.querySelector("#lightbox-date").textContent = formatPhotoDate(photo.capturedAt);
    const photos = photoNavigationItems();
    const photoIndex = photos.findIndex((candidate) => candidate.id === photo.id);
    document.querySelector("#photo-position").textContent = photos.length > 1 ? `${photoIndex + 1} OF ${photos.length}` : "ACTIVITY PHOTO";
    document.querySelector("#previous-photo").hidden = photos.length < 2;
    document.querySelector("#next-photo").hidden = photos.length < 2;
    elements.lightboxPanel.classList.add("open");
  } else {
    document.querySelector("#photo-preview").src = photo.url;
    document.querySelector("#photo-preview").alt = photo.name;
    document.querySelector("#photo-name").textContent = photo.name;
    document.querySelector("#photo-date").textContent = formatPhotoDate(photo.capturedAt);
    elements.photoPanel.classList.add("open");
  }
  syncMap();
  map.easeTo({ center: [photo.longitude, photo.latitude], zoom: Math.max(map.getZoom(), 13), duration: 700 });
}

function photoNavigationItems() {
  return state.galleryTrackId ? photosForTrack(state.galleryTrackId) : state.photos;
}

function navigatePhoto(direction) {
  const photos = photoNavigationItems();
  if (photos.length < 2) return;
  const currentIndex = photos.findIndex((photo) => photo.id === state.selectedPhotoId);
  const nextIndex = (currentIndex + direction + photos.length) % photos.length;
  selectPhoto(photos[nextIndex].id, Boolean(state.galleryTrackId));
}

function closePhoto() {
  state.selectedPhotoId = null;
  elements.photoPanel.classList.remove("open");
  elements.lightboxPanel.classList.remove("open");
  syncMap();
}

function photosForTrack(trackId) {
  return state.photos.filter((photo) => photo.trackId === trackId);
}

function refreshTrackPhotoCount() {
  const photoCount = photosForTrack(state.selectedId).length;
  document.querySelector("#track-photo-count").textContent = `${photoCount} ${photoCount === 1 ? "photo" : "photos"}`;
}

function openTrackGallery() {
  const track = state.tracks.find((candidate) => candidate.id === state.selectedId);
  if (!track) return;
  state.galleryTrackId = track.id;
  state.gallerySelectionMode = false;
  state.selectedGalleryPhotoIds.clear();
  document.querySelector("#gallery-title").textContent = track.name;
  renderTrackGallery();
  elements.galleryPanel.classList.add("open");
}

function renderTrackGallery() {
  const photos = photosForTrack(state.galleryTrackId);
	elements.galleryGrid.innerHTML = photos.length ? photos.map((photo) => `
  <button class="gallery-item ${state.selectedGalleryPhotoIds.has(photo.id) ? "selected" : ""}" type="button" data-photo-id="${photo.id}" aria-label="${state.gallerySelectionMode ? "Select" : "View"} ${escapeHTML(photo.name)}" aria-pressed="${state.selectedGalleryPhotoIds.has(photo.id)}">
      <img src="${photo.url}" alt="${escapeHTML(photo.name)}" loading="lazy">
    <span class="gallery-check"><i data-lucide="check"></i></span>
    <span class="gallery-caption">${formatPhotoDate(photo.capturedAt)}</span>
	</button>`).join("") : '<div class="gallery-empty"><i data-lucide="image-plus"></i><span>No photos yet</span></div>';
  const selectButton = document.querySelector("#select-gallery-photos");
  elements.galleryPanel.classList.toggle("selection-mode", state.gallerySelectionMode);
  selectButton.classList.toggle("active", state.gallerySelectionMode);
  selectButton.setAttribute("aria-pressed", state.gallerySelectionMode);
  selectButton.title = state.gallerySelectionMode ? "Cancel selection" : "Select photos";
  const deleteButton = document.querySelector("#delete-gallery-photos");
  deleteButton.hidden = !state.gallerySelectionMode;
  deleteButton.disabled = state.selectedGalleryPhotoIds.size === 0;
  document.querySelector("#gallery-selection-count").textContent = state.selectedGalleryPhotoIds.size;
  deleteButton.setAttribute("aria-label", `Delete ${state.selectedGalleryPhotoIds.size} selected photos`);
  lucide.createIcons();
}

function toggleGallerySelectionMode() {
  state.gallerySelectionMode = !state.gallerySelectionMode;
  state.selectedGalleryPhotoIds.clear();
  renderTrackGallery();
}

async function deleteSelectedGalleryPhotos() {
  const ids = [...state.selectedGalleryPhotoIds];
  if (!ids.length || !window.confirm(`Delete ${ids.length} selected ${ids.length === 1 ? "photo" : "photos"}?`)) return;
  const results = await Promise.all(ids.map(async (id) => ({ id, ok: (await fetch(`/api/photos/${id}`, { method: "DELETE" })).ok })));
  const deletedIds = new Set(results.filter((result) => result.ok).map((result) => result.id));
  state.photos = state.photos.filter((photo) => !deletedIds.has(photo.id));
  state.selectedGalleryPhotoIds.clear();
  refreshTrackPhotoCount();
  renderTrackGallery();
  syncMap();
  const failed = results.length - deletedIds.size;
  showToast(failed ? `${deletedIds.size} deleted · ${failed} could not be deleted` : `${deletedIds.size} ${deletedIds.size === 1 ? "photo" : "photos"} deleted`);
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
    <g class="profile-hover" hidden>
      <line class="profile-hover-line" y1="4" y2="${height - 1}"></line>
      <circle class="profile-hover-point" r="4"></circle>
    </g>
  </svg><div class="profile-tooltip" role="tooltip" hidden></div>`;

  const svg = container.querySelector("svg");
  const hover = container.querySelector(".profile-hover");
  const hoverLine = container.querySelector(".profile-hover-line");
  const hoverPoint = container.querySelector(".profile-hover-point");
  const tooltip = container.querySelector(".profile-tooltip");
  svg.addEventListener("pointermove", (event) => {
    const bounds = svg.getBoundingClientRect();
    const pointerX = Math.max(0, Math.min(bounds.width, event.clientX - bounds.left));
    const distanceKm = (pointerX / bounds.width) * distanceSpan;
    const nearest = profile.reduce((closest, point) =>
      Math.abs(point.distanceKm - distanceKm) < Math.abs(closest.distanceKm - distanceKm) ? point : closest
    );
    const x = (nearest.distanceKm / distanceSpan) * width;
    const y = height - ((nearest.elevation - track.lowestPoint) / elevationSpan) * (height - 8) - 4;
    hoverLine.setAttribute("x1", x);
    hoverLine.setAttribute("x2", x);
    hoverPoint.setAttribute("cx", x);
    hoverPoint.setAttribute("cy", y);
    tooltip.textContent = formatElevation(nearest.elevation);
    tooltip.style.left = `${(x / width) * 100}%`;
    tooltip.style.top = `${(y / height) * 100}%`;
    tooltip.classList.toggle("align-start", x < width * 0.08);
    tooltip.classList.toggle("align-end", x > width * 0.92);
    hover.hidden = false;
    tooltip.hidden = false;
  });
  svg.addEventListener("pointerleave", () => {
    hover.hidden = true;
    tooltip.hidden = true;
  });
}

function fitTracks(tracks = state.tracks) {
  const points = tracks.flatMap((track) => track.coordinates.flat());
  if (tracks === state.tracks) {
	points.push(...state.photos.filter((photo) => photo.hasLocation).map((photo) => [photo.longitude, photo.latitude]));
  }
  if (!points.length) return;
  const bounds = points.reduce((box, coordinate) => box.extend(coordinate), new maplibregl.LngLatBounds(points[0], points[0]));
  map.fitBounds(bounds, { padding: { top: 100, right: 80, bottom: 100, left: 80 }, maxZoom: 14, duration: 1100 });
}

async function loadTracks() {
  try {
    const response = await fetch("/api/tracks");
    if (!response.ok) throw new Error("Could not load activities");
    state.tracks = await response.json();
  	renderCategoryOptions();
    render();
  	if (state.tracks.length) fitTracks(filteredTracks());
  } catch (error) {
    showToast(error.message);
  }
}

async function loadPhotos() {
  try {
    const response = await fetch("/api/photos");
    if (!response.ok) throw new Error("Could not load photos");
    state.photos = await response.json();
    refreshTrackPhotoCount();
    syncMap();
  } catch (error) {
    showToast(error.message);
  }
}

async function importFiles(fileList) {
  const files = [...fileList].filter((file) => /\.(gpx|fit)$/i.test(file.name));
  if (!files.length) {
    showToast("Choose one or more GPX or FIT files");
    return;
  }
  const body = new FormData();
  files.forEach((file) => body.append("files", file));
  showToast(`Importing ${files.length} ${files.length === 1 ? "activity" : "activities"}…`);
  try {
    const response = await fetch("/api/tracks", { method: "POST", body });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || "Import failed");
  const importedIds = new Set(result.imported.map((track) => track.id));
    state.tracks = [...result.imported, ...state.tracks.filter((track) => !importedIds.has(track.id))];
  await loadPhotos();
  	renderCategoryOptions();
    render();
  if (result.imported.length) fitTracks(result.imported);
  if (result.rejected.length) {
    const rejected = result.rejected[0];
    showToast(`${result.imported.length} added · ${result.rejected.length} skipped: ${rejected.name} — ${rejected.reason}`, 7000);
  } else {
    showToast(`${result.imported.length} ${result.imported.length === 1 ? "activity" : "activities"} added`);
  }
  } catch (error) {
    showToast(error.message);
  } finally {
    elements.fileInput.value = "";
  }
}

async function importPhotos(fileList, trackId = "") {
  const files = [...fileList].filter((file) => /\.jpe?g$/i.test(file.name) || file.type === "image/jpeg");
  if (!files.length) {
    showToast("Choose one or more JPEG photos");
    return;
  }
  const body = new FormData();
  files.forEach((file) => body.append("files", file));
  showToast(trackId ? `Adding ${files.length} ${files.length === 1 ? "photo" : "photos"} to activity…` : `Checking GPS data in ${files.length} ${files.length === 1 ? "photo" : "photos"}…`);
  try {
	const endpoint = trackId ? `/api/tracks/${trackId}/photos` : "/api/photos";
	const response = await fetch(endpoint, { method: "POST", body });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || "Photo import failed");
    const importedIds = new Set(result.imported.map((photo) => photo.id));
    state.photos = [...result.imported, ...state.photos.filter((photo) => !importedIds.has(photo.id))];
    refreshTrackPhotoCount();
	if (trackId) openTrackGallery();
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
	elements.trackPhotoInput.value = "";
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
	elements.lightboxPanel.classList.remove("open");
	refreshTrackPhotoCount();
  if (state.galleryTrackId) {
    if (photosForTrack(state.galleryTrackId).length) {
      openTrackGallery();
    } else {
      state.galleryTrackId = null;
      elements.galleryPanel.classList.remove("open");
    }
  }
  syncMap();
  showToast("Photo deleted");
}

async function deleteSelectedTrack() {
  const track = state.tracks.find((candidate) => candidate.id === state.selectedId);
  if (!track || !window.confirm(`Delete “${track.name}”?`)) return;
  const response = await fetch(`/api/tracks/${track.id}`, { method: "DELETE" });
  if (!response.ok) {
    showToast("Could not delete this activity");
    return;
  }
  state.tracks = state.tracks.filter((candidate) => candidate.id !== track.id);
  state.selectedId = null;
  elements.detailPanel.classList.remove("open");
  render();
  if (state.tracks.length) fitTracks();
  showToast("Activity deleted");
}

function beginTrackRename(button) {
  const item = button.closest("[data-track-id]");
  const track = state.tracks.find((candidate) => candidate.id === item?.dataset.trackId);
  if (!track) return;
  const input = document.createElement("input");
  input.className = "track-name-input";
  input.value = track.name;
  input.maxLength = 200;
  input.setAttribute("aria-label", "Activity name");
  button.replaceWith(input);
  input.focus();
  input.select();
  input.addEventListener("click", (event) => event.stopPropagation());
  input.addEventListener("blur", () => render(), { once: true });
  input.addEventListener("keydown", async (event) => {
    event.stopPropagation();
    if (event.key === "Escape") {
      input.blur();
      return;
    }
    if (event.key !== "Enter") return;
    event.preventDefault();
    const name = input.value.trim();
    if (!name) {
      showToast("Activity name cannot be empty");
      return;
    }
    input.disabled = true;
    try {
      const response = await fetch(`/api/tracks/${track.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || "Could not rename this activity");
      track.name = result.name;
      if (state.selectedId === track.id) document.querySelector("#detail-name").textContent = result.name;
      render();
      showToast("Activity renamed");
    } catch (error) {
      input.disabled = false;
      input.focus();
      showToast(error.message);
    }
  });
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
function showToast(message, duration = 2600) {
  elements.toast.textContent = message;
  elements.toast.classList.add("visible");
  clearTimeout(toastTimer);
	toastTimer = setTimeout(() => elements.toast.classList.remove("visible"), duration);
}

document.querySelectorAll("#import-button, #import-map-button, #empty-import-button").forEach((button) => {
  button.addEventListener("click", () => elements.fileInput.click());
});
elements.fileInput.addEventListener("change", () => importFiles(elements.fileInput.files));
document.querySelector("#photo-import-button").addEventListener("click", () => elements.photoInput.click());
elements.photoInput.addEventListener("change", () => importPhotos(elements.photoInput.files));
elements.trackPhotoInput.addEventListener("change", () => importPhotos(elements.trackPhotoInput.files, state.galleryTrackId));
elements.trackList.addEventListener("click", (event) => {
  const name = event.target.closest(".track-name");
  if (name) {
    event.stopPropagation();
    beginTrackRename(name);
    return;
  }
  const item = event.target.closest("[data-track-id]");
  if (item) selectTrack(item.dataset.trackId, true);
});
document.querySelector("#fit-button").addEventListener("click", () => fitTracks());
document.querySelector("#focus-track").addEventListener("click", () => selectTrack(state.selectedId, true));
document.querySelector("#track-photos").addEventListener("click", openTrackGallery);
document.querySelector("#add-track-photos").addEventListener("click", () => elements.trackPhotoInput.click());
document.querySelector("#select-gallery-photos").addEventListener("click", toggleGallerySelectionMode);
document.querySelector("#delete-gallery-photos").addEventListener("click", deleteSelectedGalleryPhotos);
document.querySelector("#close-gallery").addEventListener("click", () => {
  state.galleryTrackId = null;
  state.gallerySelectionMode = false;
  state.selectedGalleryPhotoIds.clear();
  elements.galleryPanel.classList.remove("open");
});
elements.galleryGrid.addEventListener("click", (event) => {
  const item = event.target.closest("[data-photo-id]");
  if (!item) return;
  if (state.gallerySelectionMode) {
    if (state.selectedGalleryPhotoIds.has(item.dataset.photoId)) {
      state.selectedGalleryPhotoIds.delete(item.dataset.photoId);
    } else {
      state.selectedGalleryPhotoIds.add(item.dataset.photoId);
    }
    renderTrackGallery();
    return;
  }
  selectPhoto(item.dataset.photoId, true);
});
document.querySelector("#delete-track").addEventListener("click", deleteSelectedTrack);
document.querySelector("#close-detail").addEventListener("click", () => {
  state.selectedId = null;
  elements.detailPanel.classList.remove("open");
  render();
});
document.querySelector("#close-photo").addEventListener("click", closePhoto);
document.querySelector("#close-lightbox").addEventListener("click", closePhoto);
document.querySelector("#previous-photo").addEventListener("click", () => navigatePhoto(-1));
document.querySelector("#next-photo").addEventListener("click", () => navigatePhoto(1));
elements.lightboxPanel.addEventListener("click", (event) => {
  if (event.target === elements.lightboxPanel) closePhoto();
});
document.addEventListener("keydown", (event) => {
  if (!elements.lightboxPanel.classList.contains("open")) return;
  if (event.key === "Escape") closePhoto();
  if (event.key === "ArrowLeft") navigatePhoto(-1);
  if (event.key === "ArrowRight") navigatePhoto(1);
});
document.querySelectorAll(".delete-photo").forEach((button) => button.addEventListener("click", deleteSelectedPhoto));
document.querySelector("#category-filter").addEventListener("change", (event) => {
  state.category = event.target.value;
  applyFilters();
});
document.querySelector("#date-from").addEventListener("change", (event) => {
  state.dateFrom = event.target.value;
  applyFilters();
});
document.querySelector("#date-to").addEventListener("change", (event) => {
  state.dateTo = event.target.value;
  applyFilters();
});
document.querySelector("#sort-by").addEventListener("change", (event) => {
  state.sortBy = event.target.value;
  render();
});
document.querySelector("#sort-direction").addEventListener("click", (event) => {
  state.sortDirection = state.sortDirection === "asc" ? "desc" : "asc";
  const ascending = state.sortDirection === "asc";
  event.currentTarget.setAttribute("aria-label", ascending ? "Sort ascending" : "Sort descending");
  event.currentTarget.title = ascending ? "Sort ascending" : "Sort descending";
  event.currentTarget.innerHTML = `<i data-lucide="arrow-${ascending ? "up" : "down"}"></i>`;
  render();
});
document.querySelector("#clear-filters").addEventListener("click", () => {
  state.category = "";
  state.dateFrom = "";
  state.dateTo = "";
  document.querySelector("#category-filter").value = "";
  document.querySelector("#date-from").value = "";
  document.querySelector("#date-to").value = "";
  applyFilters();
});

function applyFilters() {
  if (state.selectedId && !filteredTracks().some((track) => track.id === state.selectedId)) {
    state.selectedId = null;
    elements.detailPanel.classList.remove("open");
  }
  render();
  if (filteredTracks().length) fitTracks(filteredTracks());
}

let dragDepth = 0;
window.addEventListener("dragenter", (event) => { event.preventDefault(); dragDepth += 1; elements.dropOverlay.classList.add("visible"); });
window.addEventListener("dragover", (event) => event.preventDefault());
window.addEventListener("dragleave", () => { dragDepth -= 1; if (dragDepth === 0) elements.dropOverlay.classList.remove("visible"); });
window.addEventListener("drop", (event) => {
  event.preventDefault();
  dragDepth = 0;
  elements.dropOverlay.classList.remove("visible");
  const files = [...event.dataTransfer.files];
  const trackFiles = files.filter((file) => /\.(gpx|fit)$/i.test(file.name));
  const photoFiles = files.filter((file) => /\.jpe?g$/i.test(file.name) || file.type === "image/jpeg");
  if (trackFiles.length) importFiles(trackFiles);
  if (photoFiles.length) importPhotos(photoFiles);
  if (!trackFiles.length && !photoFiles.length) showToast("Choose GPX, FIT, or JPEG files");
});

loadTracks();
loadPhotos();