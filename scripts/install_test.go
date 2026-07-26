package scripts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const (
	oldVersion    = "1.0.0"
	targetVersion = "1.1.0"
	olderVersion  = "0.9.0"
)

type releaseOptions struct {
	checksumMismatch   bool
	malformedManifest  bool
	signatureMissing   bool
	interruptedArchive bool
	latestVersion      string
}

type releaseServer struct {
	URL      string
	requests func() []string
	close    func()
}

func TestInstallSupportedPlatforms(t *testing.T) {
	for _, platform := range []struct {
		os, machine string
	}{
		{"Linux", "x86_64"},
		{"Linux", "aarch64"},
		{"Darwin", "x86_64"},
		{"Darwin", "arm64"},
	} {
		t.Run(platform.os+"-"+platform.machine, func(t *testing.T) {
			h := newHarness(t, releaseOptions{})
			h.unameOS = platform.os
			h.unameMachine = platform.machine
			result := h.run("--yes")
			assertSuccess(t, result)
			assertInstalledVersion(t, h.prefix, targetVersion)
			if !strings.Contains(result.stdout, "unofficial community project") {
				t.Fatalf("missing unofficial-project notice:\n%s", result.stdout)
			}
		})
	}
}

func TestInstallUpgradeAndSameVersion(t *testing.T) {
	h := newHarness(t, releaseOptions{})
	h.installExisting(oldVersion, "old")

	result := h.run("--yes")
	assertSuccess(t, result)
	assertInstalledVersion(t, h.prefix, targetVersion)
	if !strings.Contains(result.stdout, "current: "+oldVersion) ||
		!strings.Contains(result.stdout, "target:  "+targetVersion) {
		t.Fatalf("missing current-to-target summary:\n%s", result.stdout)
	}

	before := len(h.server.requests())
	result = h.run("--yes")
	assertSuccess(t, result)
	if !strings.Contains(result.stdout, "already installed") {
		t.Fatalf("same-version run was not an idempotent no-op:\n%s", result.stdout)
	}
	if got := len(h.server.requests()); got != before+1 {
		t.Fatalf("same-version run fetched release assets: requests before=%d after=%d", before, got)
	}
}

func TestInstallExplicitVersionAndDowngradePolicy(t *testing.T) {
	t.Run("explicit version", func(t *testing.T) {
		h := newHarness(t, releaseOptions{})
		result := h.run("--yes", "--version=v1.0.0")
		assertSuccess(t, result)
		assertInstalledVersion(t, h.prefix, oldVersion)
	})

	t.Run("downgrade refused", func(t *testing.T) {
		h := newHarness(t, releaseOptions{})
		h.installExisting(targetVersion, "keep")
		before := mustRead(t, filepath.Join(h.prefix, "inwx"))
		result := h.run("--yes", "--version=v1.0.0")
		assertFailureContains(t, result, "refusing downgrade")
		if after := mustRead(t, filepath.Join(h.prefix, "inwx")); !bytes.Equal(before, after) {
			t.Fatal("downgrade refusal changed the existing binary")
		}
	})

	t.Run("explicit downgrade allowed", func(t *testing.T) {
		h := newHarness(t, releaseOptions{})
		h.installExisting(targetVersion, "replace")
		result := h.run("--yes", "--version=v1.0.0", "--allow-downgrade")
		assertSuccess(t, result)
		assertInstalledVersion(t, h.prefix, oldVersion)
	})

	t.Run("latest never downgrades", func(t *testing.T) {
		h := newHarness(t, releaseOptions{latestVersion: oldVersion})
		h.installExisting(targetVersion, "keep")
		result := h.run("--yes")
		assertFailureContains(t, result, "refusing downgrade")
	})
}

