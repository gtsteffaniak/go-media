package main

import (
	"fmt"
	"strings"
)

// parseAssetSelectors splits a comma-separated filter (e.g. "heic" or "heic_jpeg_thumb,mp4_h264").
func parseAssetSelectors(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// assetMatchesSelector reports whether id matches one selector token.
// Exact id match, or id is token + "_" + suffix (e.g. "heic" → "heic_jpeg_thumb").
func assetMatchesSelector(id, token string) bool {
	if id == token {
		return true
	}
	return strings.HasPrefix(id, token+"_")
}

func assetMatchesAnySelector(id string, tokens []string) bool {
	for _, t := range tokens {
		if assetMatchesSelector(id, t) {
			return true
		}
	}
	return false
}

func filterAssets(assets []Asset, selectorsRaw string) ([]Asset, error) {
	tokens := parseAssetSelectors(selectorsRaw)
	if len(tokens) == 0 {
		return assets, nil
	}
	var out []Asset
	for _, a := range assets {
		if assetMatchesAnySelector(a.ID, tokens) {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("assets filter %q matched no loaded assets (selectors: prefix or exact id)", selectorsRaw)
	}
	return out, nil
}

func allowedAssetIDSet(assets []Asset) map[string]bool {
	m := make(map[string]bool, len(assets))
	for _, a := range assets {
		m[a.ID] = true
	}
	return m
}
