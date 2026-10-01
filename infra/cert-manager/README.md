# cert-manager

TLS for the test environment's public ingresses (nuts-knooppunt#610), HTTP-01
challenge solved through the existing ingress-nginx controller - no DNS
provider credentials needed, since there are 4 fixed hosts under
`*.test.gf.nuts.nl`, not a wildcard cert.

Installed the same way as ingress-nginx itself: manually, not through
Terraform or `ci.yaml` - a cluster add-on, not part of the application
release.

## Install

```
helm install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --version v1.21.2 \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true
kubectl wait --for=condition=Available --timeout=120s \
  -n cert-manager deployment/cert-manager deployment/cert-manager-webhook
```

Then apply both `ClusterIssuer`s - Let's Encrypt still requires an `email`
on the account even though it stopped sending expiry notices to it
([2025-01-22](https://letsencrypt.org/2025/01/22/ending-expiration-emails)):

```
kubectl apply -f cluster-issuer-staging.yaml -f cluster-issuer-production.yaml
```

## Rollout

Every ingress in `infra/ovhcloud-test/values.yaml` goes straight to
`letsencrypt-production` (see each `cert-manager.io/cluster-issuer`
annotation). The HTTP-01 setup was validated against `letsencrypt-staging`
by hand, on each host in turn, before any of this was committed - staging
certs aren't trusted by browsers/clients, but issuance is near-unlimited,
so that's the issuer to debug against if the setup ever needs changing.
Production allows only 5 duplicate certs per domain per week, so re-validate
against staging first for any change that could cause repeated issuance
(e.g. ingress annotation or host changes), one host at a time.
