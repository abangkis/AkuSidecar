import { boundedTimelineMedia, mediaViewerCanPan, mediaViewerPanPosition, moveTimelineCarouselIndex, normalizeTimelineCarouselIndex, shouldUseTimelineCarousel, timelineCarouselDotIndexes } from "./timeline-media-carousel.js";
import { collectionModeState } from "./collection-mode.js";
import { reserveMediaDimensions } from "./timeline-media-layout.js";

export function createTimelineMedia({
  $, configureNativePostLink, getPlaybackRecovery, recordUIActivity, safeMediaUrl, safePlaybackUrl, safeSourceUrl, showNotice, sourceDescriptor, sourceLabel, state
}) {
  let inlineVideoVisibilityObserver = null;

  function buildMedia(values, source, contentKind = "", nativePostUrl = "", entry = null) {
    const media = boundedTimelineMedia((Array.isArray(values) ? values : [])
      .map((value) => ({
        ...value,
        displayUrl: safeMediaUrl(value.posterUrl || value.url),
        inlinePlaybackUrl: value.playbackMode === "inline"
          ? safePlaybackUrl(value.playbackUrl, source)
          : null,
      }))
      .filter((value) => value.displayUrl), undefined, source);
    if (!media.length) return null;
    const isVideoMedia = (value) => value.kind === "video" || (contentKind === "video" && media.length === 1);
    const imageMedia = media.filter((value) => !isVideoMedia(value));
    if (shouldUseTimelineCarousel(media)) {
      return buildTimelineMediaCarousel({
        media,
        imageMedia,
        isVideoMedia,
        source,
        nativePostUrl,
        entry,
      });
    }
    const gallery = document.createElement("div");
    gallery.className = `source-layout-media media-count-${media.length}`;
    for (const value of media) {
      const isVideo = isVideoMedia(value);
      if (isVideo) {
        gallery.append(buildVideoMedia(value, source, nativePostUrl, entry));
        continue;
      }
      gallery.append(buildImageMediaControl(value, source, imageMedia));
    }
    return gallery;
  }

  function buildTimelineMediaCarousel({ media, imageMedia, isVideoMedia, source, nativePostUrl, entry }) {
    const carousel = document.createElement("section");
    carousel.className = `source-layout-media source-layout-media-carousel media-count-${media.length}`;
    carousel.tabIndex = 0;
    carousel.setAttribute("role", "region");
    carousel.setAttribute("aria-label", `${sourceLabel(source)} post media carousel`);

    const stage = document.createElement("div");
    stage.className = "source-layout-media-carousel-stage";
    const viewport = document.createElement("div");
    viewport.className = "source-layout-media-carousel-viewport";
    const previous = document.createElement("button");
    previous.type = "button";
    previous.className = "source-layout-media-carousel-previous";
    previous.setAttribute("aria-label", "Previous post image");
    previous.textContent = "‹";
    const next = document.createElement("button");
    next.type = "button";
    next.className = "source-layout-media-carousel-next";
    next.setAttribute("aria-label", "Next post image");
    next.textContent = "›";
    stage.append(viewport, previous, next);

    const navigation = document.createElement("div");
    navigation.className = "source-layout-media-carousel-navigation";
    const dots = document.createElement("div");
    dots.className = "source-layout-media-carousel-dots";
    dots.setAttribute("aria-hidden", "true");
    const status = document.createElement("span");
    status.className = "source-layout-media-carousel-status";
    status.setAttribute("aria-live", "polite");
    navigation.append(dots, status);
    carousel.append(stage, navigation);

    const carouselKey = entry?.id ? String(entry.id) : null;
    let currentIndex = normalizeTimelineCarouselIndex(
      carouselKey ? state.timelineCarouselIndexes.get(carouselKey) : 0,
      media.length,
    );
    let suppressOpenUntil = 0;

    function show(index) {
      currentIndex = normalizeTimelineCarouselIndex(index, media.length);
      if (carouselKey) state.timelineCarouselIndexes.set(carouselKey, currentIndex);
      const value = media[currentIndex];
      viewport.replaceChildren(isVideoMedia(value)
        ? buildVideoMedia(value, source, nativePostUrl, entry)
        : buildImageMediaControl(value, source, imageMedia, () => Date.now() >= suppressOpenUntil));
      previous.disabled = currentIndex === 0;
      next.disabled = currentIndex === media.length - 1;
      status.textContent = `${currentIndex + 1} of ${media.length}`;
      dots.replaceChildren(...timelineCarouselDotIndexes(media.length, currentIndex).map((index, offset, indexes) => {
        const dot = document.createElement("span");
        dot.className = "source-layout-media-carousel-dot";
        dot.classList.toggle("is-current", index === currentIndex);
        dot.classList.toggle("is-window-edge", (
          offset === 0 && index > 0 ||
          offset === indexes.length - 1 && index < media.length - 1
        ));
        return dot;
      }));
    }

    function move(delta) {
      show(moveTimelineCarouselIndex(currentIndex, delta, media.length));
    }

    previous.addEventListener("click", () => move(-1));
    next.addEventListener("click", () => move(1));
    carousel.addEventListener("keydown", (event) => {
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
      event.preventDefault();
      move(event.key === "ArrowLeft" ? -1 : 1);
    });

    let pointerStart = null;
    viewport.addEventListener("pointerdown", (event) => {
      if (!event.isPrimary || event.button !== 0 || event.target.closest("video")) return;
      pointerStart = { id: event.pointerId, x: event.clientX, y: event.clientY, captured: false };
    });
    viewport.addEventListener("pointermove", (event) => {
      if (!pointerStart || pointerStart.id !== event.pointerId || pointerStart.captured) return;
      const deltaX = event.clientX - pointerStart.x;
      const deltaY = event.clientY - pointerStart.y;
      if (Math.abs(deltaX) < 10 && Math.abs(deltaY) < 10) return;
      if (Math.abs(deltaX) <= Math.abs(deltaY)) {
        pointerStart = null;
        return;
      }
      pointerStart.captured = true;
      viewport.setPointerCapture?.(event.pointerId);
    });
    viewport.addEventListener("pointerup", (event) => {
      if (!pointerStart || pointerStart.id !== event.pointerId) return;
      const deltaX = event.clientX - pointerStart.x;
      const deltaY = event.clientY - pointerStart.y;
      pointerStart = null;
      if (Math.abs(deltaX) < 40 || Math.abs(deltaX) <= Math.abs(deltaY)) return;
      suppressOpenUntil = Date.now() + 300;
      event.preventDefault();
      move(deltaX < 0 ? 1 : -1);
    });
    viewport.addEventListener("pointercancel", () => { pointerStart = null; });

    show(currentIndex);
    return carousel;
  }

  function buildImageMediaControl(value, source, imageMedia, canOpen = () => true) {
    const control = document.createElement("button");
    control.type = "button";
    control.className = "source-layout-media-item";
    const imageIndex = imageMedia.indexOf(value);
    control.setAttribute("aria-label", `Open image ${imageIndex + 1} of ${imageMedia.length} in viewer`);
    const image = document.createElement("img");
    reserveMediaDimensions(image, value);
    image.src = value.displayUrl;
    image.alt = value.alt || `${sourceLabel(source)} post media`;
    image.loading = "lazy";
    image.referrerPolicy = "no-referrer";
    control.append(image);
    control.addEventListener("click", () => {
      if (!canOpen()) return;
      openMedia(imageMedia.map((entry) => entry.displayUrl), imageIndex);
    });
    return control;
  }

  function buildVideoMedia(value, source, nativePostUrl, entry = null) {
    const shell = document.createElement("div");
    shell.className = "source-layout-video-shell";
    const canPlayInline = Boolean(value.inlinePlaybackUrl);
    const control = buildVideoPosterControl({
      width: value.width,
      height: value.height,
      posterUrl: value.displayUrl,
      alt: value.alt,
      source,
      nativePostUrl,
      playInline: canPlayInline
        ? () => activateInlineVideo(shell, {
          width: value.width,
          height: value.height,
          playbackUrl: value.inlinePlaybackUrl,
          posterUrl: value.displayUrl,
          alt: value.alt,
          source,
          nativePostUrl,
          entry,
        })
        : null,
    });
    shell.append(control);
    return shell;
  }

  function buildVideoPosterControl({ posterUrl, alt, source, nativePostUrl, width, height, playInline = null }) {
    const canPlayInline = typeof playInline === "function";
    const control = document.createElement(canPlayInline || !nativePostUrl ? "button" : "a");
    if (control.tagName === "BUTTON") control.type = "button";
    control.className = "source-layout-media-item is-video-poster";
    const image = document.createElement("img");
    reserveMediaDimensions(image, { width, height });
    image.src = posterUrl;
    image.alt = alt || `${sourceLabel(source)} video poster`;
    image.loading = "lazy";
    image.referrerPolicy = "no-referrer";
    control.append(image);

    const cue = document.createElement("span");
    cue.className = "source-layout-video-cue";
    cue.setAttribute("aria-hidden", "true");
    cue.textContent = "▶";
    control.append(cue);
    if (!canPlayInline) {
      const label = document.createElement("span");
      label.className = "source-layout-video-label";
      label.textContent = nativePostUrl ? "Play on native post" : "Video preview";
      control.append(label);
    }

    if (canPlayInline) {
      control.setAttribute("aria-label", `Play ${sourceLabel(source)} video in AkuBrowser`);
      control.addEventListener("click", playInline);
    } else if (nativePostUrl) {
      configureNativePostLink(control, nativePostUrl, source);
      control.setAttribute("aria-label", `Open ${sourceLabel(source)} native post to play video`);
    } else {
      control.disabled = true;
      control.setAttribute("aria-label", `${sourceLabel(source)} video preview; native post unavailable`);
    }
    return control;
  }

  function activateInlineVideo(shell, { playbackUrl, posterUrl, alt, source, nativePostUrl, width, height, entry = null }) {
    if (!shell || shell.querySelector("video.source-layout-inline-video")) return;
    const control = shell.querySelector(".source-layout-media-item");
    if (!control) return;
    const video = document.createElement("video");
    reserveMediaDimensions(video, { width, height });
    video.className = "source-layout-inline-video";
    video.controls = true;
    video.playsInline = true;
    video.preload = "none";
    video.poster = posterUrl;
    video.setAttribute("aria-label", `${sourceLabel(source)} video`);
    video.addEventListener("play", () => {
      pauseOtherInlineVideos(video);
      recordUIActivity(false);
    });
    video.addEventListener("timeupdate", () => {
      if (!video.paused) recordUIActivity(false);
    });

    let fallbackUsed = false;
    const useNativeFallback = () => {
      if (fallbackUsed) return;
      fallbackUsed = true;
      if (!video.isConnected) return;
      stopObservingInlineVideoVisibility(video);
      video.pause();
      video.removeAttribute("src");
      video.replaceWith(buildVideoPosterControl({
        width,
        height,
        posterUrl,
        alt,
        source,
        nativePostUrl,
      }));
      queueInlinePlaybackRecovery(entry, source, playbackUrl);
    };
    video.addEventListener("error", useNativeFallback, { once: true });

    control.replaceWith(video);
    observeInlineVideoVisibility(video);
    // Assigning src only after the explicit click keeps Timeline rendering from
    // creating a passive request to the source video CDN.
    video.src = playbackUrl;
    const playback = video.play();
    if (playback?.catch) {
      playback.catch(() => {
        if (video.error) useNativeFallback();
      });
    }
  }

  function queueInlinePlaybackRecovery(entry, source, playbackUrl) {
    const descriptor = sourceDescriptor(source);
    if (
      descriptor?.playbackRecoveryCapability !== "native_post_recapture" ||
      !entry?.id || !safeSourceUrl(entry.item?.sourceUrl || entry.evidence?.permalink, source) ||
      !safePlaybackUrl(playbackUrl, source)
    ) return;
    if (getPlaybackRecovery().enqueue({ id: entry.id, source, playbackUrl }) && !playbackRecoveryReady()) {
      showNotice("Video refresh queued. It will run when collection is ready.");
    }
    drainPlaybackRecovery();
  }

  function playbackRecoveryReady() {
    const runtime = state.bootstrap?.collectionRuntime;
    return Boolean(state.bootstrap && !state.bootstrapLoading && !state.bootstrapError && !state.sidecarEpochReloading
      && !state.session && !state.pollInFlight && !state.mediaRecaptureActive && !state.closingNativeReaders
      && !state.bootstrap.calibration?.active && state.bootstrap.onboarding?.status === "completed"
      && !runtime?.nativeReaderOnly && !(runtime?.activeLeases > 0)
      && collectionModeState(runtime, state.bootstrap.bridge?.compatible).canCollect);
  }

  function drainPlaybackRecovery() {
    void getPlaybackRecovery().drain().catch((error) => console.warn("Video refresh queue deferred.", error));
  }

  function pauseOtherInlineVideos(activeVideo) {
    for (const video of document.querySelectorAll("video.source-layout-inline-video")) {
      if (video !== activeVideo && !video.paused) video.pause();
    }
  }

  function observeInlineVideoVisibility(video) {
    if (!(video instanceof HTMLVideoElement) || !("IntersectionObserver" in window)) return;
    if (!inlineVideoVisibilityObserver) {
      inlineVideoVisibilityObserver = new IntersectionObserver((entries, observer) => {
        for (const entry of entries) {
          const observedVideo = entry.target;
          if (!observedVideo.isConnected) {
            observer.unobserve(observedVideo);
            continue;
          }
          if (!entry.isIntersecting && !observedVideo.paused) observedVideo.pause();
        }
      }, { threshold: 0 });
    }
    inlineVideoVisibilityObserver.observe(video);
  }

  function stopObservingInlineVideoVisibility(video) {
    inlineVideoVisibilityObserver?.unobserve(video);
  }

  function stopObservingInlineVideoVisibilityWithin(root) {
    for (const video of root?.querySelectorAll?.("video.source-layout-inline-video") ?? []) {
      stopObservingInlineVideoVisibility(video);
    }
  }

  function openMedia(media, index) {
    state.media = media;
    state.mediaIndex = index;
    state.mediaZoomPreset = "fit";
    state.mediaZoomFactor = 1;
    state.mediaZoomManual = false;
    const viewer = $("#media-viewer");
    viewer.showModal();
    renderMedia();
    viewer.focus({ preventScroll: true });
  }

  function moveMedia(delta) {
    if (!state.media.length) return;
    state.mediaIndex = moveTimelineCarouselIndex(state.mediaIndex, delta, state.media.length);
    renderMedia();
  }

  function renderMedia() {
    endMediaPan();
    state.mediaZoomManual = false;
    const image = $("#media-viewer-image");
    image.removeAttribute("style");
    $("#media-viewer-surface").removeAttribute("style");
    image.src = state.media[state.mediaIndex] ?? "";
    $("#media-viewer-canvas").scrollTo(0, 0);
    syncMediaPanAffordance();
    $("#media-viewer-count").textContent = `${state.mediaIndex + 1} of ${state.media.length}`;
    $("#media-viewer-previous").disabled = state.media.length < 2 || state.mediaIndex === 0;
    $("#media-viewer-next").disabled = state.media.length < 2 || state.mediaIndex === state.media.length - 1;
  }

  function mediaZoomMetrics() {
    const canvas = $("#media-viewer-canvas");
    const image = $("#media-viewer-image");
    if (!canvas.clientWidth || !canvas.clientHeight || !image.naturalWidth || !image.naturalHeight) return null;
    const fitScale = Math.min(canvas.clientWidth / image.naturalWidth, canvas.clientHeight / image.naturalHeight);
    return {
      canvas,
      image,
      surface: $("#media-viewer-surface"),
      fitScale,
      fitWidth: image.naturalWidth * fitScale,
      fitHeight: image.naturalHeight * fitScale,
    };
  }

  function applyMediaZoom(factor, focalPoint = null) {
    const metrics = mediaZoomMetrics();
    if (!metrics) return;
    const { canvas, image, surface, fitScale, fitWidth, fitHeight } = metrics;
    const bounded = Math.min(12, Math.max(0.1, factor));
    const focusX = focalPoint?.x ?? canvas.clientWidth / 2;
    const focusY = focalPoint?.y ?? canvas.clientHeight / 2;
    const oldWidth = Math.max(canvas.scrollWidth, canvas.clientWidth);
    const oldHeight = Math.max(canvas.scrollHeight, canvas.clientHeight);
    const ratioX = (canvas.scrollLeft + focusX) / oldWidth;
    const ratioY = (canvas.scrollTop + focusY) / oldHeight;
    const width = fitWidth * bounded;
    const height = fitHeight * bounded;
    image.style.width = `${width}px`;
    image.style.height = `${height}px`;
    surface.style.width = `${Math.max(canvas.clientWidth, width)}px`;
    surface.style.height = `${Math.max(canvas.clientHeight, height)}px`;
    state.mediaZoomFactor = bounded;
    $("#media-viewer-scale").textContent = `${Math.round(fitScale * bounded * 100)}%`;
    canvas.scrollLeft = ratioX * Math.max(canvas.scrollWidth, canvas.clientWidth) - focusX;
    canvas.scrollTop = ratioY * Math.max(canvas.scrollHeight, canvas.clientHeight) - focusY;
    syncMediaPanAffordance();
  }

  function syncMediaPanAffordance() {
    const canvas = $("#media-viewer-canvas");
    const pannable = mediaViewerCanPan(canvas);
    canvas.classList.toggle("is-pannable", pannable);
    if (!pannable && state.mediaPanPointerId !== null) endMediaPan();
  }

  function mediaPanStartedOnScrollbar(event, canvas) {
    const rect = canvas.getBoundingClientRect();
    const verticalGutter = Math.max(0, canvas.offsetWidth - canvas.clientWidth);
    const horizontalGutter = Math.max(0, canvas.offsetHeight - canvas.clientHeight);
    return (verticalGutter > 0 && event.clientX >= rect.right - verticalGutter)
      || (horizontalGutter > 0 && event.clientY >= rect.bottom - horizontalGutter);
  }

  function beginMediaPan(event) {
    const canvas = $("#media-viewer-canvas");
    if (!$("#media-viewer").open || event.button !== 0 || !event.isPrimary) return;
    if (!mediaViewerCanPan(canvas) || mediaPanStartedOnScrollbar(event, canvas)) return;
    state.mediaPanPointerId = event.pointerId;
    state.mediaPanStartX = event.clientX;
    state.mediaPanStartY = event.clientY;
    state.mediaPanStartLeft = canvas.scrollLeft;
    state.mediaPanStartTop = canvas.scrollTop;
    canvas.classList.add("is-panning");
    canvas.setPointerCapture?.(event.pointerId);
    event.preventDefault();
  }

  function moveMediaPan(event) {
    if (state.mediaPanPointerId !== event.pointerId) return;
    const canvas = $("#media-viewer-canvas");
    const position = mediaViewerPanPosition({
      startLeft: state.mediaPanStartLeft,
      startTop: state.mediaPanStartTop,
      startX: state.mediaPanStartX,
      startY: state.mediaPanStartY,
      x: event.clientX,
      y: event.clientY,
    });
    if (!position) return;
    canvas.scrollLeft = position.left;
    canvas.scrollTop = position.top;
    event.preventDefault();
  }

  function endMediaPan(event = null) {
    if (event && state.mediaPanPointerId !== event.pointerId) return;
    const canvas = $("#media-viewer-canvas");
    const pointerId = state.mediaPanPointerId;
    if (pointerId !== null && canvas.hasPointerCapture?.(pointerId)) {
      canvas.releasePointerCapture?.(pointerId);
    }
    state.mediaPanPointerId = null;
    canvas.classList.remove("is-panning");
  }

  function syncMediaZoomPresetControl() {
    const button = $("#media-viewer-zoom");
    const target = state.mediaZoomPreset === "fit" ? "zoom" : "fit";
    button.textContent = target === "fit" ? "Fit" : "Zoom";
    button.setAttribute("aria-label", target === "fit" ? "Snap image to fit" : "Snap image to actual size");
    button.title = target === "fit" ? "Snap image to fit" : "Snap image to actual size";
  }

  function snapMediaZoom(preset) {
    const metrics = mediaZoomMetrics();
    state.mediaZoomPreset = preset === "zoom" ? "zoom" : "fit";
    state.mediaZoomManual = false;
    syncMediaZoomPresetControl();
    if (!metrics) return;
    const target = state.mediaZoomPreset === "zoom" ? 1 / metrics.fitScale : 1;
    applyMediaZoom(target);
  }

  function adjustMediaZoom(multiplier, focalPoint = null) {
    if (!mediaZoomMetrics()) return;
    state.mediaZoomManual = true;
    applyMediaZoom(state.mediaZoomFactor * multiplier, focalPoint);
  }

  return {
    buildMedia, buildTimelineMediaCarousel, buildImageMediaControl, buildVideoMedia, buildVideoPosterControl, activateInlineVideo, queueInlinePlaybackRecovery, playbackRecoveryReady, drainPlaybackRecovery, pauseOtherInlineVideos, observeInlineVideoVisibility, stopObservingInlineVideoVisibility, stopObservingInlineVideoVisibilityWithin, openMedia, moveMedia, renderMedia, mediaZoomMetrics, applyMediaZoom, syncMediaPanAffordance, mediaPanStartedOnScrollbar, beginMediaPan, moveMediaPan, endMediaPan, syncMediaZoomPresetControl, snapMediaZoom, adjustMediaZoom
  };
}
