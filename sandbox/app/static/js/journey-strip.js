import { journeyTargets, accessPresentation, vaultOpen, stageTimeline } from './journey-model.js';

const consentStates = ['consent-permit', 'consent-deny', 'consent-error'];
const lockStates = { open: 'lock-open', deny: 'lock-deny', error: 'lock-error' };

const mitzRoute = '#js-mitz, #js-auth-server, #jsp-mitzcheck';

export function createJourneyStrip(svg) {
  const select = selector => svg?.querySelectorAll(selector) || [];
  function stop() { select('.go').forEach(el => el.classList.remove('go')); }
  // clearStage removes what belongs to the stage that was shown: the lit route
  // and the packets' outcome. Mitz's answer, the locks and the vault are set
  // per stage by the functions below, which leave an unchanged state alone. The
  // retrieved copy is the record now and is never a stage's.
  function clearStage() {
    stop();
    select('[data-js]').forEach(el => el.classList.toggle('lit', el.dataset.js === '0'));
    select('.deny, .event-error, .token-received, .token-attached').forEach(el => el.classList.remove('deny', 'event-error', 'token-received', 'token-attached'));
  }
  function reset() { clearStage(); setShield(null); setLocks(null); setVault(false); }
  // outcome marks a refusal or a failure on what it selects. delay is how far
  // into the stage that shows: nothing looks refused before the refusal exists.
  function outcome(selector, value, delay = 0) {
    select(selector).forEach(el => {
      el.style.setProperty('--outcome-delay', `${delay}s`);
      el.classList.toggle('deny', value === 'deny');
      el.classList.toggle('event-error', value === 'error');
    });
  }
  function animate(selector, path, label, result, { delay = 0, duration = 1.1 } = {}) {
    select(`${selector}-label`).forEach(el => { el.textContent = label; });
    select(selector).forEach(el => {
      el.classList.remove('go');
      el.style.offsetPath = `path("${path}")`;
      el.style.setProperty('--go-delay', `${delay}s`);
      el.style.setProperty('--go-duration', `${duration}s`);
      el.getBoundingClientRect();
      el.classList.add('go');
    });
    outcome(selector, result);
  }
  // setShield shows Mitz's answer for the request being shown on its shield,
  // delay seconds into the stage. consent is the stage's consentPresentation,
  // or null for a stage that is not about a source request. An unchanged answer
  // is left alone unless a stage is starting (again): each request asks anew.
  function setShield(consent, delay = 0, again = false) {
    const next = { permit: 'consent-permit', deny: 'consent-deny', error: 'consent-error' }[consent?.state];
    select('#js-mitz').forEach(el => {
      const current = consentStates.find(state => el.classList.contains(state));
      if (current === next && !again) return;
      if (current) el.classList.remove(current);
      el.style.setProperty('--mitz-delay', `${delay}s`);
      if (next) { if (again) el.getBoundingClientRect(); el.classList.add(next); }
    });
  }
  // setLocks gives each lock the state of the action's lockPresentation. When a
  // stage starts (replay), every lock it decides plays its change, shut until
  // its moment: each request is checked anew. Otherwise a lock whose state did
  // not change is left alone, so a redraw starts nothing over. delays says, per
  // lock, how far into the stage its change shows.
  function setLocks(locks, delays = {}, replay = false) {
    select('.js-lock').forEach(el => {
      const name = el.dataset.lock;
      const next = lockStates[locks?.[name]?.state];
      const current = Object.values(lockStates).find(state => el.classList.contains(state));
      const again = replay && locks?.[name]?.plays;
      if (current === next && !again) return;
      if (current) el.classList.remove(current);
      el.style.setProperty('--lock-delay', `${delays[name] || 0}s`);
      if (again) el.getBoundingClientRect(); // lets the removal take effect, so the opening restarts
      if (next) el.classList.add(next);
    });
  }
  // The vault follows the locks (vaultOpen): it opens after them when a request
  // is served, again for every request.
  function setVault(open, again = false, delay = 0) {
    select('#js-vault').forEach(el => {
      const current = el.classList.contains('vault-open');
      if (current === open && !(open && again)) return;
      el.classList.remove('vault-open');
      el.style.setProperty('--vault-delay', `${delay}s`);
      if (open) { el.getBoundingClientRect(); el.classList.add('vault-open'); }
    });
  }
  // setEvidence redraws what the stage being shown says about consent and the
  // locks, and nothing else: an answer can arrive after its request is on the
  // map, and the animation in progress is left alone.
  function setEvidence({ consent = null, locks = null } = {}) { setShield(consent); setLocks(locks); setVault(vaultOpen(locks), false, 0); }
  // The answer that travels back from Mitz, in the order the lock takes its
  // state: a refusal, then a failed answer, and a permit only when every answer
  // was one.
  function answerLabel(checks) {
    const answers = checks.map(item => item.response?.consentDecision).filter(Boolean);
    const failed = checks.find(item => item.outcome === 'error');
    if (answers.some(answer => answer !== 'Permit')) return 'DENY';
    if (failed) return failed.response?.status ? String(failed.response.status) : '';
    return answers.length ? 'PERMIT' : '';
  }
  function applyStage(stage, { reducedMotion = false, consent = null, locks = null } = {}) {
    clearStage();
    const checks = stage.checks || [];
    const exchange = stage.events.some(item => item.gf === 'exchange');
    const access = accessPresentation(stage);
    // When each thing the stage changes shows; the legend follows the same
    // timeline through the playback.
    const timing = stageTimeline(stage);
    setShield(consent, checks.length && !reducedMotion ? timing.shield : 0, true);
    setLocks(locks, reducedMotion || !exchange ? {}
      : { identity: timing.identity, role: timing.role, consent: timing.consent, query: timing.query }, true);
    const event = stage.events[0];
    const targets = journeyTargets(event);
    if (!targets) return;
    const result = stage.events.some(item => item.outcome === 'error') ? 'error'
      : stage.events.some(item => item.outcome === 'deny') ? 'deny' : 'ok';
    select(targets).forEach(el => el.classList.add('lit'));
    const checkResult = checks.some(item => item.outcome === 'error') ? 'error' : checks.some(item => item.outcome === 'deny') ? 'deny' : 'ok';
    // The source's outcome shows when its response sets off, Mitz's when its
    // answer does: a request the source checks first is not refused on arrival.
    outcome(targets, result, exchange && !reducedMotion ? timing.response.delay : 0);
    if (checks.length) {
      select(mitzRoute).forEach(el => el.classList.add('lit'));
      outcome('#jsp-mitzcheck', checkResult, reducedMotion ? 0 : timing.answer.delay);
    }
    if (access.tokenReceived) select('#js-response').forEach(el => el.classList.add('token-received'));
    if (access.tokenAttached) select('#js-request').forEach(el => el.classList.add('token-attached'));
    // With the action's locks the vault follows them; a stage shown by itself
    // opens it on its own returned data.
    const vaultDelay = reducedMotion ? 0 : timing.vault ?? 0;
    if (locks) setVault(vaultOpen(locks), locks.query.plays, vaultDelay);
    else setVault(access.dataReturned, true, vaultDelay);
    if (reducedMotion) return;
    const path = [...select(targets)].find(el => el.getAttribute('d'))?.getAttribute('d');
    if (!path) return;
    if (stage.purpose === 'consent-notification') {
      // One packet, from Mitz to De Plataan: the response sprite runs the route
      // backwards.
      animate('#js-response', path, 'NOTIFY', result);
      return;
    }
    const methods = [...new Set(stage.events.map(item => item.request?.method || 'REQUEST'))];
    const responses = [...new Set(stage.events.map(item => item.response?.status).filter(Boolean))];
    // A request that is checked on arrival travels in as any request does.
    animate('#js-request', path, access.tokenRequest ? 'ACCESS' : methods.length > 1 ? `${methods[0]} +${methods.length - 1}` : methods[0], exchange ? 'ok' : result, timing.request);
    if (checks.length) {
      const mitzPath = [...select('#jsp-mitzcheck')][0]?.getAttribute('d');
      const answer = answerLabel(checks);
      if (mitzPath) {
        animate('#js-question', mitzPath, 'CONSENT?', 'ok', timing.question);
        if (answer) animate('#js-answer', mitzPath, answer, checkResult, timing.answer);
      }
    }
    if (responses.length) animate('#js-response', path, access.tokenReceived ? 'KEY' : responses.length > 1 ? `${responses[0]} +${responses.length - 1}` : String(responses[0]), result, timing.response);
  }
  function setCopy(state) {
    select('#js-copy').forEach(el => {
      el.classList.remove('copy-held', 'copy-removed');
      if (state === 'held') el.classList.add('copy-held');
      if (state === 'removed') el.classList.add('copy-removed');
    });
  }
  reset();
  return { apply: (event, options) => applyStage({ events: [event] }, options), applyStage, setEvidence, reset, stop, setCopy };
}
