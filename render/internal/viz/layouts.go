package viz

import (
	"math"
	"sort"
)

// Position is a single (x, y, z) coordinate. Emitted as a 3-element
// array in JSON for compactness.
type Position [3]float64

// ComputeLayouts produces precomputed coordinates for every node under
// every named lens. Run at build time so the browser does no layout
// work — it just reads positions.
//
// Returns: layouts[lensName][nodeID] -> {x, y, z}
func ComputeLayouts(nodes []Node, edges []Edge) map[string]map[string]Position {
	out := map[string]map[string]Position{}

	// Stable sort node IDs once
	nodeByID := map[string]*Node{}
	for i := range nodes {
		nodeByID[nodes[i].ID] = &nodes[i]
	}
	clusterIDs := uniqueClusters(nodes)
	sortedClusters := sortClustersBySize(clusterIDs, nodes)
	membersByCluster := groupByCluster(nodes)

	out["cosmic_web"] = layoutCosmicWeb(nodes, edges, sortedClusters, membersByCluster)
	out["cluster_puffs"] = layoutClusterPuffs(nodes, sortedClusters, membersByCluster)
	out["degree_shells"] = layoutDegreeShells(nodes)
	out["type_grid"] = layoutTypeGrid(nodes)
	out["flurry"] = layoutFlurry(nodes, edges, sortedClusters, membersByCluster)
	return out
}

func uniqueClusters(nodes []Node) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, n := range nodes {
		if n.Cluster != "" && !seen[n.Cluster] {
			seen[n.Cluster] = true
			out = append(out, n.Cluster)
		}
	}
	sort.Strings(out)
	return out
}

func sortClustersBySize(clusters []string, nodes []Node) []string {
	sizes := map[string]int{}
	for _, n := range nodes {
		sizes[n.Cluster]++
	}
	cp := append([]string(nil), clusters...)
	sort.SliceStable(cp, func(i, j int) bool { return sizes[cp[i]] > sizes[cp[j]] })
	return cp
}

func groupByCluster(nodes []Node) map[string][]*Node {
	out := map[string][]*Node{}
	for i := range nodes {
		c := nodes[i].Cluster
		if c == "" {
			c = "unclustered"
		}
		out[c] = append(out[c], &nodes[i])
	}
	for c := range out {
		members := out[c]
		sort.SliceStable(members, func(i, j int) bool {
			return members[i].InDegree > members[j].InDegree
		})
	}
	return out
}

func goldenAngle() float64 {
	return math.Pi * (3 - math.Sqrt(5))
}

// stableJitter is deterministic across runs given the same seed.
func stableJitter(seed int, scale float64) float64 {
	s := math.Sin(float64(seed)*12.9898) * 43758.5453
	return (s - math.Floor(s) - 0.5) * scale
}

