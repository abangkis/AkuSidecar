const ownValue = (object, key) => object && typeof object === 'object'
  ? Object.getOwnPropertyDescriptor(object, key)?.value : undefined;

const componentKinds = ['messaging', 'feed_sort', 'post', 'navigation', 'header', 'sidebar', 'footer', 'main'];
const actionKinds = ['post_link', 'save_post', 'report_post', 'account', 'message', 'feed_order', 'navigation', 'dismiss'];
const count = (value, maximum) => Number.isSafeInteger(value) && value >= 0 && value <= maximum;

const validScrollTop = value => Number.isFinite(value) && value >= 0 && value <= 10_000_000;

function projectGeometryPoint(point) {
  const scrollTop = ownValue(point, 'scrollTop');
  const gapY = ownValue(point, 'gapY');
  const correctedGapY = ownValue(point, 'correctedGapY');
  if (!validScrollTop(scrollTop) || !Number.isFinite(gapY) || Math.abs(gapY) > 16_384 ||
      (correctedGapY !== undefined && (!Number.isFinite(correctedGapY) || Math.abs(correctedGapY) > 16_384))) return null;
  return { scrollTop, gapY, ...(correctedGapY !== undefined ? { correctedGapY } : {}) };
}

export function projectLinkedInAnchorGeometrySamples(snapshot, snapshotIndex = null) {
  if (ownValue(snapshot, 'source') !== 'linkedin') return {};
  const entries = ownValue(snapshot, 'linkedinAnchorGeometrySamples');
  if (!Array.isArray(entries) || entries.length === 0 || entries.length > 8) return {};
  const projected = [];
  for (let i = 0; i < entries.length; i++) {
    const entry = ownValue(entries, String(i));
    const reportedSnapshot = ownValue(entry, 'snapshotIndex');
    const candidateIndex = ownValue(entry, 'candidateIndex');
    const scrollTopAtClick = ownValue(entry, 'scrollTopAtClick');
    const rawFirst = ownValue(entry, 'first'), rawLast = ownValue(entry, 'last');
    const first = projectGeometryPoint(rawFirst), last = projectGeometryPoint(rawLast);
    if ((reportedSnapshot !== null && !count(reportedSnapshot, 1_000_000)) || !count(candidateIndex, 1_000_000) ||
        !validScrollTop(scrollTopAtClick) || (first === null) !== (last === null) ||
        (!first && (rawFirst !== null || rawLast !== null))) return {};
    projected.push({ snapshotIndex: count(snapshotIndex, 1_000_000) ? snapshotIndex : reportedSnapshot,
      candidateIndex, scrollTopAtClick, first, last });
  }
  return { linkedinAnchorGeometrySamples: projected };
}

export function projectLinkedInAnchorMismatchPolls(snapshot, snapshotIndex = null) {
  if (ownValue(snapshot, 'source') !== 'linkedin') return {};
  const entries = ownValue(snapshot, 'linkedinAnchorMismatchPolls');
  if (!Array.isArray(entries) || entries.length === 0 || entries.length > 8) return {};
  const projected = [];
  for (let i = 0; i < entries.length; i++) {
    const entry = ownValue(entries, String(i));
    const reportedSnapshot = ownValue(entry, 'snapshotIndex');
    const candidateIndex = ownValue(entry, 'candidateIndex');
    const mismatchPolls = ownValue(entry, 'mismatchPolls');
    const horizontalPolls = ownValue(entry, 'horizontalPolls');
    const verticalPolls = ownValue(entry, 'verticalPolls');
    const verticalGapPolls = ownValue(entry, 'verticalGapPolls');
    const verticalOverlapPolls = ownValue(entry, 'verticalOverlapPolls');
    const persistence = ownValue(entry, 'persistence');
    const hasVerticalBreakdown = verticalGapPolls !== undefined || verticalOverlapPolls !== undefined;
    if ((reportedSnapshot !== null && !count(reportedSnapshot, 1_000_000)) || !count(candidateIndex, 1_000_000) ||
        !count(mismatchPolls, 32) || mismatchPolls === 0 || !count(horizontalPolls, mismatchPolls) ||
        !count(verticalPolls, mismatchPolls) || horizontalPolls + verticalPolls < mismatchPolls ||
        !['single_sample', 'repeated', 'resolved', 'interrupted'].includes(persistence) ||
        (persistence === 'single_sample' && mismatchPolls !== 1) ||
        (persistence === 'repeated' && mismatchPolls < 2) ||
        (hasVerticalBreakdown && (!count(verticalGapPolls, verticalPolls) ||
          !count(verticalOverlapPolls, verticalPolls) || verticalGapPolls + verticalOverlapPolls !== verticalPolls))) return {};
    projected.push({ snapshotIndex: count(snapshotIndex, 1_000_000) ? snapshotIndex : reportedSnapshot,
      candidateIndex, mismatchPolls, horizontalPolls, verticalPolls, persistence,
      ...(hasVerticalBreakdown ? { verticalGapPolls, verticalOverlapPolls } : {}) });
  }
  return { linkedinAnchorMismatchPolls: projected };
}

