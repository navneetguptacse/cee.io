package security

import (
	"regexp"

	"cee.io/pkg/languages"
)

type ScanResult struct {
	Rejected bool
	Reason   string
}

type patternRule struct {
	re     *regexp.Regexp
	reason string
}

var sensitivePaths = []*regexp.Regexp{
	regexp.MustCompile(`/etc/passwd`),
	regexp.MustCompile(`/etc/shadow`),
	regexp.MustCompile(`/etc/group`),
	regexp.MustCompile(`/etc/gshadow`),
	regexp.MustCompile(`/etc/hostname`),
	regexp.MustCompile(`/etc/hosts`),
	regexp.MustCompile(`/proc/self/`),
	regexp.MustCompile(`/proc/1/`),
	regexp.MustCompile(`/proc/version`),
	regexp.MustCompile(`/proc/cpuinfo`),
	regexp.MustCompile(`/proc/meminfo`),
	regexp.MustCompile(`/proc/net/`),
	regexp.MustCompile(`/proc/mounts`),
	regexp.MustCompile(`/sys/class`),
	regexp.MustCompile(`/sys/devices`),
	regexp.MustCompile(`/dev/sd[a-z]`),
	regexp.MustCompile(`/dev/nvme`),
	regexp.MustCompile(`/dev/tcp`),
	regexp.MustCompile(`/dev/udp`),
	regexp.MustCompile(`/var/run/docker\.sock`),
	regexp.MustCompile(`/root/`),
}

var cCppPatterns = []patternRule{
	{regexp.MustCompile(`\bptrace\s*\(`), "ptrace syscall"},
	{regexp.MustCompile(`sys/ptrace\.h`), "ptrace header"},
	{regexp.MustCompile(`SYS_ptrace`), "ptrace syscall constant"},
	{regexp.MustCompile(`\bsocket\s*\(`), "socket syscall"},
	{regexp.MustCompile(`\bconnect\s*\(\s*\w+\s*,`), "network connect"},
	{regexp.MustCompile(`\bbind\s*\(\s*\w+\s*,`), "network bind"},
	{regexp.MustCompile(`\blisten\s*\(\s*\w+\s*,`), "network listen"},
	{regexp.MustCompile(`\baccept\s*\(`), "network accept"},
	{regexp.MustCompile(`\bmount\s*\(`), "mount syscall"},
	{regexp.MustCompile(`\bsyscall\s*\(\s*(101|200|435)\b`), "dangerous syscall number"},
	{regexp.MustCompile(`while\s*\(\s*1\s*\)\s*\{?\s*fork\s*\(`), "fork bomb"},
	{regexp.MustCompile(`while\s*\(fork\s*\(\)`), "fork bomb"},
	{regexp.MustCompile(`:\s*fork\s*\(\)\s*\|`), "fork bomb"},
	{regexp.MustCompile(`(?i)keylog`), "keylogger indicator"},
}

var javaPatterns = []patternRule{
	{regexp.MustCompile(`Runtime\s*\.\s*getRuntime\s*\(\s*\)\s*\.\s*exec`), "Runtime.exec()"},
	{regexp.MustCompile(`ProcessBuilder`), "ProcessBuilder"},
	{regexp.MustCompile(`java\.net\.Socket`), "network socket"},
	{regexp.MustCompile(`java\.net\.ServerSocket`), "server socket"},
	{regexp.MustCompile(`java\.net\.DatagramSocket`), "UDP socket"},
	{regexp.MustCompile(`java\.net\.URL`), "URL connection"},
	{regexp.MustCompile(`java\.net\.HttpURLConnection`), "HTTP connection"},
	{regexp.MustCompile(`sun\.misc\.Unsafe`), "Unsafe access"},
	{regexp.MustCompile(`java\.lang\.reflect`), "reflection"},
}

var pythonPatterns = []patternRule{
	{regexp.MustCompile(`\bos\s*\.\s*system\s*\(`), "os.system()"},
	{regexp.MustCompile(`\bsubprocess\b`), "subprocess module"},
	{regexp.MustCompile(`\bos\s*\.\s*popen\s*\(`), "os.popen()"},
	{regexp.MustCompile(`\bctypes\b`), "ctypes native call"},
	{regexp.MustCompile(`\bos\s*\.\s*fork\s*\(`), "os.fork()"},
	{regexp.MustCompile(`import\s+pty`), "pty module"},
	{regexp.MustCompile(`import\s+socket`), "socket module"},
	{regexp.MustCompile(`from\s+socket\s+import`), "socket module"},
	{regexp.MustCompile(`while\s+True\s*:\s*os\s*\.\s*fork`), "fork bomb"},
	{regexp.MustCompile(`exec\s*\(\s*__import__`), "dynamic import execution"},
	{regexp.MustCompile(`\b__import__\s*\(\s*['"]os['"]`), "dynamic os import"},
}

var jsPatterns = []patternRule{
	{regexp.MustCompile(`child_process`), "child_process module"},
	{regexp.MustCompile(`require\s*\(\s*['"]net['"]`), "net module"},
	{regexp.MustCompile(`require\s*\(\s*['"]dgram['"]`), "dgram module"},
	{regexp.MustCompile(`require\s*\(\s*['"]http['"]`), "http module"},
	{regexp.MustCompile(`require\s*\(\s*['"]https['"]`), "https module"},
	{regexp.MustCompile(`process\s*\.\s*binding\s*\(`), "process.binding()"},
	{regexp.MustCompile(`from\s+['"]child_process['"]`), "child_process module"},
	{regexp.MustCompile(`from\s+['"]net['"]`), "net module"},
}

var goPatterns = []patternRule{
	{regexp.MustCompile(`"os/exec"`), "os/exec package"},
	{regexp.MustCompile(`"net"`), "net package"},
	{regexp.MustCompile(`"net/http"`), "net/http package"},
	{regexp.MustCompile(`syscall\.ForkExec`), "syscall.ForkExec"},
	{regexp.MustCompile(`syscall\.Ptrace`), "syscall.Ptrace"},
	{regexp.MustCompile(`syscall\.Mount`), "syscall.Mount"},
}

var rustPatterns = []patternRule{
	{regexp.MustCompile(`std::process::Command`), "std::process::Command"},
	{regexp.MustCompile(`std::net`), "std::net network access"},
	{regexp.MustCompile(`libc::ptrace`), "libc::ptrace"},
}

// AnalyzeCode scans source code for obviously dangerous calls and sensitive paths.
func AnalyzeCode(sourceCode string, languageID int) ScanResult {
	if sourceCode == "" || languageID == languages.LangMultiFile {
		return ScanResult{Rejected: false}
	}

	for _, p := range sensitivePaths {
		if p.MatchString(sourceCode) {
			return ScanResult{
				Rejected: true,
				Reason:   "Access to sensitive path: " + p.String(),
			}
		}
	}

	var rules []patternRule
	switch languageID {
	case languages.LangC, languages.LangCPP:
		rules = cCppPatterns
	case languages.LangJava:
		rules = javaPatterns
	case languages.LangPython, 92:
		rules = pythonPatterns
	case languages.LangJavaScript, languages.LangTypeScript, 93, 94, 102:
		rules = jsPatterns
	case languages.LangGo, 95:
		rules = goPatterns
	case languages.LangRust:
		rules = rustPatterns
	}

	for _, rule := range rules {
		if rule.re.MatchString(sourceCode) {
			return ScanResult{
				Rejected: true,
				Reason:   "Forbidden operation: " + rule.reason,
			}
		}
	}

	return ScanResult{Rejected: false}
}