// computeClusterCentroids force-places cluster centroids on a sphere of
// radius sphereR, pulled together by inter-cluster edge weight (log of
// cross-cluster edge count) and pushed apart by inverse-square repulsion
// — shared by cosmic_web and flurry, which both need "clusters arranged
// by how strongly they actually connect to each other" as their macro
// layer and differ only in how members are placed within a cluster.
func computeClusterCentroids(nodes []Node, edges []Edge, sortedClusters []string, sphereR float64) map[string]Position {
	G := goldenAngle()

	// Build cluster-pair edge counts
	clusterEdgeCount := map[string]int{}
	cKey := func(a, b string) string {
		if a < b {
			return a + "::" + b
		}
		return b + "::" + a
	}
	nodeByID := map[string]Node{}
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}
	for _, e := range edges {
		s, sOK := nodeByID[e.Source]
		t, tOK := nodeByID[e.Target]
		if !sOK || !tOK || s.Cluster == t.Cluster {
			continue
		}
		clusterEdgeCount[cKey(s.Cluster, t.Cluster)]++
	}

	// Init centroids on Fibonacci sphere
	type centroid struct {
		X, Y, Z, VX, VY, VZ float64
	}
	cents := map[string]*centroid{}
	for i, cid := range sortedClusters {
		t := 0.0
		if len(sortedClusters) > 1 {
			t = float64(i) / float64(len(sortedClusters)-1)
		}
		y := 1 - t*2
		rr := math.Sqrt(1 - y*y)
		theta := G * float64(i)
		R := sphereR
		cents[cid] = &centroid{
			X: R * math.Cos(theta) * rr,
			Y: R * y,
			Z: R * math.Sin(theta) * rr,
		}
	}

	// Force simulation on cluster graph
	const iters = 200
	const repulsion = 28000.0
	const spring = 0.012
	const centering = 0.004
	const damping = 0.86
	for it := 0; it < iters; it++ {
		// Repulsion
		for i, ci := range sortedClusters {
			ca := cents[ci]
			for j := i + 1; j < len(sortedClusters); j++ {
				cj := sortedClusters[j]
				cb := cents[cj]
				dx := cb.X - ca.X
				dy := cb.Y - ca.Y
				dz := cb.Z - ca.Z
				d2 := dx*dx + dy*dy + dz*dz + 1
				d := math.Sqrt(d2)
				f := repulsion / d2
				fx := dx / d * f
				fy := dy / d * f
				fz := dz / d * f
				ca.VX -= fx
				ca.VY -= fy
				ca.VZ -= fz
				cb.VX += fx
				cb.VY += fy
				cb.VZ += fz
			}
		}
		// Springs
		for key, count := range clusterEdgeCount {
			var a, b string
			for i := 0; i < len(key); i++ {
				if key[i] == ':' && i+1 < len(key) && key[i+1] == ':' {
					a = key[:i]
					b = key[i+2:]
					break
				}
			}
			ca, okA := cents[a]
			cb, okB := cents[b]
			if !okA || !okB {
				continue
			}
			dx := cb.X - ca.X
			dy := cb.Y - ca.Y
			dz := cb.Z - ca.Z
			w := spring * math.Log(1+float64(count))
			ca.VX += dx * w
			ca.VY += dy * w
			ca.VZ += dz * w
			cb.VX -= dx * w
			cb.VY -= dy * w
			cb.VZ -= dz * w
		}
		// Centering + integrate
		for _, c := range cents {
			c.VX = (c.VX - c.X*centering) * damping
			c.VY = (c.VY - c.Y*centering) * damping
			c.VZ = (c.VZ - c.Z*centering) * damping
			c.X += c.VX
			c.Y += c.VY
			c.Z += c.VZ
		}
	}

	out := map[string]Position{}
	for cid, c := range cents {
		out[cid] = Position{c.X, c.Y, c.Z}
	}
	return out
}

// LENS: COSMIC WEB
func layoutCosmicWeb(nodes []Node, edges []Edge, sortedClusters []string, membersByCluster map[string][]*Node) map[string]Position {
	out := map[string]Position{}
	cents := computeClusterCentroids(nodes, edges, sortedClusters, 280.0)

	// Place members organically around centroids
	for cid, members := range membersByCluster {
		c := cents[cid] // zero Position{0,0,0} if absent, same as before
		maxDeg := 1
		for _, m := range members {
			if m.InDegree > maxDeg {
				maxDeg = m.InDegree
			}
		}
		cloudR := 16 + 4*math.Sqrt(float64(len(members)))
		for i, m := range members {
			seed := i*31 + int(cid[0])*100
			if len(cid) > 1 {
				seed += int(cid[1])
			}
			degNorm := float64(m.InDegree) / float64(maxDeg)
			r := cloudR * (0.10 + 0.90*math.Pow(1-degNorm, 1.4))
			theta := stableJitter(seed+1, 1) * math.Pi * 2
			u := 0.5 + stableJitter(seed+2, 0.5)
			phi := math.Acos(2*u - 1)
			out[m.ID] = Position{
				c[0] + r*math.Sin(phi)*math.Cos(theta),
				c[1] + r*math.Sin(phi)*math.Sin(theta),
				c[2] + r*math.Cos(phi),
			}
		}
	}
	return out
}

// LENS: CLUSTER PUFFS — each cluster a discrete blob on a sphere.
// Macro radius pushed out so puffs don't merge into a soccer-ball;
// puff radii shrunk to make individual clusters visually discrete.
func layoutClusterPuffs(nodes []Node, sortedClusters []string, membersByCluster map[string][]*Node) map[string]Position {
	out := map[string]Position{}
	G := goldenAngle()
	const macroR = 420.0
	for i, cid := range sortedClusters {
		members, ok := membersByCluster[cid]
		if !ok {
			continue
		}
		N := math.Max(1, float64(len(sortedClusters)-1))
		y := 1 - float64(i)/N*2
		r := math.Sqrt(1 - y*y)
		theta := G * float64(i)
		cx := macroR * math.Cos(theta) * r
		cy := macroR * y
		cz := macroR * math.Sin(theta) * r
		microR := 12 + 3.2*math.Sqrt(float64(len(members)))
		for j, m := range members {
			t := (float64(j) + 0.5) / float64(len(members))
			py := 1 - t*2
			pr := math.Sqrt(1 - py*py)
			ptheta := G * float64(j)
			localR := microR * (0.35 + 0.65*math.Cbrt(t))
			out[m.ID] = Position{
				cx + localR*math.Cos(ptheta)*pr,
				cy + localR*py,
				cz + localR*math.Sin(ptheta)*pr,
			}
		}
	}
	return out
}

