import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { setMaxListeners } from 'node:events';
import * as viewer from './static/js/step-events.js';
import { createEventFeed, eventSummary, mountViewer } from './static/js/step-events.js';
import { createJourneyStrip } from './static/js/journey-strip.js';

const step = (seq, runId = 'run-one') => ({
  runId, seq, gf: 'localization', outcome: 'ok', durationMs: 14,
  request: { method: 'GET', path: '/nvi/List' }, response: { status: 200 },
});

test('feed rejects another run and replay duplicates', () => {
  const accepted = [];
  const feed = createEventFeed('run-one', { onStep: event => accepted.push(event.seq) });
  assert.equal(feed.step(step(1)), true);
  assert.equal(feed.step(step(1)), false);
  assert.equal(feed.step(step(2, 'another-run')), false);
  assert.equal(feed.step({ ...step(2), seq: -1 }), false);
  assert.equal(feed.step({ ...step(2), outcome: 'invented' }), false);
  assert.equal(feed.step({ ...step(2), gf: 'unknown' }), false);
  assert.equal(feed.step(step(2)), true);
  assert.deepEqual(accepted, [1, 2]);
});

test('retention gap clears old presentation and ended runs reject steps', () => {
  const seen = [];
  const feed = createEventFeed('run-one', {
    onStep: event => seen.push(event.seq), onGap: first => seen.push(`gap:${first}`),
    onEnd: () => seen.push('ended'),
  });
  feed.step(step(1));
  feed.gap({ firstSeq: 8 });
  feed.step(step(8));
  feed.end();
  assert.equal(feed.step(step(9)), false);
  feed.end();
  assert.deepEqual(seen, [1, 'gap:8', 8, 'ended']);
});

test('summary states the observed call, without inventing clinical success', () => {
  assert.equal(eventSummary(step(1)), 'Referral index queried');
  assert.equal(eventSummary({ ...step(2), gf: 'exchange', outcome: 'deny' }), 'Source request refused');
  assert.equal(eventSummary({ ...step(3), gf: 'exchange', outcome: 'error' }), 'Source request failed');
  assert.equal(eventSummary({ ...step(4), gf: 'consent', request: { method: 'POST' } }), 'Consent subscription submitted');
});

function journeyFixture() {
  const elements = new Map();
  const template = readFileSync(new URL('./templates/_journey_svg.html', import.meta.url), 'utf8');
  for (const match of template.matchAll(/<(?:g|path|text)\b([^>]+)>/g)) {
    const attributes = match[1];
    const id = attributes.match(/\bid="([^"]+)"/)?.[1] || `anonymous-${elements.size}`;
    const classes = new Set((attributes.match(/\bclass="([^"]+)"/)?.[1] || '').split(' '));
    elements.set(id, {
      dataset: { js: attributes.match(/\bdata-js="([^"]+)"/)?.[1], lock: attributes.match(/\bdata-lock="([^"]+)"/)?.[1] },
      style: { setProperty(name, value) { this[name] = value; } }, textContent: '',
      getAttribute: name => attributes.match(new RegExp(`\\b${name}="([^"]+)"`))?.[1],
      getBoundingClientRect: () => ({}),
      classList: {
        add: name => classes.add(name),
        remove: (...names) => names.forEach(name => classes.delete(name)),
        toggle: (name, enabled) => enabled ? classes.add(name) : classes.delete(name),
        contains: name => classes.has(name),
      },
    });
  }
  const svg = {
    querySelectorAll: selectors => [...elements].filter(([id, element]) =>
      selectors.split(',').some(value => {
        const selector = value.trim();
        return (selector === '[data-js]' && element.dataset.js !== undefined) || selector === `#${id}` ||
          (selector.startsWith('.') && element.classList.contains(selector.slice(1)));
      })).map(([, element]) => element),
  };
  const withClass = name => [...elements].filter(([, element]) => element.classList.contains(name)).map(([id]) => id).sort();
  return { elements, svg, withClass };
}

