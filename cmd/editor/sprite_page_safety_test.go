package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestObjectPageImagePasteRejectsHiddenPageOverwrite(t *testing.T) {
	g := newTileReuseTestGame(t)
	pixels := make([]int, 8*16)
	for i := range pixels {
		pixels[i] = 9
	}
	before := g.snapshot().clone()
	// The next row exists in the bank but belongs to a hidden page.
	p := g.currentObjPageRect()
	err := g.applySpritePaste(&spritePastePlacement{pixels: pixels, pixelWidth: 8, pixelHeight: 16}, 0, (p.Y+p.H)/8-1)
	if err == nil || !strings.Contains(err.Error(), "current page") {
		t.Fatalf("cross-page paste error = %v", err)
	}
	if !reflect.DeepEqual(before, g.snapshot().clone()) {
		t.Fatal("rejected paste changed artwork, poses, or selection")
	}
	if got := spriteSheetPixel(*g.model.ObjSheet, 0, p.Y+p.H); got != 3 {
		t.Fatalf("hidden page pixel = %d, want untouched color 3", got)
	}
}

func TestObjectPageInternalPasteRejectsHiddenPageOverwrite(t *testing.T) {
	g := newTileReuseTestGame(t)
	p := g.currentObjPageRect()
	g.currentSprite = ((p.Y+p.H)/8 - 1) * g.spriteCols()
	g.hasCopied = true
	g.copiedSprite = filledCopyBuffer(8, 16, 9)
	before := g.snapshot().clone()
	g.pasteCurrentSprite()
	if !strings.Contains(g.statusMsg, "current page") {
		t.Fatalf("cross-page internal paste status = %q", g.statusMsg)
	}
	if !reflect.DeepEqual(before, g.snapshot().clone()) {
		t.Fatal("rejected internal paste changed artwork, poses, or selection")
	}
}

func TestObjectPageOversizedPNGRejectedBeforePlacement(t *testing.T) {
	g := newTileReuseTestGame(t)
	var payload bytes.Buffer
	// The image fits the whole bank but exceeds the selected page.
	p := g.currentObjPageRect()
	if err := png.Encode(&payload, image.NewNRGBA(image.Rect(0, 0, 8, p.H+8))); err != nil {
		t.Fatal(err)
	}
	before := g.snapshot().clone()
	err := g.beginSpritePastePNG(payload.Bytes())
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("current %dx%d page", p.W, p.H)) {
		t.Fatalf("oversized page import error = %v", err)
	}
	if g.spritePaste != nil || !reflect.DeepEqual(before, g.snapshot().clone()) {
		t.Fatal("oversized page import began placement or changed the model")
	}
}

func TestObjectPageBankToggleRevealsPreservedPose(t *testing.T) {
	g := newTileReuseTestGame(t)
	g.selectPose(2)
	if g.objPage != 1 {
		t.Fatal("fixture did not select the pose on page1")
	}
	if err := g.setObjEditing(false); err != nil {
		t.Fatal(err)
	}
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if g.selectedPose != 2 || g.objPage != 1 || g.currentSprite != objectPageFirstTileForTest(g, 1) {
		t.Fatalf("returning to Animation selected pose%d on page%d at tile%d", g.selectedPose, g.objPage, g.currentSprite)
	}
	if !slices.Contains(g.animationFrames(), g.selectedPose) || g.animationPreviewPose() != g.selectedPose {
		t.Fatal("page frames or preview disagree with the preserved editing selection")
	}
}
