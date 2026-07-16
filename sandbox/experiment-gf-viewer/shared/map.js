/* The journey strip is deliberately framework-free in both implementations.
   It watches exactly one attribute, data-s on #viewer, and translates it into
   the CSS classes the strip's animations key on. How data-s gets set is the
   only framework-specific part, and it lives in each implementation's page. */
(function () {
  const v = document.getElementById('viewer');
  const byId = (id) => document.getElementById(id);
  function fire(id, on) {
    const el = byId(id);
    if (!el) return;
    el.classList.remove('go');
    if (on) { void el.getBoundingClientRect(); el.classList.add('go'); }
  }
  let last = -1;
  function sync() {
    const n = parseInt(v.dataset.s || '0', 10);
    if (n === last) return;
    last = n;
    v.querySelectorAll('[data-js]').forEach((el) => el.classList.toggle('lit', n >= parseInt(el.dataset.js, 10)));
    byId('js-lock1').classList.toggle('unlocked', n >= 4);
    byId('js-lock2').classList.toggle('unlocked', n >= 4);
    byId('js-lock3').classList.toggle('unlocked', n >= 4 && n < 6);
    byId('js-lock3').classList.toggle('deny', n === 6);
    byId('jsp-mitzcheck').classList.toggle('deny', n === 6);
    fire('js-pseu', n === 1 || n === 3);
    fire('js-pointer', n === 1);
    fire('js-mitzpakket', n === 1);
    fire('js-q2', n === 3);
    fire('js-answer', n === 3);
    fire('js-q3', n === 3);
    fire('js-endpoint', n === 3);
    fire('js-evidence', n === 4 || n === 6);
    fire('js-ask', n === 4 || n === 6);
    fire('js-permit', n === 4);
    fire('js-denyp', n === 6);
    fire('js-denyback', n === 6);
    fire('js-keyback', n === 4);
    fire('js-keygo', n === 5);
    fire('js-doc', n === 5);
  }
  new MutationObserver(sync).observe(v, { attributes: true, attributeFilter: ['data-s'] });
  sync();
})();