func TestInstallVerificationFailuresPreserveBinary(t *testing.T) {
	for _, test := range []struct {
		name      string
		options   releaseOptions
		extraEnv  []string
		wantError string
	}{
		{
			name:      "checksum mismatch",
			options:   releaseOptions{checksumMismatch: true},
			wantError: "SHA-256 mismatch",
		},
		{
			name:      "malformed manifest",
			options:   releaseOptions{malformedManifest: true},
			wantError: "malformed checksums.txt",
		},
		{
			name:      "signature missing",
			options:   releaseOptions{signatureMissing: true},
			wantError: "missing checksums.txt.sig",
		},
		{
			name:      "signature failure",
			extraEnv:  []string{"FAKE_COSIGN_FAIL=1"},
			wantError: "Cosign verification failed",
		},
		{
			name:      "interrupted download",
			options:   releaseOptions{interruptedArchive: true},
			wantError: "could not download",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, test.options)
			h.installExisting(oldVersion, "preserve-this-binary")
			path := filepath.Join(h.prefix, "inwx")
			before := mustRead(t, path)
			result := h.runEnv(test.extraEnv, "--yes")
			assertFailureContains(t, result, test.wantError)
			if after := mustRead(t, path); !bytes.Equal(before, after) {
				t.Fatal("failed upgrade did not preserve the existing binary")
			}
		})
	}
}

func TestInstallRejectsUnsupportedPlatformAndInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name, os, machine, argument, want string
	}{
		{"os", "Plan9", "amd64", "", "unsupported OS"},
		{"architecture", "Linux", "riscv64", "", "unsupported architecture"},
		{"relative prefix", "Linux", "amd64", "--prefix=relative", "absolute directory"},
		{"malformed version", "Linux", "amd64", "--version=v1.2", "invalid version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, releaseOptions{})
			h.unameOS = test.os
			h.unameMachine = test.machine
			args := []string{"--yes"}
			if test.argument != "" {
				args = append(args, test.argument)
			}
			assertFailureContains(t, h.run(args...), test.want)
		})
	}
}

func TestInstallRefusesUnknownExistingBinary(t *testing.T) {
	h := newHarness(t, releaseOptions{})
	mustMkdirAll(t, h.prefix)
	path := filepath.Join(h.prefix, "inwx")
	writeExecutable(t, path, "#!/bin/sh\nprintf 'not inwx\\n'\n")
	before := mustRead(t, path)
	result := h.run("--yes")
	assertFailureContains(t, result, "could not determine the existing inwx version")
	if after := mustRead(t, path); !bytes.Equal(before, after) {
		t.Fatal("unknown existing binary was changed")
	}
}

func TestInstallCustomPrefixAndPathHint(t *testing.T) {
	h := newHarness(t, releaseOptions{})
	custom := filepath.Join(h.root, "custom", "bin")
	result := h.run("--yes", "--prefix="+custom, "--version=v1.0.0")
	assertSuccess(t, result)
	assertInstalledVersion(t, custom, oldVersion)
	if !strings.Contains(result.stdout, custom+" is not in PATH") {
		t.Fatalf("missing PATH hint:\n%s", result.stdout)
	}
}

func TestInstallCosignIdentityIsExact(t *testing.T) {
	h := newHarness(t, releaseOptions{})
	result := h.run("--yes")
	assertSuccess(t, result)
	log := string(mustRead(t, h.cosignLog))
	want := `^https://github\.com/k2b-dev/inwx-cli/\.github/workflows/release\.yml@refs/tags/v1\.1\.0$`
	if !strings.Contains(log, want) ||
		!strings.Contains(log, "https://token.actions.githubusercontent.com") {
		t.Fatalf("Cosign identity was not bound to exact workflow, tag, and issuer:\n%s", log)
	}
}

