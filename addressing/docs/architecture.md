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
![Address Sync Process](https://github.com/user-attachments/assets/5113906a-1377-4231-80e6-c349e0690d82)

The Update Client will then process the changes by consolidating the changes into the local one and and update the local directory by using the feed service as described in the IHE mCSD [ITI-130](https://profiles.ihe.net/ITI/mCSD/ITI-130.html) transaction.


