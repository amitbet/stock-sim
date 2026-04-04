package updater

import "testing"

func TestPickWailsZipAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "stock-sim-windows7-amd64-html-v1.0.0.zip"},
		{Name: "stock-sim-darwin-arm64-v1.0.0.zip"},
		{Name: "stock-sim-windows-amd64-v1.0.0.zip"},
	}
	a, err := PickWailsZipAsset(assets, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "stock-sim-darwin-arm64-v1.0.0.zip" {
		t.Fatalf("got %q", a.Name)
	}
	a, err = PickWailsZipAsset(assets, "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "stock-sim-windows-amd64-v1.0.0.zip" {
		t.Fatalf("got %q", a.Name)
	}
	_, err = PickWailsZipAsset(assets, "linux", "amd64")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPickWailsZipAssetLegacyWailsSuffix(t *testing.T) {
	legacy := []ReleaseAsset{
		{Name: "stock-sim-darwin-arm64-wails-v0.9.0.zip"},
	}
	a, err := PickWailsZipAsset(legacy, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "stock-sim-darwin-arm64-wails-v0.9.0.zip" {
		t.Fatalf("got %q", a.Name)
	}
}