func TestInstallHelpAndCredentialBoundary(t *testing.T) {
	script := mustRead(t, "install.sh")
	text := string(script)
	for _, forbidden := range []string{
		"INWX_USERNAME",
		"INWX_PASSWORD",
		"INWX_SHARED_SECRET",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("installer contains credential path %s", forbidden)
		}
	}
	command := exec.Command("/bin/sh", "install.sh", "--help")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("installer help failed: %v\n%s", err, output)
	}
	for _, want := range []string{
		"--system",
		"--prefix=DIR",
		"--version=vX.Y.Z",
		"--allow-downgrade",
		"--yes",
		"unofficial community installer",
		"never reads or stores INWX credentials",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("installer help missing %q:\n%s", want, output)
		}
	}
}

func TestInstallerCommandsAreDocumentedExactly(t *testing.T) {
	repositoryRoot := filepath.Clean("..")
	canonical := "curl -fsSL https://raw.githubusercontent.com/k2b-dev/inwx-cli/main/scripts/install.sh | sh"
	pinned := "curl -fsSL https://raw.githubusercontent.com/k2b-dev/inwx-cli/v0.1.0/scripts/install.sh | sh -s -- --version=v0.1.0"
	for _, path := range []string{
		filepath.Join(repositoryRoot, "README.md"),
		filepath.Join(repositoryRoot, "docs-site", "docs", "en", "installation.md"),
	} {
		content := string(mustRead(t, path))
		for _, command := range []string{canonical, pinned} {
			if !strings.Contains(content, command) {
				t.Fatalf("%s does not contain the exact installer command %q", path, command)
			}
		}
	}
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

type harness struct {
	t            *testing.T
	root         string
	home         string
	prefix       string
	bin          string
	cosign       string
	cosignLog    string
	server       releaseServer
	unameOS      string
	unameMachine string
}

func newHarness(t *testing.T, options releaseOptions) *harness {
	t.Helper()
	root := t.TempDir()
	h := &harness{
		t:            t,
		root:         root,
		home:         filepath.Join(root, "home"),
		prefix:       filepath.Join(root, "home", ".local", "bin"),
		bin:          filepath.Join(root, "test-bin"),
		cosignLog:    filepath.Join(root, "cosign.log"),
		unameOS:      "Linux",
		unameMachine: "x86_64",
	}
	mustMkdirAll(t, h.home)
	mustMkdirAll(t, h.bin)
	h.writeFakeUname()
	h.cosign = filepath.Join(h.bin, "cosign")
	writeExecutable(t, h.cosign, `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_COSIGN_LOG"
[ "${FAKE_COSIGN_FAIL:-0}" != "1" ] || exit 1
`)
	h.server = startReleaseServer(t, options)
	t.Cleanup(h.server.close)
	return h
}

func (h *harness) writeFakeUname() {
	writeExecutable(h.t, filepath.Join(h.bin, "uname"), `#!/bin/sh
case "$1" in
  -s) printf '%s\n' "$FAKE_UNAME_S" ;;
  -m) printf '%s\n' "$FAKE_UNAME_M" ;;
  *) exit 2 ;;
esac
`)
}

func (h *harness) installExisting(version, marker string) {
	h.t.Helper()
	mustMkdirAll(h.t, h.prefix)
	writeExecutable(h.t, filepath.Join(h.prefix, "inwx"), binaryScript(version, marker))
}

func (h *harness) run(args ...string) commandResult {
	return h.runEnv(nil, args...)
}

func (h *harness) runEnv(extra []string, args ...string) commandResult {
	h.t.Helper()
	script, err := filepath.Abs("install.sh")
	if err != nil {
		h.t.Fatal(err)
	}
	command := exec.Command("/bin/sh", append([]string{script}, args...)...)
	command.Dir = h.root
	path := h.bin + string(os.PathListSeparator) + os.Getenv("PATH")
	command.Env = append(os.Environ(),
		"HOME="+h.home,
		"PATH="+path,
		"FAKE_UNAME_S="+h.unameOS,
		"FAKE_UNAME_M="+h.unameMachine,
		"FAKE_COSIGN_LOG="+h.cosignLog,
		"INWX_INSTALL_RELEASE_BASE="+h.server.URL+"/releases",
		"INWX_INSTALL_API_BASE="+h.server.URL+"/api",
		"INWX_INSTALL_COSIGN_BIN="+h.cosign,
	)
	command.Env = append(command.Env, extra...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func startReleaseServer(t *testing.T, options releaseOptions) releaseServer {
	t.Helper()
	latest := options.latestVersion
	if latest == "" {
		latest = targetVersion
	}
	assets := make(map[string][]byte)
	for _, version := range []string{olderVersion, oldVersion, targetVersion} {
		tag := "v" + version
		hashes := make([]string, 0, 4)
		for _, platform := range []string{
			"inwx_linux_amd64.tar.gz",
			"inwx_linux_arm64.tar.gz",
			"inwx_darwin_amd64.tar.gz",
			"inwx_darwin_arm64.tar.gz",
		} {
			archive := makeArchive(t, version)
			assets["/releases/download/"+tag+"/"+platform] = archive
			sum := fmt.Sprintf("%x", sha256.Sum256(archive))
			if options.checksumMismatch && version == targetVersion && platform == "inwx_linux_amd64.tar.gz" {
				sum = strings.Repeat("0", 64)
			}
			hashes = append(hashes, sum+"  "+platform)
		}
		manifest := strings.Join(hashes, "\n") + "\n"
		if options.malformedManifest && version == targetVersion {
			manifest = "not-a-checksum  inwx_linux_amd64.tar.gz\n"
		}
		assets["/releases/download/"+tag+"/checksums.txt"] = []byte(manifest)
		if !options.signatureMissing {
			assets["/releases/download/"+tag+"/checksums.txt.sig"] = []byte("test signature\n")
		}
		assets["/releases/download/"+tag+"/checksums.txt.pem"] = []byte("test certificate\n")
	}

	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, request.URL.Path)
		mu.Unlock()
		if request.URL.Path == "/api/releases/latest" {
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(response, `{"tag_name":"v%s"}`, latest)
			return
		}
		body, ok := assets[request.URL.Path]
		if !ok {
			http.NotFound(response, request)
			return
		}
		if options.interruptedArchive &&
			strings.HasSuffix(request.URL.Path, "inwx_linux_amd64.tar.gz") &&
			strings.Contains(request.URL.Path, "/v"+targetVersion+"/") {
			response.Header().Set("Content-Length", fmt.Sprint(len(body)+100))
			_, _ = response.Write(body[:len(body)/2])
			return
		}
		_, _ = response.Write(body)
	}))
	return releaseServer{
		URL: server.URL,
		requests: func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), requests...)
		},
		close: server.Close,
	}
}

