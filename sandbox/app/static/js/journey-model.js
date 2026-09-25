export function journeyTargets(event) {
  switch (event?.gf) {
    case 'pseudonym': return '#js-prs, #jsp-prs';
    case 'localization': return '#js-nvi, #jsp-share';
    case 'addressing': return '#js-mcsd, #jsp-address';
    case 'consent': return '#js-mitz, #jsp-mitzsub';
    case 'authentication': return '#js-nuts, #jsp-keyreq';
    case 'authorization': return isTokenRequest(event) ? '#js-nuts, #js-source, #js-auth-server, #jsp-access' : '#js-nuts, #jsp-keyreq';
    case 'exchange': return '#js-source, #jsp-exchange';
    default: return '';
  }
}

export const isTokenRequest = event => event.gf === 'authorization' &&
  event.request?.method === 'POST' && event.request?.path?.split('?')[0].endsWith('/request-service-access-token');

const sourceSearches = {
  Patient: { label: 'Find patient at Sunflower', result: 'the patient search result' },
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
    ? 'Sunflower issued an access key, received through Plataan’s access service.'
    : refused ? 'The access-key request was refused.'
      : failed ? 'The access-key request failed.' : 'Access request completed; receipt of a key was not established.';
  const search = sourceSearch(events[0] || {});
  if (exchange) description = dataReturned ? `Sunflower returned ${search?.result || 'a data response'}.`
    : refused ? 'Sunflower refused this request. The vault stays closed.'
      : failed ? 'The source request failed. The vault stays closed.' : 'A data response was not established. The vault stays closed.';
  const queryDescription = events[0]?.resourceType === 'Patient'
    ? 'Find Sunflower’s patient reference. POST sends the search identifier in the body.'
    : search ? `${search.label} using Sunflower’s patient reference.` : 'Request source data.';
  const requestDescription = exchange ? `${tokenAttached ? 'Access key attached' : 'Key attachment not established'} · ${queryDescription}` : '';
  return { tokenRequest, tokenReceived, tokenAttached, dataReturned, exchange, visible: tokenRequest || exchange, description, requestDescription };
}

export const actionLabels = {
  record: 'Record status', 'share-preview': 'Sharing preview', share: 'Share patient',
  subscribe: 'Subscribe to consent', discover: 'Find data sources', retrieve: 'Retrieve source data', authorize: 'Authorize access',
};
export const purposeLabels = {
  status: 'Background status reads', 'registration-cleanup': 'Check existing registrations',
  registration: 'Register resource categories', 'subscription-check': 'Check Mitz subscription',
  subscription: 'Submit Mitz subscription', localization: 'Locate available records',
  addressing: 'Find service address', authorization: 'Request access key',
  exchange: 'Request source data', pseudonymization: 'Pseudonymize identifier',
};
export const callIdentity = event => event.callId || `event-${event.seq}`;
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
  if (action.action !== 'share') return `${action.events.length} observed calls`;
  const successful = action.events.filter(event => event.outcome === 'ok');
  const removed = successful.filter(event => event.gf === 'localization' && event.request?.method === 'DELETE').length;
  const submitted = successful.filter(event => event.purpose === 'registration' && event.request?.method === 'POST').length;
  const parts = [];
  if (removed) parts.push(`${removed} registration${removed === 1 ? '' : 's'} removed`);
  if (submitted) parts.push(`${submitted} registration${submitted === 1 ? '' : 's'} submitted`);
  const subscription = successful.find(event => event.purpose === 'subscription') || successful.find(event => event.purpose === 'subscription-check');
  if (subscription) parts.push(subscriptionResult(subscription));
  return parts.filter(Boolean).join(' · ') || `${action.events.length} observed calls`;
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
          label: search?.label || (removing ? 'Remove existing registrations' : purposeLabels[purpose] || purpose),
          events: [], time: eventTime(event) };
        groups.push(group);
      }
      group.events.push(event);
    }
    for (const group of groups) {
      const parentIds = new Set(group.events.map(callIdentity));
      const children = action.events.filter(event => parentIds.has(event.parentCallId));
      if (group.purpose === 'status') group.events.push(...children);
      else if (children.length) action.groups.push({
        id: `${group.id}:prs`, purpose: 'pseudonymization', label: purposeLabels.pseudonymization,
        events: children, time: group.time, ...aggregate(children),
      });
      Object.assign(group, aggregate(group.events));
      action.groups.push(group);
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
