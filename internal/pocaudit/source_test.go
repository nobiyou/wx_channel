package pocaudit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCertificateStoreUsesOnlyCurrentUserX509Store(t *testing.T) {
	source := strings.ToLower(readRepoFile(t, "internal/poc/certstore_windows.go"))
	for _, required := range []string{"x509store", "storename]::root", "storelocation]::currentuser", ".add(", ".remove(", "sha256"} {
		if !strings.Contains(source, required) {
			t.Errorf("missing %q", required)
		}
	}
	for _, forbidden := range []string{"import-certificate", "certutil", "localmachine"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("forbidden certificate fallback %q", forbidden)
		}
	}
}

func TestCertificateSmokeSourceHasNoCollectionRuntimeDependencies(t *testing.T) {
	production := strings.ToLower(readRepoFile(t, "internal/poc/certificate_smoke.go") + readRepoFile(t, "internal/poc/certificate_smoke_windows.go"))
	for _, forbidden := range []string{"bridgestart", "runtimeproxy", "proxyfactory", "sunnynet", "nfapi", "wechatappex", "collectrestrictedpoc", "createtoken"} {
		if strings.Contains(production, forbidden) {
			t.Errorf("certificate smoke references %q", forbidden)
		}
	}
}

func TestSunnyNetInitDoesNotTuneNetwork(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "pkg", "sunnynet", "SunnyNet", "SunnyNet.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "init" || fn.Body == nil {
			return true
		}
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "SetNetworkConnectNumber" {
				t.Errorf("SunnyNet init must not call SetNetworkConnectNumber")
			}
			return true
		})
		return false
	})
}

func TestTrackedSourceContainsNoPrivateKey(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range bytes.Split(out, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		switch strings.ToLower(filepath.Ext(string(raw))) {
		case ".go", ".key", ".pem", ".cer", ".js", ".ps1", ".yaml", ".yml":
		default:
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(string(raw)))
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rsaMarker := []byte(strings.Join([]string{"BEGIN", "RSA", "PRIVATE", "KEY"}, " "))
		genericMarker := []byte(strings.Join([]string{"BEGIN", "PRIVATE", "KEY"}, " "))
		if bytes.Contains(data, rsaMarker) || bytes.Contains(data, genericMarker) {
			t.Errorf("tracked private key marker: %s", raw)
		}
	}
}

func TestPOCRuntimeDirectoriesAreIgnored(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{
		".poc-build/probe",
		".poc-tools/probe",
		".poc-secrets/probe",
		".poc-data/probe",
		".poc-runtime/probe",
		"var/probe",
	} {
		cmd := exec.Command("git", "check-ignore", "--quiet", name)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			t.Errorf("not ignored: %s", name)
		}
	}
}

func TestPinnedNFAPIDependencies(t *testing.T) {
	root := repoRoot(t)
	expected := map[string]string{
		"pkg/sunnynet/Resource/nfapi/dll/win32/nfapi.dll": "b6ad927ce7a5281f1b71be347b6ee4b920a8ef90f104c6a5cc56082fba0c3528",
		"pkg/sunnynet/Resource/nfapi/dll/x64/nfapi.dll":   "1d6f3487d3aa707b978e1a81f8e98250d334120b856b89780408eb98dbbd0910",
	}
	for name, want := range expected {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read pinned dependency %s: %v", name, err)
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != want {
			t.Errorf("pinned dependency hash mismatch: %s", name)
		}
	}
}
