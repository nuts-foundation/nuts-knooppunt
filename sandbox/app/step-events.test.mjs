import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { setMaxListeners } from 'node:events';
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
      dataset: { js: attributes.match(/\bdata-js="([^"]+)"/)?.[1] },
      style: {}, textContent: '',
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
    assert.deepEqual(withClass('unlocked'), []);
    assert.ok(!withClass('go').includes('js-keyback'), 'token HTTP result does not prove a returned key');
    assert.ok(!withClass('go').includes('js-permit'), 'subscription is not a consent verdict');
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

function mountFixture(t) {
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
    constructor() { super(); source = this; this.closed = false; }
    close() { this.closed = true; }
    send(type, value) { this.dispatchEvent(new MessageEvent(type, { data: JSON.stringify(value) })); }
  }
  const doc = new Element(); doc.createElement = tag => new Element(tag); doc.body = new Element(); doc.hidden = false;
  const window = new EventTarget(); window.matchMedia = () => Object.assign(new EventTarget(), { matches: false });
  const dock = new Element(); dock.dataset.runId = 'run-one'; dock.classes.add('on');
  const nodes = new Map(['gf-viewer-steps', 'gf-stream-status', 'gf-playback-status', 'hd-title', 'gf-stage-caption', 'gf-stage-detail', 'gf-access-contract', 'gf-contract-status', 'gf-pause', 'gf-replay', 'gf-skip', 'gf-live'].map(id => [id, new Element()]));
  nodes.get('gf-viewer-steps').bounds = { top: 100, bottom: 400, height: 300 };
  const { svg } = journeyFixture();
  dock.querySelector = selector => selector === '.dock-map svg' ? svg : nodes.get(selector.slice(1));
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
  return { mounted, source, doc, window, dock, nodes, clock, observer, saved };
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
  assert.match(content, /HTTP 201/); assert.match(content, /Body not captured/);
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
  assert.deepEqual(stages.map(stage => stage.label), ['Find patient at Sunflower', 'Retrieve allergies', 'Retrieve conditions', 'Retrieve medication']);
  assert.equal(stages[0].events.length, 2);
});

test('technical requests explain key attachment and the patient search without exposing a key', t => {
  const { mounted, source, clock, nodes } = mountFixture(t);
  source.send('step', { ...observed(1, 'exchange'), action: 'retrieve', gf: 'exchange', resourceType: 'Patient', request: { method: 'POST', path: '/fhir/Patient/_search', tokenAttached: true } });
  source.send('snapshot', {}); clock.tick(1000);
  const content = element => [element.textContent, ...element.children.map(content)].join(' ');
  const text = content(nodes.get('gf-viewer-steps'));
  assert.match(text, /Find patient at Sunflower/);
  assert.match(text, /Authorization: Bearer \[value not retained\]/);
  assert.match(text, /identifier is sent in the request body/);
  assert.match(text, /patient reference scopes the later clinical queries/);
  mounted.cleanup();
});