const permalinkFailures = new Map([
  ['candidate', ['snapshot_budget', 'owner_missing', 'owner_changed']],
  ['menu', ['menu_not_observed', 'multiple_menus']],
  ['ownership', ['foreign_reference', 'control_already_expanded', 'control_not_uniquely_expanded',
    'popover_not_anchored', 'missing_geometry', 'anchor_mismatch', 'menu_replaced']],
  ['menu_evidence', ['stale_urn', 'conflicting_urns', 'multiple_embed_actions', 'no_visible_urn_or_embed_action']],
  ['embed_dialog', ['dialog_not_started', 'dialog_not_observed', 'multiple_dialogs', 'foreign_reference',
    'dialog_replaced', 'stale_urn', 'conflicting_urns', 'no_urn']],
  ['revalidation', ['binding_or_evidence_changed']],
]);
const buttonVisibility = ['unavailable', 'hidden', 'outside', 'partial', 'visible'];

export function projectLinkedInPermalinkRecoveryFailures(snapshot, snapshotIndex = null) {
  if (ownValue(snapshot, 'source') !== 'linkedin') return {};
  const entries = ownValue(snapshot, 'linkedinPermalinkRecoveryFailures');
  if (!Array.isArray(entries) || entries.length === 0 || entries.length > 8) return {};
  const projected = [];
  for (let i = 0; i < entries.length; i++) {
    const entry = ownValue(entries, String(i));
    const reportedSnapshot = ownValue(entry, 'snapshotIndex');
    const candidateIndex = ownValue(entry, 'candidateIndex');
    const stage = ownValue(entry, 'stage');
    const reason = ownValue(entry, 'reason');
    const viewport = ownValue(entry, 'viewport');
    const scrollRoot = ownValue(entry, 'scrollRoot');
    if ((reportedSnapshot !== null && !count(reportedSnapshot, 1_000_000)) || !count(candidateIndex, 1_000_000) ||
        !permalinkFailures.get(stage)?.includes(reason) || !buttonVisibility.includes(viewport) ||
        !buttonVisibility.includes(scrollRoot)) return {};
    projected.push({ snapshotIndex: count(snapshotIndex, 1_000_000) ? snapshotIndex : reportedSnapshot,
      candidateIndex, stage, reason, viewport, scrollRoot });
  }
  return { linkedinPermalinkRecoveryFailures: projected };
}

function projectList(value, limit, allowed) {
  if (!Array.isArray(value) || value.length > limit) return null;
  const list = Array.from({ length: value.length }, (_, index) => ownValue(value, String(index)));
  return list.every(item => allowed.includes(item)) && new Set(list).size === list.length ? list : null;
}

function projectSurfaceInspection(value) {
  const dialogCount = ownValue(value, 'dialogCount');
  const complete = ownValue(value, 'complete');
  const surfaces = ownValue(value, 'surfaces');
  if (!count(dialogCount, 8) || typeof complete !== 'boolean' || !Array.isArray(surfaces) || surfaces.length > 4) return null;
  const projected = [];
  for (let index = 0; index < surfaces.length; index++) {
    const surface = ownValue(surfaces, String(index));
    const kind = ownValue(surface, 'kind'), slot = ownValue(surface, 'slot');
    const actionCount = ownValue(surface, 'actionCount'), triggerExpanded = ownValue(surface, 'triggerExpanded');
    const ownerRelation = ownValue(surface, 'ownerRelation'), triggerBinding = ownValue(surface, 'triggerBinding');
    const controlOcclusion = ownValue(surface, 'controlOcclusion'), openState = ownValue(surface, 'openState');
    const trail = projectList(ownValue(surface, 'trail'), 4, componentKinds);
    const triggerTrail = projectList(ownValue(surface, 'triggerTrail'), 4, componentKinds);
    const actionHints = projectList(ownValue(surface, 'actionHints'), 8, actionKinds);
    if (!['menu', 'dialog'].includes(kind) || slot !== index || !count(actionCount, 24) ||
        (triggerExpanded !== null && typeof triggerExpanded !== 'boolean') ||
        !['candidate_post', 'other_post', 'unknown'].includes(ownerRelation) ||
        !['target_reference', 'label_reference', 'component_toggle', 'ambiguous', 'unresolved'].includes(triggerBinding) ||
        !['open', 'closed', 'unknown'].includes(openState) ||
        !['target_hit', 'surface_hit', 'other_hit', 'unknown'].includes(controlOcclusion) || !trail || !triggerTrail || !actionHints) return null;
    projected.push({ kind, slot, trail, actionHints, actionCount, ownerRelation, triggerBinding, triggerTrail, triggerExpanded, openState, controlOcclusion });
  }
  const sampledDialogs = projected.filter(surface => surface.kind === 'dialog').length;
  if (sampledDialogs !== Math.min(dialogCount, 2) || projected.length - sampledDialogs > 2 || complete && dialogCount > 2) return null;
  return { dialogCount, complete, surfaces: projected };
}

