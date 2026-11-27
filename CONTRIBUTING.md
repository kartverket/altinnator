# Contributing guides
## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/): install it and keep it running. It hosts the local Kind cluster.
- [mise](https://mise.jdx.dev/getting-started.html): manages the pinned development tools (Go, `kubectl`, `kind`, etc.).
  You don't need to install it up front. The **Altinnator** run configuration and the install script below prompt you to install it if it's missing.

## Getting started

- JetBrains IDE: press play on the **Altinnator** run configuration. It installs the pinned tools, creates the Kind cluster and starts the controller.
- Terminal: install mise once, then run tasks with `mise run <task>`:

```bash
./scripts/install-mise.sh   # prompts to install mise if missing
mise run dev:run            # run the controller locally, with the pinned tools and Kind cluster
```

- If mise is already installed, just run `mise run dev:run`.
- Go version comes from `go.mod`; `mise.toml` pins the other tools.
- Local cluster: Kind cluster `altinnator`, context `kind-altinnator`. Override with `KIND_CLUSTER_NAME` and `KUBECONTEXT`.

> [!IMPORTANT]
> **Set the mise-installed Go as GOROOT in JetBrains.** `mise install` registers it in `.idea/workspace.xml`, but the IDE
> only reads it when the project opens. If the project was already open (e.g. first press of play), click **Set up Go** in the
> *Go SDK is not specified for this project* banner (or **Settings → Go → GOROOT**) and select
> `~/.local/share/mise/installs/go/<version>`.
> After a Go version bump, run `mise install` again to update it.

## Run Altinnator locally

### Environment variables

Altinnator gets its Maskinporten tokens from [Texas](https://github.com/nais/texas), which answers on
`http://localhost:3000/api/v1/token`: next to Altinnator in its pod in kind, or from [compose.yaml](compose.yaml) when
Altinnator runs on your host. Texas holds the Maskinporten client and its key, so Altinnator never sees the key. Three
files in `config/manager/base` hold the configuration:

- `.env` becomes the ConfigMap `altinnator-env`, Altinnator's configuration. `ALTINNATOR_ALTINN_PLATFORM_BASE_URL`
  is where Altinnator reaches Altinn: the Altinn mock from [compose.yaml](compose.yaml) on `http://localhost:8090`.
  In kind, `localhost` is the pod itself, so `deployment.yaml` sets it to `http://host.docker.internal:8090`, Docker
  Desktop's name for your host. Run `docker compose up --build` for kind too.
- `texas.env` becomes the ConfigMap `altinnator-texas`, Texas's settings that are not secret: the Maskinporten client
  ID, and where Texas finds Maskinporten, which locally is the Altinn mock.
- `maskinporten.env` becomes the Secret `altinnator-maskinporten`, which only Texas reads. It holds the Maskinporten
  client's key. It is committed, and holds a made-up key that only the mock accepts. Altinnator only runs locally
  against the mock, never against TT02 or PROD, so never put a real key in it.

### Run on your host machine

- IDE: run the **Altinnator** configuration (runs **Setup** first, loads `config/manager/base/.env`). Stop with the
  stop button.

> [!IMPORTANT]
> The JetBrains [EnvFile](https://plugins.jetbrains.com/plugin/7861-envfile) plugin must be installed, otherwise the
> environment variables from `config/manager/base/.env` are not loaded when running in the IDE.

- Terminal (after installing mise, see above):

```bash
mise run dev:run
```

### Run the Altinn mock and Texas

[compose.yaml](compose.yaml) runs the Altinn mock on `localhost:8090` and Texas on `localhost:3000`, where Texas gets
its Maskinporten tokens from the mock:

```bash
docker compose up --build
```

### Run in the local cluster

- Builds the image, loads it into Kind, applies resources and waits for readiness:

```bash
mise run deploy
mise run undeploy   # remove the deployment
```

### Clean up the local cluster

The local Kind cluster is created automatically and stays around between runs. When you no longer need it, or want to
start from scratch, delete the cluster and everything in it with:

```bash
mise run cluster:clean
```

The next `mise run dev:run`, `mise run deploy` or `mise run chainsaw:test-all` creates a fresh cluster.

## Running tests

- Unit/integration tests use [envtest](https://github.com/kubernetes-sigs/controller-runtime/blob/main/tools/setup-envtest/README.md) 
(first run downloads binaries):

```bash
mise run test
```

- The behavioral tests use [Ginkgo](https://onsi.github.io/ginkgo/) and [Gomega](https://onsi.github.io/gomega/).
  Tests of CRD validation are among them, next to the types in [api/v1alpha1](api/v1alpha1), and run against envtest.
  For the best experience writing and running them in a JetBrains IDE, install the
  [Ginkgo plugin](https://plugins.jetbrains.com/plugin/17554-ginkgo). It requires:
  - IntelliJ IDEA Ultimate or GoLand (it relies on the Go plugin).
  - The Ginkgo CLI as test executor. Install it from the project root, which picks the version in `go.mod`:
    `go install github.com/onsi/ginkgo/v2/ginkgo`. See the [Ginkgo documentation](https://onsi.github.io/ginkgo/#installing-ginkgo) for details.

- End-to-end tests use [Chainsaw](https://kyverno.github.io/chainsaw/). They need the controller running
  (`mise run dev:run`, IDE, or `mise run deploy`):

```bash
mise run chainsaw:test-all
mise run chainsaw:test-single test/chainsaw/<kind>/<test>
```

## Linting

- Linting uses [golangci-lint](https://golangci-lint.run/), configured in `.golangci.yml` and installed by mise.
- CI runs `mise run lint` on pull requests, so run it locally first:

```bash
mise run lint       # check
mise run lint:fix   # check and apply automatic fixes
```

## Git conventions

### Commit strategy

- Separate features, bug fixes, and refactorings into different commits.
- Each commit should represent one logical change that compiles and passes tests on its own.
- Do not mix refactoring or unrelated changes with feature changes in the same commit.
- Keep changes and their corresponding tests in the same commit.

### Commit messages

- Limit the subject line to 50 characters.
- Capitalize the subject line.
- Use the imperative mood in the subject line.
- Do not end the subject line with a period.
- Separate the subject from the body with a blank line.
- Wrap the body at 72 characters.
- Use the body to explain what and why, not how.

Example:
```
Add secretRef support for Entra ID config

Previously, Entra ID credentials could only be configured via an
inline AzureAdApplication or by referencing an existing one. This
adds a third path: sourcing credentials directly from existing
Kubernetes secrets, mirroring the existing Maskinporten secretRef
behaviour.
```

## Managing dependencies and patching vulnerabilities

### Managing Kubernetes versions
This version is defined in the top of the `[env]` section in [mise.toml](mise.toml), and is used when running locally and 
when testing in CI/CD. Version should match the version running in your production Kubernetes cluster.

### Managing helper tools
Versions for helper tools like Kustomize, Chainsaw, Kubectl etc. are defined under the `[tools]` section in [mise.toml](mise.toml). 
Versions should be updated either regularly, or as is required when updating other dependencies (such as upgrading 
Chainsaw when updating Go version).

### Updating Go version
#### Updating Go version
1. Find a desired stable Go version [on the go.dev website](https://go.dev/dl/#stable)
2. Investigate changes between current and desired version, for instance through [the go release history page](https://go.dev/doc/devel/release),
   or by googling "Go <old_version> to <new_version> migration guide"
3. Ensure there exists a compatible Chainsaw version for the chosen Go version, for instance by checking [the latest Chainsaw releases](https://github.com/kyverno/chainsaw/releases).
   The version of Chainsaw must support the same major and minor version as your chosen Go version. Patch versions may
   typically be updated without updating Chainsaw. If necessary, update the Chainsaw tool version in [mise.toml](mise.toml)
4. Update Go version in `go.mod`
5. Perform dependency updates for direct dependencies with `go get -u ./...`
6. Run `go mod tidy`

#### Updating Golang base image version
1. Find a Golang docker image [on Docker Hub](https://hub.docker.com/_/golang/tags) corresponding to the Go version you
   updated to previously. As of August 2026, this is called "1.26.6-alpine3.24".
2. Copy the "index digest" (top left), and paste into all relevant dockerfiles:
   1. Dockerfile
   2. Dockerfile.goreleaser
   3. Dockerfile.altinnmock
