// DOM helpers for the Trace tab's passage rows. Kept out of the component
// files so those export only components (React Fast Refresh's rule).

export function passageElement(index: number): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-passage="${index}"]`)
}

// Used when the selection changes from somewhere other than a click on the
// passage itself (arrow keys, "also in ¶ 9", a network node) — the reader
// needs to be brought to it. A direct click never scrolls.
export function scrollPassageIntoView(index: number) {
  const el = passageElement(index)
  if (!el) return
  const reduce = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches
  el.scrollIntoView({ block: "center", behavior: reduce ? "auto" : "smooth" })
}
