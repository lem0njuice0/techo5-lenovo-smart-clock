//go:build cube && !dot && !spot && !panel

package layout

// A separate build tag avoids treating this device as an Echo when its stock
// device tree does not contain an Amazon board identifier. Hardware support is
// still incomplete; the tag does not imply this build is ready to install.
func init() {
	Board = "cube"
	Model = "Lenovo Smart Clock (CD-24501F)"
	DefaultName = "Lenovo Smart Clock"
}
