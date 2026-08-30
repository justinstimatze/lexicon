package viz

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"os"
	"sort"
)

// clusterIDAlphabet mirrors cmd_renumber.go's idAlphabet for atom ids —
// same hand-typing-safe set (excludes 0/1/i/l/o), same reasoning: an
// opaque code shouldn't carry accidental meaning. clusterIDLen is
// shorter than atoms' 5 chars since the cluster keyspace needed is
// far smaller (low hundreds, not 1000+): 32^4 ≈ 1M is ample headroom.
const (
	clusterIDAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
	clusterIDLen       = 4
)

// drawClusterCode returns a "c"-prefixed opaque code not already in
// used, via crypto/rand — collision-checked the same way
// cmd_renumber.go's drawCode is, just with its own small copy of the
// alphabet-draw logic since internal/viz can't import from cmd/lexicon.
func drawClusterCode(used map[string]bool) string {
	max := big.NewInt(int64(len(clusterIDAlphabet)))
	for {
		b := make([]byte, clusterIDLen)
		for i := range b {
			idx, err := rand.Int(rand.Reader, max)
			if err != nil {
				// crypto/rand failing is a host-level problem no caller
				// here can recover from meaningfully; matches
				// cmd_renumber.go's own fatal-on-rand-failure posture.
				panic("cluster id: crypto/rand: " + err.Error())
			}
			b[i] = clusterIDAlphabet[idx.Int64()]
		}
		code := "c" + string(b)
		if !used[code] {
			return code
		}
	}
}

// clusterContinuity is the on-disk shape for docs/cluster-continuity.json.
// Auto-generated and rewritten on every ComputeClusters call that's
// given a path — never hand-edited, but git-tracked so identity
// persists across machines and the CI Pages build, the same way
// docs/id-migration-map.csv persists the lex-id renumbering map.
//
// The clustering itself is always fully recomputed from current
// elements structure; this file only preserves which cluster id a
// given community keeps across runs, so a cluster's color and label
// don't reshuffle for a reader between visits unless the underlying
// atoms actually moved.
type clusterContinuity struct {
	Members map[string][]string `json:"members"` // clusterID -> sorted member atom ids, as of the last run
}

func loadClusterContinuity(path string) clusterContinuity {
	empty := clusterContinuity{Members: map[string][]string{}}
	if path == "" {
		return empty
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return empty
	}
	var c clusterContinuity
	if err := json.Unmarshal(data, &c); err != nil {
		return empty
	}
	if c.Members == nil {
		c.Members = map[string][]string{}
	}
	return c
}

func saveClusterContinuity(path string, c clusterContinuity) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// jaccard is |A∩B| / |A∪B| over two SORTED string slices.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	i, j, inter := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - inter
	return float64(inter) / float64(union)
}

// minClusterOverlap is the Jaccard floor for treating a newly computed
// community as "the same cluster" a previous run's id referred to.
// Generous enough to survive a handful of atoms moving in or out;
// below it a cluster has fragmented or merged enough that reusing the
// old id would misrepresent it, so it's treated as new instead.
const minClusterOverlap = 0.3

// matchStableIDs assigns each freshly computed community (communities[i]
// is that community's sorted member-atom-ids) either the id of its
// best-overlapping community from the previous run, or a fresh unused
// id if no prior community overlaps it enough. Matches greedily by
// descending overlap score so the strongest correspondences claim
// their id first — relevant on a split or merge, where a community
// can overlap more than one prior id. Returns one ClusterID per input
// community (same order/length) and the continuity state to persist
// for next time.
func matchStableIDs(communities [][]string, prev clusterContinuity) ([]ClusterID, clusterContinuity) {
	type candidate struct {
		commIdx int
		prevID  string
		score   float64
	}
	prevIDs := make([]string, 0, len(prev.Members))
	for id := range prev.Members {
		prevIDs = append(prevIDs, id)
	}
	sort.Strings(prevIDs)

	var cands []candidate
	for ci, members := range communities {
		for _, pid := range prevIDs {
			if score := jaccard(members, prev.Members[pid]); score >= minClusterOverlap {
				cands = append(cands, candidate{ci, pid, score})
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		if cands[i].commIdx != cands[j].commIdx {
			return cands[i].commIdx < cands[j].commIdx
		}
		return cands[i].prevID < cands[j].prevID
	})

	assigned := make([]ClusterID, len(communities))
	usedComm := make(map[int]bool, len(communities))
	usedPrev := make(map[string]bool, len(prevIDs))
	for _, c := range cands {
		if usedComm[c.commIdx] || usedPrev[c.prevID] {
			continue
		}
		assigned[c.commIdx] = ClusterID(c.prevID)
		usedComm[c.commIdx] = true
		usedPrev[c.prevID] = true
	}

	// used tracks every id already spoken for this run — reused matches
	// plus, as they're drawn, fresh codes — so a fresh draw can't
	// collide with either.
	used := make(map[string]bool, len(prevIDs)+len(communities))
	for _, pid := range prevIDs {
		used[pid] = true
	}
	newMembers := make(map[string][]string, len(communities))
	for ci, members := range communities {
		if assigned[ci] == "" {
			code := drawClusterCode(used)
			used[code] = true
			assigned[ci] = ClusterID(code)
		}
		newMembers[string(assigned[ci])] = members
	}
	return assigned, clusterContinuity{Members: newMembers}
}
