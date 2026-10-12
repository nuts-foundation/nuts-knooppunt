export function journeyTargets(event) {
  switch (event?.gf) {
    case 'pseudonym': return '#js-prs, #jsp-prs';
    case 'localization': return '#js-nvi, #jsp-share';
    case 'addressing': return '#js-mcsd, #jsp-address';
    case 'consent':
      // A closed question travels from the source's authorization and policy
      // to Mitz; a change is the patient at Mitz and travels nowhere; a
      // subscription and the notification it leads to share one route, in
      // opposite directions.
      if (event.purpose === 'consent-check') return '#js-mitz, #js-source, #js-auth-server, #jsp-mitzcheck';
      if (event.purpose === 'consent-change') return '#js-mitz';
      return '#js-mitz, #jsp-mitzsub';
    case 'authentication': return '#js-nuts, #jsp-keyreq';
    case 'authorization': return isTokenRequest(event) ? '#js-nuts, #js-source, #js-auth-server, #jsp-access' : '#js-nuts, #jsp-keyreq';
    case 'exchange': return '#js-source, #jsp-exchange';
    default: return '';
  }
}

export const isTokenRequest = event => event.gf === 'authorization' &&
  event.request?.method === 'POST' && event.request?.path?.split('?')[0].endsWith('/request-service-access-token');

const sourceSearches = {
  Patient: { label: 'Find patient at De Zonnebloem', result: 'the patient search result' },
  AllergyIntolerance: { label: 'Retrieve allergies', result: 'the allergy search result' },
  Condition: { label: 'Retrieve conditions', result: 'the condition search result' },
  MedicationRequest: { label: 'Retrieve medication', result: 'the medication search result' },
};
const sourceSearch = event => event.gf === 'exchange' && Object.hasOwn(sourceSearches, event.resourceType)
  ? sourceSearches[event.resourceType] : null;

export function accessPresentation(stage) {
  const events = stage?.events || [];
  const tokenRequest = events.some(isTokenRequest);
  const exchange = events.some(event => event.gf === 'exchange');
  const tokenAttached = exchange && events.every(event => event.gf === 'exchange' && event.request?.tokenAttached === true);
  const tokenReceived = events.some(event => isTokenRequest(event) && event.outcome === 'ok' &&
    event.response?.status === 200 && event.response.tokenReceived === true);
  const dataReturned = exchange && events.every(event => event.gf === 'exchange' && event.outcome === 'ok' &&
    event.response?.status === 200 && event.response.body?.resourceType === 'Bundle');
  const refused = events.some(event => event.outcome === 'deny');
  const failed = events.some(event => event.outcome === 'error');
  let description = '';
  if (tokenRequest) description = tokenReceived
    ? 'De Zonnebloem issued an access key, received through Plataan’s access service.'
    : refused ? 'The access-key request was refused.'
      : failed ? 'The access-key request failed.' : 'Access request completed; receipt of a key was not established.';
  const search = sourceSearch(events[0] || {});
  if (exchange) description = dataReturned ? `De Zonnebloem returned ${search?.result || 'a data response'}.`
    : refused ? 'De Zonnebloem refused this request. The vault stays closed.'
      : failed ? 'The source request failed. The vault stays closed.' : 'A data response was not established. The vault stays closed.';
  const queryDescription = events[0]?.resourceType === 'Patient'
    ? 'Find De Zonnebloem’s patient reference. POST sends the search identifier in the body.'
    : search ? `${search.label} using De Zonnebloem’s patient reference.` : 'Request source data.';
  const requestDescription = exchange ? `${tokenAttached ? 'Access key attached' : 'Key attachment not established'} · ${queryDescription}` : '';
  return { tokenRequest, tokenReceived, tokenAttached, dataReturned, exchange, visible: tokenRequest || exchange, description, requestDescription };
}

export const actionLabels = {
  record: 'Record status', 'share-preview': 'Sharing preview', share: 'Share patient',
  subscribe: 'Subscribe to consent', discover: 'Find data sources', retrieve: 'Retrieve source data', authorize: 'Authorize access',
  consent: 'Consent changed',
};
export const purposeLabels = {
  status: 'Background status reads', 'registration-cleanup': 'Check existing registrations',
  registration: 'Register resource categories', 'subscription-check': 'Check Mitz subscription',
  subscription: 'Submit Mitz subscription', localization: 'Locate available records',
  addressing: 'Find service address', authorization: 'Request access key',
  exchange: 'Request source data', pseudonymization: 'Pseudonymize identifier',
  'consent-check': 'Consent check at Mitz', 'consent-notification': 'Mitz notification received',
  'consent-change': 'Patient changes consent in Mitz',
};
export const callIdentity = event => event.callId || `event-${event.seq}`;
export const callCount = (events, kind = '') => `${events.length} ${kind}call${events.length === 1 ? '' : 's'}`;
const eventTime = event => Date.parse(event.ts) || event.seq;
export const isBackgroundAction = action => ['record', 'share-preview'].includes(action);

