package model

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	chart       = "data-space-connector"
	trustAnchor = "trust-anchor"
)

func bundleFile(chart, version string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("chart: " + chart + "\nversion: " + version + "\n")}
}

func TestVersions(t *testing.T) {
	tests := []struct {
		name    string
		fs      fstest.MapFS
		want    []Bundle
		wantErr string
	}{
		{name: "no bundles", fs: fstest.MapFS{}, want: []Bundle{}},
		{
			name: "single bundle is latest",
			fs:   fstest.MapFS{"10.16.0/bundle.yaml": bundleFile(chart, "10.16.0")},
			want: []Bundle{{Chart: chart, Version: "10.16.0", Latest: true}},
		},
		{
			name: "semver order, not lexical",
			fs: fstest.MapFS{
				"9.2.0/bundle.yaml":         bundleFile(chart, "9.2.0"),
				"10.16.0/bundle.yaml":       bundleFile(chart, "10.16.0"),
				"10.9.1/bundle.yaml":        bundleFile(chart, "10.9.1"),
				"10.16.0-rc.1/bundle.yaml":  bundleFile(chart, "10.16.0-rc.1"),
				"10.16.0/unrelated.yaml":    {Data: []byte("ignored")},
				"no-bundle-here/notes.yaml": {Data: []byte("ignored")},
			},
			want: []Bundle{
				{Chart: chart, Version: "10.16.0", Latest: true},
				{Chart: chart, Version: "10.16.0-rc.1"},
				{Chart: chart, Version: "10.9.1"},
				{Chart: chart, Version: "9.2.0"},
			},
		},
		{
			name: "a newer pre-release is not latest",
			fs: fstest.MapFS{
				"10.16.0/bundle.yaml":      bundleFile(chart, "10.16.0"),
				"10.17.0-rc.1/bundle.yaml": bundleFile(chart, "10.17.0-rc.1"),
			},
			want: []Bundle{
				{Chart: chart, Version: "10.17.0-rc.1"},
				{Chart: chart, Version: "10.16.0", Latest: true},
			},
		},
		{
			name: "only pre-releases: the highest is latest",
			fs: fstest.MapFS{
				"10.17.0-rc.1/bundle.yaml": bundleFile(chart, "10.17.0-rc.1"),
				"10.17.0-rc.2/bundle.yaml": bundleFile(chart, "10.17.0-rc.2"),
			},
			want: []Bundle{
				{Chart: chart, Version: "10.17.0-rc.2", Latest: true},
				{Chart: chart, Version: "10.17.0-rc.1"},
			},
		},
		{
			name: "latest per chart",
			fs: fstest.MapFS{
				"10.16.0/bundle.yaml":    bundleFile(chart, "10.16.0"),
				"10.15.0/bundle.yaml":    bundleFile(chart, "10.15.0"),
				"0.2.0/bundle.yaml":      bundleFile(trustAnchor, "0.2.0"),
				"0.3.0-rc.1/bundle.yaml": bundleFile(trustAnchor, "0.3.0-rc.1"),
			},
			want: []Bundle{
				{Chart: chart, Version: "10.16.0", Latest: true},
				{Chart: chart, Version: "10.15.0"},
				{Chart: trustAnchor, Version: "0.3.0-rc.1"},
				{Chart: trustAnchor, Version: "0.2.0", Latest: true},
			},
		},
		{name: "version is not semver", fs: fstest.MapFS{"latest/bundle.yaml": bundleFile(chart, "latest")}, wantErr: "not semver"},
		{name: "major.minor only", fs: fstest.MapFS{"10.16/bundle.yaml": bundleFile(chart, "10.16")}, wantErr: "not semver"},
		{name: "major only", fs: fstest.MapFS{"10/bundle.yaml": bundleFile(chart, "10")}, wantErr: "not semver"},
		{name: "v prefix", fs: fstest.MapFS{"v10.16.0/bundle.yaml": bundleFile(chart, "v10.16.0")}, wantErr: "not semver"},
		{name: "version differs from directory", fs: fstest.MapFS{"10.16.0/bundle.yaml": bundleFile(chart, "10.15.0")}, wantErr: "does not match"},
		{name: "chart missing", fs: fstest.MapFS{"10.16.0/bundle.yaml": bundleFile("", "10.16.0")}, wantErr: "chart is missing"},
		{name: "not YAML", fs: fstest.MapFS{"10.16.0/bundle.yaml": {Data: []byte("chart: [")}}, wantErr: "10.16.0/bundle.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Versions(tc.fs)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Versions = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCharts(t *testing.T) {
	dsc10, dsc9 := Bundle{Chart: chart, Version: "10.16.0", Latest: true}, Bundle{Chart: chart, Version: "9.2.0"}
	ta := Bundle{Chart: trustAnchor, Version: "0.2.0", Latest: true}
	tests := []struct {
		name    string
		bundles []Bundle
		want    []Chart
	}{
		{"none", nil, []Chart{}},
		{
			"grouped by chart in first-seen order",
			[]Bundle{dsc10, ta, dsc9},
			[]Chart{{Name: chart, Versions: []Bundle{dsc10, dsc9}}, {Name: trustAnchor, Versions: []Bundle{ta}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Charts(tc.bundles); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Charts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestEmbeddedBundles keeps the shipped bundles loadable, so a bad bundle fails here and not at run time.
func TestEmbeddedBundles(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || !got[0].Latest || got[0].Chart != chart {
		t.Errorf("embedded bundles = %+v, want a latest %s bundle first", got, chart)
	}
}
