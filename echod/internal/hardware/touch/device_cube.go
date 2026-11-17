//go:build cube && !dot && !spot && !panel

package touch

// The Lenovo Smart Clock reports the FT6336U through the vendor mtk-tpd driver.
// Coordinates and orientation follow the published sparrow Linux bring-up. These
// are provisional until verified on the target unit.
var (
	deviceName                 = "mtk-tpd"
	Width, Height              = 800, 480
	rawFallbackW, rawFallbackH = 481, 801
	tapMove                    = 24
	notch                      = 40
	reportsLandscape           = false
)

const holdGestures = false

func toFrame(rawW, rawH, rx, ry int) (int, int) {
	return ry * Width / max(rawH, 1), (rawW - 1 - rx) * Height / max(rawW, 1)
}
