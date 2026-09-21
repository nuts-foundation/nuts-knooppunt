import { journeyTargets, accessPresentation } from './journey-model.js';

export function createJourneyStrip(svg) {
  const select = selector => svg?.querySelectorAll(selector) || [];
  function stop() { select('.go').forEach(el => el.classList.remove('go')); }
  function reset() {
    stop();
    select('[data-js]').forEach(el => el.classList.toggle('lit', el.dataset.js === '0'));
    select('.deny, .event-error, .token-received, .token-attached, .vault-open').forEach(el => el.classList.remove('deny', 'event-error', 'token-received', 'token-attached', 'vault-open'));
  }
  function outcome(selector, value) {
    select(selector).forEach(el => {
      el.classList.toggle('deny', value === 'deny');
      el.classList.toggle('event-error', value === 'error');
    });
  }
  function animate(selector, path, label, result) {
    select(`${selector}-label`).forEach(el => { el.textContent = label; });
    select(selector).forEach(el => {
      el.classList.remove('go');
      el.style.offsetPath = `path("${path}")`;
      el.getBoundingClientRect();
      el.classList.add('go');
    });
    outcome(selector, result);
  }
  function applyStage(stage, { reducedMotion = false } = {}) {
    reset();
    const event = stage.events[0];
    const targets = journeyTargets(event);
    if (!targets) return;
    const result = stage.events.some(item => item.outcome === 'error') ? 'error'
      : stage.events.some(item => item.outcome === 'deny') ? 'deny' : 'ok';
    select(targets).forEach(el => el.classList.add('lit'));
    outcome(targets, result);
    const access = accessPresentation(stage);
    if (access.tokenReceived) select('#js-response').forEach(el => el.classList.add('token-received'));
    if (access.tokenAttached) select('#js-request').forEach(el => el.classList.add('token-attached'));
    if (access.dataReturned) select('#js-vault').forEach(el => el.classList.add('vault-open'));
    if (reducedMotion) return;
    const path = [...select(targets)].find(el => el.getAttribute('d'))?.getAttribute('d');
    if (!path) return;
    const methods = [...new Set(stage.events.map(item => item.request?.method || 'REQUEST'))];
    const responses = [...new Set(stage.events.map(item => item.response?.status).filter(Boolean))];
    animate('#js-request', path, access.tokenRequest ? 'ACCESS' : methods.length > 1 ? `${methods[0]} +${methods.length - 1}` : methods[0], result);
    if (responses.length) animate('#js-response', path, access.tokenReceived ? 'KEY' : responses.length > 1 ? `${responses[0]} +${responses.length - 1}` : String(responses[0]), result);
  }
  reset();
  return { apply: (event, options) => applyStage({ events: [event] }, options), applyStage, reset, stop };
}
