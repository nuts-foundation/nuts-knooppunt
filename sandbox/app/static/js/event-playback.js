import { buildActionGroups, callIdentity, stageTimeline } from './journey-model.js';

const systemClock = {
  now: () => performance.now(),
  setTimeout: (fn, delay) => setTimeout(fn, delay),
  clearTimeout: timer => clearTimeout(timer),
};

export function createPlayback({ clock = systemClock, onStage, onChange, onCursor,
  cursor, active = true, settleMs = 1000 } = {}) {
  let events = [], groups = [], action = null, stage = null;
  // A stage plays for length ms, of which left remained when its timer was last
  // started, at since. marks are the moments inside it at which the view changes
  // (stageTimeline): the timer wakes at each of them and at the end.
  let timer = null, settling = null, length = 0, left = 0, since = 0, marks = [];
  let ready = false, started = false, paused = false, disposed = false, complete = false;
  // Whether the stage being shown has been marked as shown: that happens when
  // its clock first runs, not while the viewer is hidden or paused.
  let acknowledged = true;
  // A replayed window (page load, retention gap) is applied once at its snapshot
  // rather than regrouped and redrawn per event; later live events apply directly.
  let delivered = false;
  const watched = new Set((Array.isArray(cursor?.callIds) ? cursor.callIds : []).filter(id => typeof id === 'string' && id.length <= 160).slice(-256));
  let replayQueue = null, selected = false;
  const getCursor = () => ({ callIds: [...watched].slice(-256) });
  // remember marks events as shown. The cursor is bounded like the retained
  // events are, and it gives up the oldest ids first, but never the id of an
  // event that is still retained: that stage would count as unshown again.
  function remember(records) {
    for (const event of records) watched.add(callIdentity(event));
    if (watched.size > 256) {
      const retained = new Set(events.map(callIdentity));
      for (const id of watched) {
        if (watched.size <= 256) break;
        if (!retained.has(id)) watched.delete(id);
      }
    }
    onCursor?.(getCursor());
  }
  const leftNow = () => timer === null ? left : Math.max(0, left - (clock.now() - since));
  // The stages still to be played: what is left of a replay, and every stage
  // that has not been shown, of the action being shown and of newer ones. An
  // older action is not gone back to. Before the first action starts it is the
  // newest action's stages; the older ones are taken as shown when it does.
  function upcoming() {
    const waiting = started ? groups.filter(group => !action || group.time >= action.time).flatMap(pending) : pending(latest());
    return selected ? [...(replayQueue || []), ...waiting] : waiting;
  }
  function state() {
    return { events, groups, action, stage, selected, delivered, upcoming: upcoming(), paused: paused || !active, elapsed: stage ? (length - leftNow()) / 1000 : 0,
      status: disposed ? 'ended' : paused || !active ? 'paused' : stage ? 'playing' : !ready ? 'settling' : complete ? 'complete' : 'waiting' };
  }
  const changed = () => onChange?.(state());
  const latest = () => groups.filter(group => group.stages.length).at(-1);
  // A stage is still to be played while any of its events, or of the consent
  // checks that belong to it, has not been shown.
  const evidenceOf = item => [...item.events, ...(item.checks || [])];
  const pending = group => group?.stages.filter(item => evidenceOf(item).some(event => !watched.has(callIdentity(event)))) || [];
  function schedule() {
    if (disposed || paused || !active || !stage || timer !== null) return;
    if (!acknowledged) { acknowledged = true; remember(evidenceOf(stage)); }
    const elapsed = length - left;
    const mark = marks.find(value => value > elapsed);
    const wait = mark === undefined ? left : mark - elapsed;
    since = clock.now();
    timer = clock.setTimeout(() => {
      timer = null; left -= wait;
      if (left > 0) { changed(); schedule(); } else { stage = null; advance(); }
    }, wait);
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
    if (next) show(next); else complete = !!action;
    changed();
  }
  // show puts a stage on the map and starts its clock at zero. The stage is
  // marked as shown when that clock runs (schedule).
  function show(next) {
    const timeline = stageTimeline(next);
    stage = next; complete = false; length = left = Math.round(timeline.seconds * 1000);
    marks = [...new Set([timeline.identity, timeline.role, timeline.consent, timeline.query, timeline.source, timeline.settled]
      .filter(value => value > 0 && value < timeline.seconds).map(value => Math.round(value * 1000)))].sort((a, b) => a - b);
    acknowledged = false;
    onStage?.(next, action);
    schedule();
  }
  function suspend() {
    if (timer !== null) {
      left = leftNow();
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
    // Regrouping rebuilds every stage. The one being shown and the ones queued
    // follow by identity, and a stage whose evidence grew while it was shown (a
    // late answer from Mitz, a further page) starts over with all of it: the
    // map and the legend then tell the same events from the same moment.
    const rebuilt = item => action?.stages.find(next => next.id === item.id) || item;
    if (replayQueue) replayQueue = replayQueue.map(rebuilt);
    if (stage) {
      const grew = evidenceOf(rebuilt(stage)).length > evidenceOf(stage).length;
      stage = rebuilt(stage);
      if (grew) { suspend(); show(stage); }
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
    // restage starts the stage being shown over, for a change in how it is to
    // be presented.
    restage() { if (disposed || !stage) return; suspend(); show(stage); changed(); },
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
