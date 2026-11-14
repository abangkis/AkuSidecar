import { createTimelineFeedbackUI } from "./timeline-feedback.js";
import { createDirtyStateTracker } from "./settings-dirty-state.js";
import { normalizePage, renderPageNavigation, focusPageHeading } from "./page-navigation.js";



import { createNativePostRouter } from "./native-post-routing.js";
import { nativePostWaitReason, syncNativePostAvailability } from "./native-post-availability.js";

import { setSettingsText, setSettingsClass } from "./settings-render.js";
import { createXDailyControls } from "./x-daily-controls.js";
import { createInstagramTrialControls } from "./instagram-trial-controls.js";
import { createFrameTaskQueue } from "./ui-frame.js";
import { createScrollIdleGate } from "./timeline-scroll-layout.js";



import { createScrollPerformanceTrace } from "./scroll-performance-trace.js";

import { createPlaybackRecoveryQueue } from "./playback-recovery-queue.js";











import { LIBRARY_TAB_CLEANING, LIBRARY_TAB_LIBRARY, LIBRARY_TAB_SAVED, normalizeLibraryFilters } from "./library-state.js";
import { LIVING_TOPIC_TAB_EVIDENCE, LIVING_TOPIC_TAB_SNAPSHOT, normalizeLivingTopicNotifications } from "./living-topics-state.js";


import { createPostScreenshotUI } from "./post-screenshot-ui.js";

// Frontend composition imports: begin
import { createCaptureUi } from "./capture-ui.js";
import { createAppLifecycle } from "./app-lifecycle.js";
import { createSettingsStorageUi } from "./settings-storage-ui.js";
import { createLivingTopicsUi } from "./living-topics-ui.js";
import { createLibraryUi } from "./library-ui.js";
import { createInboxUi } from "./inbox-ui.js";
import { createSettingsFeature } from "./settings-feature.js";
import { createAutoUpdateUi } from "./auto-update-ui.js";
import { createTimelineRendering } from "./timeline-rendering.js";
import { createOnboardingUi } from "./onboarding-ui.js";
import { createProviderSetupUi } from "./provider-setup-ui.js";
import { createSettingsOperations } from "./settings-operations.js";
import { createDiagnosticsUi } from "./diagnostics-ui.js";
import { createSessionUi } from "./session-ui.js";
import { createPassiveMediaUi } from "./passive-media-ui.js";
import { createTimelineContent } from "./timeline-content.js";
import { createTimelineAiUi } from "./timeline-ai-ui.js";
import { createTimelineMedia } from "./timeline-media.js";
import { createTimelineActions } from "./timeline-actions.js";
import { createTimelineContext } from "./timeline-context.js";
// Frontend composition imports: end

// The embedded Timeline action is Read later. Existing Library detail keeps
// its independent full-copy Release/Remove/Forget semantics.

