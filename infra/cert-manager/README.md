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
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true
kubectl wait --for=condition=Available --timeout=120s \
  -n cert-manager deployment/cert-manager deployment/cert-manager-webhook
```

Then apply both `ClusterIssuer`s - Let's Encrypt uses the `email` in each
for certificate-expiry notices:

```
kubectl apply -f cluster-issuer-staging.yaml -f cluster-issuer-production.yaml
```

## Rollout

Every ingress in `infra/ovhcloud-test/values.yaml` starts on
`letsencrypt-staging` (see each `cert-manager.io/cluster-issuer`
annotation). Staging certs aren't trusted by browsers/clients, but issuance
is near-unlimited, so this is where to debug the HTTP-01 setup itself.

Once a host's staging cert issues successfully -
`kubectl describe certificate <host>-tls` shows `Ready: True` - switch that
host's annotation from `letsencrypt-staging` to `letsencrypt-production` and
redeploy. Do this one host at a time rather than all four at once:
production allows only 5 duplicate certs per domain per week, so a mistake
repeated across all four burns through that fast.