// LENS: DEGREE SHELLS — concentric shells; high-degree atoms inside
func layoutDegreeShells(nodes []Node) map[string]Position {
	out := map[string]Position{}
	G := goldenAngle()
	allSorted := make([]Node, len(nodes))
	copy(allSorted, nodes)
	sort.SliceStable(allSorted, func(i, j int) bool { return allSorted[i].InDegree > allSorted[j].InDegree })
	shells := 5
	perShell := (len(allSorted) + shells - 1) / shells
	shellRadii := []float64{70, 140, 210, 280, 350}
	for s := 0; s < shells; s++ {
		start := s * perShell
		end := start + perShell
		if end > len(allSorted) {
			end = len(allSorted)
		}
		slice := allSorted[start:end]
		R := shellRadii[s]
		for i, m := range slice {
			t := (float64(i) + 0.5) / float64(len(slice))
			y := 1 - t*2
			r := math.Sqrt(1 - y*y)
			theta := G * float64(i)
			out[m.ID] = Position{
				R * math.Cos(theta) * r,
				R * y,
				R * math.Sin(theta) * r,
			}
		}
	}
	return out
}

// LENS: TYPE GRID — literal 3D pivot table
func layoutTypeGrid(nodes []Node) map[string]Position {
	out := map[string]Position{}
	G := goldenAngle()
	typeInsSet := map[string]bool{}
	typeOutsSet := map[string]bool{}
	tiersSet := map[string]bool{}
	for _, n := range nodes {
		if n.TypeIn != "" {
			typeInsSet[n.TypeIn] = true
		}
		if n.TypeOut != "" {
			typeOutsSet[n.TypeOut] = true
		}
		if n.Tier != "" {
			tiersSet[n.Tier] = true
		}
	}
	typeIns := sortedSetKeys(typeInsSet)
	typeOuts := sortedSetKeys(typeOutsSet)
	tiers := sortedSetKeys(tiersSet)
	tinIdx := indexMap(typeIns)
	toutIdx := indexMap(typeOuts)
	tierIdx := indexMap(tiers)
	const spacing = 70.0

	cellMembers := map[[3]int][]Node{}
	for _, n := range nodes {
		k := [3]int{tinIdx[n.TypeIn], toutIdx[n.TypeOut], tierIdx[n.Tier]}
		cellMembers[k] = append(cellMembers[k], n)
	}
	for k, members := range cellMembers {
		cx := (float64(k[0]) - float64(len(typeIns)-1)/2) * spacing
		cy := (float64(k[1]) - float64(len(typeOuts)-1)/2) * spacing
		cz := (float64(k[2]) - float64(len(tiers)-1)/2) * spacing * 2
		sort.SliceStable(members, func(i, j int) bool { return members[i].InDegree > members[j].InDegree })
		r := 4 + 1.5*math.Sqrt(float64(len(members)))
		for i, m := range members {
			t := (float64(i) + 0.5) / float64(len(members))
			py := 1 - t*2
			pr := math.Sqrt(1 - py*py)
			ptheta := G * float64(i)
			ux, uy, uz := math.Cos(ptheta)*pr, py, math.Sin(ptheta)*pr
			// Cube-map the same evenly-distributed sphere point (standard
			// cubemap projection: divide by the largest component) so it
			// lands on a cube's surface instead of a sphere's — a hollow
			// cube of members, no interior points, matching type_grid's
			// own rectilinear cell layout instead of a ball of spheres.
			mx := math.Max(math.Abs(ux), math.Max(math.Abs(uy), math.Abs(uz)))
			if mx < 1e-9 {
				mx = 1
			}
			out[m.ID] = Position{
				cx + r*ux/mx,
				cy + r*uy/mx,
				cz + r*uz/mx,
			}
		}
	}
	return out
}

