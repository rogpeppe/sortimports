package main

import (
	"testing"
)

var prefixTests = []struct {
	pkg    string
	expect string
}{{
	pkg:    "gopkg.in/juju/foo.v1",
	expect: "gopkg.in/juju/foo.v1",
}, {
	pkg:    "gopkg.in/juju/foo.v1/arble/bletch",
	expect: "gopkg.in/juju/foo.v1",
}, {
	pkg:    "gopkg.in/juju/foo.v1a/arble/bletch",
	expect: "",
}, {
	pkg:    "gopkg.in/juju.v1/arble/bletch",
	expect: "gopkg.in/juju.v1",
}, {
	pkg:    "gopkg.in/juju.v1a/arble.v1/bletch",
	expect: "gopkg.in/juju.v1a/arble.v1",
}, {
	pkg:    "gopkg.in/juju.v1a/arble.v1/bletch",
	expect: "gopkg.in/juju.v1a/arble.v1",
}, {
	pkg:    "github.com/rogpeppe/sortimports",
	expect: "github.com/rogpeppe/sortimports",
}, {
	pkg:    "github.com/rogpeppe/sortimports/foo",
	expect: "github.com/rogpeppe/sortimports",
}, {
	pkg:    "launchpad.net/foo",
	expect: "launchpad.net/foo",
}, {
	pkg:    "launchpad.net/foo/bar",
	expect: "launchpad.net/foo",
}, {
	pkg:    "code.google.com/p/arble",
	expect: "code.google.com/p/arble",
}, {
	pkg:    "code.google.com/p/arble/bletch",
	expect: "code.google.com/p/arble",
}}

func TestPrefix(t *testing.T) {
	for _, test := range prefixTests {
		got := localPackagePrefix(test.pkg)
		if got != test.expect {
			t.Errorf("package %s got %s want %s", test.pkg, got, test.expect)
		}
	}
}