const endpoint = location.origin;
const defaultIntent = "What materially changed since my last check?";
const terminalStatuses = new Set(["completed", "partial", "failed", "cancelled"]);
const CAPTURE_CLEANUP_PENDING_MESSAGE = "Capture cleanup is pending. Keep related tabs unchanged until ownership is verified.";
const CAPTURE_CLEANUP_PENDING_DETAIL = "Cleanup ownership is unverified. Leave related tabs unchanged while cleanup retries.";
const BACK_TO_TOP_THRESHOLD_PX = 480;
const SOURCE_TEXT_COLLAPSE_CHARACTERS = 420;
const SOURCE_TEXT_COLLAPSE_LINES = 6;
const QUOTE_TEXT_COLLAPSE_CHARACTERS = 280;
const QUOTE_TEXT_COLLAPSE_LINES = 4;
const DEFAULT_TIMELINE_BATCH_GAP_PX = 36;
const DEFAULT_TIMELINE_BOUNDARY_RETURN_MS = 350;
const DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD = 0.92;
const AI_HIDE_CONFIRMATION_PHRASE = "HIDE STRONG AI SIGNALS";
const AI_DEEP_POLL_INTERVAL_MS = 5000;
const ONBOARDING_LEARNING_INTERVAL_MS = 7000;
const PASSIVE_MEDIA_LOOKUP_TIMEOUT_MS = 2500;
const NATIVE_POST_OPEN_TIMEOUT_MS = 35000;
const PASSIVE_MEDIA_LOOKUP_COOLDOWN_MS = 10000;
const BRIDGE_CONTEXT_RECOVERY_KEY = "akuBridgeContextRecoveryAt";
const BRIDGE_TOKEN_RECOVERY_KEY = "akuBridgeTokenRecoveryAt";
const BRIDGE_CONTEXT_RECOVERY_WINDOW_MS = 30000;
const BOOTSTRAP_TIMEOUT_MS = 45000;
const BOOTSTRAP_RETRY_MS = 3000;
const SOURCE_SESSION_PROBE_TIMEOUT_MESSAGE = "Source session check timed out. Return to AkuBrowser or open the source again to retry.";
const LOAD_PROFILE_PRESETS = {
  standard: { timelineCapacity: 12, maxItemsPerSource: 5, maxItemsTotal: 10, maxScrolls: 2 },
  expanded: { timelineCapacity: 24, maxItemsPerSource: 10, maxItemsTotal: 20, maxScrolls: 4 },
  stress: { timelineCapacity: 36, maxItemsPerSource: 15, maxItemsTotal: 30, maxScrolls: 6 },
};
const RELEASE_REASONING_DEFAULTS = Object.freeze({
  acquisition_planning: "gpt6_luna_high",
  candidate_evaluation: "gpt6_luna_high",
  semantic_event_resolution: "gpt6_luna_high",
  ai_deep_detection: "gpt6_luna_high",
});
const state = {
  bootstrap: null,
  bootstrapLoading: true,
  closingNativeReaders: false,
  nativeReaderResumeFeedback: "",
  bootstrapError: null,
  bootstrapAttempt: 0,
  bootstrapController: null,
  bootstrapRetryTimer: null,
  session: null,
  dispatchKey: null,
  dispatchRetryAfter: new Map(),
  poller: null,
  pollInFlight: false,
  captureCleanupPending: null,
  sessionProgress: { sessionId: null, value: 0 },
  currentView: "timeline",
  library: {
    filters: normalizeLibraryFilters(),
    activeTab: LIBRARY_TAB_SAVED,
    items: [],
    topicKnowledge: [],
    nextCursor: "",
    selectedId: "",
    selectedItem: null,
    loading: false,
    loadingMore: false,
    error: null,
    storage: {
      usage: null,
      recommendations: [],
      savedPressure: null,
      savedRecommendations: [],
      loading: false,
      error: null,
      requestID: 0,
      controller: null,
    },
    detailLoading: false,
    detailError: null,
    mutationLoading: false,
    mutationError: null,
    mutationAction: "",
    mutationErrorAction: "",
    notice: "",
    requestID: 0,
    detailRequestID: 0,
    controller: null,
    detailFocusTarget: null,
  },
  livingTopics: {
    topics: [],
    selectedId: "",
    detail: null,
    candidates: [],
    candidatesLoaded: false,
    activeTab: LIVING_TOPIC_TAB_SNAPSHOT,
    loading: false,
    detailLoading: false,
    candidatesLoading: false,
    mutationLoading: false,
    error: null,
    snapshotStatus: { kind: "idle", message: "", snapshotId: "" },
    understandingPollTopicId: "",
    activationPollTopicId: "",
    notificationLoading: false,
    acknowledgingTopicId: "",
    lastMove: null,
  },
  livingTopicNotifications: normalizeLivingTopicNotifications(),
  inboxSubView: "checks",
  modelUsageHelpSequence: 0,
  media: [],
  mediaIndex: 0,
  mediaZoomPreset: "fit",
  mediaZoomFactor: 1,
  mediaZoomManual: false,
  mediaPanPointerId: null,
  mediaPanStartX: 0,
  mediaPanStartY: 0,
  mediaPanStartLeft: 0,
  mediaPanStartTop: 0,
  onboardingEditing: false,
  calibration: null,
  calibrationOrdinal: 0,
  onboardingLearningIndex: 0,
  onboardingLearningTimer: null,
  onboardingLearningPaused: false,
  onboardingLearningUserPaused: false,
  resetOperation: null,
  backToTopLastScrollY: 0,
  backToTopBoundary: null,
  mediaRecaptureActive: false,
  foregroundRecaptureOffers: new Map(),
  pendingSettings: null,
  seenTimelineItems: new Set(),
  aiDeepPoller: null,
  sidePaneItems: [],
  timelineItems: [],
  timelineReadLaterInFlight: new Set(),
  timelineContentContext: new Map(),
  timelineContentContextFeedbackInFlight: new Set(),
  timelineContentContextActiveID: "",
  timelineContentContextViewportID: "",
  timelineContentContextExpanded: false,
  timelineContentContextObservedAnchor: null,
  timelineContentContextDrawerOpen: false,
  timelineContentContextPositionFrame: null,
  timelineContentContextCloseTimer: null,
  timelineContentContextLastScrollY: window.scrollY,
  timelineBatches: [],
  sourceSessionReadiness: {},
  sourceSessionProbeInFlight: false,
  sourceSessionProbeTimer: null,
  providerReadinessInFlight: false,
  providerReadinessCheckedAt: "",
  providerReadinessCheckError: "",
  providerDialogMode: "onboarding",
  passiveMediaEnrichmentTimer: null,
  passiveMediaEnrichmentActive: false,
  passiveMediaEvidenceAttempts: new Map(),
  releasedCaptureSources: new Set(),
  expandedTimelineText: new Set(),
  timelineCarouselIndexes: new Map(),
  expandedAIDetails: new Set(),
  expandedAIFeedbackOptions: new Set(),
  backgroundTimelineRefreshPending: false,
  backgroundTimelineRefreshInFlight: false,
  revealingPreparedBatch: false,
  autoLoadLastScrollY: window.scrollY,
  uiActivityTrackingInstalled: false,
  lastUIActivitySentAt: 0,
  sidecarEpochReloading: false,
  settingsUnloadBypass: false,
  settingsStorage: { usage: null, loading: false, error: null },
};
// Frontend composition bindings: begin
const {
  recoverInvalidatedBridgeContext, recoverInvalidBridgeToken, pingBridge, refreshBridgeAfterUserReturn, requestSourceSessionReadiness, openSourceFromSettings, syncNativePostLinks, configureNativePostLink, openNativePostInReaderWindow, cancelNativePostBroker, setSourceSessionStatus, renderSourceSessionError, renderSourceSessionReadiness, renderOnboardingSourceReadiness, bridgeActionLoop, renderBridge, configureBackgroundBridge, renderCollectionRuntime, renderBrowserConnection, openChromeExtensions, reloadIncompatibleBridge
} = createCaptureUi({
  $: (...args) => $(...args),
  BRIDGE_CONTEXT_RECOVERY_KEY,
  BRIDGE_CONTEXT_RECOVERY_WINDOW_MS,
  BRIDGE_TOKEN_RECOVERY_KEY,
  NATIVE_POST_OPEN_TIMEOUT_MS,
  SOURCE_SESSION_PROBE_TIMEOUT_MESSAGE,
  api,
  bridgeApi,
  bridgeSourceReadiness: (...args) => bridgeSourceReadiness(...args),
  endpoint,
  logNativePostTrace,
  getNativePointerTraces: () => nativePointerTraces,
  getNativePostOpening: () => nativePostOpening,
  renderSourceSettingsValues: (...args) => renderSourceSettingsValues(...args),
  getRouteNativePost: () => routeNativePost,
  schedulePassiveMediaEnrichment: (...args) => schedulePassiveMediaEnrichment(...args),
  setPill,
  showError,
  showNotice,
  sourceDescriptor,
  sourceDescriptors,
  state,
  syncRunButtons: (...args) => syncRunButtons(...args),
  updateOnboardingSummary: (...args) => updateOnboardingSummary(...args),
  setNativePostOpening: (value) => nativePostOpening = value,
});
const {
  installUIActivityTracking, recordUIActivity, bootstrap, renderDeployment, pollAutoUpdate, pollDatabaseHealth, renderDatabaseHealth, cleanDatabase, pollCollectionRuntime, retryBootstrap
} = createAppLifecycle({
  $: (...args) => $(...args),
  BOOTSTRAP_RETRY_MS,
  BOOTSTRAP_TIMEOUT_MS,
  api,
  bridgeActionLoop: (...args) => bridgeActionLoop(...args),
  clearNotice,
  getInstagramTrialControls: () => instagramTrialControls,
  onboardingRequiresSetup,
  pingBridge: (...args) => pingBridge(...args),
  refreshLivingTopicNotificationProjection: (...args) => refreshLivingTopicNotificationProjection(...args),
  renderActiveReasoningProvider: (...args) => renderActiveReasoningProvider(...args),
  renderAutoUpdateStatus: (...args) => renderAutoUpdateStatus(...args),
  renderBridge: (...args) => renderBridge(...args),
  renderCollectionRuntime: (...args) => renderCollectionRuntime(...args),
  renderLibrary: (...args) => renderLibrary(...args),
  renderLibrarySourceOptions: (...args) => renderLibrarySourceOptions(...args),
  renderSession: (...args) => renderSession(...args),
  renderSettings: (...args) => renderSettings(...args),
  renderSourceControls: (...args) => renderSourceControls(...args),
  renderSourceSessionReadiness: (...args) => renderSourceSessionReadiness(...args),
  renderTimeline: (...args) => renderTimeline(...args),
  setPill,
  setView,
  showCalibration: (...args) => showCalibration(...args),
  showError,
  showNotice,
  showOnboarding: (...args) => showOnboarding(...args),
  startPolling: (...args) => startPolling(...args),
  state,
  syncRunButtons: (...args) => syncRunButtons(...args),
  getXDailyControls: () => xDailyControls,
});
const {
  settingsStorageDetail, renderSettingsStorage, loadSettingsStorage
} = createSettingsStorageUi({
  $: (...args) => $(...args),
  api,
  state,
});
const {
  loadLivingTopics, refreshLivingTopicNotificationProjection, updateLivingTopicNotificationFromTopics, renderLivingTopicNotificationBadge, selectLivingTopic, loadLivingTopicDetail, createLivingTopic, renameLivingTopic, searchLivingTopicEvidence, loadLivingTopicCandidates, addLivingTopicMember, removeLivingTopicMember, moveLivingTopicMember, undoLivingTopicMemberMove, requestLivingTopicActivation, reviewLivingTopicCandidate, resetLivingTopicSnapshotStatus, createLivingTopicSnapshot, scheduleLivingTopicUnderstandingPoll, scheduleLivingTopicActivationPoll, refreshLivingTopicListProjection, renderLivingTopics, renderLivingTopicDetail, livingTopicUnderstandingSummary, livingTopicRoutingSummary, setLivingTopicDetailTab, renderLivingTopicDetailTabs, handleLivingTopicDetailTabKeydown, buildLivingTopicEvidenceCard, renderLivingTopicMoveStatus, buildLivingTopicSuggestionCard, buildLivingTopicReviewedCandidateCard, renderLivingTopicSnapshots, buildLivingTopicUnderstandingHistory, buildCurrentLivingTopicUnderstanding, buildLivingTopicHistoryEntry, buildLivingTopicStatementList, livingTopicSourceOrigin
} = createLivingTopicsUi({
  $: (...args) => $(...args),
  api,
  formatDate,
  humanize,
  sourceLabel,
  state,
});
const {
  setLibraryTab, renderLibraryTabs, maybeLoadLibraryStorage, handleLibraryTabKeydown, renderLibrarySourceOptions, toggleLibraryFilters, syncLibraryFilterToggle, readLibraryFilters, resetLibraryList, resetLibraryStorage, invalidateLibraryStorage, reloadLibraryStorage, submitLibrarySearch, clearLibrarySearch, refreshLibrary, loadMoreLibrary, loadLibrary, loadLibraryStorage, libraryErrorMessage, libraryStorageErrorMessage, libraryMutationErrorMessage, renderLibrary, renderLibraryTopicKnowledge, buildLibraryTopicKnowledgeCard, renderLibraryStorage, buildLibrarySavedPressure, buildLibraryStorageUsage, formatLibraryStorageCount, libraryStorageMetric, buildLibraryStorageRecommendation, reviewLibraryStorageRecommendation, buildLibrarySavedRecommendation, reviewSavedRecommendation, libraryStateMessage, buildLibraryCard, libraryMetaChip, selectLibraryItem, closeLibraryDetail, renderLibraryDetail, buildLibraryDetail, removeLibraryItem, releaseLibraryItem, forgetLibraryItem, keepSavedItem, doneSavedItem, mutateLibraryItem, appendLibraryDetailField, appendLibraryLabels, appendLibraryMediaMetadata
} = createLibraryUi({
  $: (...args) => $(...args),
  api,
  buildSourceIcon,
  configureNativePostLink: (...args) => configureNativePostLink(...args),
  formatDate,
  safeMediaUrl,
  safeSourceUrl,
  sourceDescriptors,
  sourceLabel,
  state,
});
const {
  syncInboxSubView, setInboxSubView, loadInbox, renderInbox, buildInboxSession, buildInboxReasoningBadge, buildSessionModelUsage, modelUsageMessage, buildModelUsageHelp, renderModelUsageReport, buildModelUsageCategory, modelUsageCategoryProfile, buildModelUsageEntry, modelUsageInvocationCount, modelUsageSummary, formatTokenCount, loadAggregateModelUsage, buildInboxPreferenceDecisions, buildInboxPreferenceDecision, buildAIDetectionDiagnostic, buildAIDetectorYield, buildEventResolutionDiagnostic, buildInboxRun, buildVisionEvaluationTelemetry, inboxRunCaptureReliabilityNotice, buildCaptureCandidateTelemetry, buildMediaAcquisitionTelemetry, buildCaptureSurfaceTelemetry, buildAcquisitionIdentityTelemetry, inboxSessionFlowText, inboxRunMetricText, buildInboxFlowInspector, buildInboxFlowItem, inboxFlowOutcomeLabel, inboxContinuityLabel, inboxStatusTone
} = createInboxUi({
  $: (...args) => $(...args),
  api,
  configureNativePostLink: (...args) => configureNativePostLink(...args),
  feedbackButton,
  formatDate,
  formatDuration,
  formatDurationBetween,
  formatReasoningEffort: (...args) => formatReasoningEffort(...args),
  formatReasoningModel: (...args) => formatReasoningModel(...args),
  formatReasoningProvider: (...args) => formatReasoningProvider(...args),
  humanize,
  invalidateLibraryStorage: (...args) => invalidateLibraryStorage(...args),
  refreshTimeline: (...args) => refreshTimeline(...args),
  safeSourceUrl,
  scheduleBackToTop: (...args) => scheduleBackToTop(...args),
  showError,
  sourceLabel,
  state,
  updatePolicyBadge: (...args) => updatePolicyBadge(...args),
});
const {
  renderSettings, syncReasoningProviderSelection, beginSettingsProviderSelection, renderSourceSettingsValues, renderReasoningProcesses, formatReasoningProvider, formatReasoningModel, formatReasoningEffort, readSettingsDraft, refreshSettingsDirtyState, renderSettingsDirtyState, saveSettings, reasoningProfileValue, persistSettings, renderReasoningProviderControl, detectReasoningExecutable, syncLoadProfileSettings, syncSemanticEventSettings, syncAIDetectionSettings, resetSemanticEventMergeThreshold
} = createSettingsFeature({
  $: (...args) => $(...args),
  DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD,
  DEFAULT_TIMELINE_BATCH_GAP_PX,
  DEFAULT_TIMELINE_BOUNDARY_RETURN_MS,
  LOAD_PROFILE_PRESETS,
  RELEASE_REASONING_DEFAULTS,
  api,
  applyPostFreshnessStyle: (...args) => applyPostFreshnessStyle(...args),
  applySingleImageFit: (...args) => applySingleImageFit(...args),
  applyStreamWidth: (...args) => applyStreamWidth(...args),
  applyTimelineBatchGap: (...args) => applyTimelineBatchGap(...args),
  applyTimelineBoundaryReturnDuration: (...args) => applyTimelineBoundaryReturnDuration(...args),
  bridgeSourceReadiness: (...args) => bridgeSourceReadiness(...args),
  openResetDialog: (...args) => openResetDialog(...args),
  openSettingsProviderDialog: (...args) => openSettingsProviderDialog(...args),
  refreshTimeline: (...args) => refreshTimeline(...args),
  releaseBackToTopBoundary: (...args) => releaseBackToTopBoundary(...args),
  renderActiveReasoningProvider: (...args) => renderActiveReasoningProvider(...args),
  renderAutoUpdateStatus: (...args) => renderAutoUpdateStatus(...args),
  renderCollectionRuntime: (...args) => renderCollectionRuntime(...args),
  getSettingsDirty: () => settingsDirty,
  shouldShowOnboardingLearning: (...args) => shouldShowOnboardingLearning(...args),
  showError,
  sourceDescriptors,
  state,
  syncAutoUpdateModeSettings: (...args) => syncAutoUpdateModeSettings(...args),
  syncOnboardingLearning: (...args) => syncOnboardingLearning(...args),
});
const {
  renderAutoUpdateStatus, autoUpdateModeLabel, autoUpdateModeDescription, syncAutoUpdateModeSettings, prepareBatchNow, openAutoUpdateSettings, handleAutoUpdateTimelineAction, resetAutoUpdateBudget, confirmXCaptureRecovery, confirmCodexUsageRestored
} = createAutoUpdateUi({
  $: (...args) => $(...args),
  api,
  formatDate,
  formatSchedulerMoment,
  formatTokenCount: (...args) => formatTokenCount(...args),
  humanize,
  renderNativeReaderNotice: (...args) => renderNativeReaderNotice(...args),
  renderSession: (...args) => renderSession(...args),
  renderTimelineActions: (...args) => renderTimelineActions(...args),
  setNoticeText,
  setView,
  showError,
  sourceLabel,
  startPolling: (...args) => startPolling(...args),
  state,
});
const {
  applyStreamWidth, applySingleImageFit, applyPostFreshnessStyle, scheduleTimelineSidePanePosition, syncTimelineSidePanePosition, applyTimelineBatchGap, resetTimelineBatchGap, applyTimelineBoundaryReturnDuration, resetTimelineBoundaryReturnDuration, scheduleBackToTop, syncBackToTopNow, timelineContentContextObstacles, syncBackToTopPosition, syncBackToTopBoundaryPosition, releaseBackToTopBoundary, returnToTop, refreshTimeline, orderTimelineForRevealedBatch, handleFinishLineAction, revealPreparedBatch, routeAIDetectedItems, renderTimeline, renderTimelineNow, renderTimelineActions, observeTimelineItem, scheduleAIDeepRefresh, timelineInteractionActive, queueBackgroundTimelineRefresh, flushBackgroundTimelineRefresh, renderTimelineSidePane, timelineSidePaneAvailable, syncTimelineSidePaneVisibility, openTimelineSidePane, closeTimelineSidePane, populateTimelineSidePane
} = createTimelineRendering({
  $: (...args) => $(...args),
  AI_DEEP_POLL_INTERVAL_MS,
  BACK_TO_TOP_THRESHOLD_PX,
  DEFAULT_TIMELINE_BATCH_GAP_PX,
  DEFAULT_TIMELINE_BOUNDARY_RETURN_MS,
  api,
  buildTimelineItem: (...args) => buildTimelineItem(...args),
  closeTimelineContentContextDrawer: (...args) => closeTimelineContentContextDrawer(...args),
  formatDate,
  measureTimelineContentContextTabs: (...args) => measureTimelineContentContextTabs(...args),
  renderAutoUpdateStatus: (...args) => renderAutoUpdateStatus(...args),
  schedulePassiveMediaEnrichment: (...args) => schedulePassiveMediaEnrichment(...args),
  scheduleTimelineContentContextPosition: (...args) => scheduleTimelineContentContextPosition(...args),
  getScrollPerformanceTrace: () => scrollPerformanceTrace,
  getScrollUIFrames: () => scrollUIFrames,
  showError,
  showNotice,
  sourceIdentity,
  startVisibleUpdate: (...args) => startVisibleUpdate(...args),
  state,
  stopObservingInlineVideoVisibilityWithin: (...args) => stopObservingInlineVideoVisibilityWithin(...args),
  syncRunButtons: (...args) => syncRunButtons(...args),
  syncTimelineContentContextTabs: (...args) => syncTimelineContentContextTabs(...args),
  getTimelineScrollIdle: () => timelineScrollIdle,
});
const {
  showOnboarding, updateOnboardingSummary, renderActiveReasoningProvider, sourceReadyForOnboarding, onboardingReadinessGate, saveOnboarding, completeOnboarding, shouldShowOnboardingLearning, syncOnboardingLearning, moveOnboardingLearning, showOnboardingLearningSlide, renderOnboardingLearningSlide, pauseOnboardingLearning, toggleOnboardingLearningPlayback, syncOnboardingLearningTimer, restartOnboardingLearningTimer, stopOnboardingLearningTimer, startPendingFirstCalibration, showCalibration, renderCalibration, buildCalibrationCard, showPreviousCalibrationSample, decideCalibration, renderSourceControls
} = createOnboardingUi({
  $: (...args) => $(...args),
  ONBOARDING_LEARNING_INTERVAL_MS,
  api,
  bridgeSourceReadiness: (...args) => bridgeSourceReadiness(...args),
  buildSourceCard: (...args) => buildSourceCard(...args),
  clearNotice,
  configureNativePostLink: (...args) => configureNativePostLink(...args),
  firstRunCalibrationPending: (...args) => firstRunCalibrationPending(...args),
  hideFailure: (...args) => hideFailure(...args),
  humanize,
  onboardingRequiresSetup,
  openOnboardingProviderDialog: (...args) => openOnboardingProviderDialog(...args),
  openSourceFromSettings: (...args) => openSourceFromSettings(...args),
  refreshTimeline: (...args) => refreshTimeline(...args),
  renderBrowserConnection: (...args) => renderBrowserConnection(...args),
  renderOnboardingSourceReadiness: (...args) => renderOnboardingSourceReadiness(...args),
  renderSettings: (...args) => renderSettings(...args),
  renderSourceSessionReadiness: (...args) => renderSourceSessionReadiness(...args),
  requestSourceSessionReadiness: (...args) => requestSourceSessionReadiness(...args),
  safeSourceUrl,
  scheduleBackToTop: (...args) => scheduleBackToTop(...args),
  setPill,
  setView,
  showCalibrationRetry: (...args) => showCalibrationRetry(...args),
  showError,
  sourceDescriptors,
  sourceLabel,
  startVisibleUpdate: (...args) => startVisibleUpdate(...args),
  state,
  syncTimelineSidePaneVisibility: (...args) => syncTimelineSidePaneVisibility(...args),
  terminalStatuses,
});
const {
  onboardingProviderEntries, renderOnboardingProviderOptions, refreshReasoningProviderReadiness, syncProviderReadinessFeedback, openOnboardingProviderDialog, openSettingsProviderDialog, recheckOnboardingProviders, saveOnboardingProviderCredential, confirmOnboardingProvider, activateSettingsReasoningProvider, skipOnboardingProvider
} = createProviderSetupUi({
  $: (...args) => $(...args),
  getONBOARDING_PROVIDER_COPY: () => ONBOARDING_PROVIDER_COPY,
  api,
  completeOnboarding: (...args) => completeOnboarding(...args),
  renderActiveReasoningProvider: (...args) => renderActiveReasoningProvider(...args),
  renderReasoningProcesses: (...args) => renderReasoningProcesses(...args),
  renderReasoningProviderControl: (...args) => renderReasoningProviderControl(...args),
  getSettingsDirty: () => settingsDirty,
  state,
});
const {
  revokeSourceAccessViaBridge, openResetDialog, closeResetDialog, syncResetConfirmation, submitReset
} = createSettingsOperations({
  $: (...args) => $(...args),
  AI_HIDE_CONFIRMATION_PHRASE,
  getREVOKE_SOURCE_ACCESS_TIMEOUT_MS: () => REVOKE_SOURCE_ACCESS_TIMEOUT_MS,
  api,
  persistSettings: (...args) => persistSettings(...args),
  refreshSettingsDirtyState: (...args) => refreshSettingsDirtyState(...args),
  state,
  syncAIDetectionSettings: (...args) => syncAIDetectionSettings(...args),
});
const {
  startScrollPerformanceTrace, saveScrollPerformanceTrace, downloadScrollPerformanceTrace, exportDiagnostics
} = createDiagnosticsUi({
  $: (...args) => $(...args),
  api,
  getScrollPerformanceTrace: () => scrollPerformanceTrace,
  setView,
});
const {
  handleTimelinePrimaryAction, startVisibleUpdate, cancelSession, startPolling, stopPolling, pollSession, releaseCaptureSurface, releaseCaptureSurfaceOnce, dispatch, renderSession, updatePolicyBadge, describeSessionProgress, renderNativeReaderNotice, closeNativeReadersAndResume, syncRunButtons, renderTimelineCalibrationProgress, preparedBatchDisabledReason, runDisabledReason, bridgeUnavailableReason, sourceAccessNeedsAttention, bridgeSourceReadiness, openSourceAccessSettings, showSessionFailure, isWindowsSecurityRuntimeFailure, matchingWindowsPortableBundleURL, firstRunCalibrationPending, showCalibrationRetry, showSessionOutcome, hideFailure
} = createSessionUi({
  $: (...args) => $(...args),
  CAPTURE_CLEANUP_PENDING_DETAIL,
  CAPTURE_CLEANUP_PENDING_MESSAGE,
  api,
  clearNotice,
  defaultIntent,
  drainPlaybackRecovery: (...args) => drainPlaybackRecovery(...args),
  endpoint,
  humanize,
  getNativePostOpening: () => nativePostOpening,
  pollCollectionRuntime: (...args) => pollCollectionRuntime(...args),
  refreshTimeline: (...args) => refreshTimeline(...args),
  renderAutoUpdateStatus: (...args) => renderAutoUpdateStatus(...args),
  requestSourceSessionReadiness: (...args) => requestSourceSessionReadiness(...args),
  retryBootstrap: (...args) => retryBootstrap(...args),
  schedulePassiveMediaEnrichment: (...args) => schedulePassiveMediaEnrichment(...args),
  setNoticeText,
  setPill,
  setView,
  shouldShowOnboardingLearning: (...args) => shouldShowOnboardingLearning(...args),
  showCalibration: (...args) => showCalibration(...args),
  showError,
  showNotice,
  sourceLabel,
  startPendingFirstCalibration: (...args) => startPendingFirstCalibration(...args),
  state,
  syncNativePostLinks: (...args) => syncNativePostLinks(...args),
  syncOnboardingLearning: (...args) => syncOnboardingLearning(...args),
  terminalStatuses,
});
const {
  schedulePassiveMediaEnrichment, passiveMediaCandidates, enrichPassiveXMedia, lookupPassiveXMediaEvidence, xMediaCandidateId
} = createPassiveMediaUi({
  PASSIVE_MEDIA_LOOKUP_COOLDOWN_MS,
  PASSIVE_MEDIA_LOOKUP_TIMEOUT_MS,
  bridgeApi,
  endpoint,
  queueBackgroundTimelineRefresh: (...args) => queueBackgroundTimelineRefresh(...args),
  sourceDescriptor,
  state,
});
const {
  buildTimelineItem, buildCollapsedDuplicate, buildExpandedTimelineItem, buildBrief, buildSourceCard, applyPostFreshness, refreshVisiblePostFreshness, buildAvatar, buildQuotedPost, buildExpandableText, buildAttachments, buildEngagement
} = createTimelineContent({
  QUOTE_TEXT_COLLAPSE_CHARACTERS,
  QUOTE_TEXT_COLLAPSE_LINES,
  SOURCE_TEXT_COLLAPSE_CHARACTERS,
  SOURCE_TEXT_COLLAPSE_LINES,
  buildAIDetectionControls: (...args) => buildAIDetectionControls(...args),
  buildActions: (...args) => buildActions(...args),
  buildForegroundRecaptureOffer: (...args) => buildForegroundRecaptureOffer(...args),
  buildMedia: (...args) => buildMedia(...args),
  buildMediaRecaptureButton: (...args) => buildMediaRecaptureButton(...args),
  buildSourceIcon,
  buildTimelineContentContextAnchor: (...args) => buildTimelineContentContextAnchor(...args),
  closeTimelineContentContextDrawer: (...args) => closeTimelineContentContextDrawer(...args),
  formatDate,
  humanize,
  initials,
  mediaRecaptureReasonForEntry: (...args) => mediaRecaptureReasonForEntry(...args),
  getPostScreenshotUI: () => postScreenshotUI,
  safeMediaUrl,
  safeSourceUrl,
  sourceDescriptor,
  sourceIdentity,
  sourceLabel,
  state,
  syncTimelineContentContextTabs: (...args) => syncTimelineContentContextTabs(...args),
});
const {
  buildAIDetectionControls, applyAIFeedback, loadAIFeedbackHistory
} = createTimelineAiUi({
  api,
  formatDate,
  humanize,
  refreshTimeline: (...args) => refreshTimeline(...args),
  showError,
  sourceLabel,
  state,
});
const {
  buildMedia, buildTimelineMediaCarousel, buildImageMediaControl, buildVideoMedia, buildVideoPosterControl, activateInlineVideo, queueInlinePlaybackRecovery, playbackRecoveryReady, drainPlaybackRecovery, pauseOtherInlineVideos, observeInlineVideoVisibility, stopObservingInlineVideoVisibility, stopObservingInlineVideoVisibilityWithin, openMedia, moveMedia, renderMedia, mediaZoomMetrics, applyMediaZoom, syncMediaPanAffordance, mediaPanStartedOnScrollbar, beginMediaPan, moveMediaPan, endMediaPan, syncMediaZoomPresetControl, snapMediaZoom, adjustMediaZoom
} = createTimelineMedia({
  $: (...args) => $(...args),
  configureNativePostLink: (...args) => configureNativePostLink(...args),
  getPlaybackRecovery: () => playbackRecovery,
  recordUIActivity: (...args) => recordUIActivity(...args),
  safeMediaUrl,
  safePlaybackUrl,
  safeSourceUrl,
  showNotice,
  sourceDescriptor,
  sourceLabel,
  state,
});
const {
  buildActions, buildTimelineReadLaterAction, buildSemanticCorrectionActions, showEventSuggestions, applySemanticCorrection, showCorrectionNotice, buildSourceLink, mediaRecaptureReasonForEntry, buildMediaRecaptureButton, buildForegroundRecaptureOffer, recaptureMedia, dispatchMediaRecapture
} = createTimelineActions({
  $: (...args) => $(...args),
  api,
  clearNotice,
  configureNativePostLink: (...args) => configureNativePostLink(...args),
  createNoticeDismissButton,
  endpoint,
  humanize,
  invalidateLibraryStorage: (...args) => invalidateLibraryStorage(...args),
  originalPostUrl,
  refreshTimeline: (...args) => refreshTimeline(...args),
  safePlaybackUrl,
  safeSourceUrl,
  setNoticeText,
  showError,
  sourceDescriptor,
  state,
  syncRunButtons: (...args) => syncRunButtons(...args),
  getTimelineFeedbackUI: () => timelineFeedbackUI,
});
const {
  timelineContentContextAnchor, timelineContentContextTrigger, measureTimelineContentContextTabs, syncTimelineContentContextTabs, timelineContentContextFeedbackKey, submitTimelineContentContextFeedback, renderTimelineContentContextMatch, renderTimelineContentContextTopicInsight, renderDirectContext, renderTimelineContentContextDrawer, contentContextDrawerMotionDuration, revealTimelineContentContextDrawer, closeTimelineContentContextDrawer, timelineContentContextFocusIsInsideDrawer, openTimelineContentContext, toggleTimelineContentContext, observeTimelineContentContextAnchor, syncTimelineContentContextExpandAction, syncTimelineContentContextPosition, scheduleTimelineContentContextPosition, handleTimelineContentContextScroll, buildTimelineContentContextAnchor
} = createTimelineContext({
  $: (...args) => $(...args),
  api,
  configureNativePostLink: (...args) => configureNativePostLink(...args),
  formatDate,
  safeSourceUrl,
  scheduleBackToTop: (...args) => scheduleBackToTop(...args),
  showError,
  sourceLabel,
  state,
  syncBackToTopNow: (...args) => syncBackToTopNow(...args),
  getTimelineContentContextAnchorResizeObserver: () => timelineContentContextAnchorResizeObserver,
});
// Frontend composition bindings: end
const settingsDirty = createDirtyStateTracker({
  readSnapshot: () => readSettingsDraft(state.bootstrap?.settings ?? {}),
  onChange: renderSettingsDirtyState,
});
const $ = (selector) => document.querySelector(selector);
const xDailyControls = createXDailyControls({
  panel: $("#x-daily-controls"),
  status: $("#x-daily-status"),
  detail: $("#x-daily-detail"),
  consent: $("#x-daily-consent"),
  startButton: $("#x-daily-start"),
  stopButton: $("#x-daily-stop"),
  feedback: $("#x-daily-feedback"),
  api: (path, options) => api(path, options),
  readContext: () => ({
    epoch: state.bootstrap?.instanceEpoch,
    development: state.bootstrap?.deployment?.mode === "development",
  }),
  onStatus: (status) => { if (state.bootstrap) state.bootstrap.xCapture = status; },
  notices: ["timeline", "inbox"].map((surface) => ({
    panel: $(`#x-daily-${surface}-notice`),
    title: $(`#x-daily-${surface}-notice-title`),
    detail: $(`#x-daily-${surface}-notice-detail`),
    button: $(`#x-daily-${surface}-notice-settings`),
  })),
  readActiveSources: () => state.bootstrap?.settings?.activeSources ?? [],
  onReviewSettings: () => {
    setView("settings");
    $("#x-daily-controls")?.scrollIntoView({ block: "center" });
  },
});
const instagramTrialControls = createInstagramTrialControls({
  panel: $("#instagram-trial-controls"), status: $("#instagram-trial-status"), detail: $("#instagram-trial-detail"),
  consent: $("#instagram-trial-consent"), startButton: $("#instagram-trial-start"), stopButton: $("#instagram-trial-stop"),
  feedback: $("#instagram-trial-feedback"), api: (path, options) => api(path, options),
  readContext: () => ({epoch: state.bootstrap?.instanceEpoch, development: state.bootstrap?.deployment?.mode === "development"}),
  onStatus: status => { if (state.bootstrap) state.bootstrap.instagramCapture = status; },
  readActiveSources: () => state.bootstrap?.settings?.activeSources ?? [],
  notices: ["timeline", "inbox"].map(surface => ({panel: $(`#instagram-trial-${surface}-notice`),
    title: $(`#instagram-trial-${surface}-notice-title`), detail: $(`#instagram-trial-${surface}-notice-detail`),
    button: $(`#instagram-trial-${surface}-notice-settings`)})),
  onReviewSettings: () => { setView("settings"); $("#instagram-trial-controls")?.scrollIntoView({block: "center"}); },
});
const postScreenshotUI = createPostScreenshotUI({
  request: (path, options) => api(path, options),
  getOriginalUrl: (entry) => originalPostUrl(entry),
});
const timelineFeedbackUI = createTimelineFeedbackUI({
  api, feedbackButton, invalidateLibraryStorage, showError, logInteractionTrace,
  document, crypto, performance,
});
const timelineScrollIdle = createScrollIdleGate();
const scrollPerformanceTrace = createScrollPerformanceTrace({ onStop: saveScrollPerformanceTrace });
const scrollUIFrames = createFrameTaskQueue({
  requestFrame: (callback) => window.requestAnimationFrame(callback),
  run(tasks) {
    scrollPerformanceTrace.measure("scroll_frame", () => {
      if (tasks.has("scroll")) {
        const movingDown = window.scrollY > state.autoLoadLastScrollY;
        state.autoLoadLastScrollY = window.scrollY;
        if (state.currentView === "timeline") {
          scrollPerformanceTrace.measure("content_context_scroll", handleTimelineContentContextScroll);
          if (movingDown && state.bootstrap?.settings?.nextBatchBehavior === "auto_at_finish") {
            const finish = $("#finish-line");
            if (finish && !finish.classList.contains("hidden") && finish.getBoundingClientRect().top <= window.innerHeight) revealPreparedBatch("continue");
          }
        }
      }
      if (tasks.has("scroll") || tasks.has("back-to-top")) scrollPerformanceTrace.measure("back_to_top", syncBackToTopNow);
      if (tasks.has("scroll") || tasks.has("side-pane")) scrollPerformanceTrace.measure("side_pane", syncTimelineSidePanePosition);
    });
  },
});