func sortedSetKeys(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func indexMap(s []string) map[string]int {
	out := map[string]int{}
	for i, k := range s {
		out[k] = i
	}
	return out
}

// LENS: FLURRY — a small number of curved arms per cluster, each a
// string of beads (atoms) spaced evenly along a bent arc — kelp fronds
// off a holdfast, not straight jittered rays — macro-arranged by the
// same real-connectivity force sim as cosmic_web so the result reads as
// several separate "flurries" connected by genuine cross-cluster edges
// rather than one undifferentiated cloud.
//
// Each arm follows a real connected sub-structure of the cluster (a
// depth-1 neighbor of the highest-in-degree hub, and everything reached
// from it over real intra-cluster edges) so which atoms end up on the
// same arm is still data-driven, not arbitrary. The geometry itself is
// a quadratic Bezier from the hub out to the arm's tip, bent sideways
// by a per-arm seeded offset so different arms curve differently (the
// "vine" bend) — beads are placed at even t-intervals along that curve
// in the order they were reached, not by raw graph depth, so a bushy
// sub-branch and a long chain both read as one continuous strand
// instead of the strand's length depending on the cluster's own
// internal topology. Bounded by construction: a Bezier curve can't run
// away the way the earlier depth*segLen version did for c00 (half this
// corpus in one cluster, see below).
func layoutFlurry(nodes []Node, edges []Edge, sortedClusters []string, membersByCluster map[string][]*Node) map[string]Position {
	out := map[string]Position{}
	cents := computeClusterCentroids(nodes, edges, sortedClusters, 380.0)
	G := goldenAngle()

	clusterOf := map[string]string{}
	for _, n := range nodes {
		clusterOf[n.ID] = n.Cluster
	}
	// Intra-cluster adjacency only — the real edges an arm grows along.
	intraAdj := map[string]map[string][]string{}
	for _, e := range edges {
		cs, ct := clusterOf[e.Source], clusterOf[e.Target]
		if cs == "" || cs != ct {
			continue
		}
		if intraAdj[cs] == nil {
			intraAdj[cs] = map[string][]string{}
		}
		intraAdj[cs][e.Source] = append(intraAdj[cs][e.Source], e.Target)
		intraAdj[cs][e.Target] = append(intraAdj[cs][e.Target], e.Source)
	}

	type pt struct{ x, y, z float64 }

	for _, cid := range sortedClusters {
		members := membersByCluster[cid]
		if len(members) == 0 {
			continue
		}
		c := cents[cid]
		root := members[0] // highest in-degree — groupByCluster already sorted it there
		adj := intraAdj[cid]

		armCount := 2 + len(members)/6
		if armCount > 7 {
			armCount = 7
		}
		if armCount > len(members) {
			armCount = len(members)
		}
		if armCount < 1 {
			armCount = 1
		}
		armDir := make([]pt, armCount)
		for k := 0; k < armCount; k++ {
			t := 0.0
			if armCount > 1 {
				t = float64(k) / float64(armCount-1)
			}
			y := 1 - t*2
			rr := math.Sqrt(math.Max(0, 1-y*y))
			theta := G * float64(k)
			armDir[k] = pt{math.Cos(theta) * rr, y, math.Sin(theta) * rr}
		}

		// BFS from the hub over real intra-cluster edges: each of the
		// hub's direct neighbors starts a new arm (round-robin across
		// armCount slots), and everything reached from it inherits that
		// arm — a connected chunk of the cluster stays on one arm
		// instead of scattering across the starfish. `order` is the
		// bead's position along its arm's string (0-based, in the order
		// reached), not raw BFS depth — a bushy sub-branch and a long
		// thin chain both end up as one continuous strand instead of the
		// strand's apparent length depending on the cluster's topology.
		type placed struct{ arm, order, isRoot int }
		info := map[string]placed{root.ID: {arm: -1, isRoot: 1}}
		visited := map[string]bool{root.ID: true}
		queue := []string{root.ID}
		armSize := make([]int, armCount)
		nextArm := 0
		for len(queue) > 0 {
			curID := queue[0]
			queue = queue[1:]
			cur := info[curID]
			for _, nb := range adj[curID] {
				if visited[nb] {
					continue
				}
				visited[nb] = true
				arm := cur.arm
				if arm < 0 { // curID is the hub itself
					arm = nextArm % armCount
					nextArm++
				}
				info[nb] = placed{arm: arm, order: armSize[arm]}
				armSize[arm]++
				queue = append(queue, nb)
			}
		}

		// Members with no path along real intra-cluster edges (isolated
		// within their own community — common in small clusters) still
		// need an arm: whichever is currently thinnest, so the starfish
		// stays roughly symmetric instead of every isolated atom piling
		// onto arm 0.
		for _, m := range members {
			if visited[m.ID] {
				continue
			}
			thin := 0
			for k := 1; k < armCount; k++ {
				if armSize[k] < armSize[thin] {
					thin = k
				}
			}
			info[m.ID] = placed{arm: thin, order: armSize[thin]}
			armSize[thin]++
			visited[m.ID] = true
		}

		// Per-arm curve: a quadratic Bezier from the hub out to the
		// arm's tip, bent sideways by a seeded per-arm offset — the
		// "vine" bend, different for every arm so the whole cluster
		// doesn't read as a symmetric sea urchin. Length is capped
		// regardless of how many beads land on the arm (c00 alone holds
		// half this corpus as of Book XI — a fixed-length curve with
		// more beads on it just reads as a thicker rope, which is the
		// right outcome, instead of the string running away to
		// thousands of units like a linear depth*step formula would).
		armLen := make([]float64, armCount)
		bend := make([]pt, armCount)
		for k := 0; k < armCount; k++ {
			armLen[k] = math.Min(50+3.5*math.Sqrt(float64(armSize[k])), 140)
			dir := armDir[k]
			ref := pt{0, 1, 0}
			if math.Abs(dir.y) > 0.9 {
				ref = pt{1, 0, 0}
			}
			ux, uy, uz := dir.y*ref.z-dir.z*ref.y, dir.z*ref.x-dir.x*ref.z, dir.x*ref.y-dir.y*ref.x
			ul := math.Sqrt(ux*ux + uy*uy + uz*uz)
			u := pt{ux / ul, uy / ul, uz / ul}
			v := pt{dir.y*u.z - dir.z*u.y, dir.z*u.x - dir.x*u.z, dir.x*u.y - dir.y*u.x}
			bendMag := armLen[k] * (0.3 + 0.25*math.Abs(stableJitter(k*53+int(cid[0]), 1)))
			bendAngle := stableJitter(k*53+int(cid[0])+1, 1) * math.Pi * 2
			bend[k] = pt{
				u.x*math.Cos(bendAngle)*bendMag + v.x*math.Sin(bendAngle)*bendMag,
				u.y*math.Cos(bendAngle)*bendMag + v.y*math.Sin(bendAngle)*bendMag,
				u.z*math.Cos(bendAngle)*bendMag + v.z*math.Sin(bendAngle)*bendMag,
			}
		}

		for i, m := range members {
			inf := info[m.ID]
			if inf.isRoot == 1 {
				out[m.ID] = Position{c[0], c[1], c[2]}
				continue
			}
			dir := armDir[inf.arm]
			denom := armSize[inf.arm] - 1
			if denom < 1 {
				denom = 1
			}
			t := float64(inf.order) / float64(denom)
			mt := 1 - t
			end := pt{c[0] + dir.x*armLen[inf.arm], c[1] + dir.y*armLen[inf.arm], c[2] + dir.z*armLen[inf.arm]}
			ctrl := pt{
				(c[0]+end.x)/2 + bend[inf.arm].x,
				(c[1]+end.y)/2 + bend[inf.arm].y,
				(c[2]+end.z)/2 + bend[inf.arm].z,
			}
			// Quadratic Bezier: B(t) = (1-t)^2*start + 2(1-t)t*ctrl + t^2*end.
			bx := mt*mt*c[0] + 2*mt*t*ctrl.x + t*t*end.x
			by := mt*mt*c[1] + 2*mt*t*ctrl.y + t*t*end.y
			bz := mt*mt*c[2] + 2*mt*t*ctrl.z + t*t*end.z

			// Small lateral wobble — rope texture, not the main shape.
			seed := i*97 + inf.arm*131 + inf.order*7
			wobble := 4.0
			out[m.ID] = Position{
				bx + stableJitter(seed, 1)*wobble,
				by + stableJitter(seed+1, 1)*wobble,
				bz + stableJitter(seed+2, 1)*wobble,
			}
		}
	}
	return out
}