func makeArchive(t *testing.T, version string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range map[string]string{
		"inwx":    binaryScript(version, "release-"+version),
		"LICENSE": "MIT License\n",
	} {
		header := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if name == "LICENSE" {
			header.Mode = 0o644
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func binaryScript(version, marker string) string {
	return fmt.Sprintf(`#!/bin/sh
if [ "${1:-}" = "version" ]; then
  printf 'inwx %s (commit test, built test)\n'
  exit 0
fi
printf '%s\n'
`, version, marker)
}

func assertSuccess(t *testing.T, result commandResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("command failed: %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
	}
}

func assertFailureContains(t *testing.T, result commandResult, want string) {
	t.Helper()
	if result.err == nil {
		t.Fatalf("command unexpectedly succeeded\nstdout:\n%s", result.stdout)
	}
	if output := result.stdout + result.stderr; !strings.Contains(output, want) {
		t.Fatalf("failure does not contain %q\nstdout:\n%s\nstderr:\n%s", want, result.stdout, result.stderr)
	}
}

func assertInstalledVersion(t *testing.T, prefix, version string) {
	t.Helper()
	command := exec.Command(filepath.Join(prefix, "inwx"), "version")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("installed binary failed: %v", err)
	}
	if !strings.HasPrefix(string(output), "inwx "+version+" ") {
		t.Fatalf("installed version mismatch: %s", output)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