window.addEventListener("message", (event) => {
  if (event.source !== window || event.origin !== endpoint || !event.data) return;
  if (event.data.type === "AKU_BROWSER_BRIDGE_READY") {
    const capabilities = {
      ...(event.data.capabilities ?? {}),
      extensionOrigin: event.data.extensionOrigin || event.data.capabilities?.extensionOrigin || "",
    };
    bridgeApi("/api/bridge/heartbeat", {
      method: "POST",
      body: { capabilities },
    }).then((response) => {
      const { bridge } = response;
      sessionStorage.removeItem(BRIDGE_CONTEXT_RECOVERY_KEY);
      sessionStorage.removeItem(BRIDGE_TOKEN_RECOVERY_KEY);
      renderBridge(bridge);
      requestSourceSessionReadiness();
    }).catch(showError);
  }
  if (event.data.type === "AKU_BROWSER_BRIDGE_ERROR") {
    if (recoverInvalidatedBridgeContext(event.data.message)) return;
    showError(new Error(event.data.message));
  }
  if (event.data.type === "AKU_BROWSER_PASSIVE_PROBE_SKIPPED" && event.data.operation === "probe_source_sessions") {
    state.sourceSessionProbeInFlight = false;
    clearTimeout(state.sourceSessionProbeTimer);
    state.sourceSessionProbeTimer = null;
    // Preserve the last observation; Headless capture owns current readiness.
    renderSourceSessionReadiness();
    updateOnboardingSummary();
  }
  if (event.data.type === "AKU_BROWSER_SOURCE_SESSIONS_RESULT") {
    state.sourceSessionReadiness = event.data.sessions && typeof event.data.sessions === "object"
      ? event.data.sessions
      : {};
    state.sourceSessionProbeInFlight = false;
    clearTimeout(state.sourceSessionProbeTimer);
    state.sourceSessionProbeTimer = null;
    if ($("#onboarding-error").textContent === SOURCE_SESSION_PROBE_TIMEOUT_MESSAGE) $("#onboarding-error").textContent = "";
    renderSourceSessionReadiness();
    updateOnboardingSummary();
  }
  if (event.data.type === "AKU_BROWSER_SOURCE_SESSIONS_FAILED") {
    state.sourceSessionProbeInFlight = false;
    clearTimeout(state.sourceSessionProbeTimer);
    state.sourceSessionProbeTimer = null;
    if ($("#onboarding-error").textContent === SOURCE_SESSION_PROBE_TIMEOUT_MESSAGE) $("#onboarding-error").textContent = "";
    renderSourceSessionError(event.data.message);
    updateOnboardingSummary();
  }
  if (event.data.type === "AKU_BROWSER_SOURCE_OPENED") {
    const source = sourceDescriptor(event.data.source);
    if (source) setSourceSessionStatus(source.id, {
      source: source.id,
      state: "loading",
      observedAt: new Date().toISOString(),
      tabCount: 1,
    });
    renderSourceSessionReadiness();
    updateOnboardingSummary();
    window.setTimeout(() => requestSourceSessionReadiness(), 2_000);
  }
  if (event.data.type === "AKU_BROWSER_SOURCE_PERMISSION_REQUIRED") {
    const source = sourceDescriptor(event.data.source);
    if (source) setSourceSessionStatus(source.id, {
      source: source.id,
      state: "permission_required",
      observedAt: new Date().toISOString(),
      tabCount: 1,
      detail: `Grant ${source.label} access in the permission tab, then AkuBrowser will continue to the source.`,
    });
    renderSourceSessionReadiness();
    updateOnboardingSummary();
  }
  if (event.data.type === "AKU_BROWSER_SOURCE_OPEN_FAILED") {
    renderSourceSessionError(event.data.message, event.data.source);
  }
  if (event.data.type === "AKU_BROWSER_DISPATCH_FAILED") {
    const runId = String(event.data.runId || "");
    if (recoverInvalidatedBridgeContext(event.data.message)) return;
    const expectedLaneWait = /No queued browser command was available/i.test(event.data.message || "");
    if (state.dispatchKey?.startsWith(`${runId}:`)) state.dispatchKey = null;
    state.dispatchRetryAfter.set(runId, Date.now() + (expectedLaneWait ? 1500 : 500));
    if (!expectedLaneWait) {
      console.warn("AkuBridge dispatch stopped; waiting for the authoritative Sidecar run outcome.", {
        runId,
        message: event.data.message,
      });
    }
  }
});