const aggregate = events => ({
  outcome: events.some(event => event.outcome === 'error') ? 'error'
    : events.some(event => event.outcome === 'deny') ? 'deny' : 'ok',
  durationMs: events.reduce((total, event) => total + (event.durationMs || 0), 0),
});

export function subscriptionResult(event) {
  if (event.gf !== 'consent' || event.outcome !== 'ok') return '';
  if (event.purpose === 'subscription') return 'Mitz subscription submitted';
  if (event.purpose !== 'subscription-check') return '';
  const body = event.response?.body;
  return body?.resourceType === 'Bundle' && body.entry?.some(entry => entry.resource?.resourceType === 'Subscription')
    ? 'Existing Mitz subscription returned' : 'Mitz subscription checked';
}

function actionSummary(action) {
  if (action.action !== 'share') return callCount(action.events, 'observed ');
  const successful = action.events.filter(event => event.outcome === 'ok');
  const removed = successful.filter(event => event.gf === 'localization' && event.request?.method === 'DELETE').length;
  const submitted = successful.filter(event => event.purpose === 'registration' && event.request?.method === 'POST').length;
  const parts = [];
  if (removed) parts.push(`${removed} registration${removed === 1 ? '' : 's'} removed`);
  if (submitted) parts.push(`${submitted} registration${submitted === 1 ? '' : 's'} submitted`);
  const subscription = successful.find(event => event.purpose === 'subscription') || successful.find(event => event.purpose === 'subscription-check');
  if (subscription) parts.push(subscriptionResult(subscription));
  return parts.filter(Boolean).join(' · ') || callCount(action.events, 'observed ');
}

export const consentChangeLabel = event => event.request?.body?.decision === 'Permit'
  ? 'Patient gives consent again in Mitz' : 'Patient withdraws consent in Mitz';

const consentChecksOf = (action, call) => action.events.filter(event =>
  event.purpose === 'consent-check' && event.parentCallId && event.parentCallId === call.callId);
const checkState = event => event.outcome === 'ok' ? 'permit' : event.outcome === 'deny' ? 'deny' : 'error';
const returnedData = event => event.outcome === 'ok' && event.response?.status === 200 && event.response.body?.resourceType === 'Bundle';
const sourceCallsOf = stage => stage.events.filter(event => event.gf === 'exchange');

// refusalDescription says, beside a refusal by Mitz, what the source's own
// responses show. That no data left the source is said only when every request
// of the stage was refused: a page served before the refusal did leave it.
function refusalDescription(calls) {
  if (!calls.length) return 'Mitz answered deny.';
  const refused = calls.filter(call => call.outcome === 'deny').length;
  if (refused === calls.length) return 'Mitz answered deny. De Zonnebloem refused the request and no data left the source.';
  const returned = calls.filter(returnedData).length, failed = calls.filter(call => call.outcome === 'error').length;
  const others = [returned && `${returned} returned data`, failed && `${failed} failed`].filter(Boolean);
  return `Mitz answered deny. De Zonnebloem refused ${refused} of ${calls.length} requests${others.length ? `; ${others.join(', ')}` : ''}.`;
}

// consentPresentation says what the map may show about consent for one stage,
// from the answers attached to its source calls and from nothing else. A call
// with no answer attached was not observed; that is never shown as "not asked".
export function consentPresentation(stage, action) {
  if (!stage || !action) return null;
  if (!stage.events.some(event => event.gf === 'exchange')) return null;
  const perCall = sourceCallsOf(stage).map(call => {
    const states = consentChecksOf(action, call).map(checkState);
    return states.includes('deny') ? 'deny' : states.includes('error') ? 'error' : states.length ? 'permit' : 'unobserved';
  });
  const state = perCall.includes('deny') ? 'deny' : perCall.includes('error') ? 'error'
    : perCall.includes('unobserved') ? 'unobserved' : 'permit';
  const permits = perCall.filter(value => value === 'permit').length;
  const mixed = state !== 'permit' && permits > 0 ? ` (permit on ${permits} of ${perCall.length} requests)` : '';
  const legend = { permit: 'Mitz: permit', deny: 'Mitz: deny', error: 'Mitz: no answer', unobserved: 'not observed' }[state] + mixed;
  return { state, legend, description: state === 'deny' ? refusalDescription(sourceCallsOf(stage)) : '' };
}

