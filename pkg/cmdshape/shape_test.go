package cmdshape

import (
	"testing"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func TestParse(t *testing.T) {
	tests := []struct {
		cmd      string
		shape    string
		filtered bool
		file     string
		noise    event.NoiseKind
	}{
		// File viewing
		{cmd: `sed -n '1,50p' main.go`, shape: "sed -n", file: "main.go", noise: event.NoiseFileView},
		{cmd: `sed -n -e '1,5p' a.go`, shape: "sed -n", file: "a.go", noise: event.NoiseFileView},
		{cmd: `sed -i '' 's/a/b/' x.go`, shape: "sed -i", file: "x.go", noise: event.NoiseFileView},
		{cmd: `cat -n README.md`, shape: "cat -n", file: "README.md", noise: event.NoiseFileView},
		{cmd: `head -n 40 docs/design.md`, shape: "head -n", file: "docs/design.md", noise: event.NoiseFileView},
		{cmd: `tail -50 log.txt`, shape: "tail", file: "log.txt", noise: event.NoiseFileView},
		{cmd: "cat > out.txt <<'EOF'\nhello\nEOF", shape: "cat", filtered: true, noise: event.NoiseOther},
		{cmd: `cat`, shape: "cat", noise: event.NoiseOther},

		// Searches
		{cmd: `grep -rn "func main" .`, shape: "grep -r", noise: event.NoiseSearch},
		{cmd: `grep -rl TODO pkg | head -20`, shape: "grep -r -l", filtered: true, noise: event.NoiseSearch},
		{cmd: `ls -la`, shape: "ls -l -a", noise: event.NoiseSearch},
		{cmd: `find . -maxdepth 2 -name '*.go'`, shape: "find -maxdepth", noise: event.NoiseSearch},
		{cmd: `git -C repo grep -n foo`, shape: "git grep", noise: event.NoiseSearch},

		// Build and test, with set-up commands and wrappers skipped
		{cmd: `cd app && go test -v ./... 2>&1 | tail -n 30`, shape: "go test -v", filtered: true, noise: event.NoiseBuildTest},
		{cmd: `export GOFLAGS=-mod=mod; CGO_ENABLED=0 go build ./...`, shape: "go build", noise: event.NoiseBuildTest},
		{cmd: `timeout 60 npm install --silent`, shape: "npm install --silent", noise: event.NoiseBuildTest},
		{cmd: `uv run pytest -q tests/`, shape: "pytest -q", noise: event.NoiseBuildTest},
		{cmd: `python3 -m pytest -x`, shape: "pytest -x", noise: event.NoiseBuildTest},
		{cmd: `npx tsc --noEmit`, shape: "tsc", noise: event.NoiseBuildTest},
		{cmd: `uv sync`, shape: "uv sync", noise: event.NoiseBuildTest},
		{cmd: `go test ./... > /dev/null`, shape: "go test", filtered: true, noise: event.NoiseBuildTest},

		// Data dumps
		{cmd: "python3 - <<'EOF'\nimport json\nprint(1)\nEOF", shape: "python3 inline", noise: event.NoiseData},
		{cmd: `python3 -c 'print(1)' | jq .`, shape: "python3 inline", filtered: true, noise: event.NoiseData},
		{cmd: `python3 scripts/survey.py`, shape: "python3 script", noise: event.NoiseData},
		{cmd: `gh api repos/x/y/pulls --jq '.[].title'`, shape: "gh api --jq", noise: event.NoiseData},
		{cmd: `kubectl get pods -o yaml`, shape: "kubectl get -o yaml", noise: event.NoiseData},
		{cmd: `git diff --stat`, shape: "git diff --stat", noise: event.NoiseData},

		// Values never reach the shape: tokens, profiles, paths, substitutions
		{cmd: `kubectl --token sk-abc123 get secrets`, shape: "kubectl get", noise: event.NoiseData},
		{cmd: `aws --profile acme-prod s3 ls`, shape: "aws s3 ls", noise: event.NoiseData},
		{cmd: `gh api repos/acme/secret-repo/issues`, shape: "gh api", noise: event.NoiseData},
		{cmd: `$(security find-generic-password -s acme-token) | head`, shape: "(computed command)", filtered: true, noise: event.NoiseOther},
		{cmd: `kubectl get pods -o jsonpath='{.items[*].metadata.name}'`, shape: "kubectl get -o", noise: event.NoiseData},
		{cmd: `git --no-pager log --oneline`, shape: "git log --oneline", noise: event.NoiseData},
		{cmd: `npm -g install typescript`, shape: "npm install", noise: event.NoiseBuildTest},

		// Loops, groups, comments and other odd input
		{cmd: `for f in a b; do cat "$f"; done`, shape: "cat", file: "$f", noise: event.NoiseFileView},
		{cmd: "# list files\nls", shape: "ls", noise: event.NoiseSearch},
		{cmd: `echo "=== status" && git status`, shape: "git status", noise: event.NoiseOther},
		{cmd: `cd /tmp`, shape: "cd", noise: event.NoiseOther},
		{cmd: `sudo -n env FOO=1 make -s test`, shape: "make -s", noise: event.NoiseBuildTest},
		{cmd: `echo $(git rev-parse HEAD) "it's"`, shape: "echo", noise: event.NoiseOther},
		{cmd: ``, shape: "", noise: event.NoiseOther},
		{cmd: `env`, shape: "", noise: event.NoiseOther},
		{cmd: `'unterminated`, shape: "unterminated", noise: event.NoiseOther},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			s := Parse(tt.cmd)
			if s.String() != tt.shape || s.Filtered != tt.filtered || s.File != tt.file || s.Noise != tt.noise {
				t.Errorf("got shape %q filtered=%v file=%q noise=%q; want %q, %v, %q, %q",
					s.String(), s.Filtered, s.File, s.Noise, tt.shape, tt.filtered, tt.file, tt.noise)
			}
		})
	}
}

func TestParseSedInPlaceIsAnEdit(t *testing.T) {
	if s := Parse(`sed -i.bak 's/a/b/' x.go`); !s.Edit || s.File != "x.go" {
		t.Errorf("got edit=%v file=%q, want an edit of x.go", s.Edit, s.File)
	}
	if s := Parse(`sed -n '1p' x.go`); s.Edit {
		t.Error("sed -n reported as an edit")
	}
}

// FuzzParse checks that no input makes Parse panic.
func FuzzParse(f *testing.F) {
	for _, s := range []string{`a | b && c; d`, "x <<E\ny\nE", `$(a "b)" c)`, "`x", `a 2>&1 >f`, `"\`, `(`} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, cmd string) { _ = Parse(cmd) })
}
