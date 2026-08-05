package httpapi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("the deliverable %s is missing: %v", rel, err)
	}
	return string(raw)
}

// sdd:verify TC-0090
func TestDeliveryArtifacts(t *testing.T) {
	dockerfile := readRepoFile(t, "deploy/docker/Dockerfile")

	t.Run("the build reaches no module proxy", func(t *testing.T) {
		// The module has no dependencies, so the build must not be allowed to fetch.
		if !strings.Contains(dockerfile, "GOPROXY=off") {
			t.Error("the build stage does not disable the module proxy")
		}
		for _, forbidden := range []string{"go mod download", "go get ", "apk add go"} {
			if strings.Contains(dockerfile, forbidden) {
				t.Errorf("the build stage runs %q, which needs the network", forbidden)
			}
		}
	})

	t.Run("the runtime is not root", func(t *testing.T) {
		if !strings.Contains(dockerfile, "USER arena") {
			t.Error("the image does not drop to a non-root user")
		}
		if !strings.Contains(dockerfile, "adduser") {
			t.Error("the image does not create its runtime user")
		}
	})

	t.Run("a health check is declared", func(t *testing.T) {
		if !strings.Contains(dockerfile, "HEALTHCHECK") {
			t.Error("the image declares no health check")
		}
		if !strings.Contains(dockerfile, "/healthz") {
			t.Error("the health check does not probe the health endpoint")
		}
	})

	t.Run("the image ships the commands the manual documents", func(t *testing.T) {
		for _, cmd := range []string{"arena", "evalctl", "faultctl", "sddctl"} {
			if !strings.Contains(dockerfile, "/usr/local/bin/"+cmd) {
				t.Errorf("the image does not install %s", cmd)
			}
		}
	})

	compose := readRepoFile(t, "deploy/docker/docker-compose.yml")

	t.Run("the stack declares its services and their dependencies", func(t *testing.T) {
		for _, service := range []string{"arena:", "order-api:", "traffic-generator:", "prometheus:"} {
			if !strings.Contains(compose, service) {
				t.Errorf("the compose stack does not declare %s", strings.TrimSuffix(service, ":"))
			}
		}
		if !strings.Contains(compose, "condition: service_healthy") {
			t.Error("the stack does not wait for its dependencies to become healthy")
		}
		if !strings.Contains(compose, "healthcheck:") {
			t.Error("the stack declares no health checks")
		}
		if strings.Count(compose, "context: ../..") < 2 {
			t.Error("the build contexts do not point at the repository root")
		}
	})

	t.Run("prometheus configuration matches the demo target", func(t *testing.T) {
		prom := readRepoFile(t, "deploy/prometheus/prometheus.yml")
		if !strings.Contains(prom, "order-api:8081") {
			t.Error("prometheus does not scrape the demo service")
		}
		rules := readRepoFile(t, "deploy/prometheus/rules.yml")
		for _, alert := range []string{"OrderApiHighErrorRate", "OrderApiHighLatency"} {
			if !strings.Contains(rules, alert) {
				t.Errorf("the alert rules do not declare %s", alert)
			}
		}
		// Every alert must name the fault case it corresponds to, so an operator can
		// reproduce it.
		if !strings.Contains(rules, "case_ref") {
			t.Error("the alert rules do not link an alert to a reproducible fault case")
		}
	})

	t.Run("the makefile exposes the documented workflow", func(t *testing.T) {
		mk := readRepoFile(t, "Makefile")
		for _, target := range []string{
			"build:", "test:", "check:", "demo:", "eval:", "serve:",
			"sdd-validate:", "image:", "up:", "down:",
		} {
			if !strings.Contains(mk, "\n"+target) {
				t.Errorf("the Makefile has no %s target", strings.TrimSuffix(target, ":"))
			}
		}
	})

	t.Run("continuous integration runs the governance gate offline", func(t *testing.T) {
		ci := readRepoFile(t, ".github/workflows/ci.yml")
		for _, step := range []string{
			"go vet ./...", "go test ./...", "go test -race ./...",
			"sddctl validate", "sddctl gate --stage deliver",
		} {
			if !strings.Contains(ci, step) {
				t.Errorf("continuous integration does not run %q", step)
			}
		}
		if !strings.Contains(ci, "CON-008") {
			t.Error("continuous integration does not assert the dependency-free constraint")
		}
	})
}

