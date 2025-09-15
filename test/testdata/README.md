# Test data

This package provides scripts for injecting test data, for:

- end-to-end tests
- local docker compose setups

It creates the following test data structure with multiple mCSD directories.

## Care Organizations

### Care Home Sunflower

A fictional care organization providing elderly care.

- URA: 00000020

### Care2Cure Hospital

A fictional hospital organization.

- URA: 00000030

## mCSD Directory Structure

### Root Directory (LRZa)

The Dutch Landelijk Register Zorgaanbieders, a national registry of care providers. It is the authentic source for
organization names and URAs (primary identifier of care organizations).

**Contains:**

- Organization registrations for both care organizations
- mCSD-directory endpoints pointing to each organization's admin directory

### Admin Directories

Each care organization maintains its own admin directory containing:

- Organization resource with detailed information
- FHIR endpoints for accessing the organization's services

### Query Directory

The aggregated directory that contains all resources from both root and admin directories after the mCSD update process
runs.
