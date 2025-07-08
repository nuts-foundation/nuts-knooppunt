# Architecture documentation

The GF Adressing follows the IHE [mCSD profile](https://profiles.ihe.net/ITI/mCSD/index.html).

## Application services

The Application has the following services:

- Data entry service: Entering data into a directory.
- Query service: Querying data from the directory.
- Update service: Updating data in the directory.
- Feed service: Feeding data into the directory.

![Addressering Application Layer](https://github.com/user-attachments/assets/ef86b090-8122-4cc5-9d02-34bf473c6e2c)

## Query service

The query service offers the following process:

The actor `Care Giver` performs an action which requires a query to the `Directory Service`. This is performed by the `Query Client` which sends a query to the `Directory`. The directory processes the query and returns the results back to the client.

The query process is descibed in the IHE mCSD [ITI-90](https://profiles.ihe.net/ITI/mCSD/ITI-90.html) transaction.

![Address Query Process](https://github.com/user-attachments/assets/3bc72493-dc13-41a4-b68e-7d928cf502ba)

## Update Service

The update service is provided to parties with a directory and wants to keep an up to date copy of the content of other directories.
It consists of two application roles: Update Client and the Directory.

The update process is described in the IHE mCSD [ITI-91](https://profiles.ihe.net/ITI/mCSD/ITI-91.html) transaction.

The update process can be triggered by an event. Usually this is a timer event.
After being triggered, the update client uses a list of directories to query. It uses the `_since` parameter to query the update server for changes since the last update.

The directory must support the `_since` parameter and return the changes since the last update.
![Address Sync Process](https://github.com/user-attachments/assets/cdbba74a-7331-49cf-897b-fb6094a79fc7)

The Update Client will then process the changes by consolidating the changes into the local one and and update the local directory by using the feed service as described in the IHE mCSD [ITI-130](https://profiles.ihe.net/ITI/mCSD/ITI-130.html) transaction.

### Consolidating from multiple sources

Each update client will be configured with a local target directory which the updates will be applied to, an optional list of authentic directories for specific properties (such as identifiers like `AGB-code` or `Organization-Type`) and one directory which is the authentic source of Organizations, their unique identifier and their directory endpoint.

The Update Client will start with the authentic source directory to get a list of organisations and their identifier (URA). Then it uses the provided endpoints to query the directories. This process will be repeated for each optional authentic directory. The last step is to query the local target directory for each of the changed organisations and consolidate the changes.

The feed client (which is part of of the Update Client) will then use the feed the updated resources back into the local target directory.
![Address Sync detail](https://github.com/user-attachments/assets/7d852042-e3ec-4c1a-b14b-4f57f5651032)

## Authoritative Directories

Each directory can be configured to be authoritative for a specific set of properties. For this the FHIRPath language is used. Each directory can contain a list of FHIRPath expressions which are used to determine if the directory is authoritative for a specific property.
If a property is claimed by a directory, the value from another directory will be ignored.

When multiple directories are configured to be authoritative values can be combined. If multiple values are not allowed, the behaviour is undefined and should result in an error.

For example, the LRZa directory is authoritative for the `identifier` of type `URA` and the `name` of type `official`. When the Organization's directory also provides a `name` of type `official`, the value should be ignored.

When consolidating, a common agreed identifier per resource must be used. For Organizations in the the dutch healthcare sector this is the `URA`. Each directory who has a system wide unique resource must identify it with the agreed `identifier`.

Examples of FHIRPath expressions for authoritative properties:

- URA identifiers of Organizations: `Organization.identifiers.where(type='http://fhir.nl/fhir/NamingSystem/ura')", "Endpoint"`
- Endpoints for mCSD directory services: `Endpoint.connectionType.coding.where(system='http://fhir.nl/fhir/NamingSystem/endpoint-connection-type').where(code='mCSD-directory')`
