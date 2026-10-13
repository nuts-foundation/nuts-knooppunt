import { createJourneyStrip } from './journey-strip.js';
import { journeyTargets, subscriptionResult, accessPresentation, callCount, consentPresentation, lockPresentation, stageTimeline, sourcePresentation, discoveryPresentation } from './journey-model.js';
import { createPlayback } from './event-playback.js';

export function createEventFeed(runId, { onStep, onGap, onEnd } = {}) {
  let lastSeq = 0;
  let ended = false;
  return {
    step(event) {
      if (ended || event?.runId !== runId || !Number.isSafeInteger(event.seq) ||
          event.seq <= lastSeq || !journeyTargets(event) ||
          !['ok', 'deny', 'error'].includes(event.outcome)) return false;
      lastSeq = event.seq;
      onStep?.(event);
      return true;
    },
    gap({ firstSeq } = {}) {
      if (ended || !Number.isSafeInteger(firstSeq) || firstSeq < 1) return;
      lastSeq = firstSeq - 1;
      onGap?.(firstSeq);
    },
    end() {
      if (ended) return;
      ended = true;
      onEnd?.();
    },
  };
}

const labels = {
  pseudonym: 'Pseudonymization', localization: 'Localization', addressing: 'Addressing',
  authentication: 'Authentication', consent: 'Consent', authorization: 'Authorization', exchange: 'Exchange',
};

export function eventSummary(event) {
  const subject = event.gf === 'exchange' ? 'Source request' : `${labels[event.gf] || 'Step'} request`;
  if (event.purpose === 'consent-check') return event.outcome === 'error' ? 'Consent request failed'
    : `Mitz answered ${event.response?.consentDecision === 'Permit' ? 'permit' : 'deny'}`;
  if (event.purpose === 'consent-notification') return 'Mitz notification received';
  if (event.purpose === 'consent-change' && event.outcome === 'ok') return event.request?.body?.decision === 'Permit'
    ? 'Consent given again in Mitz' : 'Consent withdrawn in Mitz';
  if (event.outcome === 'deny') return `${subject} refused`;
  if (event.outcome === 'error') return `${subject} failed`;
  switch (event.gf) {
    case 'localization':
      if (event.request?.method === 'DELETE') return 'Localization record removed';
      if (event.request?.method === 'POST' && !event.request?.path?.split('?')[0].endsWith('/_search')) {
        return 'Localization record submitted';
      }
      return 'Referral index queried';
    case 'addressing': return 'Directory queried';
    case 'authorization':
      return event.request?.path?.endsWith('/introspect') ? 'Access token checked' : 'Service access token requested';
    case 'consent': return event.request?.method === 'POST' ? 'Consent subscription submitted' : 'Consent subscription checked';
    case 'exchange': return 'Source request completed';
    default: return `${labels[event.gf] || 'Step'} request completed`;
  }
}

// What a submitted form may take before its navigation is considered not to
// have happened: the server's write timeout, which no pending request outlives.
const submitGraceMs = 60_000;

// shouldReload decides whether a record page is behind the record. The page
// carries the revision it was rendered at; every snapshot carries the current
// one. Different means reload, which covers a change between rendering and
// connecting, a reconnect, a missed event and a change that produced no event.
// After a form on the page was submitted the comparison pauses, so the consent
// action's own notification does not cancel its redirect.
export function shouldReload({ pageRevision, snapshotRevision, submittedAt, now }) {
  if (pageRevision === undefined || pageRevision === null || snapshotRevision === undefined) return false;
  if (submittedAt !== undefined && now - submittedAt < submitGraceMs) return false;
  return String(pageRevision) !== String(snapshotRevision);
}