// stripYAMLComments removes whole-line comments, leaving the executable content of a
// workflow. Inline trailing comments are left alone: the line around them still runs.
func stripYAMLComments(yaml string) string {
	lines := strings.Split(yaml, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// sdd:verify TC-0092
func TestReleasePipeline(t *testing.T) {
	release := readRepoFile(t, ".github/workflows/release.yml")

	t.Run("a version tag is what triggers it", func(t *testing.T) {
		if !strings.Contains(release, `tags: ["v*"]`) {
			t.Error("the workflow is not triggered by a version tag")
		}
	})

	// Tag-push permission and workflow-run permission are granted separately, so a
	// pipeline reachable only by pushing a tag is one some maintainers cannot run.
	t.Run("it can also be started without pushing a tag", func(t *testing.T) {
		if !strings.Contains(release, "workflow_dispatch:") {
			t.Error("the workflow cannot be started manually")
		}
		if !strings.Contains(release, "--target") {
			t.Error("a manual run does not create the tag, so it cannot release without one already existing")
		}
	})

	t.Run("the image is built for linux/amd64 explicitly", func(t *testing.T) {
		if !strings.Contains(release, "linux/amd64") {
			t.Error("the workflow does not name the target platform")
		}
		if !strings.Contains(release, "--platform") {
			t.Error("the build does not pin a platform, so the artifact's target is whatever the runner happens to be")
		}
	})

	t.Run("the image carries the commit it was built from", func(t *testing.T) {
		if !strings.Contains(release, "org.opencontainers.image.revision=${GITHUB_SHA}") {
			t.Error("the image is not labelled with its source commit")
		}
		if !strings.Contains(release, `"${IMAGE}:${GITHUB_SHA}"`) {
			t.Error("the image is not tagged with the commit SHA, so a version tag that moves loses its provenance")
		}
		if !strings.Contains(release, `"${IMAGE}:${VERSION}"`) {
			t.Error("the image is not tagged with the version")
		}
	})

	t.Run("the release is usable without registry access", func(t *testing.T) {
		if !strings.Contains(release, "docker save") {
			t.Error("no loadable image tarball is produced, so a private package leaves the release unusable")
		}
		if !strings.Contains(release, "SHA256SUMS") {
			t.Error("the assets are not checksummed")
		}
		if !strings.Contains(release, "gh release create") {
			t.Error("the workflow never creates a release")
		}
	})

	// The substantive assertion. Every clause above can hold in a pipeline that
	// publishes first and verifies afterwards, which would defeat the point of
	// verifying at all — so the order is checked directly rather than assumed.
	t.Run("nothing is published before it has been gated and proven to start", func(t *testing.T) {
		// Comments are stripped first: prose describing the pipeline cannot publish
		// anything, and matching it would let a comment near the top of the file
		// satisfy — or, as it happens, falsely break — an ordering claim about steps.
		steps := stripYAMLComments(release)

		publish := strings.Index(steps, "docker push")
		if publish < 0 {
			t.Fatal("the workflow never publishes the image")
		}
		for _, precondition := range []struct {
			marker string
			why    string
		}{
			{"sddctl gate --stage deliver", "the delivery gate"},
			{"sddctl validate", "the governance check"},
			{"go test ./...", "the test suite"},
			{"/healthz", "the health check on the built image"},
		} {
			at := strings.Index(steps, precondition.marker)
			if at < 0 {
				t.Errorf("the workflow never runs %s (%q)", precondition.why, precondition.marker)
				continue
			}
			if at > publish {
				t.Errorf("%s runs after the image is published; it cannot prevent a bad release from shipping", precondition.why)
			}
		}
	})
}
