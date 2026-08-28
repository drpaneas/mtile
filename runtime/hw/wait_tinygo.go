//go:build tinygo

package hw

func WaitForPresentWindow() {
	if GetReg16(VCOUNT) >= ScreenHeight {
		return
	}
	for GetReg16(VCOUNT) < ScreenHeight {
	}
}

func WaitVDraw() {
	for GetReg16(VCOUNT) >= ScreenHeight {
	}
}