test('map animates observed HTTP replies, including Mitz errors, along the same route', () => {
  const { svg, elements, withClass } = journeyFixture();
  const journey = createJourneyStrip(svg);
  journey.apply({ gf: 'consent', outcome: 'ok', request: { method: 'POST' }, response: { status: 201 } });
  assert.deepEqual(withClass('go'), ['js-request', 'js-response']);
  assert.equal(elements.get('js-response').style.offsetPath, elements.get('js-request').style.offsetPath);
  assert.match(elements.get('js-request').style.offsetPath, /^path\(/);
  assert.equal(elements.get('js-response-label').textContent, '201');
  journey.reset();
  journey.apply({ gf: 'consent', outcome: 'error', request: { method: 'GET' }, response: { status: 500 } });
  assert.deepEqual(withClass('go'), ['js-request', 'js-response']);
  assert.equal(elements.get('js-response-label').textContent, '500');
  assert.ok(withClass('event-error').includes('js-response'));
  journey.reset();
  journey.apply({ gf: 'consent', outcome: 'error', request: { method: 'GET' }, response: { status: 0 } });
  assert.deepEqual(withClass('go'), ['js-request'], 'no invented response after a transport failure');
});

test('unverified HTTP replies never imply consent or token receipt', () => {
  const { svg, withClass } = journeyFixture();
  const journey = createJourneyStrip(svg);
  for (const [gf, outcome, sprites] of [
    ['pseudonym', 'ok', ['js-request', 'js-response']], ['pseudonym', 'error', ['js-request', 'js-response']],
    ['consent', 'ok', ['js-request', 'js-response']], ['authorization', 'ok', ['js-request', 'js-response']],
    ['authorization', 'error', ['js-request', 'js-response']], ['localization', 'deny', ['js-request', 'js-response']],
    ['localization', 'ok', ['js-request', 'js-response']], ['addressing', 'error', ['js-request', 'js-response']],
    ['exchange', 'deny', ['js-request', 'js-response']], ['exchange', 'error', ['js-request', 'js-response']],
    ['exchange', 'ok', ['js-request', 'js-response']], ['unknown', 'ok', []],
  ]) {
    journey.reset(); journey.apply({ gf, outcome, response: { status: outcome === 'ok' ? 200 : 403 } });
    assert.deepEqual(withClass('go'), [...sprites].sort(), `${gf} ${outcome}`);
    assert.deepEqual(withClass('token-received'), [], 'an HTTP result alone does not prove a returned key');
    assert.deepEqual(withClass('vault-open'), [], 'an HTTP result alone does not prove returned data');
    if (gf !== 'pseudonym') assert.ok(!withClass('lit').includes('js-prs'));
  }
  journey.reset(); journey.apply({ gf: 'exchange', outcome: 'ok' }, { reducedMotion: true });
  assert.deepEqual(withClass('go'), []);
  assert.ok(withClass('lit').includes('js-source'));
  journey.reset(); assert.deepEqual(withClass('go'), []);
});

test('mixed outcomes retain every observed response status', () => {
  const { svg, withClass } = journeyFixture();
  const journey = createJourneyStrip(svg);
  journey.applyStage({ events: [{ gf: 'exchange', outcome: 'ok', response: { status: 200 } }, { gf: 'exchange', outcome: 'deny', response: { status: 403 } }] });
  assert.deepEqual(withClass('go'), ['js-request', 'js-response']);
  assert.ok(withClass('deny').includes('js-response'));
});

const playbackModule = await import('./static/js/event-playback.js').catch(() => ({}));
const model = await import('./static/js/journey-model.js');
const observed = (seq, purpose, overrides = {}) => ({
  ...step(seq), actionId: 'action-one', action: 'share', purpose, callId: `call-${seq}`,
  ts: new Date(10000 + seq * 10).toISOString(), ...overrides,
});
function fakeClock() {
  let now = 0, next = 0;
  const timers = new Map();
  return {
    now: () => now,
    setTimeout(fn, delay) { const id = ++next; timers.set(id, { at: now + delay, fn }); return id; },
    clearTimeout(id) { timers.delete(id); },
    tick(ms) {
      const end = now + ms;
      while (true) {
        const task = [...timers].sort((a, b) => a[1].at - b[1].at)[0];
        if (!task || task[1].at > end) break;
        now = task[1].at; timers.delete(task[0]); task[1].fn();
      }
      now = end;
    },
    pending: () => timers.size,
  };
}
function playbackFixture(options = {}) {
  assert.equal(typeof playbackModule.createPlayback, 'function', 'paced playback is implemented');
  const clock = fakeClock(), shown = [];
  const player = playbackModule.createPlayback({ clock, onStage: stage => shown.push(stage.purpose), ...options });
  return { clock, shown, player };
}

test('rapid completed calls stay in separate 2.5 second playback stages', () => {
  const { clock, shown, player } = playbackFixture();
  player.add(observed(1, 'registration'));
  player.add(observed(2, 'subscription', { gf: 'consent' }));
  player.snapshot();
  clock.tick(999); assert.deepEqual(shown, []);
  clock.tick(1); assert.deepEqual(shown, ['registration']);
  clock.tick(2499); assert.deepEqual(shown, ['registration']);
  clock.tick(1); assert.deepEqual(shown, ['registration', 'subscription']);
  clock.tick(2500); assert.equal(player.state().status, 'complete');
});

test('pause preserves remaining time; replay and skip are local; disposal clears clocks', () => {
  const { clock, shown, player } = playbackFixture();
  player.add(observed(1, 'registration'));
  player.add(observed(2, 'subscription', { gf: 'consent' }));
  player.snapshot(); clock.tick(1500);
  player.pause(); clock.tick(10000); assert.deepEqual(shown, ['registration']);
  player.resume(); clock.tick(1999); assert.equal(shown.length, 1);
  clock.tick(1); assert.equal(shown.length, 2);
  player.skip(); assert.equal(player.state().status, 'complete');
  player.replay(); assert.equal(shown.length, 3);
  player.dispose(); assert.equal(clock.pending(), 0);
  clock.tick(10000); assert.equal(shown.length, 3);
});

test('closed viewer retains initial pending stages and hidden pause resumes', () => {
  const { clock, shown, player } = playbackFixture({ active: false });
  player.add(observed(1, 'registration')); player.snapshot(); clock.tick(4000);
  assert.deepEqual(shown, []);
  player.setActive(true); assert.deepEqual(shown, ['registration']);
  clock.tick(500); player.setActive(false); clock.tick(10000);
  assert.equal(player.state().status, 'paused');
  player.setActive(true); clock.tick(2000); assert.equal(player.state().status, 'complete');
});

test('late PRS is placed before its parent without replacing a newer action', () => {
  assert.equal(typeof model.buildActionGroups, 'function');
  const events = [observed(1, 'registration'), observed(2, 'exchange', { actionId: 'action-two', action: 'retrieve', gf: 'exchange' }),
    observed(3, 'pseudonymization', { gf: 'pseudonym', parentCallId: 'call-1', ts: new Date(10005).toISOString() })];
  const groups = model.buildActionGroups(events);
  assert.deepEqual(groups.map(group => group.id), ['action-one', 'action-two']);
  assert.deepEqual(groups[0].stages.map(stage => stage.purpose), ['pseudonymization', 'registration']);
  const { clock, player, shown } = playbackFixture();
  events.forEach(event => player.add(event)); player.snapshot(); clock.tick(1000);
  assert.deepEqual(shown, ['exchange']);
  assert.equal(player.state().action.id, 'action-two');
});

test('snapshot grace includes PRS and duplicate delivery never repeats a stage', () => {
  const { clock, player, shown } = playbackFixture();
  player.add(observed(1, 'registration')); player.snapshot(); clock.tick(500);
  player.add(observed(2, 'pseudonymization', { gf: 'pseudonym', parentCallId: 'call-1', ts: new Date(10005).toISOString() }));
  player.snapshot(); clock.tick(500);
  assert.deepEqual(shown, ['pseudonymization']);
  assert.equal(player.add(observed(2, 'pseudonymization')), false);
  clock.tick(2500); assert.deepEqual(shown, ['pseudonymization', 'registration']);
});

test('status and page reads stay technical; registration categories remain distinct calls', () => {
  assert.equal(typeof model.buildActionGroups, 'function');
  const groups = model.buildActionGroups([
    observed(1, 'status'), observed(2, 'pseudonymization', { gf: 'pseudonym', parentCallId: 'call-1' }),
    observed(3, 'registration', { resourceType: 'Condition' }), observed(4, 'registration', { resourceType: 'Observation' }),
    observed(5, 'status', { actionId: 'page-read', action: 'record' }),
  ]);
  assert.deepEqual(groups[0].stages.map(stage => stage.purpose), ['registration']);
  assert.equal(groups[0].stages[0].events.length, 2);
  assert.equal(groups[0].groups.find(group => group.purpose === 'status').events.length, 2);
  assert.equal(groups[1].stages.length, 0);
});

test('navigation cursor skips watched history and remains bounded to opaque IDs', () => {
  let cursor;
  const first = playbackFixture({ onCursor: value => { cursor = value; } });
  first.player.add(observed(1, 'registration')); first.player.snapshot(); first.clock.tick(3500);
  const next = playbackFixture({ cursor });
  next.player.add(observed(1, 'registration')); next.player.snapshot(); next.clock.tick(3500);
  assert.deepEqual(next.shown, []);
  for (let seq = 2; seq < 400; seq++) next.player.add(observed(seq, 'registration', { actionId: `action-${seq}` }));
  next.player.snapshot(); next.clock.tick(3500);
  assert.ok(next.player.state().events.length <= 256);
  assert.ok(next.player.cursor().callIds.length <= 256);
  assert.deepEqual(Object.keys(next.player.cursor()), ['callIds']);
  assert.ok(next.player.cursor().callIds.every(id => /^call-\d+$/.test(id)));
});

test('a newly settled user action replaces older playback; late older PRS cannot steal it', () => {
  const { clock, player, shown } = playbackFixture();
  player.add(observed(1, 'registration')); player.snapshot(); clock.tick(1000);
  player.add(observed(2, 'exchange', { gf: 'exchange', actionId: 'action-two', action: 'retrieve' }));
  player.snapshot(); clock.tick(1000);
  assert.equal(player.state().action.id, 'action-two');
  assert.deepEqual(shown, ['registration', 'exchange']);
  player.add(observed(3, 'pseudonymization', { gf: 'pseudonym', parentCallId: 'call-1', ts: new Date(10005).toISOString() }));
  player.snapshot(); clock.tick(3500);
  assert.equal(player.state().action.id, 'action-two');
  assert.deepEqual(shown, ['registration', 'exchange']);
});

test('selecting an older action replays only that action despite new activity', () => {
  const { clock, player, shown } = playbackFixture();
  player.add(observed(1, 'registration'));
  player.add(observed(2, 'subscription', { gf: 'consent' }));
  player.add(observed(3, 'exchange', { gf: 'exchange', actionId: 'action-two', action: 'retrieve' }));
  player.snapshot(); clock.tick(3500);
  player.replay('action-one');
  assert.equal(player.state().action.id, 'action-one');
  assert.equal(player.state().stage.purpose, 'registration');
  player.add(observed(4, 'authorization', { gf: 'authorization', actionId: 'action-three', action: 'authorize' }));
  player.snapshot(); clock.tick(5000);
  assert.equal(player.state().action.id, 'action-one');
  assert.equal(player.state().status, 'complete');
  assert.deepEqual(shown, ['exchange', 'registration', 'subscription']);
  player.replay();
  assert.equal(player.state().action.id, 'action-one', 'Replay keeps the selected action');
  player.skip();
  assert.equal(player.state().action.id, 'action-one', 'Skip finishes only the selected action');
  assert.equal(player.state().status, 'complete');
});

test('follow latest leaves a selected replay and resumes automatic new action playback', () => {
  const { clock, player } = playbackFixture();
  player.add(observed(1, 'registration')); player.snapshot(); clock.tick(3500);
  player.replay('action-one');
  player.add(observed(2, 'exchange', { gf: 'exchange', action: 'retrieve', actionId: 'action-two' }));
  player.snapshot(); clock.tick(3500);
  assert.equal(typeof player.followLatest, 'function');
  player.followLatest();
  assert.equal(player.state().action.id, 'action-two');
  assert.equal(player.state().stage.purpose, 'exchange');
  assert.equal(player.state().selected, false);
  player.add(observed(3, 'registration', { actionId: 'action-three' }));
  player.snapshot(); clock.tick(1000);
  assert.equal(player.state().action.id, 'action-three');
});

test('repeat sharing distinguishes searching, removing and registering with PRS next to its parent', () => {
  const groups = model.buildActionGroups([
    observed(1, 'registration-cleanup', { request: { method: 'POST', path: '/nvi/List/_search' } }),
    observed(2, 'registration-cleanup', { request: { method: 'DELETE', path: '/nvi/List/{id}' } }),
    observed(3, 'registration-cleanup', { request: { method: 'DELETE', path: '/nvi/List/{id}' } }),
    observed(4, 'registration', { request: { method: 'POST', path: '/nvi/List' }, resourceType: 'Patient' }),
    observed(5, 'pseudonymization', { gf: 'pseudonym', parentCallId: 'call-4' }),
  ]);
  assert.equal(groups.length, 1);
  assert.deepEqual(groups[0].stages.map(stage => stage.label), [
    'Check existing registrations', 'Remove existing registrations', 'Pseudonymize identifier', 'Register resource categories',
  ]);
  assert.equal(groups[0].stages[1].events.length, 2);
  assert.deepEqual(groups[0].stages[2].events.map(event => event.callId), ['call-5']);
});

test('share summaries use captured evidence and never infer consent from a subscription', () => {
  const first = model.buildActionGroups([
    observed(1, 'registration', { request: { method: 'POST', path: '/nvi/List' } }),
    observed(2, 'subscription', { gf: 'consent', request: { method: 'POST', path: '/mitz/Subscription' }, response: { status: 201 } }),
  ])[0];
  assert.match(first.summary || '', /1 registration submitted/);
  assert.match(first.summary, /Mitz subscription submitted/);
  assert.doesNotMatch(first.summary, /consent (granted|approved)/i);
  const check = observed(3, 'subscription-check', { gf: 'consent', response: { status: 200, body: { resourceType: 'Bundle', entry: [{ resource: { resourceType: 'Subscription', status: 'requested' } }] } } });
  const repeat = model.buildActionGroups([
    observed(1, 'registration-cleanup', { request: { method: 'DELETE', path: '/nvi/List/{id}' } }),
    observed(2, 'registration', { request: { method: 'POST', path: '/nvi/List' } }), check,
  ])[0];
  assert.match(repeat.summary, /1 registration removed/);
  assert.match(repeat.summary, /Existing Mitz subscription returned/);
  check.response.body = null;
  assert.doesNotMatch(model.buildActionGroups([check])[0].summary, /[Ee]xisting/);
});

test('malformed stored cursors are ignored and disposal cancels the initial settle', () => {
  const { clock, player, shown } = playbackFixture({ cursor: { callIds: {} } });
  player.add(observed(1, 'registration')); player.snapshot();
  player.dispose(); assert.equal(clock.pending(), 0);
  clock.tick(10000); assert.deepEqual(shown, []);
});

function mountFixture(t, { recordRevision, reducedMotion = false } = {}) {
  const originals = new Map();
  const replace = (key, value) => { originals.set(key, globalThis[key]); globalThis[key] = value; };
  const clock = fakeClock();
  class Element extends EventTarget {
    constructor(tag = 'div') {
      super(); this.tag = tag; this.dataset = {}; this.children = []; this.textContent = '';
      this.scrollTop = 0; this.scrolls = [];
      this.classes = new Set();
      this.classList = { contains: name => this.classes.has(name), toggle: (name, value) => value ? this.classes.add(name) : this.classes.delete(name) };
    }
    append(...children) { this.children.push(...children); }
    setAttribute(name, value) { this[name] = value; }
    getBoundingClientRect() { return this.bounds || { top: 600, bottom: 635, height: 35 }; }
    scrollTo(options) { this.scrolls.push(options); this.scrollTop = options.top; }
    replaceChildren(...children) { this.children = children; }
    querySelectorAll(selector) {
      return this.children.flatMap(child => [
        ...(selector === 'details[data-key]' && child.tag === 'details' ? [child] : []), ...child.querySelectorAll(selector),
      ]);
    }
  }
  let source;
  class Source extends EventTarget {
    constructor() { super(); source = this; this.closed = false; this.listeners = new Map(); }
    close() { this.closed = true; }
    send(type, value) { this.dispatchEvent(new MessageEvent(type, { data: JSON.stringify(value) })); }
    addEventListener(type, listener, options) {
      super.addEventListener(type, listener, options);
      this.listeners.set(type, [...(this.listeners.get(type) || []), listener]);
    }
    // Calls the listeners directly, so an exception reaches the test instead of
    // being reported asynchronously the way EventTarget reports it.
    deliver(type, data) { for (const listener of this.listeners.get(type) || []) listener(new MessageEvent(type, { data })); }
  }
  const doc = new Element(); doc.createElement = tag => new Element(tag); doc.body = new Element(); doc.hidden = false;
  doc.querySelector = selector => selector === '[data-record-revision]' && recordRevision !== undefined
    ? { dataset: { recordRevision } } : null;
  const window = new EventTarget(); window.matchMedia = () => Object.assign(new EventTarget(), { matches: reducedMotion });
  const dock = new Element(); dock.dataset.runId = 'run-one'; dock.classes.add('on');
  const nodes = new Map(['gf-viewer-steps', 'gf-stream-status', 'gf-playback-status', 'hd-title', 'gf-stage-caption', 'gf-stage-detail', 'gf-access-contract', 'gf-contract-status', 'gf-pause', 'gf-replay', 'gf-skip', 'gf-live'].map(id => [id, new Element()]));
  nodes.get('gf-viewer-steps').bounds = { top: 100, bottom: 400, height: 300 };
  nodes.get('gf-pause').disabled = true; // as _viewer.html renders it until the player mounts
  const { svg, elements } = journeyFixture();
  dock.querySelector = selector => selector === '.dock-map svg' ? svg : nodes.get(selector.slice(1));
  const checks = ['identity', 'role', 'consent', 'query'].map(kind => {
    const item = new Element('span'); item.dataset.check = kind;
    const classes = new Set(['gf-check']);
    item.classList = {
      add: name => classes.add(name), remove: (...names) => names.forEach(name => classes.delete(name)),
      contains: name => classes.has(name),
    };
    item.em = new Element('em'); item.em.textContent = 'not observed';
    item.querySelector = selector => selector === 'em' ? item.em : null;
    return item;
  });
  const descend = dock.querySelectorAll.bind(dock);
  dock.querySelectorAll = selector => selector === '.gf-check' ? checks : descend(selector);
  let observer;
  class Observer { constructor(callback) { this.callback = callback; observer = this; } observe() {} disconnect() { this.disconnected = true; } }
  const saved = new Map();
  class Controller extends AbortController { constructor() { super(); setMaxListeners(30, this.signal); } }
  replace('document', doc); replace('window', window); replace('MutationObserver', Observer);
  replace('EventSource', Source); replace('AbortController', Controller);
  replace('sessionStorage', { getItem: key => saved.get(key), setItem: (key, value) => saved.set(key, value), removeItem: key => saved.delete(key) });
  replace('setTimeout', clock.setTimeout); replace('clearTimeout', clock.clearTimeout);
  t.after(() => { for (const [key, value] of originals) { if (value === undefined) delete globalThis[key]; else globalThis[key] = value; } });
  const mounted = mountViewer(dock);
  return { mounted, source, doc, window, dock, nodes, clock, observer, saved, elements, checks };
}

test('mounted SSE viewer preserves inspectable category calls and cleans up on run end', t => {
  const { mounted, source, clock, observer, nodes, saved } = mountFixture(t);
  source.send('step', observed(1, 'registration', { resourceType: 'Condition' }));
  source.send('step', observed(2, 'registration', { resourceType: 'Observation' }));
  source.send('snapshot', { lastSeq: 2 }); clock.tick(1000);
  const content = element => [element.textContent, ...element.children.map(content)].join(' ');
  const text = content(nodes.get('gf-viewer-steps'));
  assert.match(text, /Condition/); assert.match(text, /Observation/);
  assert.match(text, /#1/); assert.match(text, /#2/);
  assert.match(text, /summed call time/);
  assert.equal(mounted.player.state().stage.purpose, 'registration');
  assert.equal(clock.pending(), 1);
  assert.equal(JSON.parse(saved.get('gf-viewer-playback')).callIds.length, 2);
  source.send('run-ended', {});
  assert.equal(source.closed, true); assert.equal(observer.disconnected, true);
  assert.equal(clock.pending(), 0); assert.equal(saved.size, 0);
  assert.equal(nodes.get('gf-viewer-steps').children.length, 0);
  source.send('step', observed(3, 'subscription'));
  assert.equal(mounted.player.state().events.length, 2, 'aborted SSE listeners accept no further records');
});

test('mounted viewer pauses for visibility and page departure cancels settling', t => {
  const { source, mounted, window, doc, dock, observer, clock } = mountFixture(t);
  source.send('step', observed(1, 'registration')); source.send('snapshot', { lastSeq: 1 });
  dock.classes.delete('on'); observer.callback(); clock.tick(1000);
  assert.equal(mounted.player.state().stage, null);
  dock.classes.add('on'); observer.callback();
  assert.equal(mounted.player.state().stage.purpose, 'registration');
  doc.hidden = true; doc.dispatchEvent(new Event('visibilitychange'));
  assert.equal(mounted.player.state().status, 'paused'); assert.equal(clock.pending(), 0);
  doc.hidden = false; doc.dispatchEvent(new Event('visibilitychange'));
  assert.equal(mounted.player.state().status, 'playing');
  source.send('snapshot', { lastSeq: 1 });
  window.dispatchEvent(new Event('pagehide'));
  assert.equal(clock.pending(), 0); assert.equal(source.closed, true);
});

test('timeline keeps one selectable entry per action with separate request and response details', t => {
  const { mounted, source, clock, nodes } = mountFixture(t);
  source.send('step', observed(1, 'registration', { request: { method: 'POST', path: '/nvi/List', body: { resourceType: 'List' } } }));
  source.send('step', observed(2, 'subscription', { gf: 'consent', request: { method: 'POST', path: '/mitz/Subscription' }, response: { status: 201, body: null } }));
  source.send('step', observed(3, 'registration', { actionId: 'action-two' }));
  source.send('snapshot', { lastSeq: 3 }); clock.tick(3500);
  const descendants = element => [element, ...element.children.flatMap(descendants)];
  const buttons = descendants(nodes.get('gf-viewer-steps')).filter(element => element.tag === 'button' && element.dataset.actionId);
  assert.equal(buttons.length, 2, 'one timeline entry per user action, not per call');
  buttons[0].dispatchEvent(new Event('click'));
  assert.equal(mounted.player.state().action.id, 'action-one');
  const content = descendants(nodes.get('gf-viewer-steps')).map(element => element.textContent).join(' ');
  assert.match(content, /Request/); assert.match(content, /Response/);
  assert.match(content, /HTTP 201/); assert.match(content, /No body/);
  assert.doesNotMatch(content, /Body not captured/, 'an empty body is not a missing capture');
  assert.doesNotMatch(content, /consent granted/i);
  assert.match(nodes.get('gf-stage-caption').textContent, /Step 1 of 2/);
  mounted.cleanup();
});

test('PRS arriving before its parent keeps a valid highlighted step when the parent arrives', t => {
  const { mounted, source, clock, nodes } = mountFixture(t);
  source.send('step', observed(1, 'pseudonymization', { gf: 'pseudonym', parentCallId: 'call-2' }));
  source.send('snapshot', { lastSeq: 1 }); clock.tick(1000);
  source.send('step', observed(2, 'registration'));
  assert.match(nodes.get('gf-stage-caption').textContent, /Step 1 of 2/);
  const descendants = element => [element, ...element.children.flatMap(descendants)];
  const current = descendants(nodes.get('gf-viewer-steps')).filter(element => element.className?.includes('gf-stage-current'));
  assert.ok(current.some(element => element.className.startsWith('gf-stage ')));
  mounted.cleanup();
});

test('presenter can pause before the first action arrives', t => {
  const { mounted, source, clock, nodes } = mountFixture(t);
  assert.equal(nodes.get('gf-pause').disabled, false);
  nodes.get('gf-pause').dispatchEvent(new Event('click'));
  source.send('step', observed(1, 'registration')); source.send('snapshot', { lastSeq: 1 });
  clock.tick(5000);
  assert.equal(mounted.player.state().stage, null);
  nodes.get('gf-pause').dispatchEvent(new Event('click'));
  assert.equal(mounted.player.state().stage.purpose, 'registration');
  mounted.cleanup();
});

test('a replayed window is drawn once, not once per event', t => {
  const { mounted, source, nodes } = mountFixture(t);
  const list = nodes.get('gf-viewer-steps');
  let draws = 0;
  const replaceChildren = list.replaceChildren;
  list.replaceChildren = (...children) => { draws++; replaceChildren.apply(list, children); };
  for (let seq = 1; seq <= 40; seq++) source.send('step', observed(seq, 'registration'));
  source.send('snapshot', { lastSeq: 40 });
  assert.equal(draws, 1, 'every page load replays the retained window; drawing per event is quadratic');
  assert.equal(mounted.player.state().events.length, 40);
  draws = 0;
  source.send('replay-gap', { firstSeq: 100 });
  for (let seq = 100; seq < 140; seq++) source.send('step', observed(seq, 'registration'));
  source.send('snapshot', { lastSeq: 139 });
  assert.equal(draws, 2, 'a gap clears the history once and the retained window is drawn once');
  assert.equal(mounted.player.state().events.length, 40);
  mounted.cleanup();
});

test('token introspection stays inside the local access service', () => {
  const { svg, withClass } = journeyFixture();
  createJourneyStrip(svg).apply({ gf: 'authorization', outcome: 'ok', request: { method: 'POST', path: '/nuts/internal/auth/v2/accesstoken/introspect' }, response: { status: 200 } });
  assert.ok(withClass('lit').includes('js-nuts'));
  assert.ok(!withClass('lit').includes('js-source'));
});

test('key and open vault require separate token and data evidence', () => {
  const { svg, withClass } = journeyFixture();
  const journey = createJourneyStrip(svg);
  const token = { gf: 'authorization', outcome: 'ok', request: { method: 'POST', path: '/nuts/internal/auth/v2/{id}/request-service-access-token' }, response: { status: 200 } };
  journey.apply(token);
  assert.ok(!withClass('token-received').includes('js-response'), '200 alone is not token receipt');
  token.response.tokenReceived = true;
  journey.apply(token);
  assert.ok(withClass('token-received').includes('js-response'));
  assert.ok(withClass('lit').includes('js-auth-server'), 'show the source issuer');
  assert.deepEqual(withClass('vault-open'), [], 'a key alone does not open the data vault');
  const data = { gf: 'exchange', outcome: 'ok', response: { status: 200, body: { resourceType: 'Bundle' } } };
  journey.apply(data);
  assert.ok(withClass('vault-open').includes('js-vault'));
  journey.apply({ ...data, response: { status: 200, body: null } });
  assert.deepEqual(withClass('vault-open'), [], 'missing data evidence leaves the vault closed');
  journey.applyStage({ events: [data, { ...data, outcome: 'deny', response: { status: 403 } }] });
  assert.deepEqual(withClass('vault-open'), []);
  journey.apply({ ...data, outcome: 'error', response: { status: 500 } });
  assert.deepEqual(withClass('vault-open'), []);
  journey.apply(data, { reducedMotion: true });
  assert.ok(withClass('vault-open').includes('js-vault'));
  assert.deepEqual(withClass('go'), []);
  journey.reset(); assert.deepEqual(withClass('vault-open'), []);
});

test('auto-scroll follows step and mode changes but leaves inspection alone between steps', t => {
  const { mounted, source, clock, nodes, doc, observer } = mountFixture(t);
  const list = nodes.get('gf-viewer-steps');
  source.send('step', observed(1, 'registration'));
  source.send('step', observed(2, 'subscription', { gf: 'consent' }));
  source.send('snapshot', {}); clock.tick(1000);
  assert.equal(list.scrolls.length, 1);
  assert.ok(list.scrollTop > 0, 'bring the offscreen active step into the viewer');
  source.send('step', observed(3, 'status'));
  mounted.player.pause(); mounted.player.resume();
  assert.equal(list.scrolls.length, 1, 'ordinary rerenders must not pull the user back');
  doc.body.classes.add('hood-tech'); observer.callback();
  assert.equal(list.scrolls.length, 2, 'the same active step follows a mode switch');
  clock.tick(2500);
  assert.equal(list.scrolls.length, 3, 'the next active step follows playback');
  mounted.cleanup();
});

test('each source request visibly carries only an observed attached key', () => {
  const { svg, withClass } = journeyFixture();
  const journey = createJourneyStrip(svg);
  const request = { gf: 'exchange', outcome: 'ok', request: { method: 'POST', path: '/fhir/Patient/_search', tokenAttached: true }, response: { status: 200, body: { resourceType: 'Bundle' } } };
  journey.apply(request);
  assert.ok(withClass('token-attached').includes('js-request'));
  journey.apply({ ...request, request: { method: 'GET', path: '/fhir/Condition?patient=Patient%2F%7Bid%7D', tokenAttached: true } });
  assert.ok(withClass('token-attached').includes('js-request'));
  journey.apply({ ...request, request: { method: 'GET', path: '/fhir/Condition' } });
  assert.ok(!withClass('token-attached').includes('js-request'), 'a prior key does not imply a later request carries it');
  journey.apply(request, { reducedMotion: true });
  assert.ok(withClass('token-attached').includes('js-request'));
  assert.deepEqual(withClass('go'), []);
  journey.reset();
  assert.ok(!withClass('token-attached').includes('js-request'));
});

test('patient and clinical searches have distinct stages even when both use GET', async () => {
  const { buildActionGroups } = await import('./static/js/journey-model.js');
  const calls = [
    ['POST', 'Patient'], ['GET', 'Patient'], ['GET', 'AllergyIntolerance'], ['GET', 'Condition'], ['GET', 'MedicationRequest'],
  ].map(([method, type], index) => ({ ...step(index + 1), actionId: 'retrieval', action: 'retrieve', purpose: 'exchange', gf: 'exchange', resourceType: type, request: { method, path: `/fhir/${type}`, tokenAttached: true } }));
  const stages = buildActionGroups(calls)[0].stages;
  assert.equal(stages.length, 4, 'patient continuation stays with the patient search; clinical categories remain distinct');
  assert.deepEqual(stages.map(stage => stage.label), ['Find patient at De Zonnebloem', 'Retrieve allergies', 'Retrieve conditions', 'Retrieve medication']);
  assert.equal(stages[0].events.length, 2);
});

test('technical requests explain key attachment and the patient search without exposing a key', t => {
  const { mounted, source, clock, nodes } = mountFixture(t);
  source.send('step', { ...observed(1, 'exchange'), action: 'retrieve', gf: 'exchange', resourceType: 'Patient',
    request: { method: 'POST', path: '/fhir/Patient/_search', tokenAttached: true, headers: { Authorization: 'Bearer [redacted: credential]' } },
    response: { status: 200, body: '[not captured: body exceeds the 65536-byte capture limit]' } });
  source.send('snapshot', {}); clock.tick(1000);
  const content = element => [element.textContent, ...element.children.map(content)].join(' ');
  const text = content(nodes.get('gf-viewer-steps'));
  assert.match(text, /Find patient at De Zonnebloem/);
  assert.match(text, /Access key attached/);
  assert.match(text, /"Authorization": "Bearer \[redacted: credential\]"/);
  assert.ok(text.includes(' [not captured: body exceeds the 65536-byte capture limit]'), 'a marker is shown as text, not as a quoted JSON string');
  assert.ok(!text.includes('"[not captured'), 'a marker is shown as text, not as a quoted JSON string');
  assert.match(text, /identifier is sent in the request body/);
  assert.match(text, /patient reference scopes the later clinical queries/);
  mounted.cleanup();
});

test('an NVI action names missing PRS evidence only where PRS calls are traced', t => {
  for (const [prsEvidence, expected, absent] of [
    [true, /PRS evidence unavailable for this action/, /does not trace/],
    [false, /This deployment does not trace the Knooppunt’s internal PRS call/, /PRS evidence unavailable|Waiting briefly/],
  ]) {
    const { mounted, source, clock, nodes } = mountFixture(t);
    const descendants = element => [element, ...element.children.flatMap(descendants)];
    const text = () => descendants(nodes.get('gf-viewer-steps')).map(element => element.textContent).join(' ');
    source.send('step', observed(1, 'registration'));
    source.send('snapshot', { lastSeq: 1, prsEvidence });
    if (!prsEvidence) assert.doesNotMatch(text(), absent, 'nothing to wait for while settling');
    clock.tick(1000);
    assert.match(text(), expected, `prsEvidence=${prsEvidence}`);
    assert.doesNotMatch(text(), absent, `prsEvidence=${prsEvidence}`);
    mounted.cleanup();
  }
});

test('an unreadable event is reported, and a failing render is not disguised as one', t => {
  const { mounted, source, clock, nodes } = mountFixture(t);
  source.deliver('step', '{not json');
  assert.equal(nodes.get('gf-stream-status').textContent, 'An event could not be read');
  source.send('snapshot', { lastSeq: 0 }); clock.tick(1000);
  nodes.get('gf-stream-status').textContent = 'Live';
  const list = nodes.get('gf-viewer-steps');
  const working = list.replaceChildren;
  list.replaceChildren = () => { throw new Error('render broke'); };
  assert.throws(() => source.deliver('step', JSON.stringify(observed(1, 'registration'))), /render broke/);
  assert.equal(nodes.get('gf-stream-status').textContent, 'Live');
  list.replaceChildren = working;
  mounted.cleanup();
});

test('summaries count one call as one call', async () => {
  const { buildActionGroups } = await import('./static/js/journey-model.js');
  const read = (seq, action, overrides = {}) => observed(seq, 'status', { action, actionId: `${action}-${seq}`, ...overrides });
  assert.equal(buildActionGroups([read(1, 'record')])[0].summary, '1 observed call');
  assert.equal(buildActionGroups([read(1, 'record'), read(2, 'record', { actionId: 'record-1' })])[0].summary, '2 observed calls');
  assert.equal(buildActionGroups([read(1, 'share', { outcome: 'error' })])[0].summary, '1 observed call',
    'a share without a successful step falls back to the count');
});

test('a page restored from the back-forward cache reloads, although pagehide cleaned the viewer up', t => {
  const { window } = mountFixture(t);
  let reloads = 0;
  const location = globalThis.location;
  globalThis.location = { reload: () => { reloads++; } };
  t.after(() => { globalThis.location = location; });
  window.dispatchEvent(Object.assign(new Event('pageshow'), { persisted: false }));
  assert.equal(reloads, 0, 'an ordinary page load does not reload');
  window.dispatchEvent(new Event('pagehide'));
  window.dispatchEvent(Object.assign(new Event('pageshow'), { persisted: true }));
  assert.equal(reloads, 1, 'the closed stream is only restored by a reload');
});

// Read from the module object, so a name the model does not export yet fails the
// tests that use it and not the whole file.
const { buildActionGroups, consentPresentation, lockPresentation, consentChangeLabel, journeyTargets } = model;

const token = (seq, overrides = {}) => ({
  runId: 'run-one', seq, gf: 'authorization', outcome: 'ok', durationMs: 30, actionId: 'retrieve-1', action: 'retrieve',
  callId: `token-${seq}`, ts: new Date(19000 + seq * 10).toISOString(),
  request: { method: 'POST', path: '/nuts/internal/auth/v2/00000010/request-service-access-token' },
  response: { status: 200, tokenReceived: true }, ...overrides,
});
const lockStates = locks => Object.fromEntries(Object.entries(locks).map(([name, lock]) => [name, lock.state]));

const exchange = (seq, overrides = {}) => ({
  runId: 'run-one', seq, gf: 'exchange', outcome: 'ok', durationMs: 20, actionId: 'retrieve-1', action: 'retrieve',
  purpose: 'exchange', resourceType: 'Patient', callId: `call-${seq}`, ts: new Date(20000 + seq * 10).toISOString(),
  request: { method: 'POST', path: '/fhir/Patient/_search', tokenAttached: true },
  response: { status: 200, body: { resourceType: 'Bundle' } }, ...overrides,
});
const check = (seq, parent, decision, overrides = {}) => ({
  runId: 'run-one', seq, gf: 'consent', actor: 'knooppunt', purpose: 'consent-check', parentCallId: parent.callId,
  actionId: parent.actionId, action: parent.action, resourceType: parent.resourceType, callId: `check-${seq}`,
  outcome: decision === 'Permit' ? 'ok' : decision ? 'deny' : 'error', durationMs: 1,
  ts: new Date(Date.parse(parent.ts) - 5).toISOString(),
  request: { method: 'POST', path: '/geslotenautorisatievraag/xacml3', body: '<Request/>' },
  response: { status: decision ? 200 : 500, body: '<Response/>', consentDecision: decision || undefined }, ...overrides,
});
const stageOf = (action, purpose, resourceType) =>
  action.stages.find(stage => stage.purpose === purpose && (!resourceType || stage.events[0].resourceType === resourceType));

test('consent events light their own route, and subscriptions keep theirs', () => {
  assert.equal(journeyTargets({ gf: 'consent', purpose: 'consent-check' }), '#js-mitz, #js-source, #js-auth-server, #jsp-mitzcheck');
  assert.equal(journeyTargets({ gf: 'consent', purpose: 'consent-notification' }), '#js-mitz, #jsp-mitzsub');
  assert.equal(journeyTargets({ gf: 'consent', purpose: 'consent-change' }), '#js-mitz');
  assert.equal(journeyTargets({ gf: 'consent', purpose: 'subscription' }), '#js-mitz, #jsp-mitzsub');
  assert.equal(journeyTargets({ gf: 'consent' }), '#js-mitz, #jsp-mitzsub');
});

// The source asks Mitz while it handles a request: after the request arrived
// with the key, before it answers. The check is therefore part of its request's
// stage and not a step before it.
test('a consent check belongs to the stage of its source request and is not called PRS', () => {
  const patient = exchange(2);
  const [action] = buildActionGroups([patient, check(3, patient, 'Permit')]);
  assert.deepEqual(action.stages.map(stage => stage.purpose), ['exchange']);
  assert.deepEqual(action.stages[0].events.map(event => event.callId), ['call-2'], 'the stage’s calls are the source requests');
  assert.deepEqual(action.stages[0].checks.map(event => event.callId), ['check-3']);
  assert.equal(action.prsUnavailable, false, 'a retrieval has no NVI call to miss PRS evidence for');
  assert.deepEqual(buildActionGroups([patient])[0].stages[0].checks, []);
});

test('PRS children keep their own stage and label next to consent checks', () => {
  const nvi = { ...observed(1, 'registration'), actionId: 'a', callId: 'nvi-1' };
  const prs = { ...observed(2, 'pseudonymization', { gf: 'pseudonym', parentCallId: 'nvi-1' }), actionId: undefined };
  const [action] = buildActionGroups([nvi, prs]);
  assert.deepEqual(action.stages.map(stage => stage.purpose), ['pseudonymization', 'registration']);
  assert.equal(action.stages[0].label, 'Pseudonymize identifier');
});

test('consent state for a stage follows the answers attached to its calls', () => {
  const page1 = exchange(2), page2 = exchange(4, { resourceType: 'Patient' });
  const cases = [
    { name: 'permit on every call', events: [page1, check(3, page1, 'Permit'), page2, check(5, page2, 'Permit')], state: 'permit', legend: 'Mitz: permit' },
    { name: 'deny on any call', events: [page1, check(3, page1, 'Permit'), page2, check(5, page2, 'Deny')], state: 'deny', legend: 'Mitz: deny (permit on 1 of 2 requests)' },
    { name: 'another decision is a refusal', events: [page1, check(3, page1, 'Indeterminate')], state: 'deny', legend: 'Mitz: deny' },
    { name: 'an error answer and no refusal', events: [page1, check(3, page1, '')], state: 'error', legend: 'Mitz: no answer' },
    { name: 'no answer attached to some call', events: [page1, check(3, page1, 'Permit'), page2], state: 'unobserved', legend: 'not observed (permit on 1 of 2 requests)' },
    { name: 'no answer at all', events: [page1], state: 'unobserved', legend: 'not observed' },
  ];
  for (const { name, events, state, legend } of cases) {
    const [action] = buildActionGroups(events);
    const presentation = consentPresentation(stageOf(action, 'exchange'), action);
    assert.equal(presentation.state, state, name);
    assert.equal(presentation.legend, legend, name);
  }
});

// The four locks are on the vault and are decided when a request arrives there
// with the key: the source reads identity and role from the key, asks Mitz, and
// checks the query. Issuing the key opens nothing. Every request is checked
// anew, so a retrieval of four requests opens the locks four times.
test('nothing opens at the key; a request that arrives with it opens identity, role, consent and query', () => {
  const key = token(1), patient = exchange(2);
  const conditions = exchange(4, { resourceType: 'Condition', request: { method: 'GET', path: '/fhir/Condition', tokenAttached: true } });
  const [action] = buildActionGroups([key, patient, check(3, patient, 'Permit'), conditions, check(5, conditions, 'Permit')]);
  assert.deepEqual(action.stages.map(stage => stage.purpose), ['authorization', 'exchange', 'exchange']);
  const [keyStage, patientQuery, conditionQuery] = action.stages;
  const atKey = lockPresentation(keyStage, action);
  assert.deepEqual(lockStates(atKey), { identity: 'closed', role: 'closed', consent: 'closed', query: 'closed' }, 'a key that was issued has not been shown to the source yet');
  assert.equal(atKey.identity.legend, 'carried by the key');
  assert.equal(atKey.role.legend, 'carried by the key');
  assert.equal(atKey.query.legend, 'no request yet');
  const locks = lockPresentation(patientQuery, action);
  assert.deepEqual(lockStates(locks), { identity: 'open', role: 'open', consent: 'open', query: 'open' });
  assert.equal(locks.identity.legend, 'read from the key');
  assert.equal(locks.role.legend, 'read from the key');
  assert.equal(locks.consent.legend, 'Mitz: permit');
  assert.equal(locks.query.legend, 'allowed · request served');
  assert.deepEqual(lockStates(lockPresentation(conditionQuery, action)), { identity: 'open', role: 'open', consent: 'open', query: 'open' });
  // plays marks what a stage decides and therefore shows happening: every request, each time.
  const playing = stage => Object.entries(lockPresentation(stage, action)).filter(([, lock]) => lock.plays).map(([name]) => name);
  assert.deepEqual(playing(keyStage), []);
  assert.deepEqual(playing(patientQuery), ['identity', 'role', 'consent', 'query']);
  assert.deepEqual(playing(conditionQuery), ['identity', 'role', 'consent', 'query'], 'the second request is checked like the first');
});

// After a withdrawal the key is issued as before and the source accepts it: the
// refusal is Mitz's. A 403 comes from the source's policy, after it read the key.
test('after a withdrawal identity and role still open at the request, and consent shuts', () => {
  const key = token(1), patient = exchange(2, { outcome: 'deny', response: { status: 403 } });
  const [action] = buildActionGroups([key, patient, check(3, patient, 'Deny')]);
  const locks = lockPresentation(stageOf(action, 'exchange'), action);
  assert.deepEqual(lockStates(locks), { identity: 'open', role: 'open', consent: 'deny', query: 'closed' });
  assert.equal(locks.identity.legend, 'read from the key');
  assert.equal(locks.consent.legend, 'Mitz: deny');
  assert.equal(locks.query.legend, 'not served');
});

// What the source's answer says about the key: 200 and 403 come after it read
// the key, 401 is the key itself refused, and anything else establishes nothing.
test('a 200 whose body could not be read still shows the key was read, and no data', () => {
  const broken = exchange(2, { outcome: 'error', response: { status: 200 } });
  const [action] = buildActionGroups([token(1), broken]);
  const locks = model.lockPresentation(stageOf(action, 'exchange'), action);
  assert.deepEqual(lockStates(locks), { identity: 'open', role: 'open', consent: 'closed', query: 'closed' });
  assert.equal(locks.identity.legend, 'read from the key');
  assert.equal(model.vaultOpen(locks), false);
  const lost = exchange(2, { outcome: 'error', response: { status: 0 } });
  const [failed] = buildActionGroups([token(1), lost]);
  assert.equal(model.lockPresentation(stageOf(failed, 'exchange'), failed).identity.legend, 'not established', 'no response establishes nothing');
  const bare = exchange(2, { outcome: 'deny', request: { method: 'POST', path: '/fhir/Patient/_search' }, response: { status: 401 } });
  const [keyless] = buildActionGroups([bare]);
  assert.deepEqual([model.lockPresentation(stageOf(keyless, 'exchange'), keyless).identity].map(lock => [lock.state, lock.legend]), [['closed', 'no key attached']],
    'a 401 to a request without a key is not a key refused');
});

test('identity and role follow what the source did with the key', () => {
  const key = token(1);
  const credential = events => { const [action] = buildActionGroups([key, ...events]); return lockPresentation(stageOf(action, 'exchange'), action).identity; };
  assert.deepEqual(credential([exchange(2, { outcome: 'deny', response: { status: 401 } })]), { state: 'deny', legend: 'authentication refused', plays: true });
  assert.deepEqual(credential([exchange(2, { outcome: 'error', response: { status: 502 } })]), { state: 'closed', legend: 'not established', plays: false });
  assert.deepEqual(credential([exchange(2, { request: { method: 'POST', path: '/fhir/Patient/_search' } })]), { state: 'closed', legend: 'no key attached', plays: false });
  const paged = [exchange(2), exchange(4, { outcome: 'deny', response: { status: 401 } })];
  assert.equal(credential(paged).state, 'deny', 'one page refused for its key is a refused key');
});

// One retrieval asks Mitz once per request, so consent can be withdrawn between
// two of its requests.
test('a later refusal shuts the consent lock an earlier request opened, and the vault with it', () => {
  const key = token(1), patient = exchange(2);
  const conditions = exchange(4, { resourceType: 'Condition', outcome: 'deny', request: { method: 'GET', path: '/fhir/Condition', tokenAttached: true }, response: { status: 403 } });
  const [action] = buildActionGroups([key, patient, check(3, patient, 'Permit'), conditions, check(5, conditions, 'Deny')]);
  const [, patientQuery, conditionQuery] = action.stages;
  const served = lockPresentation(patientQuery, action);
  assert.deepEqual(lockStates(served), { identity: 'open', role: 'open', consent: 'open', query: 'open' });
  assert.equal(model.vaultOpen(served), true);
  const refused = lockPresentation(conditionQuery, action);
  assert.deepEqual(lockStates(refused), { identity: 'open', role: 'open', consent: 'deny', query: 'closed' });
  assert.equal(model.vaultOpen(refused), false);
  assert.equal(model.vaultOpen({ consent: { state: 'deny' }, query: { state: 'open' } }), false, 'a refused lock shuts the vault whatever the query lock shows');
  // Data that returned without an observed answer did return: the vault is open.
  const [unobserved] = buildActionGroups([key, patient]);
  assert.equal(model.vaultOpen(lockPresentation(stageOf(unobserved, 'exchange'), unobserved)), true);
});

// The map delays each change to the moment it happens in the stage. The legend
// and the status sentence have to follow, or they give the outcome away before
// the request has even arrived.
test('the legend shows a lock’s new state only once the stage has reached it', () => {
  const key = token(1), patient = exchange(2);
  const conditions = exchange(4, { resourceType: 'Condition', request: { method: 'GET', path: '/fhir/Condition', tokenAttached: true } });
  const [action] = buildActionGroups([key, patient, check(3, patient, 'Permit'), conditions, check(5, conditions, 'Permit')]);
  const [keyStage, patientQuery, conditionQuery] = action.stages;
  const at = (stage, seconds) => lockStates(lockPresentation(stage, action, seconds));
  const closed = { identity: 'closed', role: 'closed', consent: 'closed', query: 'closed' };
  assert.equal(lockPresentation(keyStage, action, 0).identity.legend, 'no key yet', 'the key is not issued while the presentation is on its way');
  assert.equal(lockPresentation(keyStage, action, 1.2).identity.legend, 'carried by the key');
  assert.deepEqual(at(keyStage, 2.4), closed, 'and it opens nothing');
  assert.deepEqual(at(patientQuery, 0), closed, 'nothing opens while the request is on its way');
  assert.equal(lockPresentation(patientQuery, action, 0).identity.legend, 'carried by the key');
  assert.equal(lockPresentation(patientQuery, action, 0).consent.legend, 'awaiting Mitz’s answer');
  assert.equal(lockPresentation(patientQuery, action, 0).query.legend, 'awaiting the source’s answer');
  assert.equal(lockPresentation(patientQuery, action, 0).identity.plays, false);
  assert.deepEqual(at(patientQuery, 0.89), closed);
  assert.deepEqual(at(patientQuery, 0.9), { ...closed, identity: 'open' }, 'the request arrived: identity is read from the key');
  assert.deepEqual(at(patientQuery, 1.1), { ...closed, identity: 'open', role: 'open' });
  assert.deepEqual(at(patientQuery, 2.7), { identity: 'open', role: 'open', consent: 'open', query: 'closed' });
  assert.deepEqual(at(patientQuery, 2.95), { identity: 'open', role: 'open', consent: 'open', query: 'open' });
  assert.deepEqual(at(conditionQuery, 0), closed, 'the next request starts with every lock shut: it is checked anew');
  assert.equal(lockPresentation(conditionQuery, action, 0).identity.legend, 'carried by the key');
  assert.equal(lockPresentation(conditionQuery, action, 0).consent.legend, 'awaiting Mitz’s answer');
  assert.deepEqual(at(conditionQuery, 1.1), { ...closed, identity: 'open', role: 'open' });
  assert.deepEqual(at(conditionQuery, 2.95), { identity: 'open', role: 'open', consent: 'open', query: 'open' });
  assert.deepEqual(lockStates(lockPresentation(patientQuery, action)), { identity: 'open', role: 'open', consent: 'open', query: 'open' }, 'without a time the stage’s final states');

  const timeline = model.stageTimeline;
  const plain = buildActionGroups([patient])[0].stages[0], other = buildActionGroups([observed(1, 'registration')])[0].stages[0];
  assert.deepEqual([timeline(keyStage).seconds, timeline(patientQuery).seconds, timeline(plain).seconds, timeline(other).seconds], [2.5, 4.3, 2.5, 2.5]);
  assert.deepEqual([timeline(keyStage).settled, timeline(patientQuery).settled, timeline(plain).settled, timeline(other).settled], [1.2, 3.15, 1.35, 0]);
  // In one request everything follows its arrival, in the order it is checked.
  const checked = timeline(patientQuery);
  const order = [checked.request.delay + checked.request.duration, checked.identity, checked.role, checked.question.delay, checked.shield, checked.answer.delay,
    checked.answer.delay + checked.answer.duration, checked.consent, checked.query, checked.vault, checked.response.delay];
  assert.deepEqual(order, [...order].sort((a, b) => a - b), 'arrival, identity, role, question, answer, consent, query, vault, response');
  const unchecked = timeline(plain);
  const plainOrder = [unchecked.request.delay + unchecked.request.duration, unchecked.identity, unchecked.role, unchecked.query, unchecked.vault, unchecked.response.delay];
  assert.deepEqual(plainOrder, [...plainOrder].sort((a, b) => a - b));
});

test('the playback reports the moments inside a stage at which the view changes', () => {
  const seen = [];
  const { clock, player } = playbackFixture({ onChange: state => { if (state.stage?.purpose === 'exchange') seen.push(+state.elapsed.toFixed(2)); } });
  const patient = exchange(2);
  for (const event of [patient, check(3, patient, 'Permit')]) player.add(event);
  player.snapshot(); clock.tick(1000);
  seen.length = 0;
  clock.tick(899); assert.deepEqual(seen, []);
  clock.tick(1); assert.deepEqual(seen, [0.9], 'identity');
  clock.tick(100); player.pause(); assert.equal(+player.state().elapsed.toFixed(2), 1);
  clock.tick(10000); assert.equal(+player.state().elapsed.toFixed(2), 1, 'a paused stage does not move on');
  seen.length = 0;
  player.resume(); clock.tick(99); assert.deepEqual(seen.filter(value => value >= 1.1), []);
  clock.tick(1); assert.ok(seen.includes(1.1), 'role, after the pause');
  seen.length = 0;
  clock.tick(2050); assert.deepEqual(seen, [2.7, 2.95, 3.15], 'consent, query, and the outcome');
  clock.tick(1149); assert.equal(player.state().status, 'playing');
  clock.tick(1); assert.equal(player.state().status, 'complete', 'the stage lasts 4.3 seconds in all');
  assert.equal(clock.pending(), 0);
});

// The check has to fit inside its request's stage, so that stage plays longer.
test('a source request with a consent check plays longer than one without', () => {
  const { clock, shown, player } = playbackFixture();
  const patient = exchange(2);
  const conditions = exchange(4, { resourceType: 'Condition', request: { method: 'GET', path: '/fhir/Condition', tokenAttached: true } });
  for (const event of [token(1), patient, check(3, patient, 'Permit'), conditions]) player.add(event);
  player.snapshot();
  clock.tick(1000); assert.deepEqual(shown, ['authorization']);
  clock.tick(2500); assert.deepEqual(shown, ['authorization', 'exchange']);
  clock.tick(2500); assert.deepEqual(shown, ['authorization', 'exchange'], 'the request with its check is still playing');
  clock.tick(1800); assert.deepEqual(shown, ['authorization', 'exchange', 'exchange']);
  clock.tick(2499); assert.equal(player.state().status, 'playing');
  clock.tick(1); assert.equal(player.state().status, 'complete', 'a request without a check keeps the ordinary length');
});

test('without a key the legend says so, and a 200 alone is not a key', () => {
  const closed = { identity: 'closed', role: 'closed', consent: 'closed', query: 'closed' };
  const [refused] = buildActionGroups([token(1, { outcome: 'deny', response: { status: 403 } })]);
  assert.deepEqual(lockStates(lockPresentation(refused.stages[0], refused)), closed);
  assert.equal(lockPresentation(refused.stages[0], refused).identity.legend, 'no key issued');
  const [unproven] = buildActionGroups([token(1, { response: { status: 200 } })]);
  assert.equal(lockPresentation(unproven.stages[0], unproven).identity.legend, 'no key issued');
  const [share] = buildActionGroups([observed(1, 'registration')]);
  assert.deepEqual(lockStates(lockPresentation(share.stages[0], share)), closed);
  assert.equal(lockPresentation(share.stages[0], share).identity.legend, 'no key yet');
  assert.deepEqual(lockStates(lockPresentation(undefined, share)), closed);
});

test('a request served without an observed answer opens identity, role and query and leaves consent closed', () => {
  const key = token(1), patient = exchange(2);
  const [action] = buildActionGroups([key, patient]);
  const locks = lockPresentation(stageOf(action, 'exchange'), action);
  assert.deepEqual(lockStates(locks), { identity: 'open', role: 'open', consent: 'closed', query: 'open' });
  assert.equal(locks.consent.legend, 'not observed');
  const refused = exchange(2, { outcome: 'deny', response: { status: 403 } });
  const [denied] = buildActionGroups([key, refused, check(3, refused, 'Permit')]);
  assert.deepEqual(lockStates(lockPresentation(stageOf(denied, 'exchange'), denied)), { identity: 'open', role: 'open', consent: 'open', query: 'closed' },
    'the source refused after Mitz permitted');
});

test('stages that are not source queries have no consent presentation', () => {
  const [action] = buildActionGroups([observed(1, 'registration')]);
  assert.equal(consentPresentation(action.stages[0], action), null);
  assert.equal(consentPresentation(undefined, action), null);
});

const change = (seq, decision, overrides = {}) => ({
  runId: 'run-one', seq, gf: 'consent', actor: 'sandbox-backend', purpose: 'consent-change', actionId: 'consent-1', action: 'consent',
  callId: `change-${seq}`, outcome: 'ok', durationMs: 900, ts: new Date(30000 + 900).toISOString(),
  request: { method: 'PUT', path: '/mock/consent/[redacted: BSN]', body: { decision, changeId: 'c' } },
  response: { status: 200, body: {} }, ...overrides,
});
const notification = (seq, overrides = {}) => ({
  runId: 'run-one', seq, gf: 'consent', actor: 'mitz', purpose: 'consent-notification', actionId: 'consent-1', action: 'consent',
  callId: `notify-${seq}`, outcome: 'ok', durationMs: 0, ts: new Date(30000 + 400).toISOString(),
  request: { method: 'POST', path: '/mitz/notify', body: {} }, response: { status: 204, copyRemoved: true }, ...overrides,
});

test('a consent action plays its change before its notification, whatever their timestamps', () => {
  // The notification completed while the change was still waiting on the mock,
  // so by completion time it comes first.
  const [action] = buildActionGroups([change(1, 'Deny'), notification(2)]);
  assert.equal(action.label, 'Consent changed');
  assert.deepEqual(action.stages.map(stage => stage.label), ['Patient withdraws consent in Mitz', 'Mitz notification received']);
  const [replayed] = buildActionGroups([notification(2), change(1, 'Deny')]);
  assert.deepEqual(replayed.stages.map(stage => stage.purpose), ['consent-change', 'consent-notification'], 'arrival order does not matter either');
});

// The player, not only the model. Live, the change's stage is already on screen
// when the notification's event arrives; replayed, the action is rebuilt from
// its events, where the notification has the earlier timestamp.
test('a consent action plays its change and then its notification, live and replayed', () => {
  const { clock, shown, player } = playbackFixture();
  player.add(change(1, 'Deny')); player.snapshot();
  clock.tick(1000); assert.deepEqual(shown, ['consent-change']);
  player.add(notification(2)); player.snapshot();
  clock.tick(2500); assert.deepEqual(shown, ['consent-change', 'consent-notification']);
  clock.tick(2500); assert.equal(player.state().status, 'complete');

  player.replay();
  assert.deepEqual(shown.slice(2), ['consent-change'], 'the replay starts with the change, although the notification completed first');
  clock.tick(2500); assert.deepEqual(shown.slice(2), ['consent-change', 'consent-notification']);
});

test('a consent action whose two events arrive together plays in the same order', () => {
  const { clock, shown, player } = playbackFixture();
  player.add(change(1, 'Deny')); player.add(notification(2)); player.snapshot();
  clock.tick(1000); assert.deepEqual(shown, ['consent-change']);
  clock.tick(2500); assert.deepEqual(shown, ['consent-change', 'consent-notification']);
});

// The browser half of the retention test in Task 11: the same three events
// through the feed, across a retention gap, with the fields the viewer groups
// and orders them by untouched.
test('the feed accepts the consent events in sequence and leaves their grouping fields alone', () => {
  const seen = [];
  const feed = createEventFeed('run-one', { onStep: event => seen.push(event), onGap: first => seen.push(`gap:${first}`) });
  const patient = exchange(8, { outcome: 'deny', response: { status: 403 } });
  assert.equal(feed.step(change(1, 'Deny')), true);
  assert.equal(feed.step(notification(2)), true);
  feed.gap({ firstSeq: 8 });
  assert.equal(feed.step(patient), true);
  assert.equal(feed.step(check(9, patient, 'Deny')), true);
  assert.deepEqual(seen.map(item => typeof item === 'string' ? item : item.seq), [1, 2, 'gap:8', 8, 9]);
  assert.equal(seen[1].response.copyRemoved, true);
  const received = seen[4];
  assert.equal(received.purpose, 'consent-check');
  assert.equal(received.parentCallId, patient.callId);
  assert.equal(received.actionId, patient.actionId);
  assert.equal(received.action, 'retrieve');
  assert.equal(received.resourceType, 'Patient');
  assert.equal(received.response.consentDecision, 'Deny');
  assert.equal(feed.step({ ...check(9, patient, 'Deny') }), false, 'a sequence number is accepted once');
  assert.equal(feed.step({ ...check(10, patient, 'Deny'), gf: 'unknown' }), false, 'an event the map has no place for is refused as before');
});

test('the change is named after what the patient did', () => {
  assert.equal(consentChangeLabel(change(1, 'Deny')), 'Patient withdraws consent in Mitz');
  assert.equal(consentChangeLabel(change(1, 'Permit')), 'Patient gives consent again in Mitz');
  const [action] = buildActionGroups([change(1, 'Permit'), notification(2, { response: { status: 204, copyRemoved: false } })]);
  assert.equal(action.stages[0].label, 'Patient gives consent again in Mitz');
});

test('a notification with no change of its own is an action by itself', () => {
  const [action] = buildActionGroups([notification(1, { actionId: 'unsolicited' })]);
  assert.deepEqual(action.stages.map(stage => stage.label), ['Mitz notification received']);
});

test('summaries name consent events for what they are', () => {
  const patient = exchange(2);
  assert.equal(eventSummary(check(3, patient, 'Permit')), 'Mitz answered permit');
  assert.equal(eventSummary(check(3, patient, 'Deny')), 'Mitz answered deny');
  assert.equal(eventSummary(check(3, patient, '')), 'Consent request failed');
  assert.equal(eventSummary(change(1, 'Deny')), 'Consent withdrawn in Mitz');
  assert.equal(eventSummary(change(1, 'Permit')), 'Consent given again in Mitz');
  assert.equal(eventSummary(notification(2)), 'Mitz notification received');
  assert.equal(eventSummary({ gf: 'consent', outcome: 'ok', request: { method: 'POST' } }), 'Consent subscription submitted', 'subscriptions read as before');
});

// Read from the module object, like the model's exports above.
const { shouldReload } = viewer;

test('the record reloads when the stream reports another revision than the page was rendered at', () => {
  const now = 1_000_000;
  assert.equal(shouldReload({ pageRevision: '4', snapshotRevision: 4, now }), false, 'an unchanged record settles');
  assert.equal(shouldReload({ pageRevision: '4', snapshotRevision: 5, now }), true);
  assert.equal(shouldReload({ pageRevision: '5', snapshotRevision: 4, now }), true, 'different is enough: a restarted sandbox counts from zero');
  assert.equal(shouldReload({ pageRevision: undefined, snapshotRevision: 5, now }), false, 'a page without a revision never reloads');
  assert.equal(shouldReload({ pageRevision: '4', snapshotRevision: undefined, now }), false, 'nor does a snapshot without one');
});

test('a submitted form pauses the comparison until the navigation had its chance', () => {
  const now = 1_000_000;
  assert.equal(shouldReload({ pageRevision: '4', snapshotRevision: 5, submittedAt: now - 1000, now }), false,
    'the consent action’s own notification must not cancel its redirect');
  assert.equal(shouldReload({ pageRevision: '4', snapshotRevision: 5, submittedAt: now - 59_999, now }), false);
  assert.equal(shouldReload({ pageRevision: '4', snapshotRevision: 5, submittedAt: now - 60_000, now }), true,
    'no request outlives the server’s write timeout, so after it the navigation did not happen');
});

const stripFor = () => { const { svg, elements, withClass } = journeyFixture(); return { strip: createJourneyStrip(svg), elements, svg, withClass }; };

test('the map plays every lock’s change for each request, and once per request', () => {
  const { strip, elements } = stripFor();
  const lock = name => elements.get(`js-lock-${name}`);
  const names = ['identity', 'role', 'consent', 'query'];
  const added = [];
  for (const name of names) {
    const list = lock(name).classList, add = list.add;
    list.add = value => { added.push(`${name}:${value}`); add(value); };
  }
  const open = { state: 'open' }, closed = { state: 'closed' }, opening = { state: 'open', plays: true };
  const allClosed = { identity: closed, role: closed, consent: closed, query: closed };

  strip.applyStage({ purpose: 'authorization', events: [token(1)] }, { locks: allClosed });
  for (const name of names) assert.ok(!lock(name).classList.contains('lock-open'), `issuing the key does not open ${name}`);

  const patient = exchange(2);
  strip.applyStage({ purpose: 'exchange', events: [patient], checks: [check(3, patient, 'Permit')] },
    { consent: { state: 'permit' }, locks: { identity: opening, role: opening, consent: opening, query: opening } });
  for (const name of names) assert.ok(lock(name).classList.contains('lock-open'), name);
  assert.deepEqual(names.map(name => lock(name).style['--lock-delay']), ['0.9s', '1.1s', '2.7s', '2.95s'],
    'identity and role when the request has arrived with the key, consent after Mitz’s answer, then the query lock');
  assert.ok(elements.get('js-vault').classList.contains('vault-open'), 'the vault opens with the query lock');
  assert.equal(elements.get('js-vault').style['--vault-delay'], '3.15s', 'after the last lock');
  assert.deepEqual(added.filter(item => item.endsWith(':lock-open')), ['identity:lock-open', 'role:lock-open', 'consent:lock-open', 'query:lock-open']);

  // The next request is checked anew: every lock plays its opening again, and so does the vault.
  let vaultOpenings = 0;
  const vaultClasses = elements.get('js-vault').classList, addVault = vaultClasses.add;
  vaultClasses.add = value => { if (value === 'vault-open') vaultOpenings++; addVault(value); };
  const conditions = exchange(4, { resourceType: 'Condition' });
  strip.applyStage({ purpose: 'exchange', events: [conditions], checks: [check(5, conditions, 'Permit')] },
    { consent: { state: 'permit' }, locks: { identity: opening, role: opening, consent: opening, query: opening } });
  assert.equal(added.filter(item => item.endsWith(':lock-open')).length, 8, 'the second request opens the four locks again');
  assert.equal(vaultOpenings, 1, 'and the vault after them');
  assert.ok(elements.get('js-vault').classList.contains('vault-open'));

  // A request Mitz refuses: the key was read, so identity and role open again.
  const refused = exchange(6, { outcome: 'deny', response: { status: 403 } });
  strip.applyStage({ purpose: 'exchange', events: [refused], checks: [check(7, refused, 'Deny')] },
    { consent: { state: 'deny', }, locks: { identity: opening, role: opening, consent: { state: 'deny', plays: true }, query: closed } });
  assert.ok(lock('consent').classList.contains('lock-deny'));
  assert.ok(!lock('consent').classList.contains('lock-open'));
  assert.equal(lock('consent').style['--lock-delay'], '2.7s', 'the refusal reaches the lock when the answer has arrived, not before the request');
  assert.ok(!lock('query').classList.contains('lock-open'));
  assert.ok(lock('identity').classList.contains('lock-open'));
  assert.ok(!elements.get('js-vault').classList.contains('vault-open'));

  // A request whose key the source refuses.
  const unauthorized = exchange(8, { outcome: 'deny', response: { status: 401 } });
  strip.applyStage({ purpose: 'exchange', events: [unauthorized] },
    { locks: { identity: { state: 'deny', plays: true }, role: { state: 'deny', plays: true }, consent: closed, query: closed } });
  assert.ok(lock('identity').classList.contains('lock-deny'));
  assert.equal(lock('identity').style['--lock-delay'], '0.9s', 'on arrival');
  assert.ok(lock('role').classList.contains('lock-deny'));

  // A replay starts at the key again, with every lock shut; the request then plays its openings again.
  strip.applyStage({ purpose: 'authorization', events: [token(1)] }, { locks: allClosed });
  for (const name of names) for (const state of ['lock-open', 'lock-deny', 'lock-error']) assert.ok(!lock(name).classList.contains(state), `${name} ${state}`);
  strip.applyStage({ purpose: 'exchange', events: [patient], checks: [check(3, patient, 'Permit')] },
    { consent: { state: 'permit' }, locks: { identity: opening, role: opening, consent: opening, query: opening } });
  assert.equal(added.filter(item => item === 'identity:lock-open').length, 4, 'the request opens identity again');

  strip.applyStage({ purpose: 'registration', events: [observed(1, 'registration')] }, { locks: allClosed });
  for (const name of names) for (const state of ['lock-open', 'lock-deny', 'lock-error']) {
    assert.ok(!lock(name).classList.contains(state), `${name} ${state} is cleared for an action without that evidence`);
  }

  // Reduced motion: the states are set at once, with no delay to wait out.
  strip.applyStage({ purpose: 'exchange', events: [patient], checks: [check(3, patient, 'Permit')] },
    { reducedMotion: true, consent: { state: 'permit' }, locks: { identity: opening, role: opening, consent: opening, query: opening } });
  assert.ok(lock('identity').classList.contains('lock-open'));
  assert.deepEqual(names.map(name => lock(name).style['--lock-delay']), ['0s', '0s', '0s', '0s']);
});

// What the user sees for one source request, in the order it happens: the
// request arrives with the key, the source reads the key, its policy asks Mitz,
// Mitz answers, and only then does the source answer De Plataan.
test('a source request plays in the order it happens: request, question to Mitz, answer, response', () => {
  const { strip, elements, withClass } = stripFor();
  const label = id => elements.get(`${id}-label`).textContent;
  const delay = id => elements.get(id).style['--go-delay'];
  const patient = exchange(2);

  strip.applyStage({ purpose: 'exchange', events: [patient], checks: [check(3, patient, 'Permit')] }, { consent: { state: 'permit' } });
  assert.deepEqual(withClass('go'), ['js-answer', 'js-question', 'js-request', 'js-response']);
  assert.deepEqual([label('js-request'), label('js-question'), label('js-answer'), label('js-response')], ['POST', 'CONSENT?', 'PERMIT', '200']);
  assert.deepEqual([delay('js-request'), delay('js-question'), delay('js-answer'), delay('js-response')], ['0s', '1.3s', '2.05s', '3.15s']);
  assert.equal(elements.get('js-question').style.offsetPath, elements.get('js-answer').style.offsetPath, 'question and answer share the route to Mitz');
  assert.notEqual(elements.get('js-question').style.offsetPath, elements.get('js-request').style.offsetPath);
  for (const id of ['js-source', 'jsp-exchange', 'js-mitz', 'js-auth-server', 'jsp-mitzcheck']) assert.ok(withClass('lit').includes(id), `${id} is lit`);
  assert.equal(elements.get('js-mitz').style['--mitz-delay'], '1.95s', 'the shield shows the answer once the question has reached Mitz');
  assert.ok(elements.get('js-mitz').classList.contains('consent-permit'));

  const refused = exchange(4, { outcome: 'deny', response: { status: 403 } });
  strip.applyStage({ purpose: 'exchange', events: [refused], checks: [check(5, refused, 'Deny')] }, { consent: { state: 'deny' } });
  assert.deepEqual([label('js-answer'), label('js-response')], ['DENY', '403']);
  assert.equal(delay('js-response'), '3.15s', 'the refusal travels back after Mitz’s answer');
  assert.ok(withClass('deny').includes('js-response'));
  // Nothing on the map says "refused" before the refusal exists: the request
  // travels in as any request does, and the routes take the colour of the
  // refusal when Mitz's answer and the source's response set off.
  assert.ok(!withClass('deny').includes('js-request'), 'the request is not refused on its way in');
  assert.ok(!withClass('deny').includes('js-question'));
  assert.ok(withClass('deny').includes('js-answer'));
  assert.equal(elements.get('jsp-mitzcheck').style['--outcome-delay'], '2.05s');
  assert.equal(elements.get('jsp-exchange').style['--outcome-delay'], '3.15s');
  assert.equal(elements.get('js-source').style['--outcome-delay'], '3.15s');
  assert.ok(elements.get('js-mitz').classList.contains('consent-deny'));

  // Without an observed check there is no question to draw; the request still arrives before anything opens.
  strip.applyStage({ purpose: 'exchange', events: [refused], checks: [] },
    { locks: { identity: { state: 'open', plays: true }, role: { state: 'open', plays: true }, consent: { state: 'closed' }, query: { state: 'closed' } } });
  assert.deepEqual(withClass('go'), ['js-request', 'js-response']);
  assert.equal(delay('js-response'), '1.35s');
  assert.equal(elements.get('js-lock-identity').style['--lock-delay'], '0.9s');
  assert.equal(elements.get('jsp-exchange').style['--outcome-delay'], '1.35s');
  assert.ok(!withClass('deny').includes('js-request'));
  assert.ok(!elements.get('js-mitz').classList.contains('consent-deny'), 'the shield’s answer belongs to its request');
  assert.ok(!withClass('lit').includes('jsp-mitzcheck'));
  strip.applyStage({ purpose: 'exchange', events: [patient], checks: [] },
    { locks: { identity: { state: 'open', plays: true }, role: { state: 'open', plays: true }, consent: { state: 'closed' }, query: { state: 'open', plays: true } } });
  assert.equal(elements.get('js-lock-query').style['--lock-delay'], '1.2s');
  assert.equal(elements.get('js-vault').style['--vault-delay'], '1.35s');
});

test('the retrieved copy shows the snapshot’s state and survives stage changes', () => {
  const { strip, elements } = stripFor();
  strip.setCopy('held');
  assert.ok(elements.get('js-copy').classList.contains('copy-held'));
  strip.applyStage({ purpose: 'registration', events: [observed(1, 'registration')] }, { reducedMotion: true });
  assert.ok(elements.get('js-copy').classList.contains('copy-held'), 'it is the record now, not a state of the stage being replayed');
  strip.setCopy('removed');
  assert.ok(elements.get('js-copy').classList.contains('copy-removed'));
  assert.ok(!elements.get('js-copy').classList.contains('copy-held'));
  strip.setCopy('none');
  assert.ok(!elements.get('js-copy').classList.contains('copy-removed'));
});

// What De Plataan has of the source's data is in a session's memory and nowhere
// else: the map must not call it a copy De Plataan keeps.
test('the map says the retrieved data is held in memory', () => {
  const template = readFileSync(new URL('./templates/_journey_svg.html', import.meta.url), 'utf8');
  const label = name => template.match(new RegExp(`class="js-lbl2 ${name}"[^>]*>([^<]*)<`))?.[1];
  assert.deepEqual(['copy-label-empty', 'copy-label-held', 'copy-label-gone'].map(label),
    ['No data in memory', 'Data held in memory', 'Removed from memory']);
});

test('the legend says what the query check tests', () => {
  const viewer = readFileSync(new URL('./templates/_viewer.html', import.meta.url), 'utf8');
  const legend = viewer.match(/id="gf-contract-legend">([^<]*)</)?.[1] || '';
  assert.match(legend, /tests the request against the BGZ queries its policy allows\. The query lock opens when a request passed that test and was served\.$/);
});

// countReloads replaces location for the length of a test.
function countReloads(t) {
  const counter = { reloads: 0 };
  globalThis.location = { reload: () => { counter.reloads++; } };
  t.after(() => { delete globalThis.location; });
  return counter;
}

test('a mounted record page reloads on a newer revision', t => {
  const { source, elements } = mountFixture(t, { recordRevision: '4' });
  const counter = countReloads(t);

  source.send('snapshot', { lastSeq: 0, record: { revision: 4, copy: 'held' } });
  assert.equal(counter.reloads, 0);
  assert.ok(elements.get('js-copy').classList.contains('copy-held'), 'the copy slot follows the snapshot');
  source.send('snapshot', { lastSeq: 0, record: { revision: 5, copy: 'removed' } });
  assert.equal(counter.reloads, 1);
});

// The legend and the locks are driven from the player's stage, the copy from
// the snapshot. These two go through the mounted viewer, so a callback that is
// not wired up cannot pass them.
test('a mounted viewer opens identity and role when the refused request arrives, and then shuts consent', t => {
  const { source, clock, elements, checks, nodes, mounted } = mountFixture(t);
  countReloads(t);
  const lock = name => elements.get(`js-lock-${name}`);
  const status = nodes.get('gf-contract-status');
  const patient = exchange(2, { outcome: 'deny', response: { status: 403 } });
  for (const event of [token(1), patient, check(3, patient, 'Deny')]) source.send('step', event);
  source.send('snapshot', { lastSeq: 3, record: { revision: 0, copy: 'removed' } });
  assert.ok(elements.get('js-copy').classList.contains('copy-removed'), 'the copy slot follows the snapshot at once');
  const [identity, role, consent, query] = checks;

  clock.tick(1000); // the key step starts: the presentation is on its way
  assert.equal(identity.em.textContent, 'no key yet');
  assert.equal(status.textContent, 'De Plataan presents its credentials to De Zonnebloem’s authorization server.');
  clock.tick(1200); // the key is issued, as it still is after a withdrawal
  assert.equal(identity.em.textContent, 'carried by the key');
  assert.match(status.textContent, /issued an access key/);
  assert.ok(!identity.classList.contains('open'), 'a key that was issued has not opened anything at the source');
  assert.ok(!lock('identity').classList.contains('lock-open'));
  assert.equal(query.em.textContent, 'no request yet');

  clock.tick(1300); // the request sets off with the key
  assert.equal(mounted.player.state().stage.purpose, 'exchange', 'the check is part of its request’s step');
  assert.ok(lock('identity').classList.contains('lock-open'), 'the map has the opening, delayed to the arrival');
  assert.equal(lock('identity').style['--lock-delay'], '0.9s');
  assert.ok(lock('consent').classList.contains('lock-deny'));
  assert.equal(lock('consent').style['--lock-delay'], '2.7s');
  assert.ok(!identity.classList.contains('open'), 'the legend does not open before the request arrived');
  assert.equal(identity.em.textContent, 'carried by the key');
  assert.equal(consent.em.textContent, 'awaiting Mitz’s answer');
  assert.equal(query.em.textContent, 'awaiting the source’s answer');
  assert.equal(status.textContent, 'De Zonnebloem checks this request before it answers: it reads the key and its policy asks Mitz.');
  clock.tick(900); // arrived: the source reads the key
  assert.ok(identity.classList.contains('open'));
  assert.equal(identity.em.textContent, 'read from the key');
  assert.ok(!role.classList.contains('open'));
  clock.tick(200);
  assert.ok(role.classList.contains('open'));
  assert.ok(!consent.classList.contains('deny'), 'the legend does not refuse before Mitz answered');
  clock.tick(1600); // Mitz's answer has arrived
  assert.ok(consent.classList.contains('deny'));
  assert.equal(consent.em.textContent, 'Mitz: deny');
  assert.equal(query.em.textContent, 'awaiting the source’s answer');
  clock.tick(450); // the source's refusal sets off
  assert.ok(!query.classList.contains('open'));
  assert.equal(query.em.textContent, 'not served');
  assert.equal(status.textContent, 'Mitz answered deny. De Zonnebloem refused the request and no data left the source.');
  assert.ok(identity.classList.contains('open'), 'the key was accepted: the refusal is Mitz’s');
  assert.ok(!lock('query').classList.contains('lock-open'));
  assert.ok(!elements.get('js-vault').classList.contains('vault-open'));
  assert.ok(elements.get('js-mitz').classList.contains('consent-deny'));
  const content = element => [element.textContent, ...element.children.map(content)].join(' ');
  assert.match(content(nodes.get('gf-viewer-steps')), /The source asked Mitz for this request\. Mitz answered no\./,
    'the timeline says in words what the lock shows');
  assert.ok(elements.get('js-copy').classList.contains('copy-removed'), 'a stage change does not touch the copy');
  clock.tick(1149);
  assert.equal(mounted.player.state().status, 'playing', 'the step with the check is still playing');
  clock.tick(1);
  assert.equal(mounted.player.state().status, 'complete');
});

test('a mounted viewer opens the four locks in turn and then the vault, live and replayed', t => {
  const { source, clock, elements, checks, mounted, nodes } = mountFixture(t);
  countReloads(t);
  const lock = name => elements.get(`js-lock-${name}`);
  const names = ['identity', 'role', 'consent', 'query'];
  const open = () => names.filter(name => lock(name).classList.contains('lock-open'));
  const legend = () => checks.map((item, index) => item.classList.contains('open') ? names[index] : null).filter(Boolean);
  const patient = exchange(2);
  const conditions = exchange(4, { resourceType: 'Condition', request: { method: 'GET', path: '/fhir/Condition', tokenAttached: true } });
  for (const event of [token(1), patient, check(3, patient, 'Permit'), conditions, check(5, conditions, 'Permit')]) source.send('step', event);
  source.send('snapshot', { lastSeq: 5, record: { revision: 0, copy: 'held' } });
  const [, , consent, query] = checks;
  let identityOpenings = 0;
  const identityClasses = lock('identity').classList, addIdentity = identityClasses.add;
  identityClasses.add = value => { if (value === 'lock-open') identityOpenings++; addIdentity(value); };

  clock.tick(1000); // the key step
  clock.tick(2499);
  assert.deepEqual(open(), [], 'issuing the key opens nothing on the map');
  assert.deepEqual(legend(), [], 'nor in the legend');
  assert.ok(!elements.get('js-vault').classList.contains('vault-open'), 'a key alone does not open the vault');

  clock.tick(1); // the patient request sets off with the key
  assert.deepEqual(open(), names, 'on the map every opening is set, each delayed to its moment');
  assert.deepEqual(legend(), [], 'the legend waits for the request to arrive');
  clock.tick(900);
  assert.deepEqual(legend(), ['identity']);
  clock.tick(200);
  assert.deepEqual(legend(), ['identity', 'role']);
  clock.tick(1600);
  assert.deepEqual(legend(), ['identity', 'role', 'consent']);
  assert.equal(consent.em.textContent, 'Mitz: permit');
  clock.tick(250);
  assert.deepEqual(legend(), names);
  assert.equal(query.em.textContent, 'allowed · request served');
  assert.ok(elements.get('js-vault').classList.contains('vault-open'), 'with every lock open the vault opens');
  assert.equal(elements.get('js-vault').style['--vault-delay'], '3.15s');
  assert.ok(elements.get('js-copy').classList.contains('copy-held'));
  const content = element => [element.textContent, ...element.children.map(content)].join(' ');
  assert.match(content(nodes.get('gf-viewer-steps')), /Before answering, the source asked Mitz whether this patient consents\. Mitz answered yes\./);

  assert.equal(identityOpenings, 1);
  clock.tick(1350); // the next request sets off: it is checked like the first
  assert.equal(mounted.player.state().stage.events[0].resourceType, 'Condition');
  assert.equal(identityOpenings, 2, 'the map plays every opening again');
  assert.deepEqual(open(), names);
  assert.deepEqual(legend(), [], 'and the legend starts from four shut locks');
  assert.equal(consent.em.textContent, 'awaiting Mitz’s answer');
  clock.tick(1100);
  assert.deepEqual(legend(), ['identity', 'role']);
  clock.tick(1850);
  assert.deepEqual(legend(), names);
  assert.ok(elements.get('js-vault').classList.contains('vault-open'));

  clock.tick(1350);
  mounted.player.replay();
  assert.deepEqual(open(), [], 'the replay starts at the key again, with every lock shut');
  assert.deepEqual(legend(), []);
  assert.ok(!elements.get('js-vault').classList.contains('vault-open'));
  clock.tick(2500); // the first request again
  assert.deepEqual(open(), names);
  assert.equal(identityOpenings, 3, 'and it opens identity again');
  assert.ok(elements.get('js-copy').classList.contains('copy-held'), 'the copy is the record now, also during a replay');
});

test('a mounted page without a record revision never reloads', t => {
  const { source } = mountFixture(t);
  const counter = countReloads(t);
  source.send('snapshot', { lastSeq: 0, record: { revision: 9, copy: 'none' } });
  assert.equal(counter.reloads, 0);
});

test('a submitted form on the page holds the reload back', t => {
  const { source, doc } = mountFixture(t, { recordRevision: '4' });
  const counter = countReloads(t);
  doc.dispatchEvent(new Event('submit'));
  source.send('snapshot', { lastSeq: 0, record: { revision: 5, copy: 'removed' } });
  assert.equal(counter.reloads, 0);
});

// The submission did not navigate: the request failed, or the browser dropped
// it. Nothing else changes the record afterwards, so no further snapshot comes.
// The page still has to catch up, when the grace ends.
test('a submission that did not navigate lets the page catch up when its grace ends', t => {
  const { source, doc, clock } = mountFixture(t, { recordRevision: '4' });
  const counter = countReloads(t);
  doc.dispatchEvent(new Event('submit'));
  source.send('snapshot', { lastSeq: 0, record: { revision: 5, copy: 'removed' } });
  assert.equal(counter.reloads, 0);
  clock.tick(59_999); assert.equal(counter.reloads, 0);
  clock.tick(1); assert.equal(counter.reloads, 1, 'from the revision of the last snapshot; no new one was needed');
});

test('a submission followed by nothing new leaves the page alone when its grace ends', t => {
  const { source, doc, clock } = mountFixture(t, { recordRevision: '4' });
  const counter = countReloads(t);
  source.send('snapshot', { lastSeq: 0, record: { revision: 4, copy: 'held' } });
  doc.dispatchEvent(new Event('submit'));
  clock.tick(60_000);
  assert.equal(counter.reloads, 0);
});

test('a page shown again compares at once', t => {
  const { source, doc, window } = mountFixture(t, { recordRevision: '4' });
  const counter = countReloads(t);
  doc.dispatchEvent(new Event('submit'));
  source.send('snapshot', { lastSeq: 0, record: { revision: 5, copy: 'removed' } });
  assert.equal(counter.reloads, 0);
  window.dispatchEvent(new Event('pageshow'));
  assert.equal(counter.reloads, 1);
});

test('the grace timer does not outlive the viewer', t => {
  const { source, doc, clock } = mountFixture(t, { recordRevision: '4' });
  const counter = countReloads(t);
  doc.dispatchEvent(new Event('submit'));
  source.send('run-ended', {});
  assert.equal(clock.pending(), 0);
  clock.tick(60_000);
  assert.equal(counter.reloads, 0);
});

// An answer from Mitz is read after its source request is published, and may
// take up to the evidence deadline: longer than the pause before playback
// starts. A step whose evidence grows while it is shown starts over with all of
// it, so the map and the legend tell the same step from the same moment.
for (const paused of [false, true]) {
  test(`an answer that arrives while its request is shown starts the step over with it, ${paused ? 'paused' : 'playing'}`, t => {
    const { source, clock, elements, checks, mounted } = mountFixture(t);
    countReloads(t);
    const patient = exchange(2, { outcome: 'deny', response: { status: 403 } });
    source.send('step', patient);
    source.send('snapshot', { lastSeq: 2, record: { revision: 0, copy: 'none' } });
    clock.tick(1000); // the refused request plays with no answer from Mitz observed
    const consent = checks[2], lock = elements.get('js-lock-consent');
    assert.equal(mounted.player.state().stage.purpose, 'exchange');
    assert.equal(consent.em.textContent, 'not observed');
    assert.ok(!lock.classList.contains('lock-deny'));
    clock.tick(100); // the answer arrives before the request has reached the source on the map
    if (paused) mounted.player.pause();

    source.send('step', check(3, patient, 'Deny'));
    source.send('snapshot', { lastSeq: 3, record: { revision: 0, copy: 'none' } });

    const state = mounted.player.state();
    assert.equal(state.stage.checks.length, 1, 'the step being shown is the one with the answer');
    assert.ok(state.elapsed < 0.05, 'and it starts over');
    assert.equal(lock.style['--lock-delay'], '2.7s', 'the map shows the refusal at its moment in the step, not at once');
    assert.equal(elements.get('js-question-label').textContent, 'CONSENT?', 'with the question to Mitz it had not shown');
    assert.equal(consent.em.textContent, 'awaiting Mitz’s answer', 'and the legend waits with it');
    if (paused) { clock.tick(10000); assert.equal(consent.em.textContent, 'awaiting Mitz’s answer'); mounted.player.resume(); }
    clock.tick(2699);
    assert.equal(consent.em.textContent, 'awaiting Mitz’s answer');
    clock.tick(1);
    assert.equal(consent.em.textContent, 'Mitz: deny');
    assert.ok(lock.classList.contains('lock-deny'));
    assert.ok(elements.get('js-mitz').classList.contains('consent-deny'));
  });
}

test('a second page that arrives while the first is shown starts the step over with both', t => {
  const { source, clock, nodes, checks, mounted } = mountFixture(t);
  countReloads(t);
  const served = exchange(2), later = exchange(4, { outcome: 'deny', response: { status: 403 } });
  for (const event of [served, check(3, served, 'Permit')]) source.send('step', event);
  source.send('snapshot', { lastSeq: 3, record: { revision: 0, copy: 'none' } });
  clock.tick(1000 + 3200);
  assert.equal(nodes.get('gf-contract-status').textContent, 'De Zonnebloem returned the patient search result.');
  for (const event of [later, check(5, later, 'Deny')]) source.send('step', event);
  source.send('snapshot', { lastSeq: 5, record: { revision: 0, copy: 'none' } });
  assert.equal(mounted.player.state().stage.events.length, 2);
  assert.ok(mounted.player.state().elapsed < 0.05, 'the step starts over');
  clock.tick(3150);
  assert.equal(checks[2].em.textContent, 'Mitz: deny (permit on 1 of 2 requests)');
  assert.equal(nodes.get('gf-contract-status').textContent, 'Mitz answered deny. De Zonnebloem refused 1 of 2 requests; 1 returned data.');
});

test('an answer that arrives after its request finished playing brings the request back with it', t => {
  const { source, clock, elements, checks, mounted } = mountFixture(t);
  countReloads(t);
  const patient = exchange(2, { outcome: 'deny', response: { status: 403 } });
  source.send('step', patient);
  source.send('snapshot', { lastSeq: 2, record: { revision: 0, copy: 'none' } });
  clock.tick(1000 + 2500);
  assert.equal(mounted.player.state().status, 'complete');
  assert.ok(!elements.get('js-lock-consent').classList.contains('lock-deny'));
  clock.tick(100); // 3.6 seconds after the request: within the evidence deadline
  source.send('step', check(3, patient, 'Deny'));
  source.send('snapshot', { lastSeq: 3, record: { revision: 0, copy: 'none' } });
  clock.tick(1000);
  assert.equal(mounted.player.state().stage?.checks.length, 1, 'the request plays again, now with Mitz’s answer');
  clock.tick(2700);
  assert.equal(checks[2].em.textContent, 'Mitz: deny');
  assert.ok(elements.get('js-lock-consent').classList.contains('lock-deny'));
  clock.tick(1600);
  assert.equal(mounted.player.state().status, 'complete');
  clock.tick(5000);
  assert.equal(mounted.player.state().status, 'complete', 'and once');
});

// C3: with reduced motion the map shows a step's result at once, so the legend
// and the sentences do too.
test('with reduced motion the legend and the sentences show a step’s result at once, as the map does', t => {
  const { source, clock, elements, checks, nodes, mounted } = mountFixture(t, { reducedMotion: true });
  countReloads(t);
  const patient = exchange(2);
  const found = discovery().map(event => ({ ...event, seq: event.seq + 10, callId: `found-${event.seq}`, ts: new Date(30000 + event.seq * 10).toISOString() }));
  for (const event of [patient, check(3, patient, 'Permit'), ...found]) source.send('step', event);
  source.send('snapshot', { lastSeq: 12, record: { revision: 0, copy: 'held' } });
  clock.tick(1000); // the discovery is the newest action
  assert.equal(drawnSource(elements.get('js-source')), 'source-located', 'the NVI’s answer is drawn at once');
  assert.ok(!elements.get('js-source').classList.contains('source-asking'));
  assert.match(nodes.get('gf-stage-detail').textContent, /^The NVI names URA 00000020/);
  mounted.player.replay('retrieve-1');
  assert.ok(elements.get('js-vault').classList.contains('vault-open'));
  assert.deepEqual(checks.map(item => item.classList.contains('open')), [true, true, true, true], 'no tile waits while the vault is open');
  assert.equal(nodes.get('gf-contract-status').textContent, 'De Zonnebloem returned the patient search result.');
});

test('the answer packet of a consent check follows the same order as the lock: refusal, failure, permit', () => {
  const { strip, elements } = stripFor();
  const label = () => elements.get('js-answer-label').textContent;
  const first = exchange(2), second = exchange(4), third = exchange(6);
  const consent = { state: 'error', legend: 'Mitz: no answer (permit on 1 of 2 requests)' };

  strip.applyStage({ purpose: 'exchange', events: [first, second], checks: [check(3, first, 'Permit'), check(5, second, '')] }, { consent });
  assert.equal(label(), '500', 'one permit beside a failed answer is not a permit');

  strip.applyStage({ purpose: 'exchange', events: [first, second, third], checks: [check(3, first, 'Permit'), check(5, second, ''), check(7, third, 'Deny')] }, { consent });
  assert.equal(label(), 'DENY', 'a refusal outranks both');

  strip.applyStage({ purpose: 'exchange', events: [first, second], checks: [check(3, first, 'Permit'), check(5, second, 'Permit')] }, { consent });
  assert.equal(label(), 'PERMIT');
});

// What the viewer says about the source's response comes from the source's
// responses. Mitz's refusal of one page says nothing about a page that was
// served before it.
test('a refusal is described as data-free only when every request of the step was refused', () => {
  const refused = exchange(2, { outcome: 'deny', response: { status: 403 } });
  const [denied] = buildActionGroups([refused, check(3, refused, 'Deny')]);
  const whole = 'Mitz answered deny. De Zonnebloem refused the request and no data left the source.';
  assert.equal(consentPresentation(stageOf(denied, 'exchange'), denied).description, whole);

  const served = exchange(2), later = exchange(4, { outcome: 'deny', response: { status: 403 } });
  const [paged] = buildActionGroups([served, check(3, served, 'Permit'), later, check(5, later, 'Deny')]);
  const source = stageOf(paged, 'exchange');
  assert.equal(source.events.length, 2, 'both pages are one step');
  const partial = 'Mitz answered deny. De Zonnebloem refused 1 of 2 requests; 1 returned data.';
  assert.equal(consentPresentation(source, paged).description, partial);

  const failed = exchange(2, { outcome: 'error', response: { status: 502 } });
  const [broken] = buildActionGroups([failed, check(3, failed, 'Deny')]);
  assert.equal(consentPresentation(stageOf(broken, 'exchange'), broken).description,
    'Mitz answered deny. De Zonnebloem refused 0 of 1 requests; 1 failed.');
});

test('a mounted viewer does not say no data left the source when a page was served before the refusal', t => {
  const { source, clock, nodes } = mountFixture(t);
  countReloads(t);
  const served = exchange(2), later = exchange(4, { outcome: 'deny', response: { status: 403 } });
  for (const event of [served, check(3, served, 'Permit'), later, check(5, later, 'Deny')]) source.send('step', event);
  source.send('snapshot', { lastSeq: 5, record: { revision: 0, copy: 'none' } });
  clock.tick(1000);
  const status = nodes.get('gf-contract-status');
  assert.equal(status.textContent, 'De Zonnebloem checks this request before it answers: it reads the key and its policy asks Mitz.');
  clock.tick(3150); // the source's responses set off
  assert.equal(status.textContent, 'Mitz answered deny. De Zonnebloem refused 1 of 2 requests; 1 returned data.');
  clock.tick(1149);
  assert.equal(status.textContent, 'Mitz answered deny. De Zonnebloem refused 1 of 2 requests; 1 returned data.');
});

// The change's event exists for a failed call too. Only a call that succeeded
// shows that the answer changed.
test('a consent change whose call failed is described as an attempt', t => {
  const content = element => [element.textContent, ...element.children.map(content)].join(' ');
  for (const [name, response] of [['no connection', { status: 0 }], ['rejected', { status: 400, body: {} }]]) {
    const { source, clock, nodes } = mountFixture(t);
    source.send('step', change(2, 'Deny', { outcome: 'error', response }));
    source.send('snapshot', { lastSeq: 2, record: { revision: 0, copy: 'held' } });
    clock.tick(1000);
    const text = content(nodes.get('gf-viewer-steps'));
    assert.match(text, /The call failed, so this step does not establish a change\./, name);
    assert.doesNotMatch(text, /the demo changed the answer/, name);
  }
  const { source, clock, nodes } = mountFixture(t);
  source.send('step', change(2, 'Deny'));
  source.send('snapshot', { lastSeq: 2, record: { revision: 0, copy: 'held' } });
  clock.tick(1000);
  assert.match(content(nodes.get('gf-viewer-steps')), /Simulated: the demo changed the answer in the mock Mitz, standing in for the patient\./);
});

// Discovery: what the NVI and the address directory answered about the source.
const categorySystem = 'http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-data-categories-cs';
const custodianList = (ura, code, coding = [{ system: categorySystem, code }]) => ({ resource: {
  resourceType: 'List', code: { coding },
  extension: [{ url: 'http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian',
    valueReference: { identifier: { system: 'http://fhir.nl/fhir/NamingSystem/ura', value: ura } } }],
} });
const bgzLists = ura => ['AllergyIntolerance', 'Condition', 'MedicationRequest', 'Patient'].map(code => custodianList(ura, code));
// The searchset as the Knooppunt's NVI endpoint answers it: without a resourceType.
const localize = (seq, lists, overrides = {}) => ({
  runId: 'run-one', seq, gf: 'localization', purpose: 'localization', outcome: 'ok', durationMs: 12,
  actionId: 'discover-1', action: 'discover', callId: `call-${seq}`, ts: new Date(15000 + seq * 10).toISOString(),
  request: { method: 'POST', path: '/nvi/List/_search', headers: { 'X-Tenant-Id': 'http://fhir.nl/fhir/NamingSystem/ura|00000010' } },
  response: { status: 200, body: { type: 'searchset', total: lists.length, ...(lists.length ? { entry: lists } : {}) } }, ...overrides,
});
const zonnebloem = { resourceType: 'Organization', id: 'org-1', name: 'Zorgcentrum De Zonnebloem',
  identifier: [{ system: 'urn:oid:2.16.528.1.1007.3.3', value: '00000020' }],
  endpoint: [{ reference: 'Endpoint/fhir-1' }, { reference: 'Endpoint/auth-1' }] };
const fhirEndpoint = { resourceType: 'Endpoint', id: 'fhir-1', status: 'active', address: 'http://pep-zonnebloem:8080/fhir',
  connectionType: { system: 'http://terminology.hl7.org/CodeSystem/endpoint-connection-type', code: 'hl7-fhir-rest' } };
const authEndpoint = { resourceType: 'Endpoint', id: 'auth-1', status: 'active', address: 'https://knooppunt.example/nuts/oauth2/00000020',
  connectionType: { system: 'http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-authorization-server-cs', code: 'oauth-nuts' } };
const directoryUrl = resource => `http://directory.example/fhir/${resource.resourceType}/${resource.id}`;
const resolve = (seq, resources, overrides = {}, ura = '00000020') => ({
  runId: 'run-one', seq, gf: 'addressing', purpose: 'addressing', outcome: 'ok', durationMs: 9,
  actionId: 'discover-1', action: 'discover', callId: `call-${seq}`, ts: new Date(15000 + seq * 10).toISOString(),
  request: { method: 'GET', path: `/fhir/knpt-mcsd-query/Organization?_include=Organization%3Aendpoint&identifier=urn%3Aoid%3A2.16.528.1.1007.3.3%7C${ura}` },
  response: { status: 200, body: { resourceType: 'Bundle', entry: resources.map(resource => ({ fullUrl: directoryUrl(resource), resource })) } }, ...overrides,
});
const discovery = () => [localize(1, [...bgzLists('00000010'), ...bgzLists('00000020')]), resolve(2, [zonnebloem, fhirEndpoint, authEndpoint])];
const actionOf = (events, id) => model.buildActionGroups(events).find(action => action.id === id);
const bgzLabels = ['Patient', 'Conditions', 'Allergies', 'Medication'];

test('a discovery draws the source only as far as its answers have established it', () => {
  const events = discovery(), action = actionOf(events, 'discover-1');
  const [locating, addressing] = action.stages;
  const at = (stage, elapsed) => model.sourcePresentation({ stage, action, events, elapsed });
  assert.deepEqual(at(locating, 0), { state: 'unknown', asking: true, gap: false, ura: '', categories: [], name: '', title: 'Source', address: false });
  const answered = model.stageTimeline(locating).source;
  assert.equal(at(locating, answered - 0.01).state, 'unknown', 'until the NVI’s answer is back');
  const located = { state: 'located', asking: false, gap: false, ura: '00000020', categories: bgzLabels, name: '', title: 'URA 00000020', address: false };
  assert.deepEqual(at(locating, answered), located, 'the NVI names a URA and its data categories, nothing else');
  assert.deepEqual(at(addressing, 0), { ...located, asking: true });
  assert.deepEqual(at(addressing, model.stageTimeline(addressing).source),
    { state: 'addressed', asking: false, gap: false, ura: '00000020', categories: bgzLabels, name: 'Zorgcentrum De Zonnebloem', title: 'De Zonnebloem', address: true });
});

test('the organization that asks is not a source of its own', () => {
  const events = [localize(1, bgzLists('00000010'))], action = actionOf(events, 'discover-1');
  assert.deepEqual(model.sourcePresentation({ stage: action.stages[0], action, events }),
    { state: 'unknown', asking: false, gap: false, ura: '', categories: [], name: '', title: 'Source', address: false });
});

test('outside a discovery the map draws what the run established, without the steps still to be played', () => {
  const events = [...discovery(), token(3), exchange(4)];
  const retrieve = actionOf(events, 'retrieve-1');
  const known = { state: 'addressed', asking: false, gap: false, ura: '00000020', categories: bgzLabels, name: 'Zorgcentrum De Zonnebloem', title: 'De Zonnebloem', address: true };
  assert.deepEqual(model.sourcePresentation({ events }), known, 'at rest');
  assert.deepEqual(model.sourcePresentation({ stage: retrieve.stages[0], action: retrieve, events, elapsed: 0 }), known, 'while another action plays');
  assert.equal(model.sourcePresentation({ events: discovery(), upcoming: actionOf(discovery(), 'discover-1').stages }).state, 'unknown',
    'a discovery that has yet to be played gives nothing away');
  assert.deepEqual(model.sourcePresentation({ events: [token(3), exchange(4)] }),
    { state: 'addressed', asking: false, gap: false, ura: '', categories: [], name: '', title: 'Source', address: true },
    'a source that was sent a request was known, by its address and not by a name');
  assert.equal(model.sourcePresentation({ events: [token(3)] }).state, 'unknown',
    'a key request goes to De Plataan’s own access service and establishes nothing about the source');
  assert.equal(model.sourcePresentation({ events: [token(3, { outcome: 'error', response: { status: 0 } })] }).address, false);
  assert.equal(model.sourcePresentation({ events: [observed(1, 'registration')] }).state, 'unknown');
  assert.equal(model.sourcePresentation().state, 'unknown');
});

test('every discovery starts from a source that is not known yet', () => {
  const again = discovery().map(event => ({ ...event, seq: event.seq + 10, callId: `again-${event.seq}`, actionId: 'discover-2',
    ts: new Date(30000 + event.seq * 10).toISOString() }));
  const events = [...discovery(), token(3), exchange(4), ...again], action = actionOf(events, 'discover-2');
  assert.equal(model.sourcePresentation({ stage: action.stages[0], action, events, elapsed: 0 }).state, 'unknown');
  assert.equal(model.sourcePresentation({ stage: action.stages[1], action, events, elapsed: 0 }).state, 'located');
});

test('after a retention gap the map says the earlier steps are gone, and claims neither a source nor its absence', () => {
  const events = [observed(1, 'registration')];
  assert.deepEqual(model.sourcePresentation({ events, partial: true }),
    { state: 'unknown', asking: false, gap: true, ura: '', categories: [], name: '', title: 'Source', address: false });
  assert.equal(model.sourcePresentation({ events: [exchange(4)], partial: true }).gap, false, 'what is retained still counts');
  const found = discovery(), action = actionOf(found, 'discover-1');
  const starting = model.sourcePresentation({ stage: action.stages[0], action, events: found, elapsed: 0, partial: true });
  assert.deepEqual([starting.state, starting.gap], ['unknown', false], 'a discovery still starts anew');
});

// The same rule as selectSource in mcsdclient.go: an endpoint the organization
// points at, active, valid at the time of the call, of the exact kind.
test('the map marks a web address only for an endpoint the retrieval would select', () => {
  const stateWith = resources => {
    const events = [localize(1, bgzLists('00000020')), resolve(2, resources)], action = actionOf(events, 'discover-1');
    const { state, name, address } = model.sourcePresentation({ stage: action.stages[1], action, events });
    return { state, name, address };
  };
  const addressed = address => ({ state: 'addressed', name: 'Zorgcentrum De Zonnebloem', address });
  const withFhir = change => [zonnebloem, { ...fhirEndpoint, ...change }, authEndpoint];
  assert.deepEqual(stateWith(withFhir({})), addressed(true));
  assert.deepEqual(stateWith(withFhir({ status: 'off' })), addressed(false), 'not active');
  assert.deepEqual(stateWith(withFhir({ address: '' })), addressed(false), 'no address');
  assert.deepEqual(stateWith(withFhir({ period: { end: '1969-12-31' } })), addressed(false), 'out of service before the call');
  assert.deepEqual(stateWith(withFhir({ period: { start: '1971' } })), addressed(false), 'not in service yet');
  assert.deepEqual(stateWith(withFhir({ period: { start: '1969', end: '1970-01-01' } })), addressed(true), 'a date-only end runs through that day');
  assert.deepEqual(stateWith(withFhir({ period: { end: 'unreadable' } })), addressed(true), 'an unreadable bound is no bound, as in the retrieval');
  assert.deepEqual(stateWith(withFhir({ connectionType: { system: 'http://example.org/other', code: 'hl7-fhir-rest' } })), addressed(false), 'the code of another vocabulary');
  assert.deepEqual(stateWith([{ ...zonnebloem, endpoint: [{ reference: 'Endpoint/auth-1' }] }, fhirEndpoint, authEndpoint]), addressed(false),
    'an endpoint in the answer the organization does not point at');
  assert.deepEqual(stateWith([{ ...zonnebloem, endpoint: [{ reference: directoryUrl(fhirEndpoint) }] }, fhirEndpoint]), addressed(true), 'an absolute reference');
  assert.deepEqual(stateWith([zonnebloem, authEndpoint]), addressed(false));
  assert.deepEqual(stateWith([]), { state: 'located', name: '', address: false }, 'a directory without the organization resolves nothing');
  const otherServer = { ...authEndpoint, connectionType: { ...authEndpoint.connectionType, code: 'other' } };
  assert.equal(model.discoveryPresentation(actionOf([resolve(2, [zonnebloem, fhirEndpoint, otherServer])], 'discover-1').stages[0]).description,
    'The directory resolves URA 00000020 to Zorgcentrum De Zonnebloem and gives the web address of its FHIR API.',
    'an authorization server of another kind is not the one the retrieval uses');
});

// D2: one box, one organization. What the NVI named, what the directory was
// asked and what it answered have to be about the same URA.
test('with several holders the map draws one of them, and only what was answered about that one', () => {
  const other = { ...zonnebloem, id: 'org-2', name: 'Huisartsenpraktijk De Linde', identifier: [{ system: 'urn:oid:2.16.528.1.1007.3.3', value: '00000030' }] };
  const lists = [...bgzLists('00000010'), custodianList('00000030', 'Patient'), ...bgzLists('00000020')];
  const both = [localize(1, lists), resolve(2, [zonnebloem, fhirEndpoint, authEndpoint]), resolve(3, [other, fhirEndpoint], {}, '00000030')];
  const drawn = events => { const { state, ura, categories, name, title } = model.sourcePresentation({ events }); return { state, ura, categories, name, title }; };
  assert.deepEqual(drawn(both), { state: 'addressed', ura: '00000020', categories: bgzLabels, name: 'Zorgcentrum De Zonnebloem', title: 'De Zonnebloem' },
    'the first holder in URA order, as the retrieval lists them, with the answer about that URA');
  assert.deepEqual(drawn([both[0], both[2]]), { state: 'located', ura: '00000020', categories: bgzLabels, name: '', title: 'URA 00000020' },
    'an answer about another organization does not name this one');
  const [locating, addressing] = actionOf(both, 'discover-1').stages;
  assert.equal(model.discoveryPresentation(locating).description,
    'The NVI names URA 00000020 for 4 data categories and 1 other organization. Its name and address are the address directory’s to give.');
  assert.equal(model.discoveryPresentation(addressing).description,
    'The directory resolves URA 00000020 to Zorgcentrum De Zonnebloem and gives the web addresses of its FHIR API and its authorization server. It was asked about 1 more organization.');
  const swapped = actionOf([resolve(2, [other, fhirEndpoint], {}, '00000030'), resolve(3, [zonnebloem, fhirEndpoint, authEndpoint])], 'discover-1').stages[0];
  assert.match(model.discoveryPresentation(swapped).description, /^The directory resolves URA 00000030 to Huisartsenpraktijk De Linde /,
    'the URA asked and the organization answered come from the same call');
});

test('the box is titled with what the directory answered', () => {
  const titled = organization => model.sourcePresentation({ events: [localize(1, bgzLists('00000020')), resolve(2, [organization, fhirEndpoint])] }).title;
  assert.equal(titled(zonnebloem), 'De Zonnebloem', 'the name the demo uses for this organization, when the answer names it');
  assert.equal(titled({ ...zonnebloem, name: 'Huisartsenpraktijk De Linde' }), 'Huisartsenpraktijk De Linde');
  assert.equal(titled({ ...zonnebloem, name: undefined }), 'URA 00000020', 'an answer without a name leaves the URA');
});

// D3: the same vocabulary the retrieval reads (nvi.CategoriesOf, custodianOf).
test('a data category and a custodian are read from the vocabulary that defines them', () => {
  const categoriesOf = lists => model.sourcePresentation({ events: [localize(1, lists)] }).categories;
  const foreign = { system: 'http://example.org/other', code: 'Condition' };
  assert.deepEqual(categoriesOf([custodianList('00000020', 'Patient', [foreign, { system: categorySystem, code: 'Patient' }])]), ['Patient'],
    'the coding of the data category system, wherever it stands');
  assert.deepEqual(categoriesOf([custodianList('00000020', 'Condition', [foreign])]), [], 'a code of another vocabulary names no category');
  const stage = actionOf([localize(1, [custodianList('00000020', 'Condition', [foreign])])], 'discover-1').stages[0];
  assert.equal(model.discoveryPresentation(stage).description, 'The NVI names URA 00000020. Its name and address are the address directory’s to give.');
  const lookalike = custodianList('00000020', 'Patient');
  lookalike.resource.extension[0].url = 'http://example.org/nl-gf-localization-custodian';
  assert.equal(model.sourcePresentation({ events: [localize(1, [lookalike])] }).state, 'unknown', 'an extension that only ends the same is not the custodian');
});

test('without the asking organization on the captured request the map names no holder', () => {
  const unscoped = localize(1, [...bgzLists('00000010'), ...bgzLists('00000020')], { request: { method: 'POST', path: '/nvi/List/_search', headers: {} } });
  assert.equal(model.sourcePresentation({ events: [unscoped] }).state, 'unknown', 'it cannot tell De Plataan’s own registration from a find');
  assert.equal(model.discoveryPresentation(actionOf([unscoped], 'discover-1').stages[0]).description, '');
});

test('a discovery step says what it asks and what the answer established', () => {
  const [locating, addressing] = actionOf(discovery(), 'discover-1').stages;
  assert.deepEqual(model.discoveryPresentation(locating), {
    pending: 'De Plataan asks the NVI which organizations hold data on this patient.',
    description: 'The NVI names URA 00000020 for 4 data categories. Its name and address are the address directory’s to give.',
  });
  assert.deepEqual(model.discoveryPresentation(addressing), {
    pending: 'De Plataan asks the address directory who URA 00000020 is and where to reach it.',
    description: 'The directory resolves URA 00000020 to Zorgcentrum De Zonnebloem and gives the web addresses of its FHIR API and its authorization server.',
  });
  const stageWith = events => actionOf(events, 'discover-1').stages.at(-1);
  assert.equal(model.discoveryPresentation(stageWith([localize(1, [custodianList('00000020', 'Patient')])])).description,
    'The NVI names URA 00000020 for 1 data category. Its name and address are the address directory’s to give.');
  assert.equal(model.discoveryPresentation(stageWith([localize(1, bgzLists('00000010'))])).description,
    'The NVI names no other organization for this patient.');
  assert.equal(model.discoveryPresentation(stageWith([resolve(2, [zonnebloem, fhirEndpoint])])).description,
    'The directory resolves URA 00000020 to Zorgcentrum De Zonnebloem and gives the web address of its FHIR API.');
  assert.equal(model.discoveryPresentation(stageWith([resolve(2, [zonnebloem, authEndpoint])])).description,
    'The directory resolves URA 00000020 to Zorgcentrum De Zonnebloem. It lists no web address for its FHIR API.');
  assert.equal(model.discoveryPresentation(stageWith([resolve(2, [])])).description, 'The directory holds no organization with URA 00000020.');
  assert.equal(model.discoveryPresentation(stageWith([localize(1, [], { outcome: 'error', response: { status: 502 } })])).description, '',
    'a failed search established nothing');
  assert.equal(model.discoveryPresentation(actionOf([token(3)], 'retrieve-1').stages[0]), null);
});

test('a discovery step wakes the view when its answer is back and then plays a second longer', () => {
  const seen = [];
  const { clock, shown, player } = playbackFixture({ onChange: state => { if (state.stage) seen.push(`${state.stage.purpose}@${state.elapsed.toFixed(1)}`); } });
  for (const event of discovery()) player.add(event);
  player.snapshot(); clock.tick(1000);
  assert.deepEqual(shown, ['localization']);
  seen.length = 0;
  clock.tick(2299); assert.deepEqual(seen, []);
  clock.tick(1); assert.deepEqual(seen, ['localization@2.3'], 'the NVI’s answer is back');
  clock.tick(999); assert.deepEqual(shown, ['localization']);
  clock.tick(1); assert.deepEqual(shown, ['localization', 'addressing']);
  clock.tick(3299); assert.equal(player.state().status, 'playing');
  clock.tick(1); assert.equal(player.state().status, 'complete');
});

test('the playback says which steps are still to be played', () => {
  const { clock, player } = playbackFixture();
  const upcoming = () => player.state().upcoming.map(stage => stage.purpose);
  for (const event of discovery()) player.add(event);
  player.snapshot();
  assert.deepEqual(upcoming(), ['localization', 'addressing'], 'before the action starts');
  clock.tick(1000); assert.deepEqual(upcoming(), ['addressing'], 'the step being played is not to come');
  clock.tick(6600); assert.deepEqual(upcoming(), []);
  player.add(token(3)); player.add(exchange(4));
  assert.deepEqual(upcoming(), ['authorization', 'exchange'], 'a newer action that has yet to start');
  player.snapshot(); clock.tick(1000);
  assert.deepEqual(upcoming(), ['exchange']);
  player.replay('discover-1');
  assert.deepEqual(upcoming(), ['addressing', 'exchange'], 'a replay counts what is left of it, and what has not played at all');
});

test('what arrives during a selected replay is still to be played', () => {
  const { clock, player } = playbackFixture();
  player.add(observed(1, 'registration'));
  player.snapshot(); clock.tick(3500);
  player.replay('action-one'); player.pause();
  for (const event of discovery()) player.add({ ...event, seq: event.seq + 10, callId: `late-${event.seq}` });
  player.snapshot(); clock.tick(1000);
  assert.deepEqual(player.state().upcoming.map(stage => stage.purpose), ['localization', 'addressing']);
  assert.equal(model.sourcePresentation(player.state()).state, 'unknown', 'so a discovery that has not played gives nothing away');
  player.followLatest();
  assert.deepEqual(player.state().upcoming.map(stage => stage.purpose), ['addressing']);
});

const sourceStates = ['source-pending', 'source-unknown', 'source-located', 'source-addressed'];
const drawnSource = box => sourceStates.filter(name => box.classList.contains(name)).join();
const knownSource = (state, overrides = {}) => ({ state, asking: false, gap: false, ura: '00000020', categories: bgzLabels, name: '', title: 'URA 00000020', address: false, ...overrides });

test('the map draws the source box as far as it is known', () => {
  const { strip, elements, svg } = stripFor();
  const box = elements.get('js-source');
  const chips = () => svg.querySelectorAll('.js-source-chip').map((chip, index) =>
    chip.classList.contains('on') ? svg.querySelectorAll('.js-source-category')[index].textContent : null);
  assert.equal(drawnSource(box), 'source-pending', 'nothing is drawn of the source before it is told what is known');
  strip.setSource(knownSource('unknown', { ura: '', categories: [] }));
  assert.equal(drawnSource(box), 'source-unknown');
  assert.deepEqual(chips(), [null, null, null, null, null, null]);
  strip.setSource(knownSource('located'));
  assert.equal(drawnSource(box), 'source-located');
  assert.equal(elements.get('js-source-ura').textContent, 'URA 00000020');
  assert.deepEqual(chips(), [...bgzLabels, null, null]);
  assert.ok(!box.classList.contains('source-address'));
  strip.setSource(knownSource('addressed', { name: 'Zorgcentrum De Zonnebloem', title: 'De Zonnebloem', address: true }));
  assert.equal(drawnSource(box), 'source-addressed');
  assert.equal(elements.get('js-source-name').textContent, 'De Zonnebloem');
  assert.ok(!elements.get('js-source-name').classList.contains('src-name-long'));
  strip.setSource(knownSource('addressed', { title: 'Huisartsenpraktijk Linde' }));
  assert.equal(elements.get('js-source-name').textContent, 'Huisartsenpraktijk Linde');
  assert.ok(elements.get('js-source-name').classList.contains('src-name-long'), 'a long name is set smaller to fit the box');
  strip.setSource(knownSource('addressed', { title: 'Stichting Samenwerkende Zorgcentra Noord' }));
  assert.equal(elements.get('js-source-name').textContent, 'Stichting Samenwerkende Z…');
  strip.setSource(knownSource('unknown', { ura: '', categories: [], gap: true }));
  assert.ok(box.classList.contains('source-gap'), 'a placeholder after a retention gap says the earlier steps are gone');
  strip.setSource(knownSource('addressed', { name: 'Zorgcentrum De Zonnebloem', title: 'De Zonnebloem', address: true }));
  assert.ok(!box.classList.contains('source-gap'));
  assert.ok(box.classList.contains('source-address'), 'with the web address the directory gave marked');
  strip.setSource(knownSource('addressed', { name: 'Zorgcentrum De Zonnebloem' }));
  assert.ok(!box.classList.contains('source-address'), 'an organization without a web address has none marked');
  strip.reset();
  assert.equal(drawnSource(box), 'source-pending');
});

test('the map marks that the source’s web address is known, and does not show the address', () => {
  const template = readFileSync(new URL('./templates/_journey_svg.html', import.meta.url), 'utf8');
  const marker = template.match(/<g class="src-address">([\s\S]*?)<\/g>/)?.[1] || '';
  assert.deepEqual([...marker.matchAll(/<text\b[^>]*>([^<]*)<\/text>/g)].map(match => match[1]), ['WEB ADDRESS']);
  assert.match(marker, /rotate\(-90\b/, 'written along the bar on the source’s edge');
});

test('more data categories than the box holds are counted, and a long one is cut', () => {
  const { strip, svg } = stripFor();
  const texts = () => svg.querySelectorAll('.js-source-category').map(item => item.textContent);
  strip.setSource(knownSource('located', { categories: [...bgzLabels, 'Encounter', 'Procedure', 'Immunization', 'Observation'] }));
  assert.deepEqual(texts(), [...bgzLabels, 'Encounter', '+3 more']);
  strip.setSource(knownSource('located', { categories: ['MedicationStatement'] }));
  assert.deepEqual(texts(), ['Medication…', '', '', '', '', '']);
});

test('a change of the source in the step being played is shown happening; otherwise it is simply drawn', () => {
  const { strip, elements } = stripFor();
  const box = elements.get('js-source');
  strip.setSource(knownSource('unknown', { ura: '', categories: [], asking: true }), true);
  assert.ok(box.classList.contains('source-asking'));
  assert.ok(!box.classList.contains('lit'), 'a placeholder is not lit up');
  strip.setSource(knownSource('located'), true);
  assert.ok(box.classList.contains('source-moving'));
  assert.ok(box.classList.contains('lit'), 'what the answer established lights up');
  assert.ok(!box.classList.contains('source-asking'));
  box.classList.remove('lit');
  strip.setSource(knownSource('located'), true);
  assert.ok(!box.classList.contains('lit'), 'an unchanged source is left alone');
  strip.setSource(knownSource('located', { asking: true }), true);
  assert.ok(box.classList.contains('lit'), 'the source the directory is asked about stays lit');
  box.classList.remove('lit');
  strip.setSource(knownSource('addressed', { address: true }));
  assert.ok(!box.classList.contains('source-moving'));
  assert.ok(!box.classList.contains('lit'));
  strip.setSource(knownSource('addressed', { address: true }), true);
  assert.ok(!box.classList.contains('source-moving'), 'a source drawn at rest does not appear again when a step starts');
});

test('a mounted viewer shows the source appear during a discovery, and says what each answer established', t => {
  const { source, clock, elements, nodes, mounted } = mountFixture(t);
  const box = elements.get('js-source'), detail = nodes.get('gf-stage-detail');
  assert.equal(drawnSource(box), 'source-pending', 'nothing is drawn of the source before the run’s events are in');
  for (const event of discovery()) source.send('step', event);
  assert.equal(drawnSource(box), 'source-pending');
  source.send('snapshot', { lastSeq: 2 });
  assert.equal(drawnSource(box), 'source-unknown', 'the discovery has yet to play');
  assert.ok(!box.classList.contains('source-moving'), 'at rest the state is simply drawn');

  clock.tick(1000); // the NVI is asked
  assert.equal(drawnSource(box), 'source-unknown');
  assert.ok(box.classList.contains('source-asking'));
  assert.equal(detail.textContent, 'De Plataan asks the NVI which organizations hold data on this patient.');
  clock.tick(2299);
  assert.equal(drawnSource(box), 'source-unknown', 'until the answer is back');
  clock.tick(1);
  assert.equal(drawnSource(box), 'source-located');
  assert.ok(box.classList.contains('source-moving'), 'the change is shown happening');
  assert.equal(elements.get('js-source-ura').textContent, 'URA 00000020');
  assert.equal(detail.textContent, 'The NVI names URA 00000020 for 4 data categories. Its name and address are the address directory’s to give.');

  clock.tick(1000); // the directory is asked
  assert.equal(drawnSource(box), 'source-located');
  assert.equal(detail.textContent, 'De Plataan asks the address directory who URA 00000020 is and where to reach it.');
  assert.ok(!box.classList.contains('source-address'));
  clock.tick(2300);
  assert.equal(drawnSource(box), 'source-addressed');
  assert.ok(box.classList.contains('source-address'));
  assert.match(detail.textContent, /^The directory resolves URA 00000020 to Zorgcentrum De Zonnebloem and gives the web addresses/);

  clock.tick(1000);
  assert.equal(mounted.player.state().status, 'complete');
  assert.equal(drawnSource(box), 'source-addressed', 'what the run established stays drawn');
  const content = element => [element.textContent, ...element.children.map(content)].join(' ');
  const steps = content(nodes.get('gf-viewer-steps'));
  assert.match(steps, /The NVI names URA 00000020 for 4 data categories\. Its name and address are the address directory’s to give\./);
  assert.match(steps, /The directory resolves URA 00000020 to Zorgcentrum De Zonnebloem/);

  mounted.player.replay();
  assert.equal(drawnSource(box), 'source-unknown', 'a replay asks anew');
});

test('a mounted viewer says after a retention gap that the earlier steps are gone', t => {
  const { source, elements } = mountFixture(t);
  const box = elements.get('js-source');
  source.send('replay-gap', { firstSeq: 8 });
  assert.equal(drawnSource(box), 'source-pending');
  source.send('step', observed(8, 'registration'));
  source.send('snapshot', { lastSeq: 8 });
  assert.equal(drawnSource(box), 'source-unknown', 'missing evidence proves nothing, in either direction');
  assert.ok(box.classList.contains('source-gap'));
});

test('in technical mode a discovery step keeps its HTTP summary', t => {
  const { source, clock, nodes, doc } = mountFixture(t);
  doc.body.classes.add('hood-tech');
  for (const event of discovery()) source.send('step', event);
  source.send('snapshot', { lastSeq: 2 });
  clock.tick(1000);
  assert.equal(nodes.get('gf-stage-detail').textContent, 'POST → NVI · HTTP 200 ← NVI');
});
