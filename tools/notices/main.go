// Command notices writes THIRD_PARTY_NOTICES.txt: the licenses of the
// third-party code and fonts that ship inside Seaglass.exe and its
// installer. Run it from the repository root after `npm ci` in frontend:
//
//	go run ./tools/notices          rewrite THIRD_PARTY_NOTICES.txt
//	go run ./tools/notices -check   fail if the file is out of date (CI)
//
// Go modules come from `go list -deps` for the Windows release build, so
// only code that is linked in is listed, not test or tooling modules. npm
// packages are the interface's runtime dependencies plus every package a
// production Vite build bundles (read from its source maps), so build-only
// tooling such as Vite itself is left out.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const outFile = "THIRD_PARTY_NOTICES.txt"

// component is one entry in the notices file.
type component struct {
	Name    string
	Version string
	License string // SPDX identifier where there is one
	Where   string // which shipped file contains it
	Texts   []string
	Note    string
}

func main() {
	check := flag.Bool("check", false, "fail if "+outFile+" is out of date instead of rewriting it")
	flag.Parse()
	log.SetFlags(0)
	if _, err := os.Stat("go.mod"); err != nil {
		log.Fatal("run this from the repository root")
	}
	out, err := generate()
	if err != nil {
		log.Fatal(err)
	}
	if *check {
		cur, err := os.ReadFile(outFile)
		if err != nil || normalize(string(cur)) != out {
			log.Fatalf("%s is out of date: run `go run ./tools/notices` and commit the result", outFile)
		}
		return
	}
	if err := os.WriteFile(outFile, []byte(out), 0o644); err != nil {
		log.Fatal(err)
	}
}

func generate() (string, error) {
	gos, wailsLicense, err := goComponents()
	if err != nil {
		return "", err
	}
	npm, err := npmComponents("frontend", map[string]string{
		// The package ships without a license file; it is part of the Wails
		// project and under the same MIT license as the Go module above.
		"@wailsio/runtime": wailsLicense,
	})
	if err != nil {
		return "", err
	}
	extra, err := otherComponents()
	if err != nil {
		return "", err
	}
	all := append(append(gos, npm...), extra...)
	return render(all), nil
}

// goPackage is the part of `go list -json` output used here.
type goPackage struct {
	Dir      string
	Standard bool
	Module   *struct {
		Path    string
		Version string
		Dir     string
		Main    bool
		Replace *struct {
			Version string
			Dir     string
		}
	}
	Error *struct{ Err string }
}

