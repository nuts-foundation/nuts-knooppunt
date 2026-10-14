import { buildActionGroups, callIdentity, stageTimeline } from './journey-model.js';

const systemClock = {
  now: () => performance.now(),
  setTimeout: (fn, delay) => setTimeout(fn, delay),
  clearTimeout: timer => clearTimeout(timer),
};

export function createPlayback({ clock = systemClock, onStage, onChange, onCursor,
  cursor, active = true, settleMs = 1000 } = {}) {
  let events = [], groups = [], action = null, stage = null;
  // The action as it was when the stage being shown started. The map took the
  // stage's evidence then, and the legend and the caption tell the same
  // evidence until the stage ends or starts over.
  let shownAction = null;
  // A stage plays for length ms. While its timer runs it ends at deadline;
  // otherwise left ms of it remain. marks are the moments inside it at which
  // the view changes (stageTimeline): the timer wakes at each of them and at
  // the end. The clock is the real one, so a timer that fires late, or time
  // spent drawing, moves the stage on by as much as it took. drawn is the
  // moment of the stage the view last showed.
  let timer = null, settling = null, length = 0, left = 0, deadline = 0, marks = [], drawn = 0;
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
  const leftNow = () => timer === null ? left : Math.max(0, deadline - clock.now());
  // The evidence that has yet to be shown, by call id: what has not been shown
  // of the stages still to be played, of the action being shown and of newer
  // ones. An older action is not gone back to. Before the first action starts
  // that is the newest action's; the older ones are taken as shown when it does.
  function unshown() {
    const stages = started ? groups.filter(group => !action || group.time >= action.time).flatMap(pending) : pending(latest());
    return new Set(stages.flatMap(evidenceOf).map(callIdentity).filter(id => !watched.has(id)));
  }
  function state() {
    return { events, groups, action, stage, stageAction: stage ? shownAction : null, selected, delivered, unshown: unshown(),
      paused: paused || !active, elapsed: stage ? (length - leftNow()) / 1000 : 0,
      status: disposed ? 'ended' : paused || !active ? 'paused' : stage ? 'playing' : !ready ? 'settling' : complete ? 'complete' : 'waiting' };
  }
  const changed = () => { drawn = stage ? length - leftNow() : 0; onChange?.(state()); };
  const latest = () => groups.filter(group => group.stages.length).at(-1);
  // A stage is still to be played while any of its events, or of the consent
  // checks that belong to it, has not been shown.
  const evidenceOf = item => [...item.events, ...(item.checks || [])];
  const pending = group => group?.stages.filter(item => evidenceOf(item).some(event => !watched.has(callIdentity(event)))) || [];
  function schedule() {
    if (disposed || paused || !active || !stage || timer !== null) return;
    if (!acknowledged) { acknowledged = true; remember(evidenceOf(stage)); }
    deadline = clock.now() + left;
    arm();
  }
  // arm sets the timer for the next mark, or the end, from the real time. A
  // mark the stage passed while the view was being drawn is drawn at once.
  function arm() {
    const remaining = Math.max(0, deadline - clock.now()), elapsed = length - remaining;
    const missed = marks.some(value => value > drawn && value <= elapsed);
    const mark = marks.find(value => value > elapsed);
    timer = clock.setTimeout(wake, missed ? 0 : Math.ceil(mark === undefined ? remaining : mark - elapsed));
  }
  // wake draws the stage at the moment it really is, or ends it.
  function wake() {
    timer = null;
    left = Math.max(0, deadline - clock.now());
    if (left === 0) { stage = null; advance(); return; }
    const shown = stage;
    changed();
    if (timer === null && stage === shown && !disposed && !paused && active) arm();
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
  // show puts a stage on the map with the action it belongs to, and starts its
  // clock at zero. The stage is marked as shown when that clock runs (schedule).
  function show(next, from = action) {
    const timeline = stageTimeline(next);
    stage = next; shownAction = from; complete = false; length = left = Math.round(timeline.seconds * 1000);
    marks = [...new Set([timeline.identity, timeline.role, timeline.consent, timeline.query, timeline.source, timeline.settled]
      .filter(value => value > 0 && value < timeline.seconds).map(value => Math.round(value * 1000)))].sort((a, b) => a - b);
    acknowledged = false;
    onStage?.(next, from);
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
    // Regrouping rebuilds every stage, and a stage takes its id from its first
    // call, which may have left the retained window. A stage follows by its id,
    // or else by a call it still holds; a queued stage with no call left is
    // dropped. The stage being shown keeps the evidence it started with, unless
    // it gained some (a late answer from Mitz, a further page): then it starts
    // over with all of it, and the map and the legend tell the same events from
    // the same moment. While nothing of its action has left the window, it
    // follows the action as it is now, which may have gained steps.
    const rebuilt = item => {
      const ids = new Set(evidenceOf(item).map(callIdentity));
      return action?.stages.find(next => next.id === item.id) ||
        action?.stages.find(next => evidenceOf(next).some(event => ids.has(callIdentity(event)))) || null;
    };
    if (replayQueue) replayQueue = replayQueue.map(rebuilt).filter(Boolean);
    if (stage) {
      const known = new Set(evidenceOf(stage).map(callIdentity)), next = rebuilt(stage);
      const retained = new Set(action?.stages.flatMap(evidenceOf).map(callIdentity));
      const intact = action?.id === shownAction?.id && shownAction.stages.flatMap(evidenceOf).every(event => retained.has(callIdentity(event)));
      if (next && evidenceOf(next).some(event => !known.has(callIdentity(event)))) { suspend(); show(next); }
      else if (next && intact) { stage = next; shownAction = action; }
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
    restage() { if (disposed || !stage) return; suspend(); show(stage, shownAction); changed(); },
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
