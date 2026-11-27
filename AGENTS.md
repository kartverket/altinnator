# AGENTS.md

Kubernetes operator that keeps Kartverket's resources in Altinn's Resource Registry in sync with custom resources
written by teams. See [README.md](README.md) for what it is, and [CONTRIBUTING.md](CONTRIBUTING.md) for how to run,
test and lint it.

## Resources

- Two kinds, both in `altinnator.kartverket.no/v1alpha1`: `MaskinportenSchemaResource` and `GenericAccessResource`,
  one for each Altinn resource type. Teams write them directly, and they aren't tied to Skiperator or other parts of
  SKIP.
- The spec stays close to Altinn's API. Leave out, or make optional, what a team doesn't need to set, and let
  Altinnator fill in what is the same for every resource.
- Every `MaskinportenSchemaResource` gets the same fixed policy. Policies for `GenericAccessResource` are not decided
  yet.

## Altinn

The Resource Registry's source is in [Altinn/altinn-auth](https://github.com/Altinn/altinn-auth/tree/main/src/apps/Altinn.ResourceRegistry).
Check it rather than Altinn Studio when it matters how Altinn behaves.

- We choose a resource's id, and it's unique across all of Altinn (`^[a-z0-9_-]{4,}$`). Creating one that exists
  gives 409.
- Delete is refused in PROD, so removing a resource means withdrawing it (`status: Withdrawn`, not visible, not
  delegable). Its id can never be used again.
- PUT replaces the whole resource, and even an unchanged PUT makes a new version, so compare before writing.
- Altinn can't tell Kartverket's teams apart: one token may change every Kartverket resource.
- 400, 401, 403, 404, 409 and 412 never succeed on retry. 503 and network errors may.
- A policy is XACML, sent as a file. Altinn answers bad XML with 500.
- `internal/altinnmock` plays the parts of Altinn that Altinnator calls, and keeps state, so tests can show that
  Altinnator leaves a matching resource alone, corrects a change made in Altinn and withdraws a resource. It is ported
  from altinn-management's mock. Keep it to what Altinnator and Texas call, and fix it when Altinn's source shows
  Altinn behaves differently. It also plays Maskinporten for Texas (`/token`, `/jwk` and its well-known metadata),
  with tokens that have no signature. `cmd/altinnmock` serves it on port 8090, and `compose.yaml` runs it with Texas
  for local runs.

## Conventions

- Validate no more than Altinn does, and take what Altinn returns as it is, without filling in defaults.
- Comment only a reason the code cannot show.
- Never log tokens or keys.
- Update `README.md` and `AGENTS.md` in the same PR as a change that affects them.
