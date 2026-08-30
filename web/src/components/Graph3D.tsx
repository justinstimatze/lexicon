import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useNavigate, useParams, useSearchParams } from "react-router-dom"
import ForceGraph3D, { type ForceGraphMethods } from "react-force-graph-3d"
import * as THREE from "three"
import { LineMaterial } from "three/examples/jsm/lines/LineMaterial.js"
import { LineSegments2 } from "three/examples/jsm/lines/LineSegments2.js"
import { LineSegmentsGeometry } from "three/examples/jsm/lines/LineSegmentsGeometry.js"
import graphData from "@/data/graph.json"
import type { LexGraph, LayoutName, LexNode } from "@/lib/graph"

const graph = graphData as unknown as LexGraph

type GraphNode = LexNode & { fx?: number; fy?: number; fz?: number; x?: number; y?: number; z?: number }
type GraphLink = { source: string; target: string; type: string }

const LENSES: { key: LayoutName; label: string }[] = [
  { key: "type_grid", label: "Type grid" },
  { key: "flurry", label: "Flurry" },
  { key: "degree_shells", label: "Degree shells" },
  { key: "cosmic_web", label: "Cosmic web" },
  { key: "cluster_puffs", label: "Cluster puffs" },
]

const WHY_HERE: Record<LayoutName, string> = {
  cosmic_web: "positioned by overall force layout — pulled close to what it's related to, regardless of cluster",
  cluster_puffs: "positioned by cluster — grouped with everything sharing this atom's community, above",
  degree_shells: "positioned by in-degree — more-referenced atoms sit toward the center",
  type_grid: "positioned by type-in / type-out — grouped with atoms of the same claim shape",
  flurry: "grown along real intra-cluster edges — a space-colonization branch structure per cluster, connected to its neighbors by genuine cross-cluster edges",
}

const colorByCluster = new Map(graph.clusters.map((c) => [c.id, c.color]))
const labelByCluster = new Map(graph.clusters.map((c) => [c.id, c.label]))

// Deterministic per-link hash — drives flurry's curvature/curve-rotation
// so every arc bends a different amount in a different direction instead
// of all bowing identically flat, which would read as mechanical again.
function linkSeed(a: string, b: string): number {
  const s = a < b ? a + b : b + a
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return Math.abs(h)
}

// nodeVal (relative sphere volume) scaled off in-degree, same curve
// holo.go uses — sqrt so hubs don't visually swallow the graph.
function nodeRadius(n: LexNode) {
  return 1.4 + Math.sqrt(n.in_degree || 0) * 0.55
}

// three-forcegraph's own default-sphere radius formula (Math.cbrt(val) *
// nodeRelSize) and segment count — kept as named constants, not repeated
// literals, since makeSphereMesh below has to match them exactly to look
// identical to what the library would have rendered.
const NODE_REL_SIZE = 4
const NODE_RESOLUTION = 18

// Opacity isn't natively per-instance in Three.js without a shader/vertex-
// attribute detour, but color already is (InstancedMesh.setColorAt) — a
// lerp toward the scene's own background/fog color reads the same as an
// opacity fade against this app's near-black ground, and it means every
// node material stays permanently opaque (no transparent:true anywhere in
// the node-rendering path, which is its own small perf win: a transparent
// material forces per-pixel alpha blending with no early-z rejection, so
// its cost scales with how many screen pixels the spheres/cubes cover —
// exactly what zooming in changes, which is why translucency read fine
// from a distance and tanked perf up close). Matches the literal fog/clear
// color already set in handleEngineStop below (0x16130f) rather than a
// guessed approximation.
const BACKGROUND_COLOR = new THREE.Color(0x16130f)

function tintColor(base: THREE.Color, factor: number): THREE.Color {
  return base.clone().lerp(BACKGROUND_COLOR, factor)
}

// faded is the tour spotlight — dims every node but the current atom and
// its direct neighbors, gated on `touring` at the call site so ordinary
// hover/click interaction keeps the plain, untinted fast path. The
// zoomed-out "category" band dim (previously a third state here) moved to
// the InstancedMesh far tier below — at that zoom nodes render exclusively
// through the instanced batch, never through these per-node factories.
const TOUR_FADE_FACTOR = 0.9

// type_grid's own cell layout is already positioned on a hollow cube's
// surface (see layoutTypeGrid in render/) — cube-shaped nodes carry that
// rectilinear read all the way down instead of switching back to round
// spheres at the individual-atom level. MeshLambertMaterial matches the
// library's own default node material so lighting looks consistent
// between cube and sphere nodes.
function makeCubeMesh(n: LexNode, faded: boolean) {
  const s = nodeRadius(n) * 1.6
  const geo = new THREE.BoxGeometry(s, s, s)
  const base = new THREE.Color(colorByCluster.get(n.cluster) ?? "#8a7a5c")
  const mat = new THREE.MeshLambertMaterial({ color: tintColor(base, faded ? TOUR_FADE_FACTOR : 0) })
  return new THREE.Mesh(geo, mat)
}

// Replaces the library's own default sphere for every non-type_grid lens.
// three-forcegraph hardcodes transparent:true on its default sphere
// material regardless of the nodeOpacity value handed to it (checked its
// source — sphereMaterials[color] = new MeshLambertMaterial({ color,
// transparent: true, opacity })), so nodeOpacity={1} never actually made
// those spheres opaque; only a custom nodeThreeObject controls its own
// material. Only ever called for the "atom" LOD band now (see
// nodeThreeObject below) — the category-band dim this used to carry moved
// to the InstancedMesh far tier's own tint, since this factory no longer
// runs at that zoom.
function makeSphereMesh(n: GraphNode, faded: boolean) {
  const radius = Math.cbrt(nodeRadius(n)) * NODE_REL_SIZE
  const geo = new THREE.SphereGeometry(radius, NODE_RESOLUTION, NODE_RESOLUTION)
  const base = new THREE.Color(colorByCluster.get(n.cluster) ?? "#8a7a5c")
  const mat = new THREE.MeshLambertMaterial({ color: tintColor(base, faded ? TOUR_FADE_FACTOR : 0) })
  return new THREE.Mesh(geo, mat)
}

// Mutates in place — does NOT spread into new node objects. On first
// mount, three-forcegraph resolves each link's string source/target ids
// into live node-object references exactly once (see its digest of
// graphData: `link.source = link[state.linkSource]`, gated so it only
// re-parses when the graphData wrapper is new — but an already-object-
// valued source/target is never re-resolved against a newer node set).
// The old version of this function returned a brand-new object per node
// on every lens switch, so after the very first lens, links kept
// pointing at orphaned objects frozen at whichever positions they had
// when first resolved (in practice, type_grid, the default starting
// lens) — spheres moved to the new lens, edges didn't. Mutating the same
// persistent objects keeps their identity stable across lens switches,
// so a link's resolved reference always reads live, current coordinates.
function applyLayout(nodes: GraphNode[], layoutName: LayoutName) {
  const positions = graph.layouts?.[layoutName]
  if (!positions) return nodes
  for (const n of nodes) {
    const p = positions[n.id]
    if (!p) continue
    // Set x/y/z (actual render position) alongside fx/fy/fz (the pin
    // target) — with cooldownTicks(0) the simulation never ticks, so
    // nothing ever copies the pin into a real coordinate otherwise.
    n.x = p[0]; n.y = p[1]; n.z = p[2]
    n.fx = p[0]; n.fy = p[1]; n.fz = p[2]
  }
  return nodes
}

// react-force-graph-3d's zoomToFit computes a near-zero distance against a
// fully pinned (cooldownTicks(0)) graph — verified by inspecting the live
// Three.js scene: real node meshes sit at their correct positions and
// radii, but zoomToFit still parks the camera a few dozen units from the
// origin, inside the node cloud. Computing the fit distance directly from
// the known node positions sidesteps the library's calculation entirely.
//
// Fitting to the true max is also the wrong default here on its own
// merits: the cluster layouts run a force simulation that pushes weakly-
// connected clusters far from the core (cosmic_web's p50 node-distance is
// ~126, p90 ~330, but a handful of outlier clusters reach ~960) — framing
// the true max leaves the dense 90% of the graph a barely-visible speck.
// Fitting to the 90th percentile instead frames the bulk of the graph;
// the outliers are still there, just reachable by zooming out.
//
// aspect matters here too: fovDeg is the camera's VERTICAL fov, and
// fitting to it alone is the textbook "guarantee zero clipping" formula
// — correct on a roughly-square canvas, but this one runs ~2.3:1 wide,
// so a roughly-spherical point cloud fit to the narrower (vertical) axis
// touches the top/bottom edges while leaving huge untouched margin left
// and right. Fit to whichever axis is WIDER (in angular terms) instead —
// same tolerance the 90th-percentile cutoff above already extends to
// outlier nodes: a few points near the tighter axis may sit just past
// the frame at rest, reachable by zooming out, in exchange for the graph
// actually filling a wide canvas instead of floating in the middle of it.
function cameraDistanceToFit(
  nodes: GraphNode[],
  vFovDeg: number,
  aspect: number,
  paddingFactor = 1.25,
  percentile = 0.9
) {
  const dists = nodes.map((n) => {
    const x = n.x ?? 0, y = n.y ?? 0, z = n.z ?? 0
    return Math.sqrt(x * x + y * y + z * z) + nodeRadius(n)
  })
  dists.sort((a, b) => a - b)
  const idx = Math.min(dists.length - 1, Math.floor(dists.length * percentile))
  const r = dists[idx] || 1
  const vFovRad = (vFovDeg * Math.PI) / 180
  const hFovRad = 2 * Math.atan(Math.tan(vFovRad / 2) * Math.max(aspect, 1e-6))
  const fitFovRad = Math.max(vFovRad, hFovRad)
  return (r / Math.sin(fitFovRad / 2)) * paddingFactor
}

