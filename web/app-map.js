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
  map.addSource("terrain", {
    type: "raster-dem",
    tiles: ["https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png"],
    tileSize: 256,
    maxzoom: 15,
    encoding: "terrarium",
    attribution: "Elevation tiles © Mapzen"
  });
  map.addSource("terrain-hillshade", {
    type: "raster-dem",
    tiles: ["https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png"],
    tileSize: 256,
    maxzoom: 15,
    encoding: "terrarium"
  });
  map.addLayer({
    id: "terrain-hillshade",
    type: "hillshade",
    source: "terrain-hillshade",
    layout: { visibility: "none" },
    paint: {
      "hillshade-shadow-color": "#101513",
      "hillshade-highlight-color": "#d7ff43",
      "hillshade-accent-color": "#56634b",
      "hillshade-exaggeration": 0.25
    }
  });
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
  document.querySelector("#terrain-toggle").disabled = false;
  syncMap();
});

map.on("click", "activities", (event) => {
  const id = event.features?.[0]?.properties?.id;
  if (id) selectTrack(id, true);
});
map.on("mouseenter", "activities", () => { map.getCanvas().style.cursor = "pointer"; });
map.on("mouseleave", "activities", () => { map.getCanvas().style.cursor = ""; });

function toggleTerrain() {
  if (!state.mapReady) return;
  state.terrain3D = !state.terrain3D;
  map.setTerrain(state.terrain3D ? { source: "terrain", exaggeration: 1.25 } : null);
  map.setLayoutProperty("terrain-hillshade", "visibility", state.terrain3D ? "visible" : "none");
  map.easeTo({ pitch: state.terrain3D ? 60 : 0, bearing: state.terrain3D ? -20 : 0, duration: 900 });

  const button = document.querySelector("#terrain-toggle");
  button.classList.toggle("active", state.terrain3D);
  button.setAttribute("aria-pressed", state.terrain3D);
  button.setAttribute("aria-label", state.terrain3D ? "Disable 3D terrain" : "Enable 3D terrain");
  button.title = state.terrain3D ? "Return to 2D map" : "Enable 3D terrain";
  button.querySelector("span").textContent = state.terrain3D ? "2D" : "3D";
}

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

function fitTracks(tracks = state.tracks) {
  const points = tracks.flatMap((track) => track.coordinates.flat());
  if (tracks === state.tracks) {
    points.push(...state.photos.filter((photo) => photo.hasLocation).map((photo) => [photo.longitude, photo.latitude]));
  }
  if (!points.length) return;
  const bounds = points.reduce((box, coordinate) => box.extend(coordinate), new maplibregl.LngLatBounds(points[0], points[0]));
  map.fitBounds(bounds, { padding: { top: 100, right: 80, bottom: 100, left: 80 }, maxZoom: 14, duration: 1100 });
}
