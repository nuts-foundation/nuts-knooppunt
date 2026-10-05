# De Zonnebloem's EHR

> [!WARNING]
> **Use at your own risk.** Like the rest of `sandbox/`, this package was written mostly by AI models and has had
> only light human review. It exists to show the demo end to end, not to be depended on. `sandbox/app/README.md`
> carries the full note.

Zorgcentrum De Zonnebloem's own record system in the GF Sandbox: the source side of the marker proof
(`sandbox/DESIGN.md` §4 step 0b and §5.5). It lists the demo-pool clients in De Zonnebloem's FHIR store, shows each client's
allergies, medication and conditions, adds an allergy with a free-text note, and removes an allergy added that way.
Plataan EHR finds such a record only through the GF chain: a note with a `DEMO-` word comes back highlighted on the
enriched record, and a removed record is gone from the next retrieval.

It talks to that FHIR store and nothing else: not the Knooppunt, not the NVI, and not the sandbox, although the
sandbox's process serves it. Adding a record needs no NVI registration, because the seed already registered De
Zonnebloem's data for every pool patient, per data category, and the NVI records that data exists, not which entries.
Clients outside the demo pool, including the legacy Jan Jansen fixture, are omitted and their record and allergy routes
return 404: Plataan cannot retrieve or recycle them.

## Run

```shell
ZONNEBLOEM_FHIR_BASE_URL=http://localhost:7050/fhir/sunflower-patients go run ./sandbox/app
```

The gf-sandbox process serves the EHR on `http://localhost:3001`, next to the sandbox on 8091, whenever
`ZONNEBLOEM_FHIR_BASE_URL` names De Zonnebloem's FHIR store; `ZONNEBLOEM_EHR_PORT` moves it. `sandbox/app/README.md`
lists both with the rest of the process's configuration. Under compose the `gf-sandbox` service publishes both ports,
and the sandbox's patient record links to the EHR.

## Records it creates

An `AllergyIntolerance` per saved form, with a server-assigned id, a SNOMED CT substance from a fixed list, today's
date (UTC) as `recordedDate`, the note, and `meta.tag`
`https://github.com/nuts-foundation/nuts-knooppunt/sandbox|user-created`.

The record screen removes one of these again (`POST /clients/{id}/allergies/{allergyID}/remove`). It reads the allergy
first and deletes it only when it belongs to that client and carries the tag. A seeded record cannot be removed here:
the seed's NVI registrations say De Zonnebloem holds that category of data for the client, and the seeded records keep
that true. A per-patient recycle in the sandbox deletes that patient's tagged records; a global reset clears the whole
tenant.

## Exposure

Nothing here authenticates. Anyone who can reach a hosted instance can add a note to a pool patient, and every demo
that retrieves that patient shows it, highlighted, until the patient is recycled or the dataset reset. They can also
remove a record a demo added, though not a seeded one. Substance and status come from fixed lists, the note is capped at
500 characters, a form post from another site is refused, and both applications escape what they render. Both forms
use Go's cross-origin protection: when `Sec-Fetch-Site` is absent
(as on a hosted HTTP URL), the host in `Origin` must match the request's host, including any port. Requests without
either header remain allowed for non-browser clients.

## Provenance

Manrope is vendored from Google Fonts (SIL Open Font License), latin and latin-ext subsets. The palette and typeface
follow the source screen in `sandbox/wireframe.html`; the layout is the EHR's own (`sandbox/DESIGN.md` §6.3).
