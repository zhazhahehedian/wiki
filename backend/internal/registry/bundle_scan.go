package registry

import (
	"path"
	"regexp"
	"strings"
)

// BundleFinding marks a path that looks like it ships a credential. Findings
// are shown to reviewers and owners; they never block an upload.
type BundleFinding struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

const (
	maxBundleFindings   = 50
	maxScannedFileBytes = 1 << 20
	maxScannedBytes     = 64 << 20
)

var secretPatterns = []struct {
	kind    string
	pattern *regexp.Regexp
}{
	{"private_key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |ENCRYPTED )?PRIVATE KEY-----`)},
	{"cloud_access_key", regexp.MustCompile(`\b(?:AKIA|LTAI)[0-9A-Za-z]{12,20}\b`)},
	{"api_token", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36}|xox[abprs]-[A-Za-z0-9-]{10,}|sk-[A-Za-z0-9_-]{32,})`)},
}

// scanBundleName flags credential-like file names and VCS metadata.
func scanBundleName(name string, directory bool) string {
	base := strings.ToLower(path.Base(name))
	if directory {
		if base == ".git" {
			return "vcs_directory"
		}
		return ""
	}
	switch {
	case base == ".env" || (strings.HasPrefix(base, ".env.") && !strings.HasSuffix(base, ".example") && !strings.HasSuffix(base, ".sample") && !strings.HasSuffix(base, ".template")):
		return "env_file"
	case base == "id_rsa" || base == "id_dsa" || base == "id_ecdsa" || base == "id_ed25519":
		return "private_key"
	case strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx"):
		return "private_key"
	case base == ".npmrc" || base == ".pypirc" || base == ".netrc" || base == ".git-credentials" || base == "credentials.json":
		return "credential_file"
	case strings.HasPrefix(name, ".git/") || strings.Contains(name, "/.git/"):
		return "vcs_directory"
	}
	return ""
}

func scanBundleContent(data []byte) string {
	for _, p := range secretPatterns {
		if p.pattern.Match(data) {
			return p.kind
		}
	}
	return ""
}

type bundleScanner struct {
	findings []BundleFinding
	scanned  int
	seen     map[string]bool
}

func (s *bundleScanner) add(name, kind string) {
	if kind == "" || len(s.findings) >= maxBundleFindings {
		return
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if key := name + "\x00" + kind; !s.seen[key] {
		s.seen[key] = true
		s.findings = append(s.findings, BundleFinding{Path: name, Kind: kind})
	}
}

// wantsContent reports whether a file of this size is still inside the scan budget.
func (s *bundleScanner) wantsContent(size uint64) bool {
	return size <= maxScannedFileBytes && s.scanned+int(size) <= maxScannedBytes && len(s.findings) < maxBundleFindings
}
