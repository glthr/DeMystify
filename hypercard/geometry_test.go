package hypercard

import "testing"

func TestNativePreservesCardSizeAndInteractivePartGeometry(t *testing.T) {
	blocks := validationFixture([]int32{20, 10}, false)
	validationU16(blocks[0].data, 428, 332)
	validationU16(blocks[0].data, 430, 544)
	card := validationFind(blocks, "CARD")
	const part = 42
	validationU16(card.data, part+4, 0x0181) // Hidden, disabled button.
	validationU16(card.data, part+6, -4)
	validationU16(card.data, part+8, 10)
	validationU16(card.data, part+10, 120)
	validationU16(card.data, part+12, 200)
	stack, err := Read(validationEncode(blocks), "Geometry")
	if err != nil {
		t.Fatal(err)
	}
	if stack.Width != 544 || stack.Height != 332 {
		t.Fatalf("card size: %dx%d", stack.Width, stack.Height)
	}
	got := validationCard(t, stack, 20).Parts[0]
	if got.Name != "odd" || !got.Hidden || got.Enabled || got.Rect != (Rectangle{Top: -4, Left: 10, Bottom: 120, Right: 200}) {
		t.Fatalf("part geometry/flags: %+v", got)
	}
}

func TestNativeDefaultCardSize(t *testing.T) {
	blocks := validationFixture([]int32{20}, false)
	validationU16(validationFind(blocks, "BKGD").data, 38+4, 0x0201) // Locked field.
	stack, err := Read(validationEncode(blocks), "Defaults")
	if err != nil {
		t.Fatal(err)
	}
	if stack.Width != 512 || stack.Height != 342 {
		t.Fatalf("default size: %dx%d", stack.Width, stack.Height)
	}
	// Field bit0 means lockText, rather than a disabled button.
	if !stack.Backgrounds[0].Parts[0].Enabled {
		t.Fatal("field incorrectly marked disabled")
	}
}
