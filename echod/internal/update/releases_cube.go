//go:build cube && !dot && !spot && !panel

package update

// The Echo release stream contains images for different devices. Do not offer
// or install any release until this board has its own recovery and update path.
const releases = ""
