package main

import "path/filepath"

// clusterContinuityPath resolves docs/cluster-continuity.json relative
// to renderDir, the same way cmd_renumber.go resolves
// docs/id-migration-map.csv — every renderer that computes clusters
// (shell, matrix, pivot, export-graph) shares this one file so cluster
// identity stays consistent across all of them in a single build.
func clusterContinuityPath(renderDir string) string {
	repoRoot, err := filepath.Abs(filepath.Join(renderDir, ".."))
	if err != nil {
		return ""
	}
	return filepath.Join(repoRoot, "docs", "cluster-continuity.json")
}
