package main

import "testing"

func TestAssetMatchesSelector(t *testing.T) {
	tests := []struct {
		id, token string
		want      bool
	}{
		{"jxl_unsplash", "jxl", true},
		{"jxl_unsplash", "jxl_unsplash", true},
		{"heic_jpeg_thumb", "heic", true},
		{"mp4_h264", "mp4", true},
		{"mov_h264", "mp4", false},
		{"jxl_unsplash", "jx", false},
	}
	for _, tc := range tests {
		if got := assetMatchesSelector(tc.id, tc.token); got != tc.want {
			t.Errorf("assetMatchesSelector(%q, %q) = %v, want %v", tc.id, tc.token, got, tc.want)
		}
	}
}

func TestFilterAssets(t *testing.T) {
	all := []Asset{
		{ID: "jxl_unsplash"},
		{ID: "mp4_h264"},
	}
	out, err := filterAssets(all, "jxl")
	if err != nil || len(out) != 1 || out[0].ID != "jxl_unsplash" {
		t.Fatalf("filter jxl: out=%v err=%v", out, err)
	}
	out, err = filterAssets(all, "")
	if err != nil || len(out) != 2 {
		t.Fatalf("filter empty: out=%v err=%v", out, err)
	}
	_, err = filterAssets(all, "nope")
	if err == nil {
		t.Fatal("expected error for no match")
	}
}