// Canvas-texture billboard label — cheap enough for a handful of
// category markers or a small nearest-to-camera pool; not meant for
// per-node use across the whole catalog (that's what nodeLabel/hover is for).
function makeTextSprite(text: string, color: string, fontPx: number, bgAlpha = 0.6) {
  // Drawn at 1x logical size, the canvas backing store is only as many
  // pixels as the label needs at rest — magnified by proximity (camera
  // close, or a HiDPI screen) it reads blurry and chunky. SS renders
  // the same logical size at 3x the pixel density; on-screen scale
  // below is computed from the logical (unscaled) width/height, so the
  // sprite's size in the scene is unchanged, only its sharpness is.
  const SS = 3
  const font = `600 ${fontPx}px "JetBrains Mono", ui-monospace, monospace`
  const measureCtx = document.createElement("canvas").getContext("2d")!
  measureCtx.font = font
  const padX = fontPx * 0.5
  const width = Math.ceil(measureCtx.measureText(text).width + padX * 2)
  const height = Math.ceil(fontPx * 1.7)

  const canvas = document.createElement("canvas")
  canvas.width = width * SS
  canvas.height = height * SS
  const ctx = canvas.getContext("2d")!
  ctx.scale(SS, SS)
  ctx.font = font
  ctx.textBaseline = "middle"
  ctx.fillStyle = `rgba(12, 9, 6, ${bgAlpha})`
  ctx.fillRect(0, 0, width, height)
  ctx.fillStyle = color
  ctx.fillText(text, padX, height / 2)
  const texture = new THREE.CanvasTexture(canvas)
  texture.minFilter = THREE.LinearFilter
  texture.anisotropy = 4
  // depthTest:false, not just depthWrite:false — depthWrite alone still
  // lets a nearer opaque sphere win the depth test and hide the label
  // behind it. Labels are meant to always read like a HUD overlay, not
  // compete with geometry for the same pixel. renderOrder pushes them
  // past the (still depth-tested) link lines too, so a label always
  // wins regardless of what else occupies that screen position.
  const sprite = new THREE.Sprite(new THREE.SpriteMaterial({ map: texture, transparent: true, depthWrite: false, depthTest: false }))
  sprite.renderOrder = 999
  const scale = height / 11
  sprite.scale.set((width / height) * scale, scale, 1)
  return sprite
}

function disposeSprite(s: THREE.Sprite) {
  s.material.map?.dispose()
  s.material.dispose()
}

// type_in is the coarsest category axis the data actually carries (9
// values, vs. 129 fine-grained clusters) — the right grain for a
// zoomed-out "category" label cloud that stays legible.
const TYPE_IN_VALUES = Array.from(new Set(graph.nodes.map((n) => n.type_in))).sort()

function categoryCentroids(nodes: GraphNode[]) {
  const sums = new Map<string, { x: number; y: number; z: number; n: number }>()
  for (const node of nodes) {
    if (typeof node.x !== "number") continue
    const s = sums.get(node.type_in) ?? { x: 0, y: 0, z: 0, n: 0 }
    s.x += node.x
    s.y += node.y ?? 0
    s.z += node.z ?? 0
    s.n += 1
    sums.set(node.type_in, s)
  }
  return sums
}

type LodBand = "atom" | "cluster" | "category"

function lodBandForRatio(ratio: number): LodBand {
  if (ratio > 1.35) return "category"
  if (ratio < 0.45) return "atom"
  return "cluster"
}

// Every node used to get its own THREE.Mesh regardless of zoom — 2,337+
// draw calls for nodes alone, constant no matter how far out the camera
// sat. lodBand is a single camera-relative value shared by the whole
// graph (see the ratio/setLodBand call site below), not a per-node
// distance, so the fix doesn't need lucida's per-object promote/demote
// machinery: it's a global switch. "atom" band keeps today's real
// per-node objects (makeSphereMesh/makeCubeMesh, unchanged) — that's
// already the only zoom where individual hover/click is a real
// interaction. "cluster"/"category" band renders every node through one
// shared InstancedMesh per shape instead — not individually hoverable,
// which costs nothing real: at that zoom hover-per-atom wasn't a usable
// interaction before this change either. Draw calls for nodes drop from
// thousands to one whenever the camera isn't close, which is most of the
// time given the graph's default framing.
//
// Checked, not assumed: three-render-objects (the layer under
// react-force-graph-3d) resolves hover/click hit identity by reading
// hitObject.__data, stamped once per node onto that node's OWN __threeObj
// — a naive single shared InstancedMesh for every node would make that
// property collide across whichever node's data was written there last.
// Sidestepped entirely here rather than solved: this InstancedMesh is
// added directly to the scene (scene.add, exactly like categoryGroupRef/
// nearLabelGroupRef below already do) instead of going through
// nodeThreeObject, so it's never part of state.objects and the raycaster
// never considers it. The far-tier nodes instead get an empty
// THREE.Object3D back from nodeThreeObject — Object3D.prototype.raycast
// is a no-op, so they're correctly unhoverable for free, and there's no
// geometry/material to allocate or dispose per node at that zoom either.
const UNIT_SPHERE_GEOMETRY = new THREE.SphereGeometry(1, NODE_RESOLUTION, NODE_RESOLUTION)
const UNIT_CUBE_GEOMETRY = new THREE.BoxGeometry(1, 1, 1)
const FAR_TIER_DIM_FACTOR = 0.85

function makeInstancedNodeMesh(geometry: THREE.BufferGeometry, capacity: number) {
  const mesh = new THREE.InstancedMesh(geometry, new THREE.MeshLambertMaterial(), capacity)
  // InstancedMesh's default bounding volume is the base geometry's own
  // (a unit sphere at the mesh's local origin) — it does not expand to
  // cover where per-instance matrices actually place things unless asked
  // to. Left alone, the whole batch gets frustum-culled the instant the
  // origin itself leaves view, which for a graph spread across hundreds
  // of units means nodes vanishing well before they're actually offscreen.
  mesh.frustumCulled = false
  mesh.visible = false
  mesh.count = 0
  return mesh
}

// Scratch objects reused across calls rather than allocated per node —
// this runs on every lens/layout change and on every throttled sway tick
// while flurry's far tier is visible, so per-call allocation here is
// exactly the kind of GC churn worth avoiding.
const scratchPos = new THREE.Vector3()
const scratchQuat = new THREE.Quaternion()
const scratchScale = new THREE.Vector3()
const scratchMatrix = new THREE.Matrix4()
const scratchColor = new THREE.Color()

// positionsOnly skips the color write (and the instanceColor upload) for
// the sway tick, which only ever moves nodes, never recolors them —
// avoids re-uploading a whole InstancedBufferAttribute ~12.5x/sec for
// data that hasn't changed.
function writeInstances(mesh: THREE.InstancedMesh, nodes: GraphNode[], isCube: boolean, dim: boolean, positionsOnly: boolean) {
  let i = 0
  for (const n of nodes) {
    if (typeof n.x !== "number") continue
    const r = isCube ? nodeRadius(n) * 1.6 : Math.cbrt(nodeRadius(n)) * NODE_REL_SIZE
    scratchPos.set(n.x, n.y ?? 0, n.z ?? 0)
    scratchScale.set(r, r, r)
    scratchMatrix.compose(scratchPos, scratchQuat, scratchScale)
    mesh.setMatrixAt(i, scratchMatrix)
    if (!positionsOnly) {
      scratchColor.set(colorByCluster.get(n.cluster) ?? "#8a7a5c")
      mesh.setColorAt(i, tintColor(scratchColor, dim ? FAR_TIER_DIM_FACTOR : 0))
    }
    i++
  }
  mesh.count = i
  mesh.instanceMatrix.needsUpdate = true
  if (!positionsOnly && mesh.instanceColor) mesh.instanceColor.needsUpdate = true
}

// react-force-graph-3d resolves link.source/target from the id string
// we hand it into a live node-object reference once the graph mounts —
// same reason linkVisual below branches on typeof.
function linkNodeId(v: string | GraphNode): string {
  return typeof v === "object" ? v.id : v
}