// Frontend feature: capture-ui.js#recoverInvalidatedBridgeContext

// Frontend feature: capture-ui.js#recoverInvalidBridgeToken

$("#session-view-button").addEventListener("click", () => setView("timeline"));
$("#library-view-button").addEventListener("click", () => setView("library"));
$("#topics-view-button").addEventListener("click", () => setView("topics"));
$("#inbox-view-button").addEventListener("click", () => setView("inbox"));
$("#library-saved-tab").addEventListener("click", () => setLibraryTab(LIBRARY_TAB_SAVED));
$("#library-library-tab")?.addEventListener("click", () => setLibraryTab(LIBRARY_TAB_LIBRARY));
$("#library-cleaning-tab").addEventListener("click", () => setLibraryTab(LIBRARY_TAB_CLEANING));
for (const tab of document.querySelectorAll("#library-tabs [role=\"tab\"]")) {
  tab.addEventListener("keydown", handleLibraryTabKeydown);
}
$("#living-topic-snapshot-tab").addEventListener("click", () => setLivingTopicDetailTab(LIVING_TOPIC_TAB_SNAPSHOT));
$("#living-topic-evidence-tab").addEventListener("click", () => setLivingTopicDetailTab(LIVING_TOPIC_TAB_EVIDENCE));
for (const tab of document.querySelectorAll("#living-topic-detail-tabs [role=\"tab\"]")) {
  tab.addEventListener("keydown", handleLivingTopicDetailTabKeydown);
}
$("#settings-view-button").addEventListener("click", () => {
  setView("settings");
  requestSourceSessionReadiness();
  void refreshReasoningProviderReadiness();
});
$("#inbox-refresh-button").addEventListener("click", loadInbox);
$("#library-search-form").addEventListener("submit", submitLibrarySearch);
$("#library-filters-toggle").addEventListener("click", toggleLibraryFilters);
for (const field of [$("#library-source"), $("#library-tier"), $("#library-published-from"), $("#library-published-to")]) {
  field?.addEventListener("change", syncLibraryFilterToggle);
}
$("#library-clear-button").addEventListener("click", clearLibrarySearch);
$("#library-refresh-button").addEventListener("click", refreshLibrary);
$("#library-load-more").addEventListener("click", loadMoreLibrary);
$("#library-detail-close").addEventListener("click", closeLibraryDetail);
$("#topics-refresh-button").addEventListener("click", () => loadLivingTopics(true));
$("#living-topic-create-form").addEventListener("submit", createLivingTopic);
$("#living-topic-rename-form").addEventListener("submit", renameLivingTopic);
$("#living-topic-evidence-search-form").addEventListener("submit", searchLivingTopicEvidence);
$("#living-topic-snapshot-form").addEventListener("submit", createLivingTopicSnapshot);
$("#living-topic-activation-button").addEventListener("click", requestLivingTopicActivation);
$("#model-usage-back").addEventListener("click", () => setInboxSubView("checks"));
$("#model-usage-refresh").addEventListener("click", loadAggregateModelUsage);
$("#model-usage-window").addEventListener("change", loadAggregateModelUsage);
$("#timeline-runner-button").addEventListener("click", handleTimelinePrimaryAction);
$("#source-access-setup-button").addEventListener("click", openSourceAccessSettings);
$("#native-reader-close-resume").addEventListener("click", closeNativeReadersAndResume);
$("#timeline-calibration-continue").addEventListener("click", () => {
  const calibration = state.bootstrap?.calibration?.active;
  if (calibration) showCalibration(calibration);
});
$("#timeline-prepared-button").addEventListener("click", () => revealPreparedBatch("latest"));
$("#done-button").addEventListener("click", handleFinishLineAction);
$("#retry-button").addEventListener("click", () => {
  const action = $("#retry-button").dataset.action;
  hideFailure();
  setView("timeline");
  if (action === "retry-update") startVisibleUpdate();
});
$("#cancel-button").addEventListener("click", cancelSession);
$("#processing-inbox-button").addEventListener("click", () => setView("inbox"));
$("#processing-settings-button").addEventListener("click", () => setView("settings"));
$("#auto-update-timeline-settings").addEventListener("click", handleAutoUpdateTimelineAction);
$("#reset-auto-update-budget").addEventListener("click", resetAutoUpdateBudget);
$("#confirm-codex-usage-restored").addEventListener("click", confirmCodexUsageRestored);
$("#confirm-x-capture-recovery").addEventListener("click", confirmXCaptureRecovery);
$("#prepare-batch-now").addEventListener("click", prepareBatchNow);
$("#onboarding-learning-previous").addEventListener("click", () => moveOnboardingLearning(-1, true));
$("#onboarding-learning-next").addEventListener("click", () => moveOnboardingLearning(1, true));
$("#onboarding-learning-toggle").addEventListener("click", toggleOnboardingLearningPlayback);
for (const button of document.querySelectorAll("[data-onboarding-slide]")) {
  button.addEventListener("click", () => showOnboardingLearningSlide(Number(button.dataset.onboardingSlide), true));
}
$("#onboarding-learning-panel").addEventListener("pointerenter", () => pauseOnboardingLearning(true));
$("#onboarding-learning-panel").addEventListener("pointerleave", () => pauseOnboardingLearning(false));
$("#onboarding-learning-panel").addEventListener("focusin", () => pauseOnboardingLearning(true));
$("#onboarding-learning-panel").addEventListener("focusout", () => {
  setTimeout(() => pauseOnboardingLearning($("#onboarding-learning-panel").contains(document.activeElement)), 0);
});
const settingsForm = $("#runtime-settings-form");
settingsForm.addEventListener("submit", saveSettings);
settingsForm.addEventListener("input", refreshSettingsDirtyState);
settingsForm.addEventListener("change", refreshSettingsDirtyState);
$("#collection-mode").addEventListener("change", renderCollectionRuntime);
settingsForm.addEventListener("click", (event) => {
  if (event.target.closest("button")) queueMicrotask(refreshSettingsDirtyState);
});
$("#detect-reasoning-executable").addEventListener("click", detectReasoningExecutable);
$("#reasoning-provider").addEventListener("change", beginSettingsProviderSelection);
$("#bounded-load-profile").addEventListener("change", () => syncLoadProfileSettings(true));
$("#semantic-event-mode").addEventListener("change", syncSemanticEventSettings);
$("#ai-detection-enabled").addEventListener("change", syncAIDetectionSettings);
$("#ai-detection-presentation").addEventListener("change", syncAIDetectionSettings);
$("#auto-update-mode").addEventListener("change", syncAutoUpdateModeSettings);
$("#reset-semantic-event-merge-threshold").addEventListener("click", resetSemanticEventMergeThreshold);
$("#stream-width").addEventListener("change", () => applyStreamWidth($("#stream-width").value));
$("#single-image-fit").addEventListener("change", () => applySingleImageFit($("#single-image-fit").value));
$("#post-freshness-style").addEventListener("change", () => applyPostFreshnessStyle($("#post-freshness-style").value));
$("#timeline-batch-gap").addEventListener("input", () => applyTimelineBatchGap($("#timeline-batch-gap").value));
$("#reset-timeline-batch-gap").addEventListener("click", resetTimelineBatchGap);
$("#timeline-boundary-return-ms").addEventListener("input", () => applyTimelineBoundaryReturnDuration($("#timeline-boundary-return-ms").value));
$("#reset-timeline-boundary-return").addEventListener("click", resetTimelineBoundaryReturnDuration);
$("#edit-onboarding-profile").addEventListener("click", () => showOnboarding(true));
$("#open-chrome-extensions").addEventListener("click", openChromeExtensions);
$("#bridge-reload").addEventListener("click", reloadIncompatibleBridge);
$("#onboarding-form").addEventListener("submit", saveOnboarding);
$("#onboarding-cancel").addEventListener("click", () => setView("settings"));
$("#calibration-previous").addEventListener("click", showPreviousCalibrationSample);
$("#calibration-less").addEventListener("click", () => decideCalibration({ label: "less_like_this" }));
$("#calibration-neutral").addEventListener("click", () => decideCalibration({ label: "neutral" }));
$("#calibration-more").addEventListener("click", () => decideCalibration({ label: "more_like_this" }));
for (const button of document.querySelectorAll("[data-calibration-issue]")) {
  button.addEventListener("click", () => decideCalibration({ issueCode: button.dataset.calibrationIssue }));
}
$("#open-reset-learning").addEventListener("click", () => openResetDialog("learning"));
$("#open-full-reset").addEventListener("click", () => openResetDialog("full"));
$("#database-maintenance-later").addEventListener("click", () => $("#database-maintenance-dialog").close());
$("#database-maintenance-clean").addEventListener("click", () => cleanDatabase(false));
$("#database-maintenance-backup").addEventListener("click", () => cleanDatabase(true));
$("#database-status").addEventListener("click", () => renderDatabaseHealth(state.bootstrap?.databaseHealth, true));
  $("#export-diagnostics").addEventListener("click", exportDiagnostics);
  $("#start-scroll-trace").addEventListener("click", startScrollPerformanceTrace);
  $("#stop-scroll-trace").addEventListener("click", () => scrollPerformanceTrace.stop("manual"));
  $("#download-scroll-trace").addEventListener("click", downloadScrollPerformanceTrace);
