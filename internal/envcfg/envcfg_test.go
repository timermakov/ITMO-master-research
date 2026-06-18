package envcfg

import "testing"

func TestRequiredMissing(t *testing.T) {
	t.Setenv("DWSS_TEST_MISSING", "")
	_, err := Required("DWSS_TEST_MISSING")
	if err == nil {
		t.Fatal("expected error for empty env")
	}
}

func TestRequiredInt(t *testing.T) {
	t.Setenv("DWSS_TEST_INT", "42")
	n, err := RequiredInt("DWSS_TEST_INT")
	if err != nil {
		t.Fatal(err)
	}
	if n != 42 {
		t.Fatalf("got %d", n)
	}
}