// goComponents lists the Go standard library and every module linked into
// the release build, with the license files of the module and of any code
// vendored inside it. It also returns the Wails license text.
func goComponents() ([]component, string, error) {
	cmd := exec.Command("go", "list", "-e", "-deps", "-tags", "production", "-json", ".")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("go list: %w", err)
	}

	type mod struct{ path, version, dir string }
	mods := map[string]mod{}
	licenseDirs := map[string]map[string]bool{} // module path → folders holding license files
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		var p goPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, "", fmt.Errorf("go list output: %w", err)
		}
		// The interface is embedded from frontend/dist, which only exists
		// after a build; that doesn't change which packages are linked.
		if p.Error != nil && !strings.Contains(p.Error.Err, "frontend/dist") {
			return nil, "", fmt.Errorf("go list: %s", p.Error.Err)
		}
		if p.Standard || p.Module == nil || p.Module.Main {
			continue
		}
		m := mod{p.Module.Path, p.Module.Version, p.Module.Dir}
		if r := p.Module.Replace; r != nil {
			m.version, m.dir = r.Version, r.Dir
		}
		mods[m.path] = m
		if licenseDirs[m.path] == nil {
			licenseDirs[m.path] = map[string]bool{}
		}
		for d := p.Dir; ; d = filepath.Dir(d) {
			if files, _ := licenseFiles(d); len(files) > 0 {
				licenseDirs[m.path][d] = true
			}
			if d == m.dir || filepath.Dir(d) == d || !strings.HasPrefix(d, m.dir) {
				break
			}
		}
	}

	var out []component
	var wails string
	for path, m := range mods {
		dirs := make([]string, 0, len(licenseDirs[path]))
		for d := range licenseDirs[path] {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		if !licenseDirs[path][m.dir] {
			return nil, "", fmt.Errorf("%s: no license file in %s", path, m.dir)
		}
		for _, d := range dirs {
			c, err := componentFromDir(d)
			if err != nil {
				return nil, "", fmt.Errorf("%s: %w", path, err)
			}
			c.Name, c.Version, c.Where = path, m.version, "Seaglass.exe"
			if d != m.dir {
				rel, _ := filepath.Rel(m.dir, d)
				c.Name = path + "/" + filepath.ToSlash(rel)
				c.Note = "Vendored inside " + path + "."
			}
			if path == "github.com/wailsapp/wails/v3" && d == m.dir {
				wails = c.Texts[0]
			}
			out = append(out, c)
		}
	}
	if wails == "" {
		return nil, "", errors.New("the Wails module was not found in the build")
	}

	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return nil, "", fmt.Errorf("go env GOROOT: %w", err)
	}
	std, err := componentFromDir(strings.TrimSpace(string(root)))
	if err != nil {
		return nil, "", fmt.Errorf("Go standard library: %w", err)
	}
	std.Name, std.Where = "Go standard library and runtime", "Seaglass.exe"
	std.Note = "Version: the Go release the build used (see go.mod)."
	return append(out, std), wails, nil
}

// componentFromDir reads the license files in dir and works out the license.
func componentFromDir(dir string) (component, error) {
	files, err := licenseFiles(dir)
	if err != nil {
		return component{}, err
	}
	var c component
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return component{}, err
		}
		text := normalize(string(b))
		c.Texts = append(c.Texts, text)
		if isPatentsOrNotice(f) || c.License != "" {
			continue
		}
		if c.License = classify(text); c.License == "" {
			return component{}, fmt.Errorf("unrecognised license in %s: check it is compatible with the AGPL and add it to classify", filepath.Join(dir, f))
		}
	}
	if c.License == "" {
		return component{}, fmt.Errorf("no license file in %s", dir)
	}
	return c, nil
}

// licenseFiles lists the license, notice and patent files directly in dir,
// the main license first.
func licenseFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var main, rest []string
	for _, e := range entries {
		if e.IsDir() || !isLicenseFile(e.Name()) {
			continue
		}
		if isPatentsOrNotice(e.Name()) {
			rest = append(rest, e.Name())
		} else {
			main = append(main, e.Name())
		}
	}
	sort.Strings(main)
	sort.Strings(rest)
	return append(main, rest...), nil
}

func baseName(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
}

func isLicenseFile(name string) bool {
	switch baseName(name) {
	case "license", "licence", "copying", "notice", "patents", "unlicense":
		return true
	}
	return false
}

func isPatentsOrNotice(name string) bool {
	b := baseName(name)
	return b == "patents" || b == "notice"
}

// classify names a license from its text, or returns "" when it isn't one
// of the permissive licenses Seaglass is known to be able to ship.
func classify(text string) string {
	t := strings.Join(strings.Fields(strings.ToLower(text)), " ")
	switch {
	case strings.Contains(t, "sil open font license") && strings.Contains(t, "version 1.1"):
		return "OFL-1.1"
	case strings.Contains(t, "apache license") && strings.Contains(t, "version 2.0"):
		return "Apache-2.0"
	case strings.Contains(t, "gnu general public license"), strings.Contains(t, "gnu affero"), strings.Contains(t, "gnu lesser"),
		strings.Contains(t, "mozilla public license"):
		return "" // copyleft (MPL: per file): check by hand before shipping it
	case strings.Contains(t, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "permission to use, copy, modify, and/or distribute"),
		strings.Contains(t, "permission to use, copy, modify, and distribute"):
		return "ISC"
	case strings.Contains(t, "redistribution and use in source and binary forms"):
		if strings.Contains(t, "neither the name") || strings.Contains(t, "names of its contributors") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	case strings.Contains(t, "this software is provided 'as-is'") && strings.Contains(t, "altered source versions must be plainly marked"):
		return "Zlib"
	}
	return ""
}

