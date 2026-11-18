const ownValue = (object, key) => object && typeof object === 'object'
  ? Object.getOwnPropertyDescriptor(object, key)?.value : undefined;

const componentKinds = ['messaging', 'feed_sort', 'post', 'navigation', 'header', 'sidebar', 'footer', 'main'];
const actionKinds = ['post_link', 'save_post', 'report_post', 'account', 'message', 'feed_order', 'navigation', 'dismiss'];
const count = (value, maximum) => Number.isSafeInteger(value) && value >= 0 && value <= maximum;

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
      projected.push({ ...(surfaceInspection ? { surfaceInspection } : {}), code: 'existing_surface', phase: 'pre_recovery_guard', snapshotIndex: snapshotIndex ?? recordedIndex,
        candidateIndex, menuCount, dialogCount, samples: entries, complete });
    }
    return { linkedinRecoveryBlockers: projected };
  } catch { return {}; }
}
