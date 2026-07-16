// The scripted demo run, shared verbatim by all three implementations so the
// comparison only measures the delivery mechanism, never the content.
//
// Precondition for the hypermedia rendering path (renderItem + the raw SSE
// template strings in datastar/ and htmx/): all strings below are trusted,
// static and single-line; the helpers do not HTML-escape and those SSE writers
// do not encode newlines. The SPA path is safe by construction: JSON.stringify
// encodes newlines and Preact escapes text children.

function card({ n, title, desc, ms, otel, summary }) {
  const icon = otel ? '<span class="cic otel">·</span>' : `<span class="cic">${n}</span>`;
  const chip = summary ? '' : (otel ? '<span class="chip sp">OTEL SPAN</span>' : '<span class="chip ev">STEP EVENT</span>');
  return `<div class="card${otel ? ' is-otel' : ''}">${icon}<div class="cbody"><b>${title}</b><span>${desc}</span></div><div class="cstat"><span class="ok">✓ ${ms} ms</span>${chip}</div></div>`;
}

function lane(label) {
  return `<div class="lane">${label}</div>`;
}

/* [delay ms since run start, step number for the journey strip, items]
   Items are plain data. The hypermedia servers render them server-side with
   renderItem(); the SPA server sends them as JSON and the client renders. */
const RUN = [
  [300,   1, [{ kind: 'lane', label: 'Share Anna · patiëntaanmelding' },
              { kind: 'card', n: 1, title: 'Publish pointer · NVI', desc: 'Localization record for pseudonym(BSN), holder URA 00000010.', ms: 45 },
              { kind: 'card', title: 'Pseudonymize · PRS', desc: 'The BSN becomes a pseudonym before it travels.', ms: 21, otel: true },
              { kind: 'card', n: 2, title: 'Subscribe · Mitz', desc: 'Notify De Plataan when Anna’s consent changes.', ms: 38 }]],
  [4800,  2, [{ kind: 'lane', label: 'Localize' },
              { kind: 'card', n: 3, title: 'Localize · NVI', desc: 'Who holds data on this patient? De Zonnebloem, URA 00000020.', ms: 184 }]],
  [8300,  3, [{ kind: 'lane', label: 'Address' },
              { kind: 'card', n: 4, title: 'Address · mCSD', desc: 'De Zonnebloem’s FHIR endpoint resolved in the care address book.', ms: 92 }]],
  [11800, 4, [{ kind: 'lane', label: 'Authorize · at the source' },
              { kind: 'card', n: 5, title: 'Access token · Nuts node', desc: 'Dezi VC, URA VC and Roletype VC presented for a token.', ms: 310 }]],
  [16000, 4, [{ kind: 'card', title: 'Front door · PEP', desc: 'Token introspection at De Zonnebloem’s entrance.', ms: 18, otel: true },
              { kind: 'card', title: 'Policy decision · PDP', desc: 'BGZ policy evaluated, including the Mitz consent check.', ms: 75, otel: true }]],
  [21000, 5, [{ kind: 'lane', label: 'Retrieve' },
              { kind: 'card', n: 6, title: 'Retrieve · FHIR', desc: 'The BGZ Bundle leaves the source, after every check said yes.', ms: 268 }]],
  [24800, 5, [{ kind: 'card', n: '✓', title: 'Run complete', desc: '6 step events, 3 interior spans, 0.9 s end to end.', ms: 908, summary: true }]],
];

function renderItem(item) {
  return item.kind === 'lane' ? lane(item.label) : card(item);
}

const RUN_END_MS = 26000;

/* Panel chrome, shared so every page looks identical. */
const PANEL_CSS = `
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: #14100c; color: #f0ebe1; font-family: Inter, sans-serif; font-size: 13px; display: flex; justify-content: center; padding: 30px 20px; }
  .panel { width: 560px; background: linear-gradient(180deg, #241e17 0%, #191410 100%); border: 1px solid rgba(240,235,225,0.12); border-radius: 16px; overflow: hidden; }
  .head { padding: 14px 22px 12px; border-bottom: 1px solid rgba(240,235,225,0.1); display: flex; align-items: baseline; gap: 10px; }
  .head h1 { font-family: Fraunces, serif; font-size: 17px; font-weight: 600; }
  .head .live { font-family: 'Fira Mono', monospace; font-size: 9px; letter-spacing: 0.12em; color: #7fc7bc; text-transform: uppercase; }
  .head a { margin-left: auto; color: #7fc7bc; font-size: 11.5px; }
  .mapwrap { padding: 8px 14px 0; }
  .mapwrap svg { width: 100%; height: auto; display: block; }
  #cards { padding: 6px 18px 20px; }
  .lane { font-size: 9.5px; font-weight: 700; letter-spacing: 0.14em; text-transform: uppercase; color: #8f8578; margin: 14px 0 8px; }
  .card { display: flex; gap: 12px; align-items: center; padding: 10px 13px; border: 1px solid rgba(240,235,225,0.14); border-radius: 13px; background: #251f18; margin-bottom: 8px; animation: cardin 0.35s ease; }
  .card.is-otel { border-style: dashed; background: #1e1913; margin-left: 26px; }
  @keyframes cardin { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; } }
  .cic { width: 27px; height: 27px; border-radius: 50%; display: flex; align-items: center; justify-content: center; flex-shrink: 0; font-family: 'Fira Mono', monospace; font-size: 10px; font-weight: 700; background: rgba(127,199,188,0.12); color: #7fc7bc; border: 1px solid rgba(127,199,188,0.4); }
  .cic.otel { background: #1e1913; color: #8f8578; border: 1px dashed rgba(240,235,225,0.3); font-size: 13px; }
  .cbody { flex: 1; min-width: 0; }
  .cbody b { font-size: 12px; font-weight: 600; display: block; }
  .cbody span { font-size: 10.5px; color: #b3a89a; }
  .cstat { text-align: right; flex-shrink: 0; }
  .cstat .ok { font-family: 'Fira Mono', monospace; font-size: 10.5px; color: #b5d18f; display: block; }
  .chip { display: inline-block; font-size: 8.5px; font-weight: 700; letter-spacing: 0.08em; border-radius: 999px; padding: 2px 9px; margin-top: 4px; }
  .chip.ev { color: #7fc7bc; background: rgba(127,199,188,0.1); border: 1px solid rgba(127,199,188,0.3); }
  .chip.sp { color: #8f8578; background: rgba(143,133,120,0.1); border: 1px dashed rgba(143,133,120,0.45); }
`;

const FONTS = `@import url('https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600&family=Inter:wght@400;500;600;700&family=Fira+Mono:wght@500&display=swap');`;

module.exports = { RUN, RUN_END_MS, PANEL_CSS, FONTS, renderItem };
