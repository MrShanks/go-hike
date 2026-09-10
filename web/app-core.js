const state = { tracks: [], photos: [], selectedId: null, selectedPhotoId: null, galleryTrackId: null, trackSelectionMode: false, selectedTrackIds: new Set(), gallerySelectionMode: false, selectedGalleryPhotoIds: new Set(), category: "", dateFrom: "", dateTo: "", sortBy: "startedAt", sortDirection: "desc", mapReady: false, terrain3D: false, endpointMarkers: [], activityMarkers: [], photoMarkers: [] };
const palette = ["#d7ff43", "#ff8a5b", "#55d8ff", "#f0bbff", "#72e6a1", "#ffd166"];
const elements = {
  appShell: document.querySelector(".app-shell"),
  sidebarResizer: document.querySelector("#sidebar-resizer"),
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

function activityIcon(activityType = "") {
  const type = activityType.toLowerCase();
  if (["hiking", "hikingtourtrail", "walking", "mountaineering"].includes(type)) return "mountain";
  if (["running", "trail_running"].includes(type)) return "footprints";
  if (["lap_swimming", "open_water_swimming", "swimming"].includes(type)) return "waves";
  if (["cycling", "road_cycling", "gravel_cycling"].includes(type)) return "bike";
  if (["bouldering", "rock_climbing", "climbing"].includes(type)) return "dumbbell";
  return "activity";
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

const sidebarWidthStorageKey = "tracks-sidebar-width";

function sidebarWidthBounds() {
  return { min: 280, max: Math.min(560, Math.floor(window.innerWidth * 0.6)) };
}

function setSidebarWidth(width, persist = false) {
  const bounds = sidebarWidthBounds();
  const nextWidth = Math.round(Math.max(bounds.min, Math.min(bounds.max, width)));
  elements.appShell.style.setProperty("--sidebar", `${nextWidth}px`);
  elements.sidebarResizer.setAttribute("aria-valuemax", bounds.max);
  elements.sidebarResizer.setAttribute("aria-valuenow", nextWidth);
  if (persist) localStorage.setItem(sidebarWidthStorageKey, nextWidth);
  if (typeof map !== "undefined") requestAnimationFrame(() => map.resize());
}

function restoreSidebarWidth() {
  const savedWidth = Number(localStorage.getItem(sidebarWidthStorageKey));
  setSidebarWidth(Number.isFinite(savedWidth) && savedWidth > 0 ? savedWidth : 332);
}
