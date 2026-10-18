function bootstrapCurrentSelfProjection(page) {
  const makeDto = (diagnostic) => ({
    schema: 'aku.x.bootstrap-current-self.v1',
    diagnostic,
    bootstrapState: 'absent',
    sessionState: 'absent',
    idFields: { user_id: 'absent', user_id_str: 'absent' },
    stableIdStringCandidates: 0,
    sessionIdsAgree: null,
    authFields: {
      loggedIn: 'absent',
      isAuthenticated: 'absent',
      authenticated: 'absent',
      isActive: 'absent',
    },
    entityChecks: { normalizedUsers: 'unknown', rootUsers: 'unknown' },
    canAuthorize: false,
    continuityProven: false,
  });

  const isRecord = (value) =>
    value !== null && typeof value === 'object' && !Array.isArray(value);

  const readOwn = (record, key) => {
    const descriptor = Object.getOwnPropertyDescriptor(record, key);
    if (descriptor === undefined) return { kind: 'absent' };

    const valueDescriptor = Object.getOwnPropertyDescriptor(descriptor, 'value');
    if (valueDescriptor === undefined) return { kind: 'accessor' };
    return { kind: 'value', value: valueDescriptor.value };
  };

  const stableId = (value) =>
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= 20 &&
    /^[1-9][0-9]{0,19}$/.test(value);

  const classifyValue = (value) => {
    if (value === null) return { kind: 'null' };
    if (typeof value === 'string') {
      return stableId(value) ? { kind: 'string', value } : { kind: 'invalid' };
    }
    if (typeof value === 'number') return { kind: 'number' };
    if (typeof value === 'boolean') return { kind: 'boolean' };
    if (typeof value === 'object') {
      return Array.isArray(value) ? { kind: 'array' } : { kind: 'object' };
    }
    return { kind: 'invalid' };
  };

  const readIdField = (record, key) => {
    const field = readOwn(record, key);
    if (field.kind !== 'value') return { kind: field.kind };
    return classifyValue(field.value);
  };

  const readAuthField = (record, key) => {
    const field = readOwn(record, key);
    if (field.kind !== 'value') return field.kind;
    if (typeof field.value !== 'boolean') return 'invalid';
    return field.value ? 'true' : 'false';
  };

  const checkEntity = (state, path, userId) => {
    let current = state;
    for (const key of path) {
      const field = readOwn(current, key);
      if (field.kind !== 'value') return field.kind;
      if (!isRecord(field.value)) return 'invalid';
      current = field.value;
    }

    const entry = readOwn(current, userId);
    if (entry.kind !== 'value') return entry.kind;
    if (!isRecord(entry.value)) return 'invalid';

    const idStr = readOwn(entry.value, 'id_str');
    const id = readOwn(entry.value, 'id');
    if (idStr.kind === 'accessor' || id.kind === 'accessor') return 'accessor';

    const present = [];
    if (idStr.kind !== 'absent') present.push(idStr);
    if (id.kind !== 'absent') present.push(id);
    if (present.length === 0) return 'absent';

    for (const field of present) {
      if (field.kind !== 'value' || !stableId(field.value)) return 'invalid';
      if (field.value !== userId) return 'mismatch';
    }
    return 'match';
  };

  try {
    const dto = makeDto('projected');
    if (!isRecord(page)) return makeDto('wrong_page');

    const locationField = readOwn(page, 'location');
    if (locationField.kind !== 'value' || !isRecord(locationField.value)) {
      return makeDto('wrong_page');
    }

    const originField = readOwn(locationField.value, 'origin');
    const pathField = readOwn(locationField.value, 'pathname');
    if (
      originField.kind !== 'value' ||
      pathField.kind !== 'value' ||
      typeof originField.value !== 'string' ||
      typeof pathField.value !== 'string' ||
      originField.value.length !== 'https://x.com'.length ||
      pathField.value.length !== '/home'.length ||
      originField.value !== 'https://x.com' ||
      pathField.value !== '/home'
    ) {
      return makeDto('wrong_page');
    }

    const bootstrapField = readOwn(page, '__INITIAL_STATE__');
    if (bootstrapField.kind === 'absent') return makeDto('bootstrap_missing');
    if (bootstrapField.kind === 'accessor') {
      dto.bootstrapState = 'accessor';
      dto.diagnostic = 'bootstrap_invalid';
      return dto;
    }
    if (!isRecord(bootstrapField.value)) {
      dto.bootstrapState = 'invalid';
      dto.diagnostic = 'bootstrap_invalid';
      return dto;
    }

    const state = bootstrapField.value;
    dto.bootstrapState = 'object';
    const sessionField = readOwn(state, 'session');
    if (sessionField.kind === 'absent') return dto;
    if (sessionField.kind === 'accessor') {
      dto.sessionState = 'accessor';
      dto.diagnostic = 'bootstrap_invalid';
      return dto;
    }
    if (!isRecord(sessionField.value)) {
      dto.sessionState = 'invalid';
      dto.diagnostic = 'bootstrap_invalid';
      return dto;
    }

    const session = sessionField.value;
    dto.sessionState = 'object';

    const userIdField = readIdField(session, 'user_id');
    const userIdStrField = readIdField(session, 'user_id_str');
    dto.idFields.user_id = userIdField.kind;
    dto.idFields.user_id_str = userIdStrField.kind;

    const candidateValues = [];
    if (userIdField.kind === 'string') candidateValues.push(userIdField.value);
    if (userIdStrField.kind === 'string') candidateValues.push(userIdStrField.value);
    dto.stableIdStringCandidates = candidateValues.length;
    dto.sessionIdsAgree =
      candidateValues.length < 2 ? null : candidateValues[0] === candidateValues[1];

    for (const key of ['loggedIn', 'isAuthenticated', 'authenticated', 'isActive']) {
      dto.authFields[key] = readAuthField(session, key);
    }

    const idKinds = [userIdField.kind, userIdStrField.kind];
    const allPresentIdsValid = idKinds.every((kind) => kind === 'absent' || kind === 'string');
    const hasStableId = candidateValues.length > 0;
    const idsAgree = candidateValues.length < 2 || candidateValues[0] === candidateValues[1];
    if (hasStableId && allPresentIdsValid && idsAgree) {
      const userId = candidateValues[0];
      dto.entityChecks.normalizedUsers = checkEntity(state, ['entities', 'users', 'entities'], userId);
      dto.entityChecks.rootUsers = checkEntity(state, ['users', 'entities'], userId);
    }

    return dto;
  } catch {
    return makeDto('inspection_failed');
  }
}