// Links used to get the same per-object treatment nodes did — one
// THREE.Line/tube Mesh per link, ~13,097 of them, versus 2,337 nodes.
// Once the far-tier InstancedMesh work above landed, node draw calls
// dropped to ~2 at most zoom levels but the SCENE's total draw-call
// count barely moved (measured live: 13.4k-14.9k regardless of node
// band), because links outnumber nodes 5.6-to-1 and were never touched.
// This is the same fix applied to the other side of the ledger.
//
// Links have no hover/click requirement in this app (checked: no
// onLinkHover/onLinkClick anywhere in the JSX below), so unlike nodes
// there's no raycasting-identity problem to route around — full
// consolidation is safe, not just the tiered approach nodes needed.
// linkThreeObject below hands every link an empty placeholder (same
// no-op-raycast trick as the node far tier), and these two LineSegments2
// batches — one per width tier — render everything directly, added to
// the scene the same way the InstancedMeshes are (bypassing
// nodeThreeObject/state.objects entirely).
//
// LineSegments2 (three/examples/jsm/lines) rather than plain
// THREE.LineSegments: native GL line width is capped at ~1px on most
// platforms regardless of what's requested, which would have silently
// flattened every width distinction linkWidth used to carry. LineSegments2
// draws each segment as a camera-facing quad via instancing — real,
// configurable pixel width, still one draw call for the whole batch.
//
// Two tiers (not one, not per-link) is the width analog of what
// tintColor did for opacity: true per-segment width isn't supported by
// this technique (linewidth is a single material uniform, not a
// per-instance attribute), so the many small width distinctions the old
// linkWidth callback drew (0.15 vs 0.28 vs 0.35 vs 0.8 vs 1.1 vs 1.2) get
// flattened into "touching the current focus, or flurry's lowDegree
// boost" (emphasized) vs everything else (normal) — the one distinction
// that actually mattered for reading the graph. Color still carries the
// finer distinctions (crossCluster red, per-cluster hue, dimmed
// background) exactly as before, via the same tintColor lerp-toward-
// background technique nodes use — LineSegmentsGeometry.setColors is
// RGB-only, no alpha channel, so opacity is baked into the color itself
// rather than left as material transparency (also means both batches
// stay fully opaque: no alpha-blending pass, same early-z-rejection win
// nodes got).
type CurvedLink = GraphLink & { __curve?: THREE.Curve<THREE.Vector3> | null }

// Points sampled per curved (flurry-only) link -> 5 segments per link.
// three-forcegraph itself samples at curveResolution=30 for its tube
// geometry, but that's resolving a 3D surface; a thin line reads smooth
// at a fraction of that.
const LINK_TESSELLATION = 6
const NORMAL_LINE_WIDTH_PX = 1.1
const EMPHASIZED_LINE_WIDTH_PX = 2.6

// Single global multiplier the library used to apply on top of the old
// linkColor callback's own embedded alpha — baked in here as one of the
// two opacity components combined into the final tint factor below.
function linkGlobalOpacity(focusNode: GraphNode | null, lens: LayoutName, lodBand: LodBand): number {
  if (focusNode) return 0.9
  if (lens === "flurry" && lodBand === "cluster") return 0.55
  if (lodBand === "category") return 0.03
  if (lodBand === "atom") return 0.26
  return 0.12
}

// Ported 1:1 from the linkColor/linkWidth callbacks this file used to
// hand react-force-graph-3d per link (see git history for the original
// branch-by-branch reasoning, preserved verbatim in each branch here).
function linkVisual(l: GraphLink, focusNode: GraphNode | null, lens: LayoutName, lodBand: LodBand) {
  const src = typeof l.source === "object" ? (l.source as GraphNode) : null
  const tgt = typeof l.target === "object" ? (l.target as GraphNode) : null
  const crossCluster = !!(src && tgt && src.cluster !== tgt.cluster)
  const touchesFocus = !!focusNode && !!src && !!tgt && (src.id === focusNode.id || tgt.id === focusNode.id)

  let hex: string
  let alpha: number
  let emphasized: boolean
  if (focusNode) {
    emphasized = touchesFocus
    if (!touchesFocus) {
      hex = "#8c7a5c"
      alpha = 0.06
    } else if (crossCluster) {
      hex = "#e6604c"
      alpha = 1
    } else {
      hex = colorByCluster.get(src!.cluster) ?? "#c9a45e"
      alpha = 1
    }
  } else {
    const lowDegree = lens === "flurry" && ((src?.in_degree ?? 99) <= 3 || (tgt?.in_degree ?? 99) <= 3)
    emphasized = lowDegree
    if (crossCluster) {
      if (lowDegree) {
        hex = "#e28c5a"
        alpha = 0.55
      } else {
        hex = "#d44a3a"
        alpha = lens === "flurry" ? 0.14 : 0.22
      }
    } else if (lowDegree && src) {
      hex = colorByCluster.get(src.cluster) ?? "#c9a45e"
      alpha = 1
    } else if (src) {
      hex = colorByCluster.get(src.cluster) ?? "#c9a45e"
      alpha = lens === "flurry" ? 0.85 : 0.6
    } else {
      hex = "#c9a45e"
      alpha = 0.5
    }
  }
  const effectiveOpacity = Math.max(0, Math.min(1, linkGlobalOpacity(focusNode, lens, lodBand) * alpha))
  const color = tintColor(new THREE.Color(hex), 1 - effectiveOpacity)
  return { color, emphasized }
}

interface LinkSegmentBuckets {
  normalPositions: number[]
  normalColors: number[]
  emphasizedPositions: number[]
  emphasizedColors: number[]
}

// Tessellates every link into 2-point (straight) or LINK_TESSELLATION-point
// (curved, via the __curve three-forcegraph already computed for particle/
// arrow positioning — see the comment at its call site) segment pairs, and
// buckets each into the normal/emphasized arrays syncLinks below uploads.
function buildLinkSegments(
  links: GraphLink[],
  focusNode: GraphNode | null,
  lens: LayoutName,
  lodBand: LodBand,
): LinkSegmentBuckets {
  const buckets: LinkSegmentBuckets = { normalPositions: [], normalColors: [], emphasizedPositions: [], emphasizedColors: [] }
  for (const l of links) {
    const src = typeof l.source === "object" ? (l.source as GraphNode) : null
    const tgt = typeof l.target === "object" ? (l.target as GraphNode) : null
    if (!src || !tgt || typeof src.x !== "number" || typeof tgt.x !== "number") continue
    const { color, emphasized } = linkVisual(l, focusNode, lens, lodBand)
    const curve = (l as CurvedLink).__curve
    const points = curve
      ? curve.getPoints(LINK_TESSELLATION)
      : [new THREE.Vector3(src.x, src.y ?? 0, src.z ?? 0), new THREE.Vector3(tgt.x, tgt.y ?? 0, tgt.z ?? 0)]
    const positions = emphasized ? buckets.emphasizedPositions : buckets.normalPositions
    const colors = emphasized ? buckets.emphasizedColors : buckets.normalColors
    for (let i = 0; i < points.length - 1; i++) {
      const a = points[i]
      const b = points[i + 1]
      positions.push(a.x, a.y, a.z, b.x, b.y, b.z)
      colors.push(color.r, color.g, color.b, color.r, color.g, color.b)
    }
  }
  return buckets
}

const VALID_LENS_KEYS = new Set(LENSES.map((l) => l.key))

