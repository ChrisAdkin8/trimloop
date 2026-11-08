package cmdshape

// set makes a lookup table from a list of names.
func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// setup lists commands that prepare for the real work and print little.
// Parse skips them when choosing which command of several to describe.
var setup = set("cd", "pushd", "popd", "export", "set", "unset", "source", ".",
	"mkdir", "true", "false", ":", "sleep", "local", "alias", "ulimit", "umask",
	"trap", "wait", "shopt", "echo", "printf", "for", "while", "until", "if",
	"case", "done", "fi", "esac", "elif", "else", "then", "do", "function",
	"read", "rm", "touch", "chmod", "unalias", "clear")

// prefixes are words in front of a program that do not change what it
// prints.
var prefixes = set("sudo", "time", "nice", "nohup", "env", "command", "exec",
	"builtin", "timeout", "do", "then", "else", "!", "if", "while", "until")

// filters reduce what reaches the model when output is piped into them.
var filters = set("head", "tail", "grep", "egrep", "fgrep", "rg", "jq", "yq",
	"wc", "sort", "uniq", "awk", "sed", "cut", "tr", "less", "more", "column",
	"fold", "fmt")

// runners run another program; the shape describes that program. An empty
// list means the runner takes the program directly ("npx tsc"); otherwise
// it needs one of the listed subcommands first ("uv run pytest").
var runners = map[string][]string{
	"npx": nil, "bunx": nil, "pnpx": nil, "uvx": nil, "xargs": nil, "watch": nil,
	"uv": {"run"}, "poetry": {"run"}, "pipenv": {"run"}, "bundle": {"exec"},
	"pnpm": {"exec", "dlx"}, "npm": {"exec"}, "pipx": {"run"}, "hatch": {"run"},
}

// interpreters run code: inline (python3 -c, a here-document) or a script.
var interpreters = set("python", "python3", "node", "ruby", "perl", "bash", "sh", "zsh", "deno", "bun")

// scriptLangs are interpreters whose output is usually data printed by the
// agent's own code.
var scriptLangs = set("python", "python3", "node", "ruby", "perl", "deno")

// subcommands lists programs whose subcommand changes what they print, with
// how many words it has ("gh pr view" has two).
var subcommands = map[string]int{
	"go": 1, "git": 1, "npm": 1, "pnpm": 1, "yarn": 1, "bun": 1, "cargo": 1,
	"docker": 1, "kubectl": 1, "gh": 2, "pip": 1, "pip3": 1, "uv": 1, "brew": 1,
	"terraform": 1, "helm": 1, "aws": 2, "gcloud": 2, "dotnet": 1, "poetry": 1,
	"apt": 1, "apt-get": 1, "systemctl": 1, "swift": 1, "deno": 1, "conda": 1,
	"claude": 1, "golangci-lint": 1, "mvn": 1, "gradle": 1,
}

// globalFlags are on/off options that can come before a subcommand. Any
// other option there is taken to have a value, which is skipped.
var globalFlags = map[string][]string{
	"git":       {"--no-pager", "--paginate", "-P", "-p", "--bare", "--no-replace-objects", "--literal-pathspecs"},
	"docker":    {"--debug", "-D", "--tls"},
	"npm":       {"-g", "--global", "-s", "--silent", "-q", "--quiet", "-y", "--yes"},
	"pnpm":      {"-g", "--global", "-s", "--silent", "-r", "--recursive"},
	"yarn":      {"-s", "--silent"},
	"cargo":     {"-q", "--quiet", "-v", "--verbose", "--frozen", "--locked", "--offline"},
	"uv":        {"-q", "--quiet", "-v", "--verbose", "--no-cache", "--offline"},
	"brew":      {"-q", "--quiet", "-v", "--verbose", "-d", "--debug"},
	"pip":       {"-q", "--quiet", "-v", "--verbose"},
	"pip3":      {"-q", "--quiet", "-v", "--verbose"},
	"helm":      {"--debug"},
	"terraform": {"-no-color"},
}

// bundles lists programs whose single-letter flags can be combined: grep -rn.
var bundles = set("grep", "egrep", "fgrep", "rg", "ls", "sed", "du", "cat", "tail", "head", "curl", "jq", "tree", "wc")

// flagsByProgram lists, for each program, the flags that change how much it
// prints, in the order shapes show them. A flag ending in "=" is shown with
// its value ("-o yaml").
var flagsByProgram = map[string][]string{
	"sed":     {"-n", "-i", "--in-place"},
	"grep":    {"-r", "-R", "-l", "-L", "-c", "-m", "-o", "-q", "-A", "-B", "-C", "--max-count"},
	"egrep":   {"-r", "-R", "-l", "-L", "-c", "-m", "-o", "-q", "-A", "-B", "-C"},
	"fgrep":   {"-r", "-R", "-l", "-L", "-c", "-m", "-o", "-q", "-A", "-B", "-C"},
	"rg":      {"-l", "-c", "-m", "-o", "-q", "-A", "-B", "-C", "--files", "--max-count"},
	"ls":      {"-R", "-l", "-a"},
	"find":    {"-maxdepth"},
	"tree":    {"-L", "-d"},
	"cat":     {"-n"},
	"head":    {"-n", "-c"},
	"tail":    {"-n", "-c", "-f"},
	"go":      {"-v", "-json", "-x"},
	"pytest":  {"-v", "-vv", "-q", "-s", "-x", "--tb"},
	"npm":     {"--silent", "-s", "--quiet", "--loglevel"},
	"pnpm":    {"--silent", "-s", "--reporter"},
	"yarn":    {"--silent", "-s"},
	"git":     {"-p", "--patch", "--stat", "--oneline", "--name-only", "--name-status", "-q", "--quiet"},
	"docker":  {"-q", "--quiet", "--progress"},
	"curl":    {"-s", "-v", "-I", "-i"},
	"kubectl": {"-o=", "--output=", "-w", "--watch"},
	"gh":      {"--jq", "-q", "--json", "--limit", "-L"},
	"jq":      {"-r", "-c"},
	"cargo":   {"-q", "--quiet", "-v", "--verbose"},
	"make":    {"-s", "--silent"},
	"wc":      {"-l"},
	"du":      {"-h", "-s"},
}

// defaultFlags are checked for programs not in flagsByProgram.
var defaultFlags = []string{"-q", "--quiet", "--silent", "-v", "--verbose"}

// fileViewers print a file.
var fileViewers = set("cat", "head", "tail", "less", "more", "nl", "bat")

// searchers search or list files.
var searchers = set("grep", "egrep", "fgrep", "rg", "ag", "ack", "find", "fd", "ls", "tree", "du", "locate")

// builders build, test, lint or install.
var builders = set("go", "npm", "pnpm", "yarn", "bun", "pytest", "jest", "vitest",
	"tsc", "eslint", "cargo", "make", "docker", "mvn", "gradle", "gradlew",
	"pip", "pip3", "brew", "terraform", "golangci-lint", "govulncheck", "swift",
	"xcodebuild", "dotnet", "ruff", "mypy", "prettier", "shellcheck", "tox", "nox",
	"apt", "apt-get", "helm", "uv", "poetry", "conda", "rustc", "gcc", "clang", "cmake")

// dataDumpers print data from an API, a database or a cluster.
var dataDumpers = set("curl", "wget", "http", "kubectl", "aws", "gcloud", "az",
	"jq", "yq", "sqlite3", "duckdb", "psql", "mysql", "redis-cli", "mongosh", "printenv")