export const BOOTSTRAP_CURRENT_SELF_EXPRESSION = `(() => {
  const inspect = (${bootstrapCurrentSelfProjection.toString()});
  const failed = () => ({
    schema: 'aku.x.bootstrap-current-self.v1',
    diagnostic: 'inspection_failed',
    bootstrapState: 'absent',
    sessionState: 'absent',
    idFields: { user_id: 'absent', user_id_str: 'absent' },
    stableIdStringCandidates: 0,
    sessionIdsAgree: null,
    authFields: {
      loggedIn: 'absent',
      isAuthenticated: 'absent',
      authenticated: 'absent',
      isActive: 'absent',
    },
    entityChecks: { normalizedUsers: 'unknown', rootUsers: 'unknown' },
    canAuthorize: false,
    continuityProven: false,
  });

  try {
    const liveLocation = globalThis.location;
    const page = {
      location: {
        origin: liveLocation.origin,
        pathname: liveLocation.pathname,
      },
    };
    const bootstrapDescriptor = Object.getOwnPropertyDescriptor(globalThis, '__INITIAL_STATE__');
    if (bootstrapDescriptor !== undefined) {
      Object.defineProperty(page, '__INITIAL_STATE__', bootstrapDescriptor);
    }
    return inspect(page);
  } catch {
    return failed();
  }
})()`;

export function inspectBootstrapCurrentSelf(page) {
  return bootstrapCurrentSelfProjection(page);
}

