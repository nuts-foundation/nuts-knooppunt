export const GF_STEP = {
  pseudonym: 1,
  localization: 2,
  addressing: 3,
  authentication: 4,
  consent: 4,
  authorization: 4,
  exchange: 5,
};

export function stepForEvent(evt) {
  const step = GF_STEP[evt?.gf] ?? 0;
  const outcome = evt?.outcome === 'deny' || evt?.outcome === 'error' ? evt.outcome : 'ok';
  return { step, outcome };
}
