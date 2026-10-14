package page

import "testing"

func TestCursorRoundtrip(t *testing.T) {
	c := Cursor{LastID: 123456789, Size: 20}
	got, err := Decode(c.Encode(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastID != c.LastID || got.Size != c.Size {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeEmpty(t *testing.T) {
	c, err := Decode("", 0)
	if err != nil || c.Size != DefaultSize || c.LastID != 0 {
		t.Fatalf("got %+v err=%v", c, err)
	}
}

func TestDecodeInvalid(t *testing.T) {
	if _, err := Decode("not-a-cursor", 0); err == nil {
		t.Fatal("应报错")
	}
}