function projectLinkedInMenuEvidence(value) {
    const fields = {
        component: ['popover', 'artdeco_dropdown', 'unclassified'],
        scope: ['navigation', 'feed_post', 'unclassified'],
        ariaOwnerRelation: ['candidate', 'other', 'multiple', 'absent', 'unknown'],
        triggerBinding: ['target_reference', 'label_reference', 'component_toggle', 'ambiguous', 'unresolved'],
        triggerRelation: ['candidate', 'other', 'unknown'],
    };
    const result = {};
    for (const [field, allowed] of Object.entries(fields)) {
        const code = ownValue(value, field);
        if (!allowed.includes(code)) return null;
        result[field] = code;
    }
    for (const field of ['triggerExpanded', 'popoverOpen', 'dropdownOpen', 'cssCheck']) {
        const flag = ownValue(value, field);
        if (flag !== null && typeof flag !== 'boolean') return null;
        result[field] = flag;
    }
    const hitTest = ownValue(value, 'hitTest');
    if (hitTest === null) return { ...result, hitTest: null };
    const points = ['hit', 'covered', 'miss'].map(field => ownValue(hitTest, field));
    if (!points.every(count => Number.isSafeInteger(count) && count >= 0 && count <= 5) ||
        points[0] + points[1] + points[2] !== 5) return null;
    const [hit, covered, miss] = points;
    return { ...result, hitTest: { hit, covered, miss } };
}

function projectPriorRecoveryMenus(value, menuCount, snapshotIndex) {
  if (!Array.isArray(value) || !value.length || value.length > 2) return null;
  const result = [];
  for (let index = 0; index < value.length; index++) {
    const menu = ownValue(value, String(index));
    const slot = ownValue(menu, 'slot'), cleanup = ownValue(menu, 'cleanup');
    const attemptSnapshotIndex = ownValue(menu, 'attemptSnapshotIndex');
    const attemptCandidateIndex = ownValue(menu, 'attemptCandidateIndex');
    const sameButton = ownValue(menu, 'sameButton'), menuBound = ownValue(menu, 'menuBound');
    const triggerExpanded = ownValue(menu, 'triggerExpanded');
    if (!count(slot, Math.min(1, menuCount - 1)) || result.some(entry => entry.slot >= slot) ||
        (attemptSnapshotIndex !== null && !count(attemptSnapshotIndex, snapshotIndex ?? 1_000_000)) ||
        (attemptCandidateIndex !== null && !count(attemptCandidateIndex, 1_000_000)) ||
        [sameButton, menuBound, triggerExpanded].some(flag => typeof flag !== 'boolean') ||
        !(menuBound || triggerExpanded) || !['not_attempted', 'still_visible'].includes(cleanup)) return null;
    result.push({slot, attemptSnapshotIndex, attemptCandidateIndex, sameButton, menuBound, triggerExpanded, cleanup});
  }
  return result;
}