$("#reset-confirmation-cancel").addEventListener("click", closeResetDialog);
$("#reset-confirmation-input").addEventListener("input", syncResetConfirmation);
$("#reset-confirmation-submit").addEventListener("click", submitReset);
$("#onboarding-provider-confirm").addEventListener("click", confirmOnboardingProvider);
$("#onboarding-provider-skip").addEventListener("click", skipOnboardingProvider);
$("#onboarding-provider-recheck").addEventListener("click", recheckOnboardingProviders);
$("#onboarding-provider-save-credential").addEventListener("click", saveOnboardingProviderCredential);
$("#onboarding-provider-dialog").addEventListener("close", () => {
  $("#onboarding-provider-secret").value = "";
  state.pendingOnboardingSources = null;
  renderReasoningProviderControl(state.bootstrap?.settings);
});
$("#timeline-side-pane-toggle").addEventListener("click", openTimelineSidePane);
$("#timeline-side-pane-close").addEventListener("click", closeTimelineSidePane);
$("#timeline-content-context-close").addEventListener("click", () => closeTimelineContentContextDrawer());
$("#timeline-content-context-expand").addEventListener("click", () => {
  state.timelineContentContextExpanded = !state.timelineContentContextExpanded;
  scheduleTimelineContentContextPosition();
});
document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape" || !state.timelineContentContextDrawerOpen) return;
  event.preventDefault();
  closeTimelineContentContextDrawer();
});
$("#back-to-top").addEventListener("click", returnToTop);
$("#media-viewer-zoom").addEventListener("click", () => snapMediaZoom(state.mediaZoomPreset === "fit" ? "zoom" : "fit"));
$("#media-viewer-zoom-out").addEventListener("click", () => adjustMediaZoom(1 / 1.2));
$("#media-viewer-zoom-in").addEventListener("click", () => adjustMediaZoom(1.2));
$("#media-viewer-previous").addEventListener("click", () => moveMedia(-1));
$("#media-viewer-next").addEventListener("click", () => moveMedia(1));
$("#media-viewer-image").addEventListener("load", () => snapMediaZoom(state.mediaZoomPreset));
$("#media-viewer-canvas").addEventListener("wheel", (event) => {
  if (!$("#media-viewer").open) return;
  event.preventDefault();
  const rect = $("#media-viewer-canvas").getBoundingClientRect();
  adjustMediaZoom(event.deltaY < 0 ? 1.12 : 1 / 1.12, { x: event.clientX - rect.left, y: event.clientY - rect.top });
}, { passive: false });
$("#media-viewer-canvas").addEventListener("pointerdown", beginMediaPan);
$("#media-viewer-canvas").addEventListener("pointermove", moveMediaPan);
$("#media-viewer-canvas").addEventListener("pointerup", endMediaPan);
$("#media-viewer-canvas").addEventListener("pointercancel", endMediaPan);
$("#media-viewer").addEventListener("keydown", (event) => {
  if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
    event.preventDefault();
    moveMedia(event.key === "ArrowLeft" ? -1 : 1);
  } else if (event.key === "+" || event.key === "=") {
    event.preventDefault();
    adjustMediaZoom(1.2);
  } else if (event.key === "-" || event.key === "_") {
    event.preventDefault();
    adjustMediaZoom(1 / 1.2);
  } else if (event.key === "0") {
    event.preventDefault();
    snapMediaZoom("fit");
  }
});
window.addEventListener("scroll", () => {
  scrollPerformanceTrace.scroll();
  timelineScrollIdle.noteScroll();
  scrollUIFrames.schedule("scroll");
}, { passive: true });
document.addEventListener("load", event => {
  if (event.target?.tagName === "IMG" && event.target.closest("#result-items")) scrollPerformanceTrace.mediaLoad();
}, true);
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") scrollPerformanceTrace.stop("hidden");
});
window.addEventListener("resize", () => {
  scheduleBackToTop();
  scheduleTimelineSidePanePosition();
  scheduleTimelineContentContextPosition();
  syncTimelineContentContextTabs();
  if ($("#media-viewer").open) {
    if (state.mediaZoomManual) applyMediaZoom(state.mediaZoomFactor);
    else snapMediaZoom(state.mediaZoomPreset);
  }
}, { passive: true });
window.addEventListener("beforeunload", (event) => {
  if (!settingsDirty.isDirty() || state.settingsUnloadBypass) return;
  event.preventDefault();
  event.returnValue = "";
});
const timelineSidePaneLayoutObserver = new ResizeObserver(scheduleTimelineSidePanePosition);
for (const element of [$(".timeline-heading-row"), $("#processing-panel"), $("#result-items")]) {
  if (element) timelineSidePaneLayoutObserver.observe(element);
}
const timelineContentContextAnchorResizeObserver = new ResizeObserver((entries) => {
  if (!state.timelineContentContextDrawerOpen) return;
  if (!entries.some((entry) => entry.target === state.timelineContentContextObservedAnchor)) return;
  scheduleTimelineContentContextPosition();
});
document.addEventListener("visibilitychange", () => {
  syncOnboardingLearningTimer();
  if (document.visibilityState === "visible") {
    schedulePassiveMediaEnrichment();
    recordUIActivity(true);
    refreshBridgeAfterUserReturn();
  }
});
window.addEventListener("focus", refreshBridgeAfterUserReturn);

// Frontend feature: app-lifecycle.js#installUIActivityTracking

// Frontend feature: app-lifecycle.js#recordUIActivity

// Frontend feature: app-lifecycle.js#bootstrap

// Frontend feature: app-lifecycle.js#renderDeployment

// Frontend feature: app-lifecycle.js#pollAutoUpdate

// Frontend feature: app-lifecycle.js#shownDatabaseIssues
// Frontend feature: app-lifecycle.js#databaseCleanupBusy
// Frontend feature: app-lifecycle.js#databaseHealthPoller
// Frontend feature: app-lifecycle.js#databaseHealthPollInFlight

// Frontend feature: app-lifecycle.js#pollDatabaseHealth

// Frontend feature: app-lifecycle.js#renderDatabaseHealth

// Frontend feature: app-lifecycle.js#cleanDatabase

function setView(view) {
  view = normalizePage(view);
  if (state.bootstrapLoading || !state.bootstrap) {
    state.currentView = view;
    renderPageNavigation(document, view, {loading: true});
    return;
  }
  if (onboardingRequiresSetup()) {
    showOnboarding(false);
    return;
  }
  if (state.bootstrap?.calibration?.active) {
    showCalibration(state.bootstrap.calibration.active);
    return;
  }
  state.currentView = view;
  state.onboardingEditing = false;
  const timeline = view === "timeline";
  const library = view === "library";
  const topics = view === "topics";
  const inbox = view === "inbox";
  const settings = view === "settings";
  renderPageNavigation(document, view, {focus: false});
  if (!timeline) {
    closeTimelineSidePane();
    closeTimelineContentContextDrawer({ clearActive: true, focusTrigger: false });
  }
  syncTimelineSidePaneVisibility();
  focusPageHeading(document, view);
  if (library) {
    renderLibraryTabs();
    loadLibrary();
    maybeLoadLibraryStorage();
  }
  if (topics) void loadLivingTopics(true);
  if (settings) void loadSettingsStorage();
  if (inbox) {
    syncInboxSubView();
    if (state.inboxSubView === "usage") loadAggregateModelUsage();
    else loadInbox();
  }
  scheduleBackToTop();
}

// Frontend feature: settings-storage-ui.js#settingsStorageDetail

// Frontend feature: settings-storage-ui.js#renderSettingsStorage

// Frontend feature: settings-storage-ui.js#loadSettingsStorage

function onboardingRequiresSetup() {
  return state.bootstrap?.onboarding?.status === "not_started";
}

// Frontend feature: living-topics-ui.js#loadLivingTopics

// Frontend feature: living-topics-ui.js#refreshLivingTopicNotificationProjection

// Frontend feature: living-topics-ui.js#updateLivingTopicNotificationFromTopics

// Frontend feature: living-topics-ui.js#renderLivingTopicNotificationBadge

// Frontend feature: living-topics-ui.js#selectLivingTopic

// Frontend feature: living-topics-ui.js#loadLivingTopicDetail

// Frontend feature: living-topics-ui.js#createLivingTopic

// Frontend feature: living-topics-ui.js#renameLivingTopic

// Frontend feature: living-topics-ui.js#searchLivingTopicEvidence

// Frontend feature: living-topics-ui.js#loadLivingTopicCandidates

// Frontend feature: living-topics-ui.js#addLivingTopicMember

// Frontend feature: living-topics-ui.js#removeLivingTopicMember

// Frontend feature: living-topics-ui.js#moveLivingTopicMember

// Frontend feature: living-topics-ui.js#undoLivingTopicMemberMove

// Frontend feature: living-topics-ui.js#requestLivingTopicActivation

// Frontend feature: living-topics-ui.js#reviewLivingTopicCandidate

// Frontend feature: living-topics-ui.js#resetLivingTopicSnapshotStatus

// Frontend feature: living-topics-ui.js#createLivingTopicSnapshot

// Frontend feature: living-topics-ui.js#scheduleLivingTopicUnderstandingPoll

// Frontend feature: living-topics-ui.js#scheduleLivingTopicActivationPoll

// Frontend feature: living-topics-ui.js#refreshLivingTopicListProjection

// Frontend feature: living-topics-ui.js#renderLivingTopics

// Frontend feature: living-topics-ui.js#renderLivingTopicDetail

// Frontend feature: living-topics-ui.js#livingTopicUnderstandingSummary

// Frontend feature: living-topics-ui.js#livingTopicRoutingSummary

// Frontend feature: living-topics-ui.js#setLivingTopicDetailTab

// Frontend feature: living-topics-ui.js#renderLivingTopicDetailTabs

