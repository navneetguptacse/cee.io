package tests

import (
	"testing"

	"cee.io/pkg/languages"
	"cee.io/pkg/security"
)

func TestCodeAnalyzer_SensitivePaths(t *testing.T) {
	dangerous := []string{
		`open("/etc/passwd").read()`,
		`f = open("/etc/shadow")`,
		`cat /etc/hosts`,
		`check /proc/self/status`,
		`cat /dev/tcp/1.1.1.1/80`,
		`docker = open("/var/run/docker.sock")`,
	}

	for _, code := range dangerous {
		res := security.AnalyzeCode(code, languages.LangPython)
		if !res.Rejected {
			t.Errorf("expected code to be rejected, but passed: %s", code)
		}
	}
}

func TestCodeAnalyzer_LanguageSpecific(t *testing.T) {
	cases := []struct {
		code   string
		langID int
		reject bool
	}{
		// Python
		{`import os; os.system("ls")`, languages.LangPython, true},
		{`import subprocess; subprocess.run(["ls"])`, languages.LangPython, true},
		{`import ctypes; ctypes.CDLL(None)`, languages.LangPython, true},
		{`print("safe python code")`, languages.LangPython, false},

		// C / C++
		{`#include <sys/ptrace.h>`, languages.LangCPP, true},
		{`int s = socket(AF_INET, SOCK_STREAM, 0);`, languages.LangC, true},
		{`int main() { return 0; }`, languages.LangCPP, false},

		// Java
		{`Runtime.getRuntime().exec("calc");`, languages.LangJava, true},
		{`new ProcessBuilder("sh").start();`, languages.LangJava, true},
		{`public class Main { public static void main(String[] args) {} }`, languages.LangJava, false},

		// JavaScript
		{`const { exec } = require('child_process');`, languages.LangJavaScript, true},
		{`const net = require('net');`, languages.LangJavaScript, true},
		{`console.log("safe js");`, languages.LangJavaScript, false},

		// Go
		{`import "os/exec"`, languages.LangGo, true},
		{`package main; func main() {}`, languages.LangGo, false},
	}

	for _, c := range cases {
		res := security.AnalyzeCode(c.code, c.langID)
		if res.Rejected != c.reject {
			t.Errorf("for code [%s] lang [%d]: expected rejected=%v, got rejected=%v (reason: %s)",
				c.code, c.langID, c.reject, res.Rejected, res.Reason)
		}
	}
}

func TestSSRFValidation(t *testing.T) {
	blockedURLs := []string{
		"http://localhost:8080/callback",
		"http://127.0.0.1/hook",
		"http://10.0.0.5/hook",
		"http://192.168.1.1/hook",
		"http://172.16.0.1/hook",
		"http://169.254.169.254/latest/meta-data/",
		"ftp://example.com/file",
		"file:///etc/passwd",
	}

	for _, u := range blockedURLs {
		if err := security.ValidateCallbackURL(u); err == nil {
			t.Errorf("expected URL to be blocked by SSRF filter: %s", u)
		}
	}

	// Empty is allowed (no callback)
	if err := security.ValidateCallbackURL(""); err != nil {
		t.Errorf("empty callback should be valid, got: %v", err)
	}
}

func TestSanitizeOptions(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"-O2 -Wall", "-O2 -Wall"},
		{"-O2; rm -rf /", "-O2 rm -rf /"},
		{"-DVALUE=100", "-DVALUE=100"},
		{"`whoami`", "whoami"},
		{"$(cat /etc/passwd)", "cat /etc/passwd"},
		{"-I/usr/include/json.hpp", "-I/usr/include/json.hpp"},
	}

	for _, tt := range tests {
		got := security.SanitizeOptions(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeOptions(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