// npmComponents lists the interface's runtime dependencies and every
// package bundled into its production build. fallback holds license texts
// for packages that ship without a license file.
func npmComponents(frontend string, fallback map[string]string) ([]component, error) {
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := readJSON(filepath.Join(frontend, "package.json"), &pkg); err != nil {
		return nil, err
	}
	dirs := map[string]bool{}
	for name := range pkg.Dependencies {
		dirs["node_modules/"+name] = true
	}
	bundled, err := bundledPackages(frontend)
	if err != nil {
		return nil, err
	}
	for _, d := range bundled {
		dirs[d] = true
	}

	var out []component
	for d := range dirs {
		dir := filepath.Join(frontend, filepath.FromSlash(d))
		var meta struct {
			Name    string          `json:"name"`
			Version string          `json:"version"`
			License json.RawMessage `json:"license"`
		}
		if err := readJSON(filepath.Join(dir, "package.json"), &meta); err != nil {
			return nil, fmt.Errorf("%w (run `npm ci` in frontend first)", err)
		}
		c := component{Name: meta.Name, Version: meta.Version, Where: "Seaglass.exe (interface)"}
		if strings.HasPrefix(meta.Name, "@fontsource/") {
			c.Where = "Seaglass.exe (interface fonts)"
		}
		files, err := licenseFiles(dir)
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			fc, err := componentFromDir(dir)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", meta.Name, err)
			}
			c.License, c.Texts = fc.License, fc.Texts
		} else if text, ok := fallback[meta.Name]; ok {
			c.License, c.Texts = classify(text), []string{text}
		} else {
			return nil, fmt.Errorf("%s ships no license file: add its license text to the fallback list", meta.Name)
		}
		if declared := spdx(meta.License); declared != "" && declared != c.License {
			return nil, fmt.Errorf("%s: package.json says %s but its license file reads as %s", meta.Name, declared, c.License)
		}
		out = append(out, c)
	}
	return out, nil
}

// spdx reads package.json's "license", a string or an old-style object.
func spdx(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct{ Type string }
	if json.Unmarshal(raw, &o) == nil {
		return o.Type
	}
	return ""
}