// Frontend feature: living-topics-ui.js#handleLivingTopicDetailTabKeydown

// Frontend feature: living-topics-ui.js#buildLivingTopicEvidenceCard

// Frontend feature: living-topics-ui.js#renderLivingTopicMoveStatus

// Frontend feature: living-topics-ui.js#buildLivingTopicSuggestionCard

// Frontend feature: living-topics-ui.js#buildLivingTopicReviewedCandidateCard

// Frontend feature: living-topics-ui.js#renderLivingTopicSnapshots

// Frontend feature: living-topics-ui.js#buildLivingTopicUnderstandingHistory

// Frontend feature: living-topics-ui.js#buildCurrentLivingTopicUnderstanding

// Frontend feature: living-topics-ui.js#buildLivingTopicHistoryEntry

// Frontend feature: living-topics-ui.js#buildLivingTopicStatementList

// Frontend feature: living-topics-ui.js#livingTopicSourceOrigin

// Frontend feature: library-ui.js#setLibraryTab

// Frontend feature: library-ui.js#renderLibraryTabs

// Frontend feature: library-ui.js#maybeLoadLibraryStorage

// Frontend feature: library-ui.js#handleLibraryTabKeydown

// Frontend feature: inbox-ui.js#syncInboxSubView

// Frontend feature: inbox-ui.js#setInboxSubView

// Frontend feature: library-ui.js#renderLibrarySourceOptions

// Frontend feature: library-ui.js#toggleLibraryFilters

// Frontend feature: library-ui.js#syncLibraryFilterToggle

// Frontend feature: library-ui.js#readLibraryFilters

// Frontend feature: library-ui.js#resetLibraryList

// Frontend feature: library-ui.js#resetLibraryStorage

// Frontend feature: library-ui.js#invalidateLibraryStorage

// Frontend feature: library-ui.js#reloadLibraryStorage

// Frontend feature: library-ui.js#submitLibrarySearch

// Frontend feature: library-ui.js#clearLibrarySearch

// Frontend feature: library-ui.js#refreshLibrary

// Frontend feature: library-ui.js#loadMoreLibrary

// Frontend feature: library-ui.js#loadLibrary

// Frontend feature: library-ui.js#loadLibraryStorage

// Frontend feature: library-ui.js#libraryErrorMessage

// Frontend feature: library-ui.js#libraryStorageErrorMessage

// Frontend feature: library-ui.js#libraryMutationErrorMessage

// Frontend feature: library-ui.js#renderLibrary

// Frontend feature: library-ui.js#renderLibraryTopicKnowledge

// Frontend feature: library-ui.js#buildLibraryTopicKnowledgeCard

// Frontend feature: library-ui.js#renderLibraryStorage

// Frontend feature: library-ui.js#buildLibrarySavedPressure

// Frontend feature: library-ui.js#buildLibraryStorageUsage

// Frontend feature: library-ui.js#formatLibraryStorageCount

// Frontend feature: library-ui.js#libraryStorageMetric

// Frontend feature: library-ui.js#buildLibraryStorageRecommendation

// Frontend feature: library-ui.js#reviewLibraryStorageRecommendation

// Frontend feature: library-ui.js#buildLibrarySavedRecommendation

// Frontend feature: library-ui.js#reviewSavedRecommendation

// Frontend feature: library-ui.js#libraryStateMessage

// Frontend feature: library-ui.js#buildLibraryCard

// Frontend feature: library-ui.js#libraryMetaChip

// Frontend feature: library-ui.js#selectLibraryItem

// Frontend feature: library-ui.js#closeLibraryDetail

// Frontend feature: library-ui.js#renderLibraryDetail

// Frontend feature: library-ui.js#buildLibraryDetail

// Frontend feature: library-ui.js#removeLibraryItem

// Frontend feature: library-ui.js#releaseLibraryItem

// Frontend feature: library-ui.js#forgetLibraryItem

// Frontend feature: library-ui.js#keepSavedItem

// Frontend feature: library-ui.js#doneSavedItem

// Frontend feature: library-ui.js#mutateLibraryItem

// Frontend feature: library-ui.js#appendLibraryDetailField

// Frontend feature: library-ui.js#appendLibraryLabels

// Frontend feature: library-ui.js#appendLibraryMediaMetadata

// Frontend feature: capture-ui.js#pingBridge

// Frontend feature: capture-ui.js#lastBridgeReturnRefreshAt
// Frontend feature: capture-ui.js#refreshBridgeAfterUserReturn

// Frontend feature: capture-ui.js#requestSourceSessionReadiness

// Frontend feature: capture-ui.js#openSourceFromSettings

const nativePointerTraces = new WeakMap();
let nativePostOpening = false;
// Frontend feature: capture-ui.js#syncNativePostLinks
document.addEventListener("pointerdown", (event) => {
  if (!event.isTrusted || event.button !== 0) return;
  const link = event.target?.closest?.("a[data-aku-native-post]");
  if (!link) return;
  syncNativePostAvailability(link, nativePostWaitReason(state.bootstrap?.collectionRuntime, state.session, nativePostOpening || state.closingNativeReaders, link.dataset.akuNativePost));
  if (link.dataset.akuNativeWait) return;
  const pointerTrace = `pointer_${crypto.randomUUID().replaceAll("-", "")}`;
  nativePointerTraces.set(link, { trace: pointerTrace, at: Date.now() });
  logNativePostTrace(pointerTrace, "pointerdown");
}, true);

// Frontend feature: capture-ui.js#configureNativePostLink

const routeNativePost = createNativePostRouter({
  readRuntime: async () => {
    const response = await api("/api/collection/runtime");
    return response.collectionRuntime;
  },
  openHeadless: (request) => {
    // The typed reader action borrows the exclusive profile on the server.
    // Keep the original trusted-click broker alive through that handoff.
    window.postMessage(request, endpoint);
  },
  openForeground: (request, runtime) => {
    if (!runtime.available && !state.bootstrap?.bridge?.compatible) {
      throw new Error("AkuBridge is not ready to open this native post.");
    }
    window.postMessage(request, endpoint);
  },
});

// Frontend feature: capture-ui.js#openNativePostInReaderWindow

// Frontend feature: capture-ui.js#cancelNativePostBroker

// Frontend feature: capture-ui.js#setSourceSessionStatus

// Frontend feature: capture-ui.js#renderSourceSessionError

// Frontend feature: capture-ui.js#renderSourceSessionReadiness

// Frontend feature: capture-ui.js#renderOnboardingSourceReadiness

// Frontend feature: capture-ui.js#bridgeActionLoop

// Frontend feature: capture-ui.js#renderBridge

// Frontend feature: capture-ui.js#configureBackgroundBridge

// Frontend feature: capture-ui.js#renderCollectionRuntime
// Frontend feature: app-lifecycle.js#pollCollectionRuntime
// Frontend feature: settings-feature.js#renderSettings

// Frontend feature: auto-update-ui.js#renderAutoUpdateStatus

// Frontend feature: auto-update-ui.js#autoUpdateModeLabel

// Frontend feature: auto-update-ui.js#autoUpdateModeDescription

// Frontend feature: auto-update-ui.js#syncAutoUpdateModeSettings

// Frontend feature: auto-update-ui.js#prepareBatchNow

// Frontend feature: auto-update-ui.js#openAutoUpdateSettings

// Frontend feature: settings-feature.js#syncReasoningProviderSelection

// Frontend feature: settings-feature.js#beginSettingsProviderSelection

// Frontend feature: auto-update-ui.js#handleAutoUpdateTimelineAction

// Frontend feature: auto-update-ui.js#resetAutoUpdateBudget

// Frontend feature: auto-update-ui.js#confirmXCaptureRecovery

// Frontend feature: auto-update-ui.js#confirmCodexUsageRestored

// Frontend feature: settings-feature.js#renderSourceSettingsValues

// Frontend feature: settings-feature.js#renderReasoningProcesses

// Frontend feature: settings-feature.js#formatReasoningProvider

// Frontend feature: settings-feature.js#formatReasoningModel

// Frontend feature: settings-feature.js#formatReasoningEffort

// Frontend feature: settings-feature.js#readSettingsDraft

// Frontend feature: settings-feature.js#refreshSettingsDirtyState

// Frontend feature: settings-feature.js#renderSettingsDirtyState

// Frontend feature: settings-feature.js#saveSettings

// Frontend feature: settings-feature.js#reasoningProfileValue

// Frontend feature: settings-feature.js#persistSettings

// Frontend feature: settings-feature.js#renderReasoningProviderControl

// Frontend feature: settings-feature.js#detectReasoningExecutable

// Frontend feature: settings-feature.js#syncLoadProfileSettings

// Frontend feature: settings-feature.js#syncSemanticEventSettings

// Frontend feature: settings-feature.js#syncAIDetectionSettings

// Frontend feature: settings-feature.js#resetSemanticEventMergeThreshold

// Frontend feature: timeline-rendering.js#applyStreamWidth

// Frontend feature: timeline-rendering.js#applySingleImageFit

// Frontend feature: timeline-rendering.js#applyPostFreshnessStyle

// Frontend feature: timeline-rendering.js#scheduleTimelineSidePanePosition

// Frontend feature: timeline-rendering.js#syncTimelineSidePanePosition

// Frontend feature: timeline-rendering.js#applyTimelineBatchGap

// Frontend feature: timeline-rendering.js#resetTimelineBatchGap

// Frontend feature: timeline-rendering.js#applyTimelineBoundaryReturnDuration

// Frontend feature: timeline-rendering.js#resetTimelineBoundaryReturnDuration

// Frontend feature: onboarding-ui.js#showOnboarding

// Frontend feature: onboarding-ui.js#updateOnboardingSummary

// Frontend feature: onboarding-ui.js#renderActiveReasoningProvider

// Frontend feature: onboarding-ui.js#sourceReadyForOnboarding

// Frontend feature: onboarding-ui.js#onboardingReadinessGate

// Frontend feature: onboarding-ui.js#saveOnboarding

// Frontend feature: onboarding-ui.js#completeOnboarding

const ONBOARDING_PROVIDER_COPY = {
  "codex-app-server": {
    tag: "Default",
    description: "Local Codex App Server — the most compliant and reliable option. Reasoning runs through your signed-in Codex app and stays on its usage boundary.",
    panelLabel: "CODEX APP CONNECTION",
    setup: "AkuBrowser uses the Codex app session on this device.\n\nBefore continuing:\n• Keep the Codex app running\n• Sign in inside Codex\n• No API key is stored by AkuBrowser",
  },
  "gemini-flash-lite": {
    tag: "Google API key",
    description: "Uses a Google AI Studio API key. Free-tier access and limits depend on the selected model and project tier; on the free tier Google may use that data to improve its products.",
    setup: "Get a key in Google AI Studio: sign in with Google, create or select a project, create or copy the API key, then return here. Paste it below and validate it before continuing. Limits vary by model and project tier; check the current quota link below rather than relying on a universal request allowance.",
    ready: "Gemini accepted your API key for the configured model's metadata check, and AkuBrowser stored it in this operating system's secure credential store. Actual generation and current quota are checked when you use the provider. AkuBrowser never displays the saved key again.",
    credentialURL: "https://aistudio.google.com/apikey",
    quotaURL: "https://ai.google.dev/gemini-api/docs/rate-limits",
    pricingURL: "https://ai.google.dev/gemini-api/docs/pricing",
  },
  "ollama-nemotron": {
    tag: "Local",
    description: "Runs Nemotron 3.5 Lightning fully locally through Ollama. Nothing leaves this machine. Keep Ollama running with the model pulled.",
    panelLabel: "LOCAL MODEL SETUP",
    setup: "Run Ollama locally at 127.0.0.1:11434.\n\nPull this model once:\nollama pull nemotron-3.5-lightning\n\nKeep Ollama running while AkuBrowser is active. No API key is stored, and reasoning requests stay on this device.",
  },
  "ollama-qwen": {
    tag: "Local",
    description: "Runs Qwen 3.8 27B fully locally through Ollama. Nothing leaves this machine. Keep Ollama running with the model pulled.",
    panelLabel: "LOCAL MODEL SETUP",
    setup: "Run Ollama locally at 127.0.0.1:11434.\n\nPull this model once:\nollama pull qwen3.8:27b\n\nKeep Ollama running while AkuBrowser is active. No API key is stored, and reasoning requests stay on this device.",
  },
};

// Frontend feature: provider-setup-ui.js#onboardingProviderEntries

// Frontend feature: provider-setup-ui.js#renderOnboardingProviderOptions

// Frontend feature: capture-ui.js#renderBrowserConnection

// Frontend feature: capture-ui.js#openChromeExtensions

// Frontend feature: capture-ui.js#reloadIncompatibleBridge

// Frontend feature: provider-setup-ui.js#refreshReasoningProviderReadiness

// Frontend feature: provider-setup-ui.js#syncProviderReadinessFeedback

// Frontend feature: provider-setup-ui.js#openOnboardingProviderDialog

