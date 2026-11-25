# LRZA sync: capacity estimate

The size of the LRZA Directory determines how long a sync takes, and so the limits the sync runs under
(see [Settings derived from this estimate](#settings-derived-from-this-estimate)). No official count was
available when this estimate was made (November 2026), so it is derived from what the Implementation Guide
says the Directory contains and from public statistics on care providers. It deliberately errs on the high
side. Replace it with the LRZA's own figures once known.

## What the LRZA Directory contains

From the [Care Services Directory](https://minvws.github.io/generiekefuncties-docs/en/csd.html) page of the
Generieke Functies IG:

- **Organization**: imported from the KvK and the Dezi-register. "An Organization SHALL have a URA
  identifier, or be `partOf` another Organization." A KvK vestiging becomes an Organization of its own
  "when a URA number is linked to [it]".
- **Location**: "Establishments (NL: vestigingen) registered in the KvK are imported by the LRZa Directory
  as Locations."
- **HealthcareService, Endpoint, OrganizationAffiliation**: registered by service providers (IT vendors) on
  behalf of care providers. Small at first, growing with adoption.

The IG leaves open whether every care provider registered in the KvK is imported, or only those holding a
URA. This estimate assumes every care provider, which gives the larger directory.

## Public figures

- CBS counts 154,373 providers of care and welfare in 2024
  ([86025NED](https://www.cbs.nl/nl-nl/cijfers/detail/86025ned)).
- The CBS business register counts 219,407 registered businesses in care and welfare in 2024, 199,243 of
  them sole proprietorships
  ([source](https://www.cbs.nl/nl-nl/longread/aanvullende-statistische-diensten/2025/data-over-zelfstandigen-in-zorg-en-welzijn-trends-ondernemingen-klanten/3-ondernemingen-in-zorg-en-welzijn)).
- No published count was found for URA holders or for the LRZA register itself.

## Estimate

| Resource type | Resources | Pages of 100 | Basis |
|---|---|---|---|
| Organization | 150,000–250,000 | 1,500–2,500 | the CBS figures above |
| Location | 200,000–300,000 | 2,000–3,000 | about one vestiging per sole proprietorship, several per larger provider |
| HealthcareService, Endpoint, OrganizationAffiliation | up to 300,000 together | up to 3,000 | grows with adoption; each at most in the order of Organization |
| **Full sync** | **550,000–850,000** | **5,500–8,500** | |

The largest single type is about 3,000 pages.

An incremental sync reads only the changes since the previous one, so it is far smaller. The IG advises
syncing about every 15 minutes.

## Sync duration

A page costs one GET to the LRZA plus one transaction of up to 100 PUTs to the query directory. Assuming
about 1 second per page (not yet measured), a full sync takes **1.5 to 2.5 hours**. LRZA rate limiting, a
slower query directory, or a smaller maximum page size on the LRZA side make it longer.

A full sync runs on the first sync after every start, because the sync timestamp is kept in memory only.
While it runs it holds the sync slot, so syncs after a write wait for it.

## Settings derived from this estimate

All in `component.go`:

- `syncTimeout` (6 hours) bounds a whole sync: more than twice the upper estimate of a full sync. It guards
  against a sync that never ends, for example a server that keeps returning a `next` link.
- `pageTimeout` (5 minutes) bounds the time between two processed pages, so a sync that stops making
  progress fails long before `syncTimeout`. One page is at most two requests of
  `httpclient.DefaultRequestTimeout` (2 minutes) each.

The cron job that triggers `/lrza/update` can run more often than a full sync takes: a call that comes in
while a sync runs waits for it, until the caller's request times out.

## Open questions for the LRZA (MinVWS / CIBG)

1. What rate limit does the SLA set for Update Clients?