// lockPresentation says what each of the four locks on the source's vault may
// show while a stage of an action is on the map. All four are decided when a
// request arrives at the source with the key, and every request is checked
// anew: a retrieval of four requests opens the locks four times.
// - identity and role open when the source accepted the key: its policy
//   enforcement point reads both from the key and hands them to its policy;
// - consent is Mitz's answer for that request;
// - query opens when the source served it.
// Issuing the key opens nothing: the authorization server accepted the
// presentation, and the key that carries the result has yet to reach the vault.
// plays marks what the stage decides and therefore shows happening: the stage
// of a request plays every lock it opens or refuses, each time, also when the
// action is replayed.
// elapsed is how many seconds the stage has played. A lock the stage changes
// shows what it showed before until the stage has reached that change
// (stageTimeline), so the legend never runs ahead of the map. For a request
// "before" is the vault as no request left it: four shut locks. Without elapsed
// the result is the stage's final state, which the map takes and delays itself.
export function lockPresentation(stage, action, elapsed = Infinity) {
  const stages = action?.stages || [];
  const at = stages.findIndex(item => item === stage || (stage?.id && item.id === stage.id));
  const locks = locksAfter(at < 0 ? (stage ? [stage] : []) : stages.slice(0, at + 1), action);
  const request = isRequest(stage);
  const earlier = at > 0 ? stages.slice(0, at) : [];
  const before = locksAfter(request ? earlier.filter(item => !isRequest(item)) : earlier, action);
  const timeline = stage ? stageTimeline(stage) : {};
  for (const [name, lock] of Object.entries(locks)) {
    lock.plays = request && lock.state !== 'closed';
    const changes = lock.state !== before[name].state || lock.legend !== before[name].legend;
    if (changes && elapsed < (timeline[name] ?? 0)) Object.assign(lock, { state: before[name].state, legend: awaiting[name] || before[name].legend, plays: false });
  }
  return locks;
}
const awaiting = { consent: 'awaiting Mitz’s answer', query: 'awaiting the source’s answer' };
const isRequest = stage => !!stage?.events?.some(event => event.gf === 'exchange');

// The vault is open while the query lock is and no lock is refused: data that
// returned did return, also without an observed answer from Mitz, and a
// refusal shuts the vault as soon as it is known.
export const vaultOpen = locks => locks?.query.state === 'open' && !['deny', 'error'].includes(locks.consent.state);

// keyAt says what the source's answers show about the key a request carried.
// A 200 and a 403 come after its policy enforcement point read the key and
// handed identity and role to its policy (pep/nginx/js/authorize.js); that is
// read from the status, whether or not the body could be captured. A 401 to a
// request that carried a key is authentication refused: the status does not say
// which part of the source refused it. Anything else establishes nothing.
function keyAt(request) {
  const calls = request.events.filter(event => event.gf === 'exchange');
  if (calls.some(call => call.request?.tokenAttached !== true)) return { state: 'closed', legend: 'no key attached' };
  if (calls.some(call => call.response?.status === 401)) return { state: 'deny', legend: 'authentication refused' };
  if (calls.every(call => [200, 403].includes(call.response?.status))) return { state: 'open', legend: 'read from the key' };
  return { state: 'closed', legend: 'not established' };
}

function locksAfter(shown, action) {
  const keyRequested = shown.some(item => accessPresentation(item).tokenRequest);
  const keyIssued = shown.some(item => accessPresentation(item).tokenReceived);
  const request = shown.filter(isRequest).at(-1);
  const credential = request ? keyAt(request)
    : { state: 'closed', legend: keyIssued ? 'carried by the key' : keyRequested ? 'no key issued' : 'no key yet' };
  const answer = shown.map(item => consentPresentation(item, action)).filter(Boolean).at(-1);
  const consent = answer
    ? { state: { permit: 'open', deny: 'deny', error: 'error', unobserved: 'closed' }[answer.state], legend: answer.legend }
    : { state: 'closed', legend: 'not observed' };
  const query = !request ? { state: 'closed', legend: 'no request yet' }
    : accessPresentation(request).dataReturned ? { state: 'open', legend: 'allowed · request served' }
      : { state: 'closed', legend: 'not served' };
  return { identity: { ...credential }, role: { ...credential }, consent, query };
}