// Frontend feature: provider-setup-ui.js#openSettingsProviderDialog

// Frontend feature: provider-setup-ui.js#recheckOnboardingProviders

// Frontend feature: provider-setup-ui.js#saveOnboardingProviderCredential

// Frontend feature: provider-setup-ui.js#confirmOnboardingProvider

// Frontend feature: provider-setup-ui.js#activateSettingsReasoningProvider

// Frontend feature: provider-setup-ui.js#skipOnboardingProvider

const REVOKE_SOURCE_ACCESS_TIMEOUT_MS = 5000;

// revokeSourceAccessViaBridge asks the extension to drop every optional
// source host permission and unregister its capture scripts before a full
// reset persists. Full reset fails closed when the Bridge cannot confirm it;
// the browser profile and the Bridge installation are always preserved.
// Frontend feature: settings-operations.js#revokeSourceAccessViaBridge

// Frontend feature: settings-operations.js#openResetDialog

// Frontend feature: settings-operations.js#closeResetDialog

// Frontend feature: diagnostics-ui.js#lastScrollPerformanceReport

// Frontend feature: diagnostics-ui.js#startScrollPerformanceTrace

// Frontend feature: diagnostics-ui.js#saveScrollPerformanceTrace

// Frontend feature: diagnostics-ui.js#downloadScrollPerformanceTrace

// Frontend feature: diagnostics-ui.js#exportDiagnostics

// Frontend feature: settings-operations.js#syncResetConfirmation

// Frontend feature: settings-operations.js#submitReset

// Frontend feature: timeline-rendering.js#scheduleBackToTop

// Frontend feature: timeline-rendering.js#syncBackToTopNow

// Frontend feature: timeline-rendering.js#timelineContentContextObstacles

// Frontend feature: timeline-rendering.js#syncBackToTopPosition

// Frontend feature: timeline-rendering.js#syncBackToTopBoundaryPosition

// Frontend feature: timeline-rendering.js#releaseBackToTopBoundary

// Frontend feature: timeline-rendering.js#returnToTop

// Frontend feature: session-ui.js#handleTimelinePrimaryAction

// Frontend feature: app-lifecycle.js#retryBootstrap

// Frontend feature: session-ui.js#startVisibleUpdate

// Frontend feature: session-ui.js#cancelSession

// Frontend feature: session-ui.js#startPolling

// Frontend feature: session-ui.js#stopPolling

// Frontend feature: session-ui.js#pollSession

// Frontend feature: session-ui.js#releaseCaptureSurface

// Frontend feature: session-ui.js#releaseCaptureSurfaceOnce

// Frontend feature: session-ui.js#dispatch

// Frontend feature: session-ui.js#renderSession

// Frontend feature: session-ui.js#updatePolicyBadge

// Frontend feature: onboarding-ui.js#shouldShowOnboardingLearning

// Frontend feature: onboarding-ui.js#syncOnboardingLearning

// Frontend feature: onboarding-ui.js#moveOnboardingLearning

// Frontend feature: onboarding-ui.js#showOnboardingLearningSlide

// Frontend feature: onboarding-ui.js#renderOnboardingLearningSlide

// Frontend feature: onboarding-ui.js#pauseOnboardingLearning

// Frontend feature: onboarding-ui.js#toggleOnboardingLearningPlayback

// Frontend feature: onboarding-ui.js#syncOnboardingLearningTimer

// Frontend feature: onboarding-ui.js#restartOnboardingLearningTimer

// Frontend feature: onboarding-ui.js#stopOnboardingLearningTimer

// Frontend feature: session-ui.js#describeSessionProgress

// Frontend feature: session-ui.js#renderNativeReaderNotice

// Frontend feature: session-ui.js#closeNativeReadersAndResume

// Frontend feature: session-ui.js#syncRunButtons

// Frontend feature: session-ui.js#renderTimelineCalibrationProgress

// Frontend feature: session-ui.js#preparedBatchDisabledReason

// Frontend feature: session-ui.js#runDisabledReason

// Frontend feature: session-ui.js#bridgeUnavailableReason

// Frontend feature: session-ui.js#sourceAccessNeedsAttention

// Frontend feature: session-ui.js#bridgeSourceReadiness

// Frontend feature: session-ui.js#openSourceAccessSettings

// Frontend feature: onboarding-ui.js#startPendingFirstCalibration

// Frontend feature: onboarding-ui.js#showCalibration

// Frontend feature: onboarding-ui.js#renderCalibration

// Frontend feature: onboarding-ui.js#buildCalibrationCard

// Frontend feature: onboarding-ui.js#showPreviousCalibrationSample

// Frontend feature: onboarding-ui.js#decideCalibration

// Frontend feature: timeline-rendering.js#refreshTimeline

// Frontend feature: timeline-rendering.js#orderTimelineForRevealedBatch

// Frontend feature: timeline-rendering.js#handleFinishLineAction

// Frontend feature: timeline-rendering.js#revealPreparedBatch

// Frontend feature: inbox-ui.js#loadInbox

// Frontend feature: inbox-ui.js#renderInbox

// Frontend feature: inbox-ui.js#buildInboxSession

// Frontend feature: inbox-ui.js#buildInboxReasoningBadge

// Frontend feature: inbox-ui.js#buildSessionModelUsage

// Frontend feature: inbox-ui.js#modelUsageMessage

// Frontend feature: inbox-ui.js#buildModelUsageHelp

// Frontend feature: inbox-ui.js#renderModelUsageReport

// Frontend feature: inbox-ui.js#buildModelUsageCategory

// Frontend feature: inbox-ui.js#modelUsageCategoryProfile

// Frontend feature: inbox-ui.js#buildModelUsageEntry

// Frontend feature: inbox-ui.js#modelUsageInvocationCount

// Frontend feature: inbox-ui.js#modelUsageSummary

// Frontend feature: inbox-ui.js#formatTokenCount

// Frontend feature: inbox-ui.js#loadAggregateModelUsage

// Frontend feature: inbox-ui.js#buildInboxPreferenceDecisions

// Frontend feature: inbox-ui.js#buildInboxPreferenceDecision

// Frontend feature: inbox-ui.js#buildAIDetectionDiagnostic

// Frontend feature: inbox-ui.js#buildAIDetectorYield

// Frontend feature: inbox-ui.js#buildEventResolutionDiagnostic

// Frontend feature: inbox-ui.js#buildInboxRun

// Frontend feature: inbox-ui.js#buildVisionEvaluationTelemetry

// Frontend feature: inbox-ui.js#inboxRunCaptureReliabilityNotice

// Frontend feature: inbox-ui.js#buildCaptureCandidateTelemetry

// Frontend feature: inbox-ui.js#buildMediaAcquisitionTelemetry

// Frontend feature: inbox-ui.js#buildCaptureSurfaceTelemetry

// Frontend feature: inbox-ui.js#buildAcquisitionIdentityTelemetry

// Frontend feature: inbox-ui.js#inboxSessionFlowText

// Frontend feature: inbox-ui.js#inboxRunMetricText

// Frontend feature: inbox-ui.js#buildInboxFlowInspector

// Frontend feature: inbox-ui.js#buildInboxFlowItem

// Frontend feature: inbox-ui.js#inboxFlowOutcomeLabel

// Frontend feature: inbox-ui.js#inboxContinuityLabel

// Frontend feature: inbox-ui.js#inboxStatusTone

// Frontend feature: timeline-rendering.js#routeAIDetectedItems

// Frontend feature: timeline-rendering.js#renderTimeline

// Frontend feature: timeline-rendering.js#timelinePresentationSnapshot

// Frontend feature: timeline-rendering.js#renderTimelineNow

// Frontend feature: timeline-rendering.js#renderTimelineActions

// Frontend feature: passive-media-ui.js#schedulePassiveMediaEnrichment

// Frontend feature: passive-media-ui.js#passiveMediaCandidates

// Frontend feature: passive-media-ui.js#enrichPassiveXMedia

// Frontend feature: passive-media-ui.js#lookupPassiveXMediaEvidence

// Frontend feature: passive-media-ui.js#xMediaCandidateId

// Frontend feature: timeline-rendering.js#observeTimelineItem

// Frontend feature: timeline-rendering.js#scheduleAIDeepRefresh

// Frontend feature: timeline-rendering.js#timelineInteractionActive

// Frontend feature: timeline-rendering.js#queueBackgroundTimelineRefresh

// Frontend feature: timeline-rendering.js#flushBackgroundTimelineRefresh

document.addEventListener("focusout", () => {
  window.setTimeout(() => void flushBackgroundTimelineRefresh(), 0);
});

// Frontend feature: timeline-rendering.js#renderTimelineSidePane

// Frontend feature: timeline-rendering.js#timelineSidePaneAvailable

// Frontend feature: timeline-rendering.js#syncTimelineSidePaneVisibility

// Frontend feature: timeline-rendering.js#openTimelineSidePane

// Frontend feature: timeline-rendering.js#closeTimelineSidePane

// Frontend feature: timeline-rendering.js#populateTimelineSidePane

// Frontend feature: timeline-content.js#buildTimelineItem

// Frontend feature: timeline-content.js#buildCollapsedDuplicate

// Frontend feature: timeline-content.js#buildExpandedTimelineItem

// Frontend feature: timeline-ai-ui.js#buildAIDetectionControls

// Frontend feature: timeline-ai-ui.js#applyAIFeedback

// Frontend feature: timeline-ai-ui.js#loadAIFeedbackHistory

// Frontend feature: timeline-content.js#buildBrief

// Frontend feature: timeline-content.js#postHeaderContexts

// Frontend feature: timeline-content.js#buildSourceCard

// Frontend feature: timeline-content.js#applyPostFreshness

// Frontend feature: timeline-content.js#refreshVisiblePostFreshness

window.setInterval(refreshVisiblePostFreshness, 60 * 1000);

// Frontend feature: timeline-content.js#buildAvatar

// Frontend feature: timeline-content.js#buildQuotedPost

// Frontend feature: timeline-content.js#buildExpandableText

// Frontend feature: timeline-media.js#buildMedia

// Frontend feature: timeline-media.js#buildTimelineMediaCarousel

// Frontend feature: timeline-media.js#buildImageMediaControl

// Frontend feature: timeline-media.js#buildVideoMedia

// Frontend feature: timeline-media.js#buildVideoPosterControl

// Frontend feature: timeline-media.js#activateInlineVideo

// Frontend feature: timeline-media.js#queueInlinePlaybackRecovery

// Frontend feature: timeline-media.js#playbackRecoveryReady

const playbackRecovery = createPlaybackRecoveryQueue({
  isReady: playbackRecoveryReady,
  resolveEntry: (request) => {
    const entry = state.timelineItems.find((item) => item.id === request.id);
    if ((entry?.source || entry?.item?.source) !== request.source
      || !safeSourceUrl(entry.item?.sourceUrl || entry.evidence?.permalink, request.source)) return null;
    return entry.evidence?.media?.some((media) => media.playbackMode === "inline"
      && safePlaybackUrl(media.playbackUrl, request.source) === request.playbackUrl) ? entry : null;
  },
  recover: (entry) => recaptureMedia(entry, null, "background", "playback_error"),
});

// Frontend feature: timeline-media.js#drainPlaybackRecovery

// Frontend feature: timeline-media.js#pauseOtherInlineVideos

// Frontend feature: timeline-media.js#inlineVideoVisibilityObserver

// Frontend feature: timeline-media.js#observeInlineVideoVisibility

// Frontend feature: timeline-media.js#stopObservingInlineVideoVisibility

// Frontend feature: timeline-media.js#stopObservingInlineVideoVisibilityWithin

// Frontend feature: timeline-content.js#buildAttachments

// Frontend feature: timeline-content.js#buildEngagement

// Frontend feature: timeline-actions.js#buildActions

// Frontend feature: timeline-context.js#timelineContentContextAnchor

// Frontend feature: timeline-context.js#timelineContentContextTrigger

// Frontend feature: timeline-context.js#measureTimelineContentContextTabs

// Frontend feature: timeline-context.js#syncTimelineContentContextTabs

// Frontend feature: timeline-context.js#timelineContentContextFeedbackKey

// Frontend feature: timeline-context.js#submitTimelineContentContextFeedback

// Frontend feature: timeline-context.js#renderTimelineContentContextMatch

// Frontend feature: timeline-context.js#renderTimelineContentContextTopicInsight

// Frontend feature: timeline-context.js#renderDirectContext

// Frontend feature: timeline-context.js#renderTimelineContentContextDrawer

// Frontend feature: timeline-context.js#contentContextDrawerMotionDuration

// Frontend feature: timeline-context.js#revealTimelineContentContextDrawer

// Frontend feature: timeline-context.js#closeTimelineContentContextDrawer

// Frontend feature: timeline-context.js#timelineContentContextFocusIsInsideDrawer

// Frontend feature: timeline-context.js#openTimelineContentContext

