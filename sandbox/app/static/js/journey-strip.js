import { stepForEvent } from './journey-model.js';

export function createJourneyStrip(svg) {
  function light(step) {
    svg.querySelectorAll('[data-js]').forEach((el) => el.classList.toggle('lit', Number(el.dataset.js) <= step));
  }
  function setDeny(deny) {
    svg.querySelectorAll('#js-lock3, #jsp-mitzcheck').forEach((el) => el.classList.toggle('deny', deny));
  }
  function reset() {
    light(0);
    setDeny(false);
    document.body.dataset.s = '0';
  }
  function apply(evt) {
    const { step, outcome } = stepForEvent(evt);
    light(step);
    setDeny(outcome === 'deny');
    document.body.dataset.s = String(step);
  }
  reset();
  return { apply, reset };
}

const root = document.querySelector('#hood-dock .dock-map svg');
if (root) window.GFJourney = createJourneyStrip(root);
