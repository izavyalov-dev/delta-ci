package state

import "testing"

func TestValidateFixSuggestionPatchUnifiedDiff(t *testing.T) {
	patch := `--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
-fmt.Println("old")
+fmt.Println("new")
`
	if err := validateFixSuggestionPatch(FixSuggestionPatchFormatUnifiedDiff, patch); err != nil {
		t.Fatalf("expected valid patch, got %v", err)
	}
}

func TestValidateFixSuggestionPatchRejectsInvalid(t *testing.T) {
	if err := validateFixSuggestionPatch(FixSuggestionPatchFormatUnifiedDiff, "not a patch"); err == nil {
		t.Fatalf("expected invalid patch error")
	}
}