// stageTimeline says how long a stage plays and when, in seconds into it, each
// thing it changes shows: the packets, Mitz's shield, the locks, the vault and
// the moment its outcome is settled. The map delays its animations by these
// values and the legend follows them through the playback, so both tell the
// stage in the order it happened. pending is what the status line says until
// the outcome is settled.
//
// A source request plays in the order it happens: it arrives with the key, the
// source reads identity and role from the key, its policy asks Mitz and Mitz
// answers (when that was observed), the query lock and the vault follow, and
// only then does the source answer. Nothing changes before the request arrived.
export function stageTimeline(stage) {
  if (accessPresentation(stage).tokenRequest) return {
    seconds: 2.5, request: { delay: 0, duration: 1.1 }, response: { delay: 1.2, duration: 1.1 }, identity: 1.2, role: 1.2, settled: 1.2,
    pending: 'De Plataan presents its credentials to De Zonnebloem’s authorization server.',
  };
  if (!isRequest(stage)) return {
    seconds: 2.5, request: { delay: 0, duration: 1.1 }, response: { delay: 1.2, duration: 1.1 }, settled: 0, pending: '',
  };
  if (stage.checks?.length) return {
    seconds: 4.3, request: { delay: 0, duration: 0.9 }, identity: 0.9, role: 1.1,
    question: { delay: 1.3, duration: 0.65 }, shield: 1.95, answer: { delay: 2.05, duration: 0.65 },
    consent: 2.7, query: 2.95, vault: 3.15, response: { delay: 3.15, duration: 1.05 }, settled: 3.15,
    pending: 'De Zonnebloem checks this request before it answers: it reads the key and its policy asks Mitz.',
  };
  return {
    seconds: 2.5, request: { delay: 0, duration: 0.9 }, identity: 0.9, role: 1.05, consent: 1.2, query: 1.2, vault: 1.35,
    response: { delay: 1.35, duration: 1.1 }, settled: 1.35, pending: 'De Zonnebloem checks this request before it answers.',
  };
}

export function buildActionGroups(events) {
  const calls = new Map(events.filter(event => event.callId).map(event => [event.callId, event]));
  const actions = new Map();
  for (const event of [...events].sort((a, b) => eventTime(a) - eventTime(b) || a.seq - b.seq)) {
    const parent = calls.get(event.parentCallId);
    const id = parent?.actionId || event.actionId || 'legacy';
    const action = parent?.action || event.action || 'observed';
    if (!actions.has(id)) actions.set(id, { id, action, label: actionLabels[action] || 'Observed activity', events: [], groups: [], stages: [], time: Infinity });
    const group = actions.get(id);
    group.events.push(event);
    if (!event.parentCallId) group.time = Math.min(group.time, eventTime(event));
  }
  for (const action of actions.values()) {
    const primary = action.events.filter(event => !calls.has(event.parentCallId));
    const groups = [];
    for (const event of primary) {
      const purpose = event.purpose || event.gf;
      const removing = purpose === 'registration-cleanup' && event.request?.method === 'DELETE';
      const search = sourceSearch(event);
      const key = `${purpose}:${event.gf}:${search ? event.resourceType : event.request?.method}`;
      let group = groups.at(-1);
      if (group?.key !== key) {
        group = { id: `${action.id}:${callIdentity(event)}`, key, purpose,
          label: purpose === 'consent-change' ? consentChangeLabel(event)
            : search?.label || (removing ? 'Remove existing registrations' : purposeLabels[purpose] || purpose),
          events: [], time: eventTime(event) };
        groups.push(group);
      }
      group.events.push(event);
    }
    for (const group of groups) {
      const parentIds = new Set(group.events.map(callIdentity));
      const children = action.events.filter(event => parentIds.has(event.parentCallId));
      if (group.purpose === 'status') group.events.push(...children);
      else {
        // PRS is called before the NVI request it serves and is a stage before
        // it. Mitz is asked while the source handles a request, after the
        // request arrived and before it is answered: those checks belong to the
        // request's own stage, which plays them in that order.
        const prs = children.filter(child => child.purpose !== 'consent-check');
        if (prs.length) action.groups.push({
          id: `${group.id}:prs`, purpose: 'pseudonymization', label: purposeLabels.pseudonymization,
          events: prs, time: group.time, ...aggregate(prs),
        });
        group.checks = children.filter(child => child.purpose === 'consent-check');
      }
      Object.assign(group, aggregate(group.events));
      action.groups.push(group);
    }
    // A consent action is ordered by cause, not by completion: its notification
    // completes while its change is still waiting on the mock.
    if (action.action === 'consent') {
      const rank = group => ({ 'consent-change': 0, 'consent-notification': 1 }[group.purpose] ?? 2);
      action.groups = action.groups.map((group, index) => ({ group, index }))
        .sort((a, b) => rank(a.group) - rank(b.group) || a.index - b.index).map(({ group }) => group);
    }
    action.stages = isBackgroundAction(action.action) ? [] : action.groups.filter(group => group.purpose !== 'status');
    if (!Number.isFinite(action.time)) action.time = Math.min(...action.events.map(eventTime));
    Object.assign(action, aggregate(action.events));
    action.summary = actionSummary(action);
    action.prsUnavailable = action.stages.some(group => group.events.some(event => event.gf === 'localization')) &&
      !action.stages.some(group => group.purpose === 'pseudonymization');
  }
  return [...actions.values()].sort((a, b) => a.time - b.time);
}