export function mountViewer(dock) {
  const runId = dock.dataset.runId;
  if (!runId) return;
  const list = dock.querySelector('#gf-viewer-steps');
  const status = dock.querySelector('#gf-stream-status');
  const playbackStatus = dock.querySelector('#gf-playback-status');
  const title = dock.querySelector('#hd-title');
  const stageCaption = dock.querySelector('#gf-stage-caption');
  const stageDetail = dock.querySelector('#gf-stage-detail');
  const contract = dock.querySelector('#gf-access-contract');
  const contractStatus = dock.querySelector('#gf-contract-status');
  const pause = dock.querySelector('#gf-pause');
  const replay = dock.querySelector('#gf-replay');
  const skip = dock.querySelector('#gf-skip');
  const live = dock.querySelector('#gf-live');
  const checks = [...(dock.querySelectorAll?.('.gf-check') || [])];
  const pageRevision = document.querySelector?.('[data-record-revision]')?.dataset.recordRevision;
  // The revision of the latest snapshot is kept: the comparison is made again
  // when a submission's grace ends and when the page is shown again, and
  // neither of those comes with a snapshot of its own.
  let submittedAt, latestRevision, graceTimer;
  const compare = () => {
    if (!shouldReload({ pageRevision, snapshotRevision: latestRevision, submittedAt, now: Date.now() })) return false;
    location.reload();
    return true;
  };
  const journey = createJourneyStrip(dock.querySelector('.dock-map svg'));
  const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
  const source = new EventSource(`/demo/runs/${encodeURIComponent(runId)}/events`);
  const listeners = new AbortController();
  const storageKey = 'gf-viewer-playback';
  let saved;
  try {
    const value = JSON.parse(sessionStorage.getItem(storageKey));
    if (value?.runId === runId) saved = value;
  } catch { /* Storage can be unavailable in private browser contexts. */ }
  let hasGap = false, ended = false, prsEvidence = true;
  const openDetails = new Set();
  const knownDetails = new Set();
  let activeFunctional, activeTechnical, activeCalls = new Set();
  let lastFollowStage = null, lastFollowMode = false, lastFollowOpen = false, followChanged = false;

  function node(tag, className, text) {
    const element = document.createElement(tag);
    element.className = className;
    if (text !== undefined) element.textContent = text;
    return element;
  }
  function details(key, className, summary, initiallyOpen = false) {
    const element = node('details', className);
    element.dataset.key = key;
    element.open = openDetails.has(key) || (initiallyOpen && !knownDetails.has(key));
    knownDetails.add(key);
    element.append(node('summary', '', summary));
    return element;
  }
  const outcomeLabel = outcome => ({ ok: 'Completed', deny: 'Refused', error: 'Failed' }[outcome]);
  const timeLabel = time => new Date(time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  const serviceName = gf => ({ pseudonym: 'PRS', localization: 'NVI', consent: 'Mitz', addressing: 'mCSD / LRZA',
    authentication: 'Plataan access service', authorization: 'Plataan access service', exchange: 'Source' }[gf] || 'Service');
  function result(group) {
    return `${outcomeLabel(group.outcome)} · ${callCount(group.events)} · ${Math.round(group.durationMs * 100) / 100} ms summed call time`;
  }
  function responseLabel(event) { return event.response?.status ? `HTTP ${event.response.status}` : 'No HTTP response'; }
  const isCurrentStage = (group, state) => group.events.some(event => state.stage?.events.includes(event));
  function callDetails(event, children, current) {
    const category = event.resourceType ? ` · ${event.resourceType}` : '';
    const ownCurrent = activeCalls.has(event);
    current ||= ownCurrent;
    const call = details(`call:${event.seq}`, `gf-call gf-step-${event.outcome}${current ? ' gf-stage-current' : ''}`, '');
    call.children[0].append(
      node('span', 'gf-call-title', `${serviceName(event.gf)} · ${eventSummary(event)}${category}`),
      node('span', 'gf-call-meta', `#${event.seq} · ${responseLabel(event)} · ${event.durationMs} ms`),
      node('code', 'gf-call-path', `${event.request?.method || ''} ${event.request?.path || ''}`),
    );
    if (ownCurrent && !activeTechnical) activeTechnical = call.children[0];
    const exchange = node('div', 'gf-http-exchange');
    for (const direction of ['request', 'response']) {
      const message = event[direction];
      const section = node('section', `gf-http-${direction}`);
      section.append(node('h5', '', direction === 'request' ? 'Request →' : 'Response ←'));
      section.append(node('p', 'gf-http-status', direction === 'request'
        ? `${event.request?.method || ''} ${event.request?.path || ''}` : responseLabel(event)));
      if (direction === 'request' && event.request?.tokenAttached === true) section.append(node('p', 'gf-step-result', 'Access key attached in the Authorization header.'));
      if (direction === 'request' && event.gf === 'exchange' && event.resourceType === 'Patient' && event.request?.method === 'POST') section.append(node('p', 'gf-step-result', 'Patient search: the identifier is sent in the request body to keep it out of the URL. The returned patient reference scopes the later clinical queries.'));
      if (direction === 'response' && event.response?.tokenReceived) section.append(node('p', 'gf-step-result', 'Captured evidence: access token received. The token value is not retained.'));
      if (direction === 'response' && subscriptionResult(event)) section.append(node('p', 'gf-step-result', subscriptionResult(event)));
      if (message?.headers && Object.keys(message.headers).length) {
        section.append(node('div', 'gf-http-label', 'Headers'), node('pre', '', JSON.stringify(message.headers, null, 2)));
      }
      section.append(node('div', 'gf-http-label', 'Body'));
      section.append(message?.body == null ? node('p', 'gf-step-result', 'No body')
        : node('pre', '', typeof message.body === 'string' ? message.body : JSON.stringify(message.body, null, 2)));
      exchange.append(section);
    }
    call.append(exchange);
    if (event.ts) call.append(node('p', 'gf-step-result', `Completed at ${timeLabel(event.ts)}`));
    if (children.length) {
      const internal = node('div', 'gf-internal');
      const consentChecks = children.every(child => child.purpose === 'consent-check');
      internal.append(node('p', 'gf-http-label', consentChecks
        ? 'Closed question the source asked Mitz for this request'
        : 'Internal PRS calls made within this request'));
      if (consentChecks) internal.append(node('p', 'gf-step-result',
        'Recorded by the mock Mitz and read from its decision log by trace id. De Plataan cannot see this call; the viewer shows it for the demo.'));
      for (const child of children) internal.append(callDetails(child, [], false));
      if (followChanged && document.body.classList.contains('hood-tech') && children.some(child => activeCalls.has(child))) call.open = true;
      call.append(internal);
    }
    return call;
  }
  function technicalCalls(action, state) {
    const container = node('div', 'gf-technical');
    container.append(node('p', 'gf-step-result', `${result(action)}. Calls below are ordered by completion; calls made inside a request (PRS, the source's question to Mitz) are nested under it.`));
    const calls = node('ol', 'gf-calls');
    const background = details(`status:${action.id}`, 'gf-background-action', 'Background status reads');
    let backgroundCalls = 0;
    const ids = new Set(action.events.map(event => event.callId).filter(Boolean));
    for (const event of action.events.filter(event => !ids.has(event.parentCallId))) {
      const children = action.events.filter(child => child.parentCallId && child.parentCallId === event.callId);
      const current = state.stage?.events.some(item => item === event || children.includes(item));
      const item = node('li', 'gf-call-item');
      item.append(callDetails(event, children, current));
      if (event.purpose === 'status') { background.append(item); backgroundCalls++; }
      else calls.append(item);
    }
    container.append(calls);
    if (backgroundCalls) container.append(background);
    container.append(node('p', 'hd-note', 'Requests and responses are shown as sent and received. Credentials, JWE and blind_factor values, and BSNs on shared deployments, are replaced by a [redacted: …] marker. A Mitz subscription response is not a consent decision.'));
    return container;
  }
  function functionalStages(action, state) {
    const container = node('div', 'gf-functional');
    const stages = node('ol', 'gf-stages');
    for (const [index, group] of action.stages.entries()) {
      const current = isCurrentStage(group, state);
      const row = node('li', `gf-stage gf-step-${group.outcome}${current ? ' gf-stage-current' : ''}`);
      row.append(node('span', 'gf-stage-number', String(index + 1)));
      const content = node('div', 'gf-stage-content');
      content.append(node('h4', '', group.label));
      const replies = [...new Set(group.events.map(responseLabel))].join(', ');
      content.append(node('p', 'gf-step-result', `${outcomeLabel(group.outcome)} · ${callCount(group.events)} · ${replies}`));
      const subscription = group.events.map(subscriptionResult).find(Boolean);
      if (subscription) content.append(node('p', 'gf-step-result', subscription));
      const categories = [...new Set(group.events.map(event => event.resourceType).filter(Boolean))];
      if (categories.length) content.append(node('p', 'gf-step-result', categories.join(' · ')));
      if (group.purpose === 'pseudonymization') content.append(node('p', 'gf-step-result', 'Internal PRS evidence for the following NVI calls'));
      const discovery = discoveryPresentation(group);
      if (discovery?.description) content.append(node('p', 'gf-step-result', discovery.description));
      const consent = consentPresentation(group, action);
      if (group.checks?.length) content.append(node('p', 'gf-step-result',
        consent.state === 'permit' ? 'Before answering, the source asked Mitz whether this patient consents. Mitz answered yes.'
          : consent.state === 'deny' ? 'The source asked Mitz for this request. Mitz answered no.'
            : 'The source asked Mitz and got no usable answer.'));
      if (group.purpose === 'consent-notification') content.append(node('p', 'gf-step-result',
        group.events.some(event => event.response?.copyRemoved === true)
          ? 'Through De Plataan’s subscription. The data retrieved earlier was removed from the record.'
          : 'Through De Plataan’s subscription. Nothing had to be removed from the record.'));
      if (group.purpose === 'consent-change') content.append(node('p', 'gf-step-result',
        group.events.every(event => event.outcome === 'ok')
          ? 'Simulated: the demo changed the answer in the mock Mitz, standing in for the patient.'
          : 'Simulated: the demo asked the mock Mitz to change the answer, standing in for the patient. The call failed, so this step does not establish a change.'));
      if (current) content.append(node('span', 'gf-stage-now', state.paused ? 'Paused here' : 'Playing this step'));
      const access = accessPresentation(group);
      if (access.requestDescription) content.append(node('p', 'gf-step-result', access.requestDescription));
      if (access.visible) content.append(node('p', 'gf-step-result', access.description));
      row.append(content); stages.append(row);
      if (current && !activeFunctional) activeFunctional = row;
    }
    container.append(stages);
    if (action.prsUnavailable) container.append(node('p', 'hd-note', !prsEvidence
      ? 'This deployment does not trace the Knooppunt’s internal PRS call.'
      : state.status === 'settling' ? 'Waiting briefly for internal PRS evidence…' : 'PRS evidence unavailable for this action.'));
    return container;
  }
  function render(state) {
    const focusedAction = document.activeElement?.dataset?.actionId;
    const technicalMode = document.body.classList.contains('hood-tech');
    const visible = dock.classList.contains('on') && !document.hidden;
    followChanged = !!state.stage && visible && (state.stage.id !== lastFollowStage?.id || technicalMode !== lastFollowMode || !lastFollowOpen);
    activeCalls = new Set(state.stage?.events || []);
    activeFunctional = activeTechnical = null;
    for (const detail of list.querySelectorAll('details[data-key]')) {
      if (detail.open) openDetails.add(detail.dataset.key); else openDetails.delete(detail.dataset.key);
    }
    const retainedKeys = new Set(['background', ...state.groups.flatMap(action => [action.id, `status:${action.id}`]), ...state.events.map(event => `call:${event.seq}`)]);
    for (const key of openDetails) if (!retainedKeys.has(key)) openDetails.delete(key);
    for (const key of knownDetails) if (!retainedKeys.has(key)) knownDetails.delete(key);
    const action = state.action || state.groups.filter(group => group.stages.length).at(-1);
    const timeline = node('ol', 'gf-timeline');
    let restoreFocus;
    for (const item of state.groups.filter(group => group.stages.length)) {
      const selected = action?.id === item.id;
      const entry = node('li', `gf-action gf-step-${item.outcome}${selected ? ' gf-action-selected' : ''}`);
      const button = node('button', 'gf-action-select');
      button.type = 'button'; button.dataset.actionId = item.id;
      button.setAttribute('aria-expanded', String(selected));
      button.setAttribute('aria-label', `Replay ${item.label} at ${timeLabel(item.time)}`);
      const heading = node('span', 'gf-action-heading');
      heading.append(node('strong', '', item.label), node('time', '', timeLabel(item.time)));
      button.append(heading, node('span', 'gf-action-result', `${outcomeLabel(item.outcome)} · ${callCount(item.events)}`),
        node('span', 'gf-action-summary', item.summary), node('span', 'gf-action-play', selected && state.stage ? state.paused ? 'Ⅱ Paused on this action' : '▶ Playing this action' : '▶ Replay this action'));
      button.addEventListener('click', () => player.replay(item.id));
      if (focusedAction === item.id) restoreFocus = button;
      entry.append(button);
      if (selected) entry.append(functionalStages(item, state), technicalCalls(item, state));
      timeline.append(entry);
    }
    const background = details('background', 'gf-background gf-technical', 'Background reads');
    for (const item of state.groups.filter(group => !group.stages.length)) {
      const section = details(item.id, 'gf-background-action', `${item.label} · ${timeLabel(item.time)} · ${callCount(item.events)}`);
      section.append(technicalCalls(item, state)); background.append(section);
    }
    const empty = node('p', 'hd-idle', 'The timeline starts with your first action. Page status reads are available in Technical mode.');
    list.replaceChildren(state.groups.some(group => group.stages.length) ? timeline : empty, background);
    restoreFocus?.focus?.({ preventScroll: true });
    title.textContent = action ? `${action.label} · ${timeLabel(action.time)}` : 'Action timeline';
    const states = { settling: 'Preparing playback · waiting up to 1 s for evidence', playing: `${state.selected ? 'Replaying selected action' : 'Playing latest action'} · slowed for clarity`,
      paused: 'Playback paused', complete: state.selected ? 'Replay complete · Follow latest to resume live playback' : 'Playback complete · select any action to replay', waiting: 'Waiting for an action', ended: 'Playback ended' };
    playbackStatus.textContent = states[state.status];
    if (state.stage) {
      stageCaption.textContent = `Step ${action.stages.findIndex(group => isCurrentStage(group, state)) + 1} of ${action.stages.length} · ${state.stage.label}`;
      const service = serviceName(state.stage.events[0]?.gf);
      const methods = [...new Set(state.stage.events.map(event => event.request?.method))].filter(Boolean).join(', ');
      const replies = [...new Set(state.stage.events.map(responseLabel))].join(', ');
      const returned = state.stage.events.some(event => event.response?.status);
      stageDetail.textContent = `${methods} → ${service} · ${replies}${returned ? ` ← ${service}` : ''}`;
    } else {
      stageCaption.textContent = action ? 'Select an action to replay its observed steps' : 'Your action, step by step';
      stageDetail.textContent = 'Playback is slowed for clarity. HTTP timings remain unchanged.';
    }
    const access = accessPresentation(state.stage);
    const consent = consentPresentation(state.stage, action);
    contract.hidden = !access.visible && !consent;
    // The map delays each change to its moment in the stage. The status line
    // and the legend follow the same timeline through the playback, so neither
    // gives the outcome away before the map has shown it. The map takes its
    // evidence when a stage starts, and the playback starts a stage over when
    // its evidence grows.
    // With reduced motion the map shows a stage's result at once, and so do the
    // legend and the sentences.
    const timing = state.stage ? stageTimeline(state.stage) : null;
    const elapsed = state.stage && !motion.matches ? state.elapsed : Infinity;
    contractStatus.textContent = timing && elapsed < timing.settled ? timing.pending
      : consent && consent.state === 'deny' ? consent.description : access.description;
    const locks = lockPresentation(state.stage, action, elapsed);
    for (const item of checks) {
      const lock = locks[item.dataset.check];
      item.classList.remove('open', 'deny', 'error');
      if (lock.state !== 'closed') item.classList.add(lock.state);
      item.querySelector('em').textContent = lock.legend;
    }
    if (access.visible && !technicalMode) stageDetail.textContent = access.tokenRequest
      ? access.tokenReceived ? 'Request access → De Zonnebloem · source-issued key → Plataan' : 'Request access through Plataan’s access service → De Zonnebloem'
      : access.requestDescription;
    // A discovery step says what it asks until its answer is back, and then
    // what the answer established. The map draws the source as far as the calls
    // shown so far establish it, once the run's events are in.
    const discovery = discoveryPresentation(state.stage);
    if (discovery && !technicalMode) stageDetail.textContent = elapsed < timing.settled ? discovery.pending
      : discovery.description || stageDetail.textContent;
    if (state.delivered) journey.setSource(sourcePresentation({
      stage: state.stage, action, events: state.events, upcoming: state.upcoming, elapsed, partial: hasGap,
    }), !!state.stage);
    pause.textContent = state.paused ? 'Resume' : 'Pause';
    pause.disabled = ended;
    live.disabled = ended || !state.selected;
    replay.disabled = ended || !action;
    skip.disabled = ended || !state.stage;
    dock.classList.toggle('playback-paused', state.paused);
    if (!state.stage) journey.stop();
    if (followChanged) {
      const target = technicalMode ? activeTechnical : activeFunctional;
      if (target) {
        const bounds = target.getBoundingClientRect(), viewport = list.getBoundingClientRect();
        if (bounds.top < viewport.top || bounds.bottom > viewport.bottom) list.scrollTo({
          top: Math.max(0, list.scrollTop + bounds.top - viewport.top - 10), behavior: motion.matches ? 'auto' : 'smooth',
        });
      }
    }
    lastFollowStage = state.stage; lastFollowMode = technicalMode; lastFollowOpen = visible;
  }
  const player = createPlayback({
    cursor: saved, active: dock.classList.contains('on') && !document.hidden,
    onStage: (stage, action) => journey.applyStage(stage, {
      reducedMotion: motion.matches, consent: consentPresentation(stage, action), locks: lockPresentation(stage, action),
    }),
    onChange: render,
    onCursor: cursor => {
      try { sessionStorage.setItem(storageKey, JSON.stringify({ runId, ...cursor })); } catch { /* Optional persistence. */ }
    },
  });
  // Only class changes control visibility; render changes must not retrigger playback.
  let lastOpen = dock.classList.contains('on');
  let lastTechnicalMode = document.body.classList.contains('hood-tech');
  const visibilityObserver = new MutationObserver(() => {
    const open = dock.classList.contains('on');
    if (open !== lastOpen) { lastOpen = open; player.setActive(open && !document.hidden); }
    const technicalMode = document.body.classList.contains('hood-tech');
    if (technicalMode !== lastTechnicalMode) { lastTechnicalMode = technicalMode; render(player.state()); }
  });
  visibilityObserver.observe(dock, { attributes: true, attributeFilter: ['class'] });
  visibilityObserver.observe(document.body, { attributes: true, attributeFilter: ['class'] });
  const visibility = () => player.setActive(dock.classList.contains('on') && !document.hidden);
  document.addEventListener('visibilitychange', visibility, { signal: listeners.signal });
  // Capture phase, so a form inside any part of the page counts. The pause ends
  // by itself: when the grace is over the navigation did not happen, and the
  // page compares with the latest revision it was told, because no further
  // snapshot may ever come. A page shown again, from the back-forward cache or
  // after a navigation that did not happen, compares at once.
  const endPause = () => { clearTimeout(graceTimer); graceTimer = undefined; submittedAt = undefined; compare(); };
  document.addEventListener('submit', () => {
    submittedAt = Date.now();
    clearTimeout(graceTimer);
    graceTimer = setTimeout(endPause, submitGraceMs);
  }, { capture: true, signal: listeners.signal });
  window.addEventListener('pageshow', endPause, { signal: listeners.signal });
  listeners.signal.addEventListener('abort', () => clearTimeout(graceTimer));
  pause.addEventListener('click', () => player.state().paused ? player.resume() : player.pause(), { signal: listeners.signal });
  replay.addEventListener('click', () => player.replay(), { signal: listeners.signal });
  skip.addEventListener('click', () => player.skip(), { signal: listeners.signal });
  live.addEventListener('click', () => player.followLatest(), { signal: listeners.signal });
  motion.addEventListener('change', () => {
    if (motion.matches) journey.stop();
  }, { signal: listeners.signal });
  function cleanup() {
    source.close(); visibilityObserver.disconnect(); listeners.abort(); player.dispose(); journey.reset();
  }
  const feed = createEventFeed(runId, {
    onStep: event => player.add(event),
    onGap() {
      hasGap = true; player.reset(); journey.reset();
      status.textContent = 'Earlier events expired · restoring the retained window';
    },
    onEnd() {
      ended = true; cleanup();
      try { sessionStorage.removeItem(storageKey); } catch { /* Optional persistence. */ }
      list.replaceChildren(); title.textContent = 'Run ended';
      status.textContent = 'Run ended · reopen the patient to start again';
    },
  });
  // Only parsing is guarded: a failure while applying an event is a bug to
  // surface, not an unreadable event.
  function receive(callback) {
    return message => {
      let data;
      try { data = JSON.parse(message.data); }
      catch { status.textContent = 'An event could not be read'; return; }
      callback(data);
    };
  }
  source.addEventListener('step', receive(event => feed.step(event)), { signal: listeners.signal });
  source.addEventListener('snapshot', receive(snapshot => {
    if (typeof snapshot?.prsEvidence === 'boolean') prsEvidence = snapshot.prsEvidence;
    if (snapshot?.record) {
      journey.setCopy(snapshot.record.copy);
      latestRevision = snapshot.record.revision;
      if (compare()) return;
    }
    player.snapshot(); status.textContent = hasGap ? 'Live · earlier events expired' : 'Live · observed calls received';
  }), { signal: listeners.signal });
  source.addEventListener('replay-gap', receive(gap => feed.gap(gap)), { signal: listeners.signal });
  source.addEventListener('run-ended', () => feed.end(), { signal: listeners.signal });
  source.addEventListener('open', () => { status.textContent = 'Connected · waiting for run activity'; }, { signal: listeners.signal });
  source.addEventListener('error', () => {
    status.textContent = source.readyState === EventSource.CLOSED
      ? 'Stream unavailable · reopen the patient to start again' : 'Connection interrupted · reconnecting';
  }, { signal: listeners.signal });
  window.addEventListener('pagehide', cleanup, { once: true, signal: listeners.signal });
  // Outside listeners.signal on purpose: cleanup runs on pagehide, and a page
  // the back-forward cache restores must still reload to reopen its stream.
  window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });
  render(player.state());
  return { player, cleanup };
}

const dock = globalThis.document?.querySelector('#hood-dock');
if (dock) mountViewer(dock);