// bundledPackages builds the interface into a temporary folder with source
// maps and returns the package folders (relative to frontend) whose code
// ended up in the bundle.
func bundledPackages(frontend string) ([]string, error) {
	tmp, err := os.MkdirTemp("", "seaglass-notices-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	vite := filepath.Join("node_modules", "vite", "bin", "vite.js")
	cmd := exec.Command("node", vite, "build", "--mode", "production", "--sourcemap", "--outDir", tmp, "--emptyOutDir", "--logLevel", "warn")
	cmd.Dir = frontend
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("vite build (run `npm ci` in frontend first): %w", err)
	}
	var sources []string
	err = filepath.WalkDir(tmp, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".map") {
			return err
		}
		var m struct{ Sources []string }
		if err := readJSON(p, &m); err != nil {
			return err
		}
		sources = append(sources, m.Sources...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	pkgs := packagesInSources(sources)
	if len(pkgs) == 0 {
		return nil, errors.New("the build's source maps name no packages")
	}
	return pkgs, nil
}

var nodeModule = regexp.MustCompile(`node_modules/(?:@[^/]+/)?[^/]+`)

// packagesInSources turns source-map paths into the package folders they
// sit in, such as "node_modules/svelte" or, for a nested copy,
// "node_modules/a/node_modules/b".
func packagesInSources(sources []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range sources {
		s = filepath.ToSlash(s)
		locs := nodeModule.FindAllStringIndex(s, -1)
		if len(locs) == 0 {
			continue
		}
		dir := s[locs[0][0]:locs[len(locs)-1][1]]
		if !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}

// otherComponents are files that ship without coming from Go modules or npm.
func otherComponents() ([]component, error) {
	sdl, err := componentFromDir(filepath.Join("internal", "pad", "sdl3"))
	if err != nil {
		return nil, fmt.Errorf("SDL3: %w", err)
	}
	readme, err := os.ReadFile(filepath.Join("internal", "pad", "sdl3", "README.md"))
	if err != nil {
		return nil, err
	}
	if m := regexp.MustCompile(`\) (\d+\.\d+\.\d+)`).FindSubmatch(readme); m != nil {
		sdl.Version = string(m[1])
	}
	sdl.Name, sdl.Where = "SDL 3", "Seaglass.exe (embedded SDL3.dll)"
	return []component{
		sdl,
		{
			Name: "Microsoft Edge WebView2 Runtime bootstrapper", License: "Microsoft redistributable",
			Where: "Seaglass-setup.exe",
			Note: "MicrosoftEdgeWebview2Setup.exe installs Microsoft's WebView2 Runtime when Windows lacks it. " +
				"It is Microsoft's redistributable and covered by Microsoft's own license terms: " +
				"https://developer.microsoft.com/microsoft-edge/webview2/",
		},
		{
			Name: "NSIS (Nullsoft Scriptable Install System)", License: "Zlib",
			Where: "Seaglass-setup.exe (installer)",
			Note:  "The installer is built with NSIS, whose code in installers is under the zlib/libpng license with exceptions for its compressors: https://nsis.sourceforge.io/License",
		},
	}, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// normalize uses LF line ends, drops trailing spaces and blank lines at the
// ends, so the output is the same on every machine.
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n") + "\n"
}

func render(cs []component) string {
	sort.Slice(cs, func(i, j int) bool { return strings.ToLower(cs[i].Name) < strings.ToLower(cs[j].Name) })
	var b strings.Builder
	b.WriteString(`Third-party notices for Seaglass
================================

Seaglass is licensed under the GNU Affero General Public License v3.0
(LICENSE). It includes the third-party software and fonts below, each
under its own license.

This file is generated by "go run ./tools/notices"; don't edit it by hand.

`)
	nameW, verW, licW := len("Component"), len("Version"), len("License")
	for _, c := range cs {
		nameW, verW, licW = max(nameW, len(c.Name)), max(verW, len(c.Version)), max(licW, len(c.License))
	}
	row := func(a, v, l, w string) {
		fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %s", nameW, a, verW, v, licW, l, w)
		b.WriteString("\n")
	}
	row("Component", "Version", "License", "Included in")
	row(strings.Repeat("-", nameW), strings.Repeat("-", verW), strings.Repeat("-", licW), strings.Repeat("-", len("Included in")))
	for _, c := range cs {
		row(c.Name, c.Version, c.License, c.Where)
	}

	printed := map[string]string{} // license text → component it was printed under
	for _, c := range cs {
		b.WriteString("\n\n" + strings.Repeat("=", 72) + "\n")
		b.WriteString(strings.TrimSpace(c.Name + " " + c.Version))
		fmt.Fprintf(&b, "\nLicense: %s\nIncluded in: %s\n", c.License, c.Where)
		if c.Note != "" {
			b.WriteString(c.Note + "\n")
		}
		for _, t := range c.Texts {
			if first, ok := printed[t]; ok {
				fmt.Fprintf(&b, "\nSame license text as %s above.\n", first)
				continue
			}
			printed[t] = c.Name
			b.WriteString("\n" + t)
		}
	}
	return normalize(b.String())
}