const INVALID_MESSAGE = 'bootstrap_projection_invalid';
const BOOTSTRAP_STATES = ['absent', 'object', 'invalid', 'accessor'];
const ID_KINDS = ['absent', 'accessor', 'null', 'string', 'number', 'boolean', 'object', 'array', 'invalid'];
const AUTH_KINDS = ['absent', 'accessor', 'true', 'false', 'invalid'];
const ENTITY_KINDS = ['unknown', 'absent', 'invalid', 'accessor', 'match', 'mismatch'];
const DIAGNOSTICS = ['projected', 'wrong_page', 'bootstrap_missing', 'bootstrap_invalid', 'inspection_failed'];
const ROOT_KEYS = [
  'schema',
  'diagnostic',
  'bootstrapState',
  'sessionState',
  'idFields',
  'stableIdStringCandidates',
  'sessionIdsAgree',
  'authFields',
  'entityChecks',
  'canAuthorize',
  'continuityProven',
];
const ID_KEYS = ['user_id', 'user_id_str'];
const AUTH_KEYS = ['loggedIn', 'isAuthenticated', 'authenticated', 'isActive'];
const ENTITY_KEYS = ['normalizedUsers', 'rootUsers'];
const DEFAULT_ID_FIELDS = { user_id: 'absent', user_id_str: 'absent' };
const DEFAULT_AUTH_FIELDS = {
  loggedIn: 'absent',
  isAuthenticated: 'absent',
  authenticated: 'absent',
  isActive: 'absent',
};
const DEFAULT_ENTITY_CHECKS = { normalizedUsers: 'unknown', rootUsers: 'unknown' };

function invalidProjection() {
  throw new Error(INVALID_MESSAGE);
}

function isPlainOrNullRecord(value) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  if (prototype === null) return true;
  if (typeof prototype !== 'object' || Array.isArray(prototype)) return false;
  return Object.getPrototypeOf(prototype) === null;
}

function readFixedDataObject(value, keys) {
  if (!isPlainOrNullRecord(value)) invalidProjection();
  const ownKeys = Reflect.ownKeys(value);
  if (ownKeys.length !== keys.length || ownKeys.some((key) => typeof key !== 'string' || !keys.includes(key))) {
    invalidProjection();
  }

  const result = Object.create(null);
  for (const key of keys) {
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    if (descriptor === undefined) invalidProjection();
    const valueDescriptor = Object.getOwnPropertyDescriptor(descriptor, 'value');
    if (valueDescriptor === undefined) invalidProjection();
    result[key] = valueDescriptor.value;
  }
  return result;
}

function requireEnum(value, allowed) {
  if (typeof value !== 'string' || !allowed.includes(value)) invalidProjection();
  return value;
}

function requireDefaultProjection(dto) {
  if (
    dto.bootstrapState !== 'absent' ||
    dto.sessionState !== 'absent' ||
    dto.idFields.user_id !== 'absent' ||
    dto.idFields.user_id_str !== 'absent' ||
    dto.stableIdStringCandidates !== 0 ||
    dto.sessionIdsAgree !== null ||
    dto.authFields.loggedIn !== 'absent' ||
    dto.authFields.isAuthenticated !== 'absent' ||
    dto.authFields.authenticated !== 'absent' ||
    dto.authFields.isActive !== 'absent' ||
    dto.entityChecks.normalizedUsers !== 'unknown' ||
    dto.entityChecks.rootUsers !== 'unknown'
  ) {
    invalidProjection();
  }
}

