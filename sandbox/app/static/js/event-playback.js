import { buildActionGroups, callIdentity } from './journey-model.js';

const systemClock = {
  now: () => performance.now(),
  setTimeout: (fn, delay) => setTimeout(fn, delay),
  clearTimeout: timer => clearTimeout(timer),
};

export function createPlayback({ clock = systemClock, onStage, onChange, onCursor,
  cursor, active = true, stageMs = 2500, settleMs = 1000 } = {}) {
  let events = [], groups = [], action = null, stage = null;
  let timer = null, settling = null, due = 0, remaining = stageMs;
  let ready = false, started = false, paused = false, disposed = false, complete = false;
  // A replayed window (page load, retention gap) is applied once at its snapshot
  // rather than regrouped and redrawn per event; later live events apply directly.
  let delivered = false;
  const watched = new Set((Array.isArray(cursor?.callIds) ? cursor.callIds : []).filter(id => typeof id === 'string' && id.length <= 160).slice(-256));
  let replayQueue = null, selected = false;
  const getCursor = () => ({ callIds: [...watched].slice(-256) });
  function remember(records) {
    for (const event of records) watched.add(callIdentity(event));
    while (watched.size > 256) watched.delete(watched.values().next().value);
    onCursor?.(getCursor());
  }
  function state() {
    return { events, groups, action, stage, selected, paused: paused || !active,
      status: disposed ? 'ended' : paused || !active ? 'paused' : stage ? 'playing' : !ready ? 'settling' : complete ? 'complete' : 'waiting' };
  }
  const changed = () => onChange?.(state());
  const latest = () => groups.filter(group => group.stages.length).at(-1);
  const pending = group => group?.stages.filter(item => item.events.some(event => !watched.has(callIdentity(event)))) || [];
  function schedule() {
    if (disposed || paused || !active || !stage || timer !== null) return;
    due = clock.now() + remaining;
    timer = clock.setTimeout(() => { timer = null; stage = null; advance(); }, remaining);
  }
  function advance() {
    if (disposed || !ready || paused || !active || stage) return;
    if (!started) {
      action = latest() || null;
      remember(events.filter(event => !action?.events.includes(event)));
      started = true;
    }
    let next = selected ? replayQueue?.shift() : pending(action)[0];
    if (!next && !selected) {
      replayQueue = null;
      const newest = latest();
      if (newest && (!action || newest.time >= action.time)) action = newest;
      next = pending(action)[0];
    }
    if (next) {
      stage = next; complete = false; remaining = stageMs;
      remember(next.events);
      onStage?.(next, action);
      schedule();
    } else complete = !!action;
    changed();
  }
  function suspend() {
    if (timer !== null) {
      remaining = Math.max(0, due - clock.now());
      clock.clearTimeout(timer); timer = null;
    }
  }
  function resumeClock() { if (stage) schedule(); else advance(); changed(); }
  function regroup() {
    groups = buildActionGroups(events);
    if (action) {
      const retained = groups.find(group => group.id === action.id);
      if (!retained) { suspend(); stage = null; replayQueue = null; selected = false; }
      action = retained || latest() || null;
    }
    changed();
  }
  return {
    add(event) {
      if (disposed || events.some(item => item.seq === event.seq || callIdentity(item) === callIdentity(event))) return false;
      events = [...events, event].slice(-256);
      if (delivered) regroup();
      return true;
    },
    snapshot() {
      if (disposed) return;
      if (!delivered) { delivered = true; regroup(); }
      if (settling !== null) return;
      // One grace period per received batch, never reset by a later PRS delivery.
      settling = clock.setTimeout(() => {
        settling = null; ready = true;
        const newest = latest();
        if (!selected && started && newest && action?.id !== newest.id && (!action || newest.time >= action.time)) {
          suspend(); stage = null; replayQueue = null; action = newest;
          remember(events.filter(event => !action.events.includes(event)));
        }
        advance(); changed();
      }, settleMs);
    },
    pause() { paused = true; suspend(); changed(); },
    resume() { paused = false; resumeClock(); },
    setActive(value) { active = value; if (!active) suspend(); else resumeClock(); changed(); },
    replay(actionId = action?.id || latest()?.id) {
      if (disposed) return;
      const target = groups.find(group => group.id === actionId && group.stages.length);
      if (!target) return;
      suspend(); action = target; stage = null; selected = true;
      replayQueue = [...action.stages];
      ready = true; started = true; paused = false; advance();
    },
    followLatest() {
      if (disposed) return;
      suspend(); stage = null; selected = false; replayQueue = null;
      ready = true; started = false; paused = false; advance();
    },
    skip() {
      if (disposed) return;
      suspend(); if (action) remember(action.events);
      stage = null; replayQueue = null; complete = !!action;
      advance(); changed();
    },
    reset() {
      suspend(); if (settling !== null) clock.clearTimeout(settling);
      settling = null; events = []; groups = []; action = null; stage = null;
      ready = false; started = false; complete = false; replayQueue = null; selected = false; delivered = false;
      changed();
    },
    dispose() {
      suspend(); if (settling !== null) clock.clearTimeout(settling);
      settling = null; disposed = true; stage = null; changed();
    },
    state, cursor: getCursor,
  };
}