// Frontend feature: timeline-context.js#toggleTimelineContentContext

// Frontend feature: timeline-context.js#observeTimelineContentContextAnchor

// Frontend feature: timeline-context.js#syncTimelineContentContextExpandAction

// Frontend feature: timeline-context.js#syncTimelineContentContextPosition

// Frontend feature: timeline-context.js#scheduleTimelineContentContextPosition

// Frontend feature: timeline-context.js#handleTimelineContentContextScroll

// Frontend feature: timeline-actions.js#buildTimelineReadLaterAction

// Frontend feature: timeline-context.js#buildTimelineContentContextAnchor

// Frontend feature: timeline-actions.js#buildSemanticCorrectionActions

// Frontend feature: timeline-actions.js#showEventSuggestions

// Frontend feature: timeline-actions.js#applySemanticCorrection

// Frontend feature: timeline-actions.js#showCorrectionNotice

// Frontend feature: timeline-actions.js#buildSourceLink

function originalPostUrl(entry) {
  return safeSourceUrl(entry.item?.sourceUrl || entry.evidence?.permalink, entry.source || entry.item?.source);
}

function safeSourceUrl(value, source) {
  const href = safeMediaUrl(value);
  if (!href) return null;
  const url = new URL(href);
  const descriptor = sourceDescriptor(source);
  if (!descriptor) return null;
  const trustedHost = (descriptor.nativeHosts ?? []).includes(url.hostname);
  const nativePath = (descriptor.nativePathTokens ?? []).some((token) => url.pathname.includes(token));
  if (!trustedHost || !nativePath) return null;
  if (source === "x") {
    const match = url.pathname.match(/^\/([^/]+)\/status\/(\d+)(?:\/.*)?$/);
    if (!match) return null;
    return `https://x.com/${match[1]}/status/${match[2]}`;
  }
  return url.href;
}

// Frontend feature: timeline-actions.js#mediaRecaptureReasonForEntry

// Frontend feature: timeline-actions.js#buildMediaRecaptureButton

// Frontend feature: timeline-actions.js#buildForegroundRecaptureOffer

// Frontend feature: timeline-actions.js#recaptureMedia

// Frontend feature: timeline-actions.js#dispatchMediaRecapture

function feedbackButton(label) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "feedback-button";
  button.textContent = label;
  return button;
}

function logInteractionTrace(action, trace, phase, details = {}) {
  console.info(`${action}_trace`, { trace, phase, ...details });
}

function logNativePostTrace(trace, phase, details = {}) {
  if (window.akuNativePostDiagnostics) window.akuNativePostDiagnostics.record(trace, phase, details);
  else logInteractionTrace("native_post", trace, phase, details);
}

// Frontend feature: timeline-media.js#openMedia

// Frontend feature: timeline-media.js#moveMedia

// Frontend feature: timeline-media.js#renderMedia

// Frontend feature: timeline-media.js#mediaZoomMetrics

// Frontend feature: timeline-media.js#applyMediaZoom

// Frontend feature: timeline-media.js#syncMediaPanAffordance

// Frontend feature: timeline-media.js#mediaPanStartedOnScrollbar

// Frontend feature: timeline-media.js#beginMediaPan

// Frontend feature: timeline-media.js#moveMediaPan

// Frontend feature: timeline-media.js#endMediaPan

// Frontend feature: timeline-media.js#syncMediaZoomPresetControl

// Frontend feature: timeline-media.js#snapMediaZoom

// Frontend feature: timeline-media.js#adjustMediaZoom

// Frontend feature: session-ui.js#showSessionFailure

// Frontend feature: session-ui.js#isWindowsSecurityRuntimeFailure

// Frontend feature: session-ui.js#matchingWindowsPortableBundleURL

// Frontend feature: session-ui.js#firstRunCalibrationPending

// Frontend feature: session-ui.js#showCalibrationRetry

// Frontend feature: session-ui.js#showSessionOutcome

// Frontend feature: session-ui.js#hideFailure

function setPill(selector, text, tone) {
  const node = $(selector);
  setSettingsText(node, text);
  setSettingsClass(node, `status-pill status-${tone}`);
}

function showError(error) {
  if (error?.recoveryInitiated) return;
  console.error(error);
  const notice = $("#provider-notice");
  notice.className = "notice notice-danger";
  notice.setAttribute("role", "alert");
  setNoticeText(notice, error.message || String(error));
  setPill(
    "#sidecar-status",
    error.code === "sidecar_unavailable" ? "AkuSidecar offline" : "AkuSidecar attention",
    "danger",
  );
}

function clearNotice() {
  const notice = $("#provider-notice");
  notice.className = "notice hidden";
  notice.setAttribute("role", "status");
  notice.replaceChildren();
}

function showNotice(message) {
  const notice = $("#provider-notice");
  notice.className = "notice notice-complete";
  notice.setAttribute("role", "status");
  setNoticeText(notice, message);
}

function setNoticeText(notice, message) {
  notice.replaceChildren(document.createTextNode(message), createNoticeDismissButton());
}

function createNoticeDismissButton() {
  const dismiss = document.createElement("button");
  dismiss.type = "button";
  dismiss.className = "notice-dismiss";
  dismiss.setAttribute("aria-label", "Dismiss notification");
  dismiss.title = "Dismiss notification";
  dismiss.textContent = "×";
  dismiss.addEventListener("click", clearNotice);
  return dismiss;
}

function sourceIdentity(value, source) {
  const text = String(value ?? "").trim();
  if (!text) return { displayName: sourceLabel(source), secondary: "" };
  if (sourceDescriptor(source)?.identityFormat !== "display_handle") return { displayName: text, secondary: "" };
  const match = text.match(/^(.+?)\s+(@[A-Za-z0-9_]+)(?:\s+[·•]\s+(.+))?$/);
  return {
    displayName: match?.[1]?.trim() || text,
    secondary: match ? [match[2], match[3]].filter(Boolean).join(" · ") : "",
  };
}

function initials(value) {
  const letters = String(value ?? "LI").trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase()).join("");
  return letters || "LI";
}

function sourceLabel(source) {
  return sourceDescriptor(source)?.displayName || String(source || "Unknown source");
}

function buildSourceIcon(source) {
  const descriptor = sourceDescriptor(source);
  const icon = document.createElement("span");
  icon.className = `timeline-source-icon timeline-source-icon-${source}`;
  icon.title = sourceLabel(source);
  icon.setAttribute("aria-label", `Source: ${sourceLabel(source)}`);
  if (source === "instagram") {
    const glyph = document.createElement("span");
    glyph.className = "instagram-source-glyph";
    glyph.setAttribute("aria-hidden", "true");
    icon.append(glyph);
    return icon;
  }
  icon.style.background = descriptor?.iconBackground || "var(--panel-strong)";
  icon.style.color = descriptor?.iconForeground || "var(--text)";
  icon.textContent = descriptor?.iconText || sourceLabel(source).slice(0, 1);
  return icon;
}

function sourceDescriptors() {
  return Array.isArray(state.bootstrap?.sources) ? state.bootstrap.sources : [];
}

function sourceDescriptor(source) {
  return sourceDescriptors().find((descriptor) => descriptor.id === source);
}

// Frontend feature: onboarding-ui.js#renderSourceControls

function humanize(value) {
  if (!value) return "";
  return String(value).replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatDate(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function formatSchedulerMoment(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "";
  const today = new Date();
  const sameDay = date.getFullYear() === today.getFullYear()
    && date.getMonth() === today.getMonth()
    && date.getDate() === today.getDate();
  return new Intl.DateTimeFormat(undefined, sameDay ? { timeStyle: "short" } : { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function formatDurationBetween(startedAt, completedAt) {
  if (!startedAt || !completedAt) return "";
  const duration = new Date(completedAt).valueOf() - new Date(startedAt).valueOf();
  return Number.isFinite(duration) && duration >= 0 ? formatDuration(duration) : "";
}

function formatDuration(milliseconds) {
  const seconds = Math.max(0, Math.round(Number(milliseconds) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return remainder ? `${minutes}m ${remainder}s` : `${minutes}m`;
}

function safeMediaUrl(value) {
  if (typeof value !== "string") return null;
  try {
    const url = new URL(value);
    return url.protocol === "https:" ? url.href : null;
  } catch {
    return null;
  }
}

function safePlaybackUrl(value, source) {
  if (typeof value !== "string") return null;
  try {
    const url = new URL(value);
    if (url.protocol !== "https:" || url.port || url.username || url.password) return null;
    const host = url.hostname.toLowerCase();
    if (source === "x") {
      if (
        host !== "video.twimg.com" ||
        !/^\/(?:amplify_video|ext_tw_video|tweet_video)\//.test(url.pathname) || !/\.mp4$/i.test(url.pathname)
      ) return null;
    } else if (source === "linkedin") {
      if (
        host !== "dms.licdn.com" ||
        !/^\/playlist\//i.test(url.pathname) ||
        !/\/mp4-\d{2,4}p(?:-|\/)/i.test(url.pathname)
      ) return null;
    } else if (source === "facebook") {
      if (
        !["fbcdn.net", "fbsbx.com"].some((suffix) => host === suffix || host.endsWith(`.${suffix}`)) ||
        !/\.mp4$/i.test(url.pathname)
      ) return null;
    } else if (source === "instagram") {
      if (
        !["fbcdn.net", "cdninstagram.com"].some((suffix) => host === suffix || host.endsWith(`.${suffix}`)) ||
        !/\.mp4$/i.test(url.pathname)
      ) return null;
    } else {
      return null;
    }
    url.hash = "";
    return url.href;
  } catch {
    return null;
  }
}

async function api(path, options = {}) {
  const init = { method: options.method || "GET", cache: "no-store", headers: {} };
  if (options.instanceEpoch) init.headers["X-Aku-Split-Epoch"] = options.instanceEpoch;
  if (options.signal) init.signal = options.signal;
  if (options.feedbackTraceId) init.headers["X-Aku-Feedback-Trace"] = options.feedbackTraceId;
  if (options.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }
  const response = await fetchFromSidecar(path, init);
  enforceCurrentSidecarEpoch(response);
  if (options.responseType === "blob") {
    if (!response.ok) {
      const payload = await response.json().catch(() => ({}));
      const error = new Error(payload?.message || `HTTP ${response.status}`);
      error.code = payload?.error || `http_${response.status}`;
      error.details = payload?.details;
      throw error;
    }
    return response.blob();
  }
  const payload = response.status === 204 ? null : await response.json();
  if (!response.ok) {
    const error = new Error(payload?.message || `HTTP ${response.status}`);
    error.code = payload?.error || `http_${response.status}`;
    error.details = payload?.details;
    throw error;
  }
  return payload;
}

async function bridgeApi(path, options = {}) {
  const init = {
    method: options.method || "GET",
    cache: "no-store",
    headers: {
      "X-Aku-Bridge-Token": state.bootstrap.bridgeToken,
      "X-Aku-Bridge-Id": "aku-browser-page",
      "X-Aku-Bridge-Contract": state.bootstrap.bridgeContractVersion,
    },
  };
  if (options.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }
  const response = await fetchFromSidecar(path, init);
  enforceCurrentSidecarEpoch(response);
  if (response.status === 204) return null;
  const payload = await response.json();
  if (!response.ok) {
    const error = new Error(payload?.message || `HTTP ${response.status}`);
    error.code = payload?.error || `http_${response.status}`;
    if (recoverInvalidBridgeToken(error.code)) {
      error.recoveryInitiated = true;
      throw error;
    }
    if (error.code === "invalid_bridge_token") {
      error.message = "AkuSidecar keeps changing its Bridge token. Check whether another AkuSidecar instance or security software is restarting the runtime, then refresh AkuBrowser once.";
    }
    throw error;
  }
  return payload;
}

function enforceCurrentSidecarEpoch(response) {
  const responseEpoch = response.headers.get("X-Aku-Sidecar-Instance-Epoch");
  const currentEpoch = state.bootstrap?.instanceEpoch;
  if (!responseEpoch || !currentEpoch || responseEpoch === currentEpoch || state.sidecarEpochReloading) return;
  state.sidecarEpochReloading = true;
  window.location.reload();
  const error = new Error("AkuSidecar restarted. Refreshing the AkuBrowser interface.");
  error.name = "SidecarEpochChangedError";
  error.code = "sidecar_epoch_changed";
  error.recoveryInitiated = true;
  throw error;
}

async function fetchFromSidecar(path, init) {
  try {
    return await fetch(path, init);
  } catch (error) {
    if (error instanceof TypeError) {
      const unavailable = new Error(
        "AkuSidecar is offline or unreachable. Start it through AkuSupervisor; AkuBrowser will reconnect automatically.",
      );
      unavailable.name = "SidecarUnavailableError";
      unavailable.code = "sidecar_unavailable";
      unavailable.cause = error;
      throw unavailable;
    }
    throw error;
  }
}

syncRunButtons();
bootstrap();
