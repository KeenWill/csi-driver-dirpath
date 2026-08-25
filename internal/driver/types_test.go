package driver

import "testing"

func TestParseVolumeID(t *testing.T) {
	for _, test := range []struct {
		value string
		valid bool
	}{
		{value: "vol-123.alpha", valid: true},
		{value: ""},
		{value: "../volume"},
		{value: "volume/child"},
		{value: ".hidden"},
	} {
		t.Run(test.value, func(t *testing.T) {
			id, err := parseVolumeID(test.value)
			if test.valid && (err != nil || string(id) != test.value) {
				t.Fatalf("parseVolumeID(%q) = %q, %v", test.value, id, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("parseVolumeID(%q) succeeded", test.value)
			}
		})
	}
}

func TestParseBasePath(t *testing.T) {
	path, err := ParseBasePath("/srv/dirpath/../storage")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/srv/storage" {
		t.Fatalf("ParseBasePath cleaned path = %q, want /srv/storage", path)
	}
	for _, value := range []string{"", "relative/path"} {
		if _, err := ParseBasePath(value); err == nil {
			t.Fatalf("ParseBasePath(%q) succeeded", value)
		}
	}
}

func TestParseDriverModes(t *testing.T) {
	for _, value := range []string{"marker", "fsid", "device"} {
		if _, err := ParseFenceMode(value); err != nil {
			t.Fatalf("ParseFenceMode(%q): %v", value, err)
		}
	}
	if _, err := ParseFenceMode("strict"); err == nil {
		t.Fatal("ParseFenceMode accepted an unknown mode")
	}
	for _, value := range []string{"real", "noop"} {
		if _, err := ParseMountMode(value); err != nil {
			t.Fatalf("ParseMountMode(%q): %v", value, err)
		}
	}
	if _, err := ParseMountMode("fake"); err == nil {
		t.Fatal("ParseMountMode accepted an unknown mode")
	}
}

func TestParseRequiredIdentifiers(t *testing.T) {
	if _, err := ParseNodeID(""); err == nil {
		t.Fatal("ParseNodeID accepted an empty value")
	}
	if _, err := ParseFenceToken(""); err == nil {
		t.Fatal("ParseFenceToken accepted an empty value")
	}
	if node, err := ParseNodeID("node-a"); err != nil || node != "node-a" {
		t.Fatalf("ParseNodeID = %q, %v", node, err)
	}
	if token, err := ParseFenceToken("token"); err != nil || token != "token" {
		t.Fatalf("ParseFenceToken = %q, %v", token, err)
	}
}