export function validateBootstrapCurrentSelf(value) {
  try {
    const root = readFixedDataObject(value, ROOT_KEYS);
    if (root.schema !== 'aku.x.bootstrap-current-self.v1') invalidProjection();

    const diagnostic = requireEnum(root.diagnostic, DIAGNOSTICS);
    const bootstrapState = requireEnum(root.bootstrapState, BOOTSTRAP_STATES);
    const sessionState = requireEnum(root.sessionState, BOOTSTRAP_STATES);
    const idValues = readFixedDataObject(root.idFields, ID_KEYS);
    const authValues = readFixedDataObject(root.authFields, AUTH_KEYS);
    const entityValues = readFixedDataObject(root.entityChecks, ENTITY_KEYS);

    const idFields = {
      user_id: requireEnum(idValues.user_id, ID_KINDS),
      user_id_str: requireEnum(idValues.user_id_str, ID_KINDS),
    };
    const authFields = {
      loggedIn: requireEnum(authValues.loggedIn, AUTH_KINDS),
      isAuthenticated: requireEnum(authValues.isAuthenticated, AUTH_KINDS),
      authenticated: requireEnum(authValues.authenticated, AUTH_KINDS),
      isActive: requireEnum(authValues.isActive, AUTH_KINDS),
    };
    const entityChecks = {
      normalizedUsers: requireEnum(entityValues.normalizedUsers, ENTITY_KINDS),
      rootUsers: requireEnum(entityValues.rootUsers, ENTITY_KINDS),
    };

    const candidateCount = root.stableIdStringCandidates;
    if (!Number.isInteger(candidateCount) || candidateCount < 0 || candidateCount > 2) {
      invalidProjection();
    }
    const idStringCount = Number(idFields.user_id === 'string') + Number(idFields.user_id_str === 'string');
    if (candidateCount !== idStringCount) invalidProjection();

    const sessionIdsAgree = root.sessionIdsAgree;
    if (candidateCount < 2) {
      if (sessionIdsAgree !== null) invalidProjection();
    } else if (typeof sessionIdsAgree !== 'boolean') {
      invalidProjection();
    }
    if (sessionIdsAgree === true || sessionIdsAgree === false) {
      if (candidateCount !== 2 || idStringCount !== 2) invalidProjection();
    }

    if (root.canAuthorize !== false || root.continuityProven !== false) invalidProjection();

    const sessionIdEligible =
      candidateCount > 0 &&
      [idFields.user_id, idFields.user_id_str].every((kind) => kind === 'absent' || kind === 'string') &&
      sessionIdsAgree !== false;
    const hasEntityMatch =
      entityChecks.normalizedUsers === 'match' || entityChecks.rootUsers === 'match';
    if (hasEntityMatch && !sessionIdEligible) invalidProjection();
    if (!sessionIdEligible && (entityChecks.normalizedUsers !== 'unknown' || entityChecks.rootUsers !== 'unknown')) {
      invalidProjection();
    }
    if (sessionIdEligible && (entityChecks.normalizedUsers === 'unknown' || entityChecks.rootUsers === 'unknown')) {
      invalidProjection();
    }

    if (sessionState !== 'object') {
      if (
        idFields.user_id !== 'absent' ||
        idFields.user_id_str !== 'absent' ||
        Object.values(authFields).some((kind) => kind !== 'absent') ||
        candidateCount !== 0 ||
        entityChecks.normalizedUsers !== 'unknown' ||
        entityChecks.rootUsers !== 'unknown'
      ) {
        invalidProjection();
      }
    }

    if (diagnostic === 'wrong_page' || diagnostic === 'bootstrap_missing' || diagnostic === 'inspection_failed') {
      requireDefaultProjection({
        bootstrapState,
        sessionState,
        idFields,
        stableIdStringCandidates: candidateCount,
        sessionIdsAgree,
        authFields,
        entityChecks,
      });
    } else if (diagnostic === 'projected') {
      if (bootstrapState !== 'object' || (sessionState !== 'absent' && sessionState !== 'object')) {
        invalidProjection();
      }
    } else if (diagnostic === 'bootstrap_invalid') {
      if (!['invalid', 'accessor'].includes(bootstrapState) && !['invalid', 'accessor'].includes(sessionState)) {
        invalidProjection();
      }
    }

    const sanitized = {
      schema: 'aku.x.bootstrap-current-self.v1',
      diagnostic,
      bootstrapState,
      sessionState,
      idFields,
      stableIdStringCandidates: candidateCount,
      sessionIdsAgree,
      authFields,
      entityChecks,
      canAuthorize: false,
      continuityProven: false,
    };
    if (Buffer.byteLength(JSON.stringify(sanitized), 'utf8') > 2048) invalidProjection();
    return sanitized;
  } catch {
    return invalidProjection();
  }
}
