package orchestrator

import (
	"strings"
	"testing"
)

func TestBuildFixValidationStepIncludesPatchApplyAndTests(t *testing.T) {
	patch := `--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-fmt.Println("old")
+fmt.Println("new")`
	step := buildFixValidationStep(patch)
	if !strings.Contains(step, "git apply --check .delta-ci/fix.patch") {
		t.Fatalf("missing git apply --check")
	}
	if !strings.Contains(step, "git apply .delta-ci/fix.patch") {
		t.Fatalf("missing git apply")
	}
	if !strings.Contains(step, fixValidationTestCommand) {
		t.Fatalf("missing validation test command")
	}
	if !strings.Contains(step, "cat > .delta-ci/fix.patch <<'DELTA_CI_PATCH_") {
		t.Fatalf("missing heredoc delimiter")
	}
}

func TestFixPatchDelimiterAvoidsCollision(t *testing.T) {
	patch := "DELTA_CI_PATCH_ABC\nbody\nDELTA_CI_PATCH_ABC"
	delim := fixPatchDelimiter(patch)
	if strings.Contains("\n"+patch+"\n", "\n"+delim+"\n") {
		t.Fatalf("delimiter should not collide with patch content")
	}
}
