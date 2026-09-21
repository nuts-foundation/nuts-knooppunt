import { createJourneyStrip } from './journey-strip.js';
import { journeyTargets, subscriptionResult, accessPresentation } from './journey-model.js';
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
  let hasGap = false, ended = false;
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
  const callCount = events => `${events.length} call${events.length === 1 ? '' : 's'}`;
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
      if (direction === 'request' && event.request?.tokenAttached === true) section.append(node('p', 'gf-step-result', 'Access key attached · Authorization: Bearer [value not retained]'));
      if (direction === 'request' && event.gf === 'exchange' && event.resourceType === 'Patient' && event.request?.method === 'POST') section.append(node('p', 'gf-step-result', 'Patient search: the identifier is sent in the request body to keep it out of the URL. The returned patient reference scopes the later clinical queries.'));
      if (direction === 'response' && event.response?.tokenReceived) section.append(node('p', 'gf-step-result', 'Captured evidence: access token received. The token value is not retained.'));
      if (direction === 'response' && subscriptionResult(event)) section.append(node('p', 'gf-step-result', subscriptionResult(event)));
      if (message?.headers && Object.keys(message.headers).length) {
        section.append(node('div', 'gf-http-label', 'Captured headers'), node('pre', '', JSON.stringify(message.headers, null, 2)));
      }
      section.append(node('div', 'gf-http-label', 'Captured body'));
      section.append(message?.body == null ? node('p', 'gf-step-result', 'Body not captured') : node('pre', '', JSON.stringify(message.body, null, 2)));
      exchange.append(section);
    }
    call.append(exchange);
    if (event.ts) call.append(node('p', 'gf-step-result', `Completed at ${timeLabel(event.ts)}`));
    if (children.length) {
      const internal = node('div', 'gf-internal');
      internal.append(node('p', 'gf-http-label', 'Internal PRS calls made within this request'));
      for (const child of children) internal.append(callDetails(child, [], false));
      if (followChanged && document.body.classList.contains('hood-tech') && children.some(child => activeCalls.has(child))) call.open = true;
      call.append(internal);
    }
    return call;
  }
  function technicalCalls(action, state) {
    const container = node('div', 'gf-technical');
    container.append(node('p', 'gf-step-result', `${result(action)}. Calls below are ordered by completion; internal PRS calls are nested under their parent request.`));
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
    container.append(node('p', 'hd-note', 'Details contain captured, sanitized fields only. A Mitz subscription response is not a consent decision.'));
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
      if (current) content.append(node('span', 'gf-stage-now', state.paused ? 'Paused here' : 'Playing this step'));
      const access = accessPresentation(group);
      if (access.requestDescription) content.append(node('p', 'gf-step-result', access.requestDescription));
      if (access.visible) content.append(node('p', 'gf-step-result', access.description));
      row.append(content); stages.append(row);
      if (current && !activeFunctional) activeFunctional = row;
    }
    container.append(stages);
    if (action.prsUnavailable) container.append(node('p', 'hd-note', state.status === 'settling'
      ? 'Waiting briefly for internal PRS evidence…' : 'PRS evidence unavailable for this action.'));
    return container;
  }
  function render(state) {
    const focusedAction = document.activeElement?.dataset?.actionId;
    const technicalMode = document.body.classList.contains('hood-tech');
    const visible = dock.classList.contains('on') && !document.hidden;
    followChanged = !!state.stage && visible && (state.stage !== lastFollowStage || technicalMode !== lastFollowMode || !lastFollowOpen);
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
    const states = { settling: 'Preparing playback · waiting up to 1 s for evidence', playing: `${state.selected ? 'Replaying selected action' : 'Playing latest action'} · 2.5 s per step`,
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
    contract.hidden = !access.visible;
    contractStatus.textContent = access.description;
    if (access.visible && !technicalMode) stageDetail.textContent = access.tokenRequest
      ? access.tokenReceived ? 'Request access → Sunflower · source-issued key → Plataan' : 'Request access through Plataan’s access service → Sunflower'
      : access.requestDescription;
    pause.textContent = state.paused ? 'Resume' : 'Pause';
    pause.disabled = ended;
    live.disabled = ended || !state.selected;
    replay.disabled = ended || !action;
    skip.disabled = ended || !state.stage;
    dock.classList.toggle('playback-paused', state.paused);
    dock.classList.toggle('playback-idle', !state.stage);
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
    onStage: stage => journey.applyStage(stage, { reducedMotion: motion.matches }),
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
  function receive(callback) {
    return message => {
      try { callback(JSON.parse(message.data)); }
      catch { status.textContent = 'An event could not be read'; }
    };
  }
  source.addEventListener('step', receive(event => feed.step(event)), { signal: listeners.signal });
  source.addEventListener('snapshot', receive(() => {
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
  window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });
  render(player.state());
  return { player, cleanup };
}

const dock = globalThis.document?.querySelector('#hood-dock');
if (dock) mountViewer(dock);