export function Graph3D() {
  const fgRef = useRef<ForceGraphMethods<GraphNode, GraphLink> | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)
  const { lens: lensParam } = useParams<{ lens?: string }>()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [lens, setLensState] = useState<LayoutName>(() =>
    lensParam && VALID_LENS_KEYS.has(lensParam as LayoutName) ? (lensParam as LayoutName) : "type_grid",
  )
  // Keeps internal lens state in sync with the URL param — covers
  // back/forward navigation and a pasted /graph/flurry link landing
  // after mount. An unrecognized segment (typo, stale link) redirects to
  // the bare /graph route instead of silently rendering the default
  // lens under a URL that names a lens that doesn't exist.
  useEffect(() => {
    if (!lensParam) return
    if (VALID_LENS_KEYS.has(lensParam as LayoutName)) {
      setLensState(lensParam as LayoutName)
    } else {
      navigate("/graph", { replace: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lensParam])
  // Wraps the raw setter so every lens change — a manual click or the
  // idle tour's own auto-cycle — keeps /graph's URL in sync. `replace`,
  // not push: the tour advances on its own every ~24s, and a lens is
  // current-view state to mirror, not a step worth its own back-button
  // entry (every prior setLens(...) call site is unchanged below —
  // they're all just calling this wrapper now instead of React's setter).
  // Caught live: navigate(path) with a bare path string replaces the
  // whole location, including search — the very first idle-tour lens
  // cycle was silently dropping ?perf=1 mid-session. Passing search
  // through explicitly keeps ?perf=1 (or any other query param) alive
  // across every lens change, tour-driven or manual.
  function setLens(next: LayoutName) {
    setLensState(next)
    navigate(
      { pathname: next === "type_grid" ? "/graph" : `/graph/${next}`, search: searchParams.toString() },
      { replace: true },
    )
  }
  const [selected, setSelected] = useState<GraphNode | null>(null)
  // Hover previews the panel/link-highlight without persisting; a click
  // pins it (selected) so it survives the mouse moving away.
  const [hovered, setHovered] = useState<GraphNode | null>(null)
  const [touring, setTouring] = useState(false)
  const [showLegend, setShowLegend] = useState(false)
  const [lodBand, setLodBand] = useState<LodBand>("cluster")
  // ?perf=1 (e.g. /#/graph/flurry?perf=1) gates every piece of this file's
  // perf instrumentation — off by default, zero runtime cost for anyone who
  // never passes the flag, since the frame-stall rAF loop below doesn't
  // even start and the stats readout never mounts. Mirrors lucida's own
  // ?perf=1/?debug=1 convention (mixed3d.mjs) rather than inventing a new one.
  const perfMode = searchParams.get("perf") === "1"
  const perfModeRef = useRef(perfMode)
  perfModeRef.current = perfMode
  const [perfStats, setPerfStats] = useState<{ calls: number; triangles: number; geometries: number; textures: number } | null>(null)
  // fitDistanceRef/categoryGroupRef/nearLabelGroupRef are imperative
  // Three.js state (camera scale, billboard sprites) that the idle-tick
  // effect below reads every 400ms — refs, not state, so that polling
  // doesn't itself trigger renders.
  const fitDistanceRef = useRef(1)
  // Set once the first real camera fit lands (inside handleEngineStop) —
  // guards the resize-refit effect below from racing it before the graph
  // has settled, and from re-fitting off default/unmeasured node data.
  const hasFittedRef = useRef(false)
  // three-forcegraph calls onEngineStop far more often than "the layout
  // just settled" — verified live: with cooldownTicks(0) it refires the
  // callback repeatedly (observed ~9x in 18s) for the SAME graphData, not
  // just on a real lens switch. Without this guard, every refire yanks the
  // camera back to the default overview mid-interaction — that's the
  // "resets after about a second" / tour-goes-blank bug. Only the first
  // firing per distinct `data` reference (i.e. per actual lens change)
  // should move the camera or rebuild the fog/label sprites.
  const lastFitDataRef = useRef<typeof data | null>(null)
  const categoryGroupRef = useRef<THREE.Group | null>(null)
  const nearLabelGroupRef = useRef<THREE.Group | null>(null)
  // The two InstancedMesh batches nodeThreeObject's far tier renders
  // through (see the block above lodBandForRatio) — created once, lazily,
  // on first scene access and reused across every lens/layout change
  // rather than recreated, so switching lenses only repopulates instance
  // data instead of reallocating geometry/material.
  const sphereInstancedRef = useRef<THREE.InstancedMesh | null>(null)
  const cubeInstancedRef = useRef<THREE.InstancedMesh | null>(null)
  // Sole writer is the idle-tick interval effect below; handleEngineStop
  // and the sway tick read it to know whether the far tier is current
  // without needing lodBand (React state, one render behind) plumbed
  // into their own closures.
  const lodBandRef = useRef<LodBand>("cluster")
  // The two batched link-line objects buildLinkSegments populates —
  // "normal" and "emphasized" width tiers, one LineSegments2 each,
  // created once lazily and repopulated in place (see syncLinks below).
  const normalLinkLinesRef = useRef<LineSegments2 | null>(null)
  const emphasizedLinkLinesRef = useRef<LineSegments2 | null>(null)
  // Guards the sway tick's positions-only path from running before the
  // first full (color-included) build has ever landed — LineSegments2's
  // shader expects an instanceColorStart attribute once vertexColors is
  // on, and a positions-only geometry rebuild before colors exist once
  // would leave that attribute never set at all.
  const linksReadyRef = useRef(false)
  // react-force-graph-3d defaults its canvas to window.innerWidth/Height
  // unless given explicit width/height — without this it renders a
  // viewport-sized canvas that the actual (smaller) container clips,
  // showing only a blown-up crop of the true frame.
  const [size, setSize] = useState({ width: 0, height: 0 })
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const measure = () => setSize({ width: el.clientWidth, height: el.clientHeight })
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const baseNodes = useMemo(
    () => graph.nodes.map((n) => ({ ...n }) as GraphNode),
    [],
  )
  const links = useMemo<GraphLink[]>(
    () => graph.edges.map((e) => ({ source: e.source, target: e.target, type: e.type })),
    [],
  )
  const data = useMemo(() => ({ nodes: applyLayout(baseNodes, lens), links }), [baseNodes, links, lens])

  // Always-fresh mirrors for the idle-tick/tour effect below, which is
  // set up once on mount (empty deps) and would otherwise close over
  // stale data/lens from the first render.
  const dataRef = useRef(data)
  dataRef.current = data
  const lensRef = useRef(lens)
  lensRef.current = lens
  const selectedRef = useRef(selected)
  selectedRef.current = selected
  const hoveredRef = useRef(hovered)
  hoveredRef.current = hovered
  // Set the first time a lens button is clicked. Idle tour still previews
  // nodes after that, but stops silently swapping the lens the user chose
  // — it only wanders lenses on its own before anyone's picked one.
  const lensManualRef = useRef(false)
  // Set inside the idle-tick effect below so the "tour" button can trigger
  // the same fly-to/lens-cycle logic on demand, not just after idling out.
  const startTourRef = useRef<((force?: boolean) => void) | null>(null)
  const focusNode = hovered ?? selected
  // Read by syncLinks' sway-tick call (a stable useCallback, so it can't
  // close over the current render's focusNode directly) — same mirror
  // pattern as dataRef/lensRef/selectedRef above.
  const focusNodeRef = useRef(focusNode)
  focusNodeRef.current = focusNode
  // Node-level spotlight for the idle tour only — gated on `touring`, not
  // on focusNode generally, so ordinary hover/click keeps the always-
  // opaque fast path (the link-dimming above already reacts to any
  // focusNode; this is the node-mesh equivalent, deliberately scoped
  // narrower since it forces a full node-mesh rebuild and tour hops are
  // ~6s apart while hover fires far more often).
  const tourFocusIds = useMemo(() => {
    if (!touring || !focusNode) return null
    const ids = new Set<string>([focusNode.id])
    for (const l of links) {
      const s = linkNodeId(l.source)
      const t = linkNodeId(l.target)
      if (s === focusNode.id) ids.add(t)
      if (t === focusNode.id) ids.add(s)
    }
    return ids
  }, [touring, focusNode, links])
  // Read via ref inside handleEngineStop rather than closing over `size`
  // directly — keeps the callback's identity stable across resizes (it's
  // still recreated on `data` changes, same as before) while always
  // seeing the latest measured size when the library actually calls it.
  const sizeRef = useRef(size)
  sizeRef.current = size

  // Lazily creates the two far-tier InstancedMesh batches on first scene
  // access and repopulates whichever one the current lens uses from
  // current node positions/colors. Stable identity (empty deps) since it
  // reads everything else through refs — safe to call from effects that
  // only set up once on mount as well as ones that re-run per lens/data
  // change. positionsOnly skips the color re-upload for the sway tick.
  const syncFarTier = useCallback((positionsOnly = false) => {
    const scene = fgRef.current?.scene?.()
    if (!scene) return
    if (!sphereInstancedRef.current) {
      sphereInstancedRef.current = makeInstancedNodeMesh(UNIT_SPHERE_GEOMETRY, graph.nodes.length)
      scene.add(sphereInstancedRef.current)
    }
    if (!cubeInstancedRef.current) {
      cubeInstancedRef.current = makeInstancedNodeMesh(UNIT_CUBE_GEOMETRY, graph.nodes.length)
      scene.add(cubeInstancedRef.current)
    }
    // Non-null: both refs were just ensured above, unconditionally.
    const sphere = sphereInstancedRef.current!
    const cube = cubeInstancedRef.current!
    const isCube = lensRef.current === "type_grid"
    const active = isCube ? cube : sphere
    const inactive = isCube ? sphere : cube
    inactive.visible = false
    const showFarTier = lodBandRef.current !== "atom"
    active.visible = showFarTier
    if (!showFarTier) return
    writeInstances(active, dataRef.current.nodes, isCube, lodBandRef.current === "category", positionsOnly)
    // Logged only on a full resync (layout/band change), not the ~12.5Hz
    // sway tick — that's the number worth seeing, and logging it every
    // positions-only tick would be noise, not signal.
    if (perfModeRef.current && !positionsOnly) {
      console.debug(`[perf] far tier resync: ${active.count} instances (${isCube ? "cube" : "sphere"}, band=${lodBandRef.current})`)
    }
  }, [])

  // Batched replacement for every link's own object — see the long
  // comment above buildLinkSegments for why. Lazily creates the two
  // width-tier LineSegments2 batches on first scene access, same pattern
  // as syncFarTier above. positionsOnly (the sway tick) skips the color
  // upload — sway never recolors or re-tiers a link, only moves it.
  const syncLinks = useCallback((positionsOnly = false) => {
    const scene = fgRef.current?.scene?.()
    if (!scene) return
    if (positionsOnly && !linksReadyRef.current) return
    const { width, height } = sizeRef.current
    if (!normalLinkLinesRef.current) {
      const mat = new LineMaterial({ vertexColors: true, linewidth: NORMAL_LINE_WIDTH_PX })
      mat.resolution.set(width || 1, height || 1)
      const obj = new LineSegments2(new LineSegmentsGeometry(), mat)
      obj.frustumCulled = false // see the InstancedMesh comment above on why: bounding volume doesn't track instance data by default
      scene.add(obj)
      normalLinkLinesRef.current = obj
    }
    if (!emphasizedLinkLinesRef.current) {
      const mat = new LineMaterial({ vertexColors: true, linewidth: EMPHASIZED_LINE_WIDTH_PX })
      mat.resolution.set(width || 1, height || 1)
      const obj = new LineSegments2(new LineSegmentsGeometry(), mat)
      obj.frustumCulled = false
      scene.add(obj)
      emphasizedLinkLinesRef.current = obj
    }
    // Non-null: both refs were just ensured above, unconditionally.
    const normalObj = normalLinkLinesRef.current!
    const emphasizedObj = emphasizedLinkLinesRef.current!
    // Resolution feeds LineMaterial's screen-space width calculation —
    // cheap to set every call rather than wiring a separate resize path.
    ;(normalObj.material as LineMaterial).resolution.set(width || 1, height || 1)
    ;(emphasizedObj.material as LineMaterial).resolution.set(width || 1, height || 1)

    const { normalPositions, normalColors, emphasizedPositions, emphasizedColors } = buildLinkSegments(
      dataRef.current.links as GraphLink[],
      focusNodeRef.current,
      lensRef.current,
      lodBandRef.current,
    )
    normalObj.visible = normalPositions.length > 0
    if (normalPositions.length) {
      normalObj.geometry.setPositions(new Float32Array(normalPositions))
      if (!positionsOnly) normalObj.geometry.setColors(new Float32Array(normalColors))
    }
    emphasizedObj.visible = emphasizedPositions.length > 0
    if (emphasizedPositions.length) {
      emphasizedObj.geometry.setPositions(new Float32Array(emphasizedPositions))
      if (!positionsOnly) emphasizedObj.geometry.setColors(new Float32Array(emphasizedColors))
    }
    if (!positionsOnly) linksReadyRef.current = true
    if (perfModeRef.current && !positionsOnly) {
      const total = normalPositions.length / 6 + emphasizedPositions.length / 6
      console.debug(`[perf] link batch resync: ${total} segments (${normalPositions.length / 6} normal, ${emphasizedPositions.length / 6} emphasized)`)
    }
  }, [])

  // Frame-stall logging, gated on ?perf=1 like everything else here — the
  // whole rAF loop doesn't even start when the flag is off, so it costs
  // nothing for a normal visit. dt > 50ms logs at debug level rather than
  // warn (lucida's mixed3d.mjs pattern — a per-stall stack trace is pure
  // noise once you're already watching for this); dt > 5000ms is tagged
  // separately as a long pause (tab backgrounded, OS sleep/wake) rather
  // than a render stall, since those have a different cause and don't
  // want to be confused with one while reading the log.
  useEffect(() => {
    if (!perfMode) return
    let raf = 0
    let last = performance.now()
    const tick = (now: number) => {
      const dt = now - last
      last = now
      if (dt > 5000) {
        console.debug(`[perf] long pause: ${Math.round(dt)}ms (tab backgrounded / OS sleep, not a render stall)`)
      } else if (dt > 50) {
        console.debug(`[perf] frame stall: ${Math.round(dt)}ms`)
      }
      raf = window.requestAnimationFrame(tick)
    }
    raf = window.requestAnimationFrame(tick)
    return () => window.cancelAnimationFrame(raf)
  }, [perfMode])

  // Gentle sway for the flurry lens — a slow, individually-phased drift
  // per node, like kelp or a deep-sea siphonophore moving in a current.
  // Mutating x/y/z on the node objects alone does nothing: three-forcegraph
  // only copies node/link positions into the actual THREE.js objects
  // inside layoutTick(), which only runs while its internal engineRunning
  // flag is true — with cooldownTicks(0) that flag flips back off after
  // exactly one pass, so a mutation made outside that window is invisible
  // until the next lens switch (verified live: a static camera showed
  // pixel-identical sphere positions 3s apart). react-force-graph-3d's ref
  // only forwards an explicit method allowlist (checked its source —
  // resetCountdown/tickFrame aren't on it); d3ReheatSimulation is, and is
  // the one call on that list that flips engineRunning back on. With
  // cooldownTicks(0) every call still takes the "stop after one tick"
  // branch, so the real d3 physics step never runs and resetting alpha to
  // 1 is inert — only the always-run position sync fires, for both node
  // meshes and link curves (calcLinkCurve reads live x/y/z off the same
  // node objects link.source/target already point to, so curved strands
  // follow the sway too). Throttled well below 60Hz: the sway period here
  // is 3.5-8s per node, so a faster resync buys nothing visually, and
  // every sync re-curves all ~13k flurry-lens links — real cost to pay 60
  // times a second for no visible gain. Base positions are captured once
  // per lens/data change so the sway orbits the layout's real position
  // instead of drifting away from it.
  useEffect(() => {
    if (lens !== "flurry") return
    const base = new Map<string, { x: number; y: number; z: number }>()
    for (const n of dataRef.current.nodes) {
      if (typeof n.x === "number") base.set(n.id, { x: n.x, y: n.y ?? 0, z: n.z ?? 0 })
    }
    let raf = 0
    let lastSync = 0
    const seedOf = (id: string) => {
      let h = 0
      for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0
      return Math.abs(h)
    }
    const tick = (tMs: number) => {
      const t = tMs / 1000
      for (const n of dataRef.current.nodes) {
        const b = base.get(n.id)
        if (!b) continue
        const seed = seedOf(n.id)
        const freq = 0.12 + (seed % 100) / 100 / 6
        const amp = 2 + (seed % 50) / 50 * 2.2
        const phase = (seed % 628) / 100
        n.x = b.x + Math.sin(t * freq + phase) * amp
        n.y = b.y + Math.sin(t * freq * 0.8 + phase * 1.3) * amp * 0.8
        n.z = b.z + Math.cos(t * freq * 1.1 + phase * 0.7) * amp
      }
      if (tMs - lastSync > 80) {
        lastSync = tMs
        fgRef.current?.d3ReheatSimulation()
        // Positions only — sway never recolors, so this skips the
        // instanceColor re-upload the full sync does. A no-op write when
        // the far tier isn't currently visible (lodBand === "atom").
        syncFarTier(true)
        // d3ReheatSimulation above also re-runs calcLinkCurve for every
        // link (that's the "every sync re-curves all ~13k flurry-lens
        // links" cost this file already paid before syncLinks existed) —
        // __curve is fresh by the time this reads it.
        syncLinks(true)
      }
      raf = window.requestAnimationFrame(tick)
    }
    raf = window.requestAnimationFrame(tick)
    return () => window.cancelAnimationFrame(raf)
  }, [data, lens, syncFarTier, syncLinks])

  // onEngineStop is the library's own "the graph is ready" signal — in
  // principle fires once per lens switch, since that's a fresh graphData.
  // In practice it refires repeatedly for the SAME graphData (see
  // lastFitDataRef above), so this bails unless `data` actually changed.
  const handleEngineStop = useCallback(() => {
    if (lastFitDataRef.current === data) return
    lastFitDataRef.current = data
    const { width, height } = sizeRef.current
    const aspect = width > 0 && height > 0 ? width / height : 1
    // flurry's per-member arm reach (~100 units) compounds on top of
    // the same far-flung-centroid outliers cosmic_web already has (a
    // documented, accepted ~960-unit tail from the cluster force sim) —
    // p90 alone no longer excludes enough of that tail to keep the
    // default view framed on the typical-sized starfish rather than
    // the handful of outliers. A tighter percentile here only changes
    // what counts as "in frame by default"; nothing is un-renderable.
    const dist = cameraDistanceToFit(data.nodes, 50, aspect, 1.25, lens === "flurry" ? 0.7 : 0.9)
    fgRef.current?.cameraPosition({ x: 0, y: 0, z: dist }, { x: 0, y: 0, z: 0 }, 800)
    fitDistanceRef.current = dist
    hasFittedRef.current = true
    // OrbitControls has no floor on zoom-in by default — nothing stops
    // the camera from ending up inside a node or a few units from a
    // near-label sprite, which is incoherent (a single letter filling
    // the frame) rather than "close." Scaled off the fit distance so a
    // tighter lens (flurry) still allows getting reasonably close.
    const controls = fgRef.current?.controls() as { minDistance?: number } | undefined
    if (controls) controls.minDistance = Math.max(18, dist * 0.05)
    // Fog range scaled off the fit distance, not fixed — a fixed range
    // tuned for cosmic_web's ~1000-unit fit left cluster_puffs and
    // type_grid (whose fit distance runs ~1200-1300, a real difference
    // in their layouts' spread) mostly beyond fog.far, rendering dim.
    const scene = fgRef.current?.scene?.()
    if (scene) {
      if (scene.fog instanceof THREE.Fog) {
        scene.fog.near = dist * 0.2
        scene.fog.far = dist * 1.8
      } else {
        scene.fog = new THREE.Fog(0x16130f, dist * 0.2, dist * 1.8)
      }
      if (!categoryGroupRef.current) {
        const g = new THREE.Group()
        g.visible = false
        scene.add(g)
        categoryGroupRef.current = g
      }
      if (!nearLabelGroupRef.current) {
        const g = new THREE.Group()
        scene.add(g)
        nearLabelGroupRef.current = g
      }
      // Category-level LOD labels: type_in (9 values) is the coarsest
      // grouping the data carries — legible as a label cloud, unlike
      // the 129 fine-grained clusters. Recomputed per lens since each
      // lens places nodes (and so category centroids) differently.
      const centroids = categoryCentroids(data.nodes)
      const catGroup = categoryGroupRef.current
      while (catGroup.children.length) disposeSprite(catGroup.children.pop() as THREE.Sprite)
      for (const typeIn of TYPE_IN_VALUES) {
        const c = centroids.get(typeIn)
        if (!c || c.n === 0) continue
        const sprite = makeTextSprite(typeIn, "#f2d9a8", 72)
        sprite.position.set(c.x / c.n, c.y / c.n, c.z / c.n)
        catGroup.add(sprite)
      }
    }
    // Repopulates the far-tier InstancedMesh from the layout that just
    // landed — a lens switch changes every node's position (and, for
    // type_grid, the active shape), so this needs a full resync, colors
    // included, not just the positions-only path the sway tick uses.
    syncFarTier(false)
    // Same reasoning for links: new positions (and, for flurry, freshly
    // computed __curve) need a full rebuild, not the sway tick's
    // positions-only path.
    syncLinks(false)
  }, [data, lens, syncFarTier, syncLinks])

  // handleEngineStop above covers layout/lens changes; this covers the
  // other thing link color depends on — which node (if any) is
  // hovered/selected. Fires once per distinct focusNode, not per mouse-
  // move (React bails on an identical object reference), matching the
  // cost the old per-link linkColor callback already paid on every
  // hover — this replaces 13k individual per-link re-evaluations with
  // one batched CPU pass + one GPU upload per LineSegments2, not a new
  // cost on top of the old one.
  useEffect(() => {
    syncLinks(false)
  }, [focusNode, syncLinks])

  // A window resize or phone rotation changes the aspect ratio the fit
  // above was computed against — without this the framing just stays
  // whatever it was, correct for the old aspect and not the new one.
  // Snap (0ms) rather than fly: this is a size correction, not a
  // deliberate move worth animating. Skipped until the first real fit
  // (inside handleEngineStop) has landed, and skipped on the very first
  // resize-observer callback that fires it, which is just the initial
  // measurement racing onEngineStop's own first fit, not a real resize.
  useEffect(() => {
    if (!hasFittedRef.current) return
    const aspect = size.width > 0 && size.height > 0 ? size.width / size.height : 1
    const dist = cameraDistanceToFit(dataRef.current.nodes, 50, aspect)
    fgRef.current?.cameraPosition({ x: 0, y: 0, z: dist }, { x: 0, y: 0, z: 0 }, 0)
    fitDistanceRef.current = dist
    // LineMaterial's screen-space width calculation needs the canvas's
    // current pixel size — syncLinks also sets this on every call it
    // makes for other reasons, but a pure resize with no other trigger
    // (no hover, no band crossing) wouldn't otherwise touch it.
    if (normalLinkLinesRef.current) (normalLinkLinesRef.current.material as LineMaterial).resolution.set(size.width || 1, size.height || 1)
    if (emphasizedLinkLinesRef.current) (emphasizedLinkLinesRef.current.material as LineMaterial).resolution.set(size.width || 1, size.height || 1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [size.width, size.height])

  // Zoom-driven LOD (atom-detail / molecule-cluster / category) and the
  // idle-orbit -> screensaver-tour ladder, ported forward from holo.go's
  // idleTick/tourStep. One interval covers both since they're both just
  // "what should the camera/scene be doing right now" checks — no need
  // for two timers. Refs (not the lens/data props) are read throughout
  // so this can be set up once on mount rather than re-armed on every
  // lens switch or graph reflow.
  useEffect(() => {
    let lastInteraction = Date.now()
    let tourActive = false
    // Set by the manual "tour" button. The click that starts a manual
    // tour is itself a pointerdown, which resets lastInteraction to ~0ms
    // ago — without this flag, the idle-check interval below sees "recent
    // interaction" on its very next 400ms tick and calls stopTour() on
    // the tour it just started. This tells that check to leave a manually
    // started tour alone until a REAL subsequent interaction cancels it.
    let manualTour = false
    let tourTimeout: number | undefined
    let layoutTimeout: number | undefined
    let tourCurrent: GraphNode | null = null
    let tourCandidates: GraphNode[] = []

    function bumpInteraction() {
      lastInteraction = Date.now()
      manualTour = false
      stopTour()
    }
    const events: (keyof DocumentEventMap)[] = ["pointerdown", "wheel", "touchstart", "keydown"]
    events.forEach((ev) => document.addEventListener(ev, bumpInteraction, { passive: true }))

    // Separate from bumpInteraction on purpose — bumpInteraction cancels
    // the idle tour and shouldn't fire on a bare mouse-move (that would
    // kill the tour just from hovering the page without clicking). This
    // only tracks "is the camera actively being dragged/zoomed right
    // now," to gate near-label recompute below on the camera having
    // actually settled rather than re-picking the nearest 14 nodes every
    // 400ms tick while someone's mid-rotate — which is both flicker and
    // most of why the offset fix above still looked chaotic while moving.
    let lastMove = Date.now()
    const noteMove = () => { lastMove = Date.now() }
    document.addEventListener("pointermove", noteMove, { passive: true })
    document.addEventListener("wheel", noteMove, { passive: true })

    function tourPickNext(): GraphNode | null {
      const { nodes, links: currentLinks } = dataRef.current
      if (tourCurrent) {
        const relIds = currentLinks
          .filter((l) => linkNodeId(l.source as string | GraphNode) === tourCurrent!.id || linkNodeId(l.target as string | GraphNode) === tourCurrent!.id)
          .map((l) => {
            const sid = linkNodeId(l.source as string | GraphNode)
            return sid === tourCurrent!.id ? linkNodeId(l.target as string | GraphNode) : sid
          })
        if (relIds.length) {
          const next = nodes.find((n) => n.id === relIds[Math.floor(Math.random() * relIds.length)])
          if (next && typeof next.x === "number") return next
        }
      }
      if (!tourCandidates.length) return null
      return tourCandidates[Math.floor(Math.random() * tourCandidates.length)]
    }

    function tourStep() {
      if (!tourActive) return
      const target = tourPickNext()
      if (target && typeof target.x === "number") {
        const focusDistance = 55
        const originDist = Math.hypot(target.x, target.y ?? 0, target.z ?? 0) || 1
        const ratio = 1 + focusDistance / originDist
        fgRef.current?.cameraPosition(
          { x: target.x * ratio, y: (target.y ?? 0) * ratio, z: (target.z ?? 0) * ratio },
          { x: target.x, y: target.y ?? 0, z: target.z ?? 0 },
          4200,
        )
        tourCurrent = target
        setSelected(target)
      }
      tourTimeout = window.setTimeout(tourStep, 5800)
    }

    // Every ~28s of tour, advance to the next lens so the graph's
    // structure morphs while idle — the same layout-cycle holo.go ran.
    // Once lensManualRef is set, keep rescheduling (node-previewing
    // continues) but stop actually swapping the lens someone chose.
    function layoutCycleStep() {
      if (!tourActive) return
      if (!lensManualRef.current) {
        const idx = LENSES.findIndex((l) => l.key === lensRef.current)
        setLens(LENSES[(idx + 1) % LENSES.length].key)
      }
      layoutTimeout = window.setTimeout(layoutCycleStep, 28000)
    }

    function startTour(force = false) {
      if (tourActive) return
      // A click pins the panel to that atom on purpose — idle tour
      // previews when nothing's pinned, it doesn't override a real choice.
      // The manual "tour" button passes force=true to override a pin on
      // request — setSelected(null) from that same click hasn't landed in
      // selectedRef yet (state updates are async), so the check alone
      // isn't enough to honor an explicit ask.
      if (!force && selectedRef.current) return
      tourActive = true
      manualTour = force
      setTouring(true)
      tourCurrent = null
      tourCandidates = dataRef.current.nodes.filter((n) => (n.in_degree || 0) >= 6)
      const controls = fgRef.current?.controls() as { autoRotate?: boolean } | undefined
      if (controls) controls.autoRotate = false
      tourStep()
      layoutTimeout = window.setTimeout(layoutCycleStep, 24000)
    }

    startTourRef.current = startTour

    function stopTour() {
      if (!tourActive) return
      tourActive = false
      manualTour = false
      setTouring(false)
      if (tourTimeout) window.clearTimeout(tourTimeout)
      if (layoutTimeout) window.clearTimeout(layoutTimeout)
    }

    function refreshNearLabels(camera: THREE.Camera) {
      const group = nearLabelGroupRef.current
      if (!group) return
      const camPos = camera.position
      // Skip whatever's already named in the detail panel — otherwise the
      // same atom gets captioned twice a few pixels apart.
      const focusId = hoveredRef.current?.id ?? selectedRef.current?.id
      // Widened past the eventual label count on purpose — the greedy
      // screen-space pass below rejects candidates that land too close to
      // one already accepted, so most of this pool never becomes a label.
      // Too narrow a pool and a dense region (flurry's packed beads) has
      // nothing left to fall back to once its nearest few collide.
      const candidates = dataRef.current.nodes
        .filter((n) => typeof n.x === "number" && n.id !== focusId)
        .map((n) => ({
          n,
          d: Math.hypot((n.x ?? 0) - camPos.x, (n.y ?? 0) - camPos.y, (n.z ?? 0) - camPos.z),
        }))
        .sort((a, b) => a.d - b.d)
        .slice(0, 60)
      // Offset in camera-space (right/up), not world-space — a fixed
      // world offset points at a fixed world direction that, depending on
      // where the camera's rotated to, can point straight at a neighbor
      // bead (flurry's beads sit close along a curve) or back toward the
      // camera through the sphere itself. Right/up relative to the
      // camera's own basis always reads as "up-and-right of the node" on
      // screen, whichever way the view is currently rotated.
      const right = new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld, 0).normalize()
      const up = new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld, 1).normalize()
      // Nearest-to-camera isn't the same axis as "won't overlap another
      // label" — several of the 14 nearest can sit right next to each
      // other on screen (dense flurry beads especially) and their labels
      // pile into an unreadable stack even though each one individually
      // clears its own sphere. Project to screen space and greedily
      // reject anything too close to a label already accepted, closest
      // candidate first, so what actually gets picked is spread out.
      const { width: sw, height: sh } = sizeRef.current
      const viewProj = new THREE.Matrix4().multiplyMatrices(camera.projectionMatrix, camera.matrixWorldInverse)
      const MIN_LABEL_GAP_PX = 90
      const MAX_LABELS = 10
      const placedScreen: { x: number; y: number }[] = []
      const withDist: { n: GraphNode; d: number }[] = []
      for (const c of candidates) {
        if (withDist.length >= MAX_LABELS) break
        const v = new THREE.Vector3(c.n.x ?? 0, c.n.y ?? 0, c.n.z ?? 0).applyMatrix4(viewProj)
        if (v.z > 1 || v.z < -1) continue // behind the camera or past the far plane
        const sx = (v.x * 0.5 + 0.5) * sw
        const sy = (1 - (v.y * 0.5 + 0.5)) * sh
        if (sx < 0 || sx > sw || sy < 0 || sy > sh) continue
        const collides = placedScreen.some((p) => Math.hypot(p.x - sx, p.y - sy) < MIN_LABEL_GAP_PX)
        if (collides) continue
        placedScreen.push({ x: sx, y: sy })
        withDist.push(c)
      }
      while (group.children.length) disposeSprite(group.children.pop() as THREE.Sprite)
      for (const { n, d } of withDist) {
        const label = n.name.length > 42 ? `${n.name.slice(0, 39)}…` : n.name
        // Dimmer + smaller than the detail panel on purpose — these are
        // ambient "what's nearby" tags, not a second claim about what's
        // under the cursor, and shouldn't read as competing with it.
        // Background alpha needs to be well above "dim," though: these
        // sprites sit close together (that's the whole point of the
        // near-label pool), and at low alpha two overlapping cards blend
        // both texts into an unreadable smear instead of one winning
        // cleanly.
        const sprite = makeTextSprite(label, "#a8987a", 30, 0.82)
        // Sprites are sized in world units, same as a physical sign — get
        // close enough to one of the 14 nearest-picked nodes (easy to do,
        // since it's whichever node the camera happens to end up beside)
        // and its label balloons to fill the screen the same way the node
        // itself would. NEAR_LABEL_SETTLE_DIST is the distance below which
        // that starts being true; shrinking the sprite in proportion to
        // how far inside it the camera has gotten caps the apparent size
        // instead of letting it grow unbounded as d -> 0.
        const NEAR_LABEL_SETTLE_DIST = 45
        const shrink = Math.min(1, d / NEAR_LABEL_SETTLE_DIST)
        sprite.scale.multiplyScalar(shrink)
        const r = nodeRadius(n)
        const nx = n.x ?? 0, ny = n.y ?? 0, nz = n.z ?? 0
        const clear = r + 2.5
        sprite.position.set(
          nx + right.x * clear + up.x * clear,
          ny + right.y * clear + up.y * clear,
          nz + right.z * clear + up.z * clear,
        )
        group.add(sprite)
      }
    }

    function clearNearLabels() {
      const group = nearLabelGroupRef.current
      if (!group) return
      while (group.children.length) disposeSprite(group.children.pop() as THREE.Sprite)
    }

    const intervalId = window.setInterval(() => {
      const idleMs = Date.now() - lastInteraction
      const controls = fgRef.current?.controls() as
        | { autoRotate?: boolean; autoRotateSpeed?: number; target?: THREE.Vector3 }
        | undefined
      if (idleMs > 30000) {
        if (!tourActive) {
          if (controls) controls.autoRotate = false
          startTour()
        }
      } else if (manualTour) {
        // A manually started tour is running on borrowed idle-time — leave
        // it be until bumpInteraction sees a real subsequent interaction.
      } else if (idleMs > 8000) {
        stopTour()
        if (controls) {
          controls.autoRotate = true
          controls.autoRotateSpeed = 0.55
        }
      } else {
        stopTour()
        if (controls) controls.autoRotate = false
      }

      // renderer.info is free — Three.js already computes it every frame
      // for its own bookkeeping, nobody was reading it (checked lucida's
      // mixed3d.mjs/scene3d.mjs — it doesn't either). This is the actual
      // before/after number for the InstancedMesh tiering above, not a
      // felt impression of "it's smoother now."
      if (perfModeRef.current) {
        const info = fgRef.current?.renderer?.()?.info
        if (info) {
          setPerfStats({
            calls: info.render.calls,
            triangles: info.render.triangles,
            geometries: info.memory.geometries,
            textures: info.memory.textures,
          })
        }
      }

      const camera = fgRef.current?.camera()
      if (camera && controls?.target) {
        const dist = camera.position.distanceTo(controls.target)
        const ratio = dist / (fitDistanceRef.current || 1)
        const band = lodBandForRatio(ratio)
        const bandChanged = lodBandRef.current !== band
        lodBandRef.current = band
        setLodBand((prev) => (prev === band ? prev : band))
        if (categoryGroupRef.current) categoryGroupRef.current.visible = band === "category"
        // Crossing a band boundary flips which tier renders every node
        // (real objects <-> the shared InstancedMesh) and, within the far
        // tier, whether it's dimmed (category) or not (cluster) — worth a
        // resync only on the actual crossing, not every 400ms tick. Link
        // color/opacity also depend on band (linkGlobalOpacity), same gate.
        if (bandChanged) {
          syncFarTier(false)
          syncLinks(false)
        }
        // Gate the actual recompute on the camera having been still for a
        // beat — while actively dragging/zooming, leave whatever's already
        // showing alone rather than re-picking the nearest 14 every 400ms,
        // which read as labels chasing the rotation. They catch up once
        // you stop.
        if (band === "atom") {
          if (Date.now() - lastMove > 250) refreshNearLabels(camera)
        } else {
          clearNearLabels()
        }
      }
    }, 400)

    return () => {
      events.forEach((ev) => document.removeEventListener(ev, bumpInteraction))
      document.removeEventListener("pointermove", noteMove)
      document.removeEventListener("wheel", noteMove)
      window.clearInterval(intervalId)
      stopTour()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div ref={containerRef} className="relative h-[70vh] w-full overflow-hidden rounded-md border bg-background">
      <h1 className="sr-only">Connection graph</h1>
      <div className="absolute top-3 left-3 right-3 z-10 flex flex-wrap gap-1.5">
        {LENSES.map((l) => (
          <button
            key={l.key}
            onClick={() => {
              lensManualRef.current = true
              setLens(l.key)
            }}
            className={`rounded-sm border px-2.5 py-1 font-mono text-[11px] tracking-wide uppercase transition-colors ${
              lens === l.key
                ? "border-primary/60 bg-primary/15 text-primary"
                : "border-rule bg-bg-well/60 text-ink-faint hover:text-ink-dim"
            }`}
          >
            {l.label}
          </button>
        ))}
        <span className="mx-1 w-px self-stretch bg-rule" aria-hidden="true" />
        <button
          onClick={() => {
            setSelected(null)
            startTourRef.current?.(true)
          }}
          title="Fly the camera through a few well-connected atoms until you interact"
          className="rounded-sm border border-rule bg-bg-well/60 px-2.5 py-1 font-mono text-[11px] tracking-wide text-ink-faint uppercase transition-colors hover:text-ink-dim"
        >
          ▶ tour
        </button>
        <button
          onClick={() => setShowLegend((v) => !v)}
          className={`rounded-sm border px-2.5 py-1 font-mono text-[11px] tracking-wide uppercase transition-colors ${
            showLegend
              ? "border-primary/60 bg-primary/15 text-primary"
              : "border-rule bg-bg-well/60 text-ink-faint hover:text-ink-dim"
          }`}
        >
          legend
        </button>
      </div>

      {showLegend && (
        <div className="absolute top-12 max-[599px]:top-20 bottom-3 left-3 z-10 w-72 max-w-[calc(100%-1.5rem)] overflow-y-auto rounded-sm border border-rule bg-bg-well/90 p-3 font-mono text-[11px] text-ink-dim backdrop-blur-sm">
          <div className="flex items-center gap-1.5">
            <span className="inline-block h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: "#8ab4b0" }} />
            node color — which cluster (community) it belongs to
          </div>
          <div className="mt-1.5 flex items-center gap-1.5">
            <span className="inline-block h-0.5 w-4 shrink-0" style={{ backgroundColor: "#8ab4b0" }} />
            tinted line — connects two atoms in the same cluster
          </div>
          <div className="mt-1.5 flex items-center gap-1.5">
            <span className="inline-block h-0.5 w-4 shrink-0" style={{ backgroundColor: "#d4573f" }} />
            red line — connects across two different clusters
          </div>
          <div className="mt-1.5 flex items-center gap-1.5">
            <span className="inline-block h-1.5 w-1.5 shrink-0 rounded-full" style={{ backgroundColor: "#5ec8d8" }} />
            cyan dot — flows from a molecule toward the atom it decomposes into
          </div>
          <div className="mt-2 border-t border-rule pt-2">
            hover a node to preview it and light up just its own lines; click
            to pin it (survives moving the mouse, and blocks the idle tour
            from taking over). the sparse ring you'll see far outside the
            dense core isn't a different kind of thing — it's real atoms
            whose only edges are thin, so nothing pulls them in close.
          </div>
          <div className="mt-2 border-t border-rule pt-2">
            <div className="mb-1 text-ink-faint uppercase tracking-wide">the five views</div>
            <ul className="flex flex-col gap-1">
              {LENSES.map((l) => (
                <li key={l.key} className={l.key === lens ? "text-primary" : undefined}>
                  <span className="uppercase">{l.label}</span> — {WHY_HERE[l.key]}
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}

      {focusNode && (
        <div className="absolute top-3 right-3 z-10 w-64 rounded-sm border border-rule bg-bg-well/90 p-3 font-mono text-[11px] text-ink backdrop-blur-sm">
          <div className="text-ink-faint">{focusNode.id}</div>
          <div className="mb-1 text-sm font-semibold text-foreground">{focusNode.name}</div>
          <div className="text-ink-dim">
            {focusNode.type_in} → {focusNode.type_out}
          </div>
          <div className="mt-1 text-ink-dim">in-degree {focusNode.in_degree}</div>
          <div className="mt-2 flex items-center gap-1.5">
            <span
              className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: colorByCluster.get(focusNode.cluster) ?? "#5a4f3a" }}
            />
            <span className="text-ink-dim">{labelByCluster.get(focusNode.cluster) ?? focusNode.cluster}</span>
          </div>
          <div className="mt-2 border-t border-rule pt-2 text-ink-faint">{WHY_HERE[lens]}</div>
        </div>
      )}

      {lodBand !== "cluster" && (
        <div className="absolute bottom-3 left-3 z-10 rounded-sm border border-rule bg-bg-well/60 px-2 py-1 font-mono text-[10px] tracking-wide text-ink-faint uppercase">
          {lodBand === "category" ? "category view — zoom in for atoms" : "atom detail"}
        </div>
      )}

      {touring && (
        <div className="absolute bottom-3 right-3 z-10 rounded-sm border border-rule bg-bg-well/60 px-2 py-1 font-mono text-[10px] tracking-wide text-ink-faint uppercase">
          touring — click or scroll to take over
        </div>
      )}

      {perfMode && perfStats && (
        <div className="absolute bottom-3 left-1/2 z-10 -translate-x-1/2 rounded-sm border border-rule bg-bg-well/60 px-2 py-1 font-mono text-[10px] tracking-wide text-ink-faint uppercase tabular-nums">
          {perfStats.calls} calls · {perfStats.triangles.toLocaleString()} tris · {perfStats.geometries} geo · {perfStats.textures} tex
        </div>
      )}

      <ForceGraph3D
        ref={fgRef}
        graphData={data}
        width={size.width || undefined}
        height={size.height || undefined}
        nodeId="id"
        nodeLabel={(n) => `${(n as GraphNode).id} · ${(n as GraphNode).name}`}
        nodeVal={(n) => nodeRadius(n as GraphNode)}
        nodeColor={(n) => colorByCluster.get((n as GraphNode).cluster) ?? "#8a7a5c"}
        // Unused for rendering now — every lens supplies its own
        // nodeThreeObject below, and three-forcegraph's default sphere
        // material path hardcodes transparent:true regardless of this
        // value (checked its source), so setting nodeOpacity here never
        // actually made a default sphere opaque. Still passed because the
        // library reads it for other bookkeeping (e.g. link/particle
        // radius offsets use nodeVal, not this, but it's part of the
        // same watched-prop group that triggers a node-object rebuild —
        // see makeSphereMesh below, which is what actually needs to
        // re-run when lodBand changes).
        nodeOpacity={lodBand === "category" ? 0.14 : 1}
        nodeResolution={NODE_RESOLUTION}
        nodeThreeObject={(n) => {
          const node = n as GraphNode
          // "cluster"/"category" band: this node renders through the
          // shared far-tier InstancedMesh instead (see syncFarTier above),
          // not through its own object. An empty Object3D still gives
          // three-forcegraph something to position (harmless — it's
          // invisible) without allocating geometry/material per node at
          // this zoom, and Object3D.prototype.raycast is a no-op, so it's
          // correctly un-hoverable for free rather than needing to be
          // special-cased out of onNodeHover/onNodeClick below.
          if (lodBand !== "atom") return new THREE.Object3D()
          const faded = !!tourFocusIds && !tourFocusIds.has(node.id)
          return lens === "type_grid" ? makeCubeMesh(node, faded) : makeSphereMesh(node, faded)
        }}
        // Every link's own object used to come from here (color, width,
        // opacity, curvature all driven by the callbacks that stood
        // where linkThreeObject now is) — see the buildLinkSegments/
        // linkVisual/syncLinks block above lodBandForRatio for the
        // batched replacement, ported branch-for-branch from what used
        // to live in linkColor/linkWidth below. linkCurvature and
        // linkCurveRotation stay wired: three-forcegraph still computes
        // link.__curve from them regardless of linkThreeObject (checked
        // its source — calcLinkCurve runs "for all links, including
        // custom replaced, so it can be used in directional
        // functionality"), and syncLinks reads that same __curve to
        // tessellate flurry's curved links instead of re-deriving the
        // bezier math itself.
        linkThreeObject={() => new THREE.Object3D()}
        linkCurvature={(l) => {
          if (lens !== "flurry") return 0
          const src = typeof l.source === "object" ? (l.source as GraphNode) : null
          const tgt = typeof l.target === "object" ? (l.target as GraphNode) : null
          if (!src || !tgt) return 0
          const crossCluster = src.cluster !== tgt.cluster
          const seed = linkSeed(src.id, tgt.id)
          // Cross-cluster filaments curve gently (still read as threads
          // reaching across void); same-cluster strands curve more —
          // that curve is what makes a strand of beads read as a kelp
          // frond instead of a ruler-straight rod.
          const base = crossCluster ? 0.12 : 0.28
          const spread = crossCluster ? 0.18 : 0.4
          return base + (seed % 1000) / 1000 * spread
        }}
        linkCurveRotation={(l) => {
          if (lens !== "flurry") return 0
          const src = typeof l.source === "object" ? (l.source as GraphNode) : null
          const tgt = typeof l.target === "object" ? (l.target as GraphNode) : null
          if (!src || !tgt) return 0
          return ((linkSeed(src.id, tgt.id) % 6283) / 1000) // 0..2π, seeded so arcs bow in varied directions
        }}
        linkDirectionalParticles={(l) => ((l as GraphLink).type === "decomposes-into" ? 2 : 0)}
        linkDirectionalParticleSpeed={0.006}
        linkDirectionalParticleWidth={1.4}
        linkDirectionalParticleColor={() => "#5ec8d8"}
        backgroundColor="rgba(0,0,0,0)"
        showNavInfo={false}
        cooldownTicks={0}
        onEngineStop={handleEngineStop}
        onNodeClick={(n) => setSelected(n as GraphNode)}
        onNodeHover={(n) => setHovered(n as GraphNode | null)}
        onBackgroundClick={() => setSelected(null)}
      />
    </div>
  )
}
