# De Zonnebloem's EHR

> [!WARNING]
> **Use at your own risk.** Like the rest of `sandbox/`, this program was written mostly by AI models and has had
> only light human review. It exists to show the demo end to end, not to be depended on. `sandbox/app/README.md`
> carries the full note.

Zorgcentrum De Zonnebloem's own record system in the GF Sandbox: the source side of the marker proof
(`sandbox/DESIGN.md` §4 step 0b and §5.5). It lists the demo-pool clients in De Zonnebloem's FHIR store, shows each client's
allergies, medication and conditions, and adds an allergy with a free-text note. Plataan EHR finds such a record only
through the GF chain; a note with a `DEMO-` word comes back highlighted on the enriched record.

It talks to that FHIR store and nothing else: not the Knooppunt, not the NVI, not the sandbox. Adding a record needs
no NVI registration, because the seed already registered De Zonnebloem's data for every pool patient, per data
category, and the NVI records that data exists, not which entries. Clients outside the demo pool, including the
legacy Jan Jansen fixture, are omitted and their record and allergy routes return 404: Plataan cannot retrieve or
recycle them.

## Run

```shell
ZONNEBLOEM_FHIR_BASE_URL=http://localhost:7050/fhir/sunflower-patients go run ./sandbox/zonnebloem
```

Serves on `http://localhost:3001`. Under compose it is the `zonnebloem-ehr` service in the `sandbox` profile, on the
same port, and the sandbox's patient record links to it.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `3001` | listen port |
| `ZONNEBLOEM_FHIR_BASE_URL` | none, required | De Zonnebloem's FHIR store, for example `http://hapi-fhir:7050/fhir/sunflower-patients`. Startup refuses a missing value or one that is not an absolute http(s) URL |

## Records it creates

An `AllergyIntolerance` per saved form, with a server-assigned id, a SNOMED CT substance from a fixed list, today's
date (UTC) as `recordedDate`, the note, and `meta.tag`
`https://github.com/nuts-foundation/nuts-knooppunt/sandbox|user-created`. A per-patient recycle in the sandbox deletes
that patient's tagged records; a global reset clears the whole tenant.

## Exposure

Nothing here authenticates. Anyone who can reach a hosted instance can add a note to a pool patient, and every demo
that retrieves that patient shows it, highlighted, until the patient is recycled or the dataset reset. Substance and
status come from fixed lists, the note is capped at 500 characters, a form post from another site is refused, and both
applications escape what they render. The form uses Go's cross-origin protection: when `Sec-Fetch-Site` is absent
(as on a hosted HTTP URL), the host in `Origin` must match the request's host, including any port. Requests without
either header remain allowed for non-browser clients.

## Provenance

Manrope is vendored from Google Fonts (SIL Open Font License), latin and latin-ext subsets. The styling is ported
from the source screen in `sandbox/wireframe.html`.
