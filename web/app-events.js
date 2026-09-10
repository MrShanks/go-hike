lucide.createIcons();
restoreSidebarWidth();

elements.sidebarResizer.addEventListener("pointerdown", (event) => {
  if (event.button !== 0) return;
  elements.sidebarResizer.setPointerCapture(event.pointerId);
  elements.sidebarResizer.classList.add("dragging");
  document.body.classList.add("resizing-sidebar");
});
elements.sidebarResizer.addEventListener("pointermove", (event) => {
  if (!elements.sidebarResizer.hasPointerCapture(event.pointerId)) return;
  setSidebarWidth(event.clientX);
});
elements.sidebarResizer.addEventListener("pointerup", (event) => {
  if (!elements.sidebarResizer.hasPointerCapture(event.pointerId)) return;
  elements.sidebarResizer.releasePointerCapture(event.pointerId);
  elements.sidebarResizer.classList.remove("dragging");
  document.body.classList.remove("resizing-sidebar");
  setSidebarWidth(event.clientX, true);
});
elements.sidebarResizer.addEventListener("keydown", (event) => {
  if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
  event.preventDefault();
  const direction = event.key === "ArrowLeft" ? -1 : 1;
  const step = event.shiftKey ? 40 : 10;
  setSidebarWidth(Number(elements.sidebarResizer.getAttribute("aria-valuenow")) + direction * step, true);
});
window.addEventListener("resize", () => setSidebarWidth(Number(elements.sidebarResizer.getAttribute("aria-valuenow"))));

document.querySelectorAll("#import-button, #import-map-button, #empty-import-button").forEach((button) => {
  button.addEventListener("click", () => elements.fileInput.click());
});
elements.fileInput.addEventListener("change", () => importFiles(elements.fileInput.files));
document.querySelector("#photo-import-button").addEventListener("click", () => elements.photoInput.click());
elements.photoInput.addEventListener("change", () => importPhotos(elements.photoInput.files));
elements.trackPhotoInput.addEventListener("change", () => importPhotos(elements.trackPhotoInput.files, state.galleryTrackId));
elements.trackList.addEventListener("click", (event) => {
  const item = event.target.closest("[data-track-id]");
  if (state.trackSelectionMode && item) {
    if (state.selectedTrackIds.has(item.dataset.trackId)) {
      state.selectedTrackIds.delete(item.dataset.trackId);
    } else {
      state.selectedTrackIds.add(item.dataset.trackId);
    }
    render();
    return;
  }
  const name = event.target.closest(".track-name");
  if (name) {
    event.stopPropagation();
    beginTrackRename(name);
    return;
  }
  if (item) selectTrack(item.dataset.trackId, true);
});
document.querySelector("#select-tracks").addEventListener("click", toggleTrackSelectionMode);
document.querySelector("#delete-selected-tracks").addEventListener("click", deleteSelectedTracks);
document.querySelector("#fit-button").addEventListener("click", () => fitTracks());
document.querySelector("#focus-track").addEventListener("click", () => selectTrack(state.selectedId, true));
document.querySelector("#track-photos").addEventListener("click", openTrackGallery);
document.querySelector("#terrain-toggle").addEventListener("click", toggleTerrain);
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
