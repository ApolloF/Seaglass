package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestClassifyRecognisesShippedLicenses(t *testing.T) {
	cases := map[string]string{
		"MIT License\n\nPermission is hereby granted, free of charge, to any person":           "MIT",
		"Permission to use, copy, modify, and/or distribute this software for any":             "ISC",
		"Redistribution and use in source and binary forms ... Neither the name of":            "BSD-3-Clause",
		"Redistribution and use in source and binary forms, with or without":                   "BSD-2-Clause",
		"This software is provided 'as-is' ... Altered source versions must be plainly marked": "Zlib",
		"SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007":                                 "OFL-1.1",
		"Apache License\n  Version 2.0, January 2004":                                          "Apache-2.0",
	}
	for text, want := range cases {
		if got := classify(text); got != want {
			t.Errorf("classify(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestClassifyRejectsCopyleftAndUnknownText(t *testing.T) {
	for _, text := range []string{
		"GNU GENERAL PUBLIC LICENSE Version 3 ... Permission is hereby granted, free of charge",
		"GNU LESSER GENERAL PUBLIC LICENSE",
		"All rights reserved.",
	} {
		if got := classify(text); got != "" {
			t.Errorf("classify(%q) = %q, want it rejected", text, got)
		}
	}
}

func TestPackagesInSourcesFindsTopLevelScopedAndNestedPackages(t *testing.T) {
	got := packagesInSources([]string{
		"../../node_modules/svelte/src/internal/client/index.js",
		"../../node_modules/svelte/src/index-client.js",
		`..\..\node_modules\@wailsio\runtime\dist\index.js`,
		"../../node_modules/a/node_modules/@b/c/index.js",
		"../src/App.svelte",
	})
	want := []string{"node_modules/@wailsio/runtime", "node_modules/a/node_modules/@b/c", "node_modules/svelte"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeIsStableAcrossLineEndings(t *testing.T) {
	if got := normalize("\r\na  \r\nb\t\r\n\r\n"); got != "a\nb\n" {
		t.Errorf("got %q", got)
	}
}

func TestRenderPrintsEachLicenseTextOnce(t *testing.T) {
	out := render([]component{
		{Name: "b", Version: "v1", License: "MIT", Where: "Seaglass.exe", Texts: []string{"MIT text\n"}},
		{Name: "a", Version: "v2", License: "MIT", Where: "Seaglass.exe", Texts: []string{"MIT text\n"}},
	})
	if n := strings.Count(out, "MIT text"); n != 1 {
		t.Errorf("license text printed %d times, want once", n)
	}
	if !strings.Contains(out, "Same license text as a above.") {
		t.Error("the second component doesn't point at the first")
	}
	if strings.Index(out, "\na v2\n") > strings.Index(out, "\nb v1\n") {
		t.Error("components aren't sorted by name")
	}
}
