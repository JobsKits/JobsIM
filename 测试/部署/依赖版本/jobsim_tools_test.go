package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func multiSourceClient(t *testing.T, sources map[string]string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, ok := sources[r.URL.String()]
		if !ok {
			t.Fatalf("unexpected source: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header), Request: r}, nil
	})}
}
func TestJobsIMNodeLTSChoosesNumericLatestAndExactOfficialDigest(t *testing.T) {
	const index = `[{"version":"v28.0.0-alpha.1","lts":false,"files":[]},{"version":"v24.9.0","lts":"old","files":[]},{"version":"v26.13.1","lts":"current","files":["osx-arm64-tar","win-x64-zip","linux-arm64"]}]`
	for _, tt := range []struct{ os, arch, name string }{{"darwin", "arm64", "node-v26.13.1-darwin-arm64.tar.gz"}, {"windows", "amd64", "node-v26.13.1-win-x64.zip"}, {"linux", "arm64", "node-v26.13.1-linux-arm64.tar.gz"}} {
		t.Run(tt.os, func(t *testing.T) {
			v := versionResolver{multiSourceClient(t, map[string]string{nodeMetadataURL: index, "https://nodejs.org/dist/v26.13.1/SHASUMS256.txt": testSHA + "  " + tt.name + "\n"})}
			r, err := v.resolveNode(context.Background(), tt.os, tt.arch)
			if err != nil || r.Version != "26.13.1" || r.Filename != tt.name || r.SHA256 != testSHA {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}
func TestJobsIMNodeRejectsMissingDuplicateOrBadTargetDigest(t *testing.T) {
	const index = `[{"version":"v26.13.1","lts":"current","files":["linux-arm64"]}]`
	for _, sums := range []string{"", "wrong  node-v26.13.1-linux-arm64.tar.gz\n", testSHA + "  node-v26.13.1-linux-arm64.tar.gz\n" + testSHA + "  node-v26.13.1-linux-arm64.tar.gz\n"} {
		v := versionResolver{multiSourceClient(t, map[string]string{nodeMetadataURL: index, "https://nodejs.org/dist/v26.13.1/SHASUMS256.txt": sums})}
		if _, err := v.resolveNode(context.Background(), "linux", "arm64"); err == nil {
			t.Fatal("invalid digest accepted")
		}
	}
}
func TestJobsIMPostgresReportsVersionWithoutInventingBinary(t *testing.T) {
	client := metadataClient(t, postgresMetadataURL, `[{"major":"17","latestMinor":"12","supported":true,"current":false},{"major":"18","latestMinor":"7","supported":true,"current":true}]`, 200)
	code, out, stderr := execute(t, client, "--tool", "postgres", "--os", "darwin", "--arch", "arm64")
	if code != 0 || out != "postgres\tdarwin\tarm64\t18.7\t-\t-\t-\tunsupported\n" {
		t.Fatalf("%d %q %s", code, out, stderr)
	}
}
func TestJobsIMNATSUsesExactOfficialAssetAndDigest(t *testing.T) {
	const url = "https://github.com/nats-io/nats-server/releases/download/v2.15.3/nats-server-v2.15.3-linux-arm64.tar.gz"
	client := metadataClient(t, "https://api.github.com/repos/nats-io/nats-server/releases/latest", githubFixture("v2.15.3", "nats-server-v2.15.3-linux-arm64.tar.gz", url, "sha256:"+testSHA), 200)
	code, out, stderr := execute(t, client, "--tool", "nats-server", "--os", "linux", "--arch", "arm64")
	if code != 0 || !strings.Contains(out, "\t2.15.3\t") || !strings.Contains(out, url) {
		t.Fatalf("%d %q %s", code, out, stderr)
	}
}