// Diagnostic metadata cannot supply identity, release resources or admit a post.
export function projectLinkedInRecoveryBlockers(snapshot, snapshotIndex = null) {
  try {
    if (ownValue(snapshot, 'source') !== 'linkedin') return {};
    const values = ownValue(snapshot, 'linkedinRecoveryBlockers');
    if (!Array.isArray(values) || values.length < 1 || values.length > 8) return {};
    const count = value => Number.isSafeInteger(value) && value >= 0 && value <= 32;
    const selectors = new Set(['role_menu', 'role_dialog', 'data_test_modal', 'artdeco_modal', 'unknown']);
    const relations = new Set(['matches', 'different', 'absent', 'unknown']);
    const flags = ['hasArea', 'intersectsViewport', 'displayed', 'cssVisible', 'opacityPositive', 'hiddenAttribute', 'ariaHidden', 'inert'];
    const ancestorFlags = ['displayNone', 'visibilityHidden', 'opacityZero', 'hiddenAttribute', 'ariaHidden', 'inert'];
    const indexValid = value => value === null || Number.isSafeInteger(value) && value >= 0 && value <= 1_000_000;
    const projected = [];
    for (let index = 0; index < values.length; index++) {
      const value = ownValue(values, String(index));
      const menuCount = ownValue(value, 'menuCount'), dialogCount = ownValue(value, 'dialogCount');
      const complete = ownValue(value, 'complete'), samples = ownValue(value, 'samples');
      const recordedIndex = ownValue(value, 'snapshotIndex'), candidateIndex = ownValue(value, 'candidateIndex');
      if (ownValue(value, 'code') !== 'existing_surface' || !count(menuCount) ||
          ownValue(value, 'phase') !== 'pre_recovery_guard' || !indexValid(recordedIndex) || !indexValid(snapshotIndex) ||
          !indexValid(candidateIndex) ||
          !(dialogCount === null || count(dialogCount)) || menuCount + (dialogCount ?? 0) < 1 ||
          typeof complete !== 'boolean' || !Array.isArray(samples) || samples.length < 1 || samples.length > 4) return {};
      const entries = [];
      for (let n = 0; n < samples.length; n++) {
        const sample = ownValue(samples, String(n));
        const kind = ownValue(sample, 'kind'), selector = ownValue(sample, 'selector');
        if (!['menu', 'dialog'].includes(kind) || !selectors.has(selector) ||
            ownValue(sample, 'guardVisible') !== true) return {};
        const aria = ownValue(sample, 'aria'), projectedARIA = {};
        for (const field of ['labelledBy', 'controls', 'owns']) {
          const relation = ownValue(aria, field);
          if (!relations.has(relation)) return {};
          projectedARIA[field] = relation;
        }
        const result = { kind, selector, aria: projectedARIA, guardVisible: true };
        for (const field of flags) {
          const flag = ownValue(sample, field);
          if (flag !== null && typeof flag !== 'boolean') return {};
          result[field] = flag;
        }
        const ancestors = ownValue(sample, 'ancestors');
        const inspected = ownValue(ancestors, 'inspected'), ancestorComplete = ownValue(ancestors, 'complete');
        if (!Number.isSafeInteger(inspected) || inspected < 0 || inspected > 16 || typeof ancestorComplete !== 'boolean') return {};
        result.ancestors = { inspected, complete: ancestorComplete };
        for (const field of ancestorFlags) {
          const flag = ownValue(ancestors, field);
          if (flag !== null && typeof flag !== 'boolean' || ancestorComplete && flag === null || !ancestorComplete && flag === false) return {};
          result.ancestors[field] = flag;
        }
        if (kind === 'menu') {
          const menuEvidence = projectLinkedInMenuEvidence(ownValue(sample, 'menuEvidence'));
          if (menuEvidence) result.menuEvidence = menuEvidence;
        }
        entries.push(result);
      }
      if (entries.filter(entry => entry.kind === 'menu').length > menuCount ||
          entries.filter(entry => entry.kind === 'dialog').length > (dialogCount ?? 0) ||
          complete && (dialogCount === null || entries.length !== menuCount + dialogCount ||
            entries.some(entry => flags.some(field => entry[field] === null) || !entry.ancestors.complete ||
              Object.values(entry.aria).includes('unknown') || entry.selector === 'unknown'))) return {};
      const surfaceInspection = projectSurfaceInspection(ownValue(value, 'surfaceInspection'));
      const priorRecoveryMenus = projectPriorRecoveryMenus(ownValue(value, 'priorRecoveryMenus'), menuCount, snapshotIndex ?? recordedIndex);
      projected.push({ ...(surfaceInspection ? { surfaceInspection } : {}), ...(priorRecoveryMenus ? { priorRecoveryMenus } : {}),
        code: 'existing_surface', phase: 'pre_recovery_guard', snapshotIndex: snapshotIndex ?? recordedIndex,
        candidateIndex, menuCount, dialogCount, samples: entries, complete });
    }
    return { linkedinRecoveryBlockers: projected };
  } catch { return {}; }
}
