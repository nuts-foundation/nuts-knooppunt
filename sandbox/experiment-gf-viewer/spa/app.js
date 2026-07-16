/* GF viewer · SPA client. The server sends JSON over SSE; this app owns the
   state (step, items) and renders the cards client-side with Preact + htm.
   Note the duplication this buys us: the card markup below must stay in sync
   with shared/scenario.js's server-side renderItem(), which the hypermedia
   variants use directly. */
import { html, render, useState, useEffect } from './preact-standalone.js';

function Card({ item }) {
  const icon = item.otel
    ? html`<span class="cic otel">·</span>`
    : html`<span class="cic">${item.n}</span>`;
  const chip = item.summary
    ? null
    : item.otel
      ? html`<span class="chip sp">OTEL SPAN</span>`
      : html`<span class="chip ev">STEP EVENT</span>`;
  return html`<div class="card${item.otel ? ' is-otel' : ''}">
    ${icon}
    <div class="cbody"><b>${item.title}</b><span>${item.desc}</span></div>
    <div class="cstat"><span class="ok">✓ ${item.ms} ms</span>${chip}</div>
  </div>`;
}

function Item({ item }) {
  return item.kind === 'lane'
    ? html`<div class="lane">${item.label}</div>`
    : html`<${Card} item=${item} />`;
}

function Panel() {
  const [step, setStep] = useState(0);
  const [items, setItems] = useState([]);

  useEffect(() => {
    const es = new EventSource('/run');
    es.onmessage = (e) => {
      const msg = JSON.parse(e.data);
      if (msg.type === 'step') setStep(msg.n);
      else if (msg.type === 'item') setItems((prev) => [...prev, msg.item]);
      else if (msg.type === 'finished') es.close(); // stop EventSource auto-reconnect
    };
    return () => es.close();
  }, []);

  /* The journey strip stays vanilla and outside the component tree; it only
     watches the data-s attribute, so project the step state onto it. */
  useEffect(() => {
    document.getElementById('viewer').dataset.s = String(step);
  }, [step]);

  return html`${items.map((item, i) => html`<${Item} key=${i} item=${item} />`)}`;
}

render(html`<${Panel} />`, document.getElementById('cards'));
