package zai

import (
	"testing"
)

func TestNormalizeRegion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		want    Region
		wantErr bool
	}{
		{"", RegionInternational, false},
		{"international", RegionInternational, false},
		{"intl", RegionInternational, false},
		{"global", RegionInternational, false},
		{"cn", RegionChina, false},
		{"china", RegionChina, false},
		{"chinese", RegionChina, false},
		{"INTERNATIONAL", RegionInternational, false},
		{"CN", RegionChina, false},
		{"us", "", true},
		{"europe", "", true},
	}

	for _, tt := range tests {
		got, err := NormalizeRegion(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizeRegion(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeRegion(%q): got=%v, want=%v", tt.input, got, tt.want)
		}
	}
}

func TestPlanForProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		providerID string
		wantPlan   Plan
		wantOk     bool
	}{
		{"zai_payg", PlanGeneral, true},
		{"zai_coding", PlanCoding, true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		got, ok := PlanForProvider(tt.providerID)
		if ok != tt.wantOk {
			t.Errorf("PlanForProvider(%q): ok=%v, want=%v", tt.providerID, ok, tt.wantOk)
			continue
		}
		if got != tt.wantPlan {
			t.Errorf("PlanForProvider(%q): got=%v, want=%v", tt.providerID, got, tt.wantPlan)
		}
	}
}

func TestResolveOpenAIBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		plan     Plan
		region   Region
		override string
		want     string
		wantErr  bool
	}{
		{"General International", PlanGeneral, RegionInternational, "", GeneralInternationalOpenAIBase, false},
		{"General China", PlanGeneral, RegionChina, "", GeneralChinaOpenAIBase, false},
		{"Coding International", PlanCoding, RegionInternational, "", CodingInternationalOpenAIBase, false},
		{"Coding China", PlanCoding, RegionChina, "", CodingChinaOpenAIBase, false},
		{"Override wins", PlanGeneral, RegionChina, "https://custom.example.com/v1", "https://custom.example.com/v1", false},
		{"Empty override falls back", PlanGeneral, RegionInternational, "", GeneralInternationalOpenAIBase, false},
		{"Unknown plan", Plan("unknown"), RegionInternational, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOpenAIBase(tt.plan, tt.region, tt.override)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveOpenAIBase: err=%v, wantErr=%v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveOpenAIBase: got=%q, want=%q", got, tt.want)
			}
		})
	}
}

func TestKeyMismatchHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		plan     Plan
		secret   string
		contains string
	}{
		{"General plan no hint", PlanGeneral, "abc123", ""},
		{"Coding plan has hint", PlanCoding, "abc123", "Coding Plan"},
		{"Empty secret no hint", PlanCoding, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KeyMismatchHint(tt.plan, tt.secret)
			if tt.contains == "" {
				if got != "" {
					t.Errorf("KeyMismatchHint: got=%q, want empty", got)
				}
			} else if got == "" || !contains(got, tt.contains) {
				t.Errorf("KeyMismatchHint: got=%q, want to contain %q", got, tt.contains)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsAt(s, substr)))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAppendKeyMismatchHint(t *testing.T) {
	t.Parallel()
	err := AppendKeyMismatchHint(nil, "zai_coding", "key")
	if err != nil {
		t.Errorf("AppendKeyMismatchHint on nil error should return nil")
	}

	origErr := &testErr{msg: "original"}
	wrapped := AppendKeyMismatchHint(origErr, "zai_coding", "key-from-coding-plan")
	if wrapped == nil {
		t.Fatal("expected wrapped error")
	}
	if !contains(wrapped.Error(), "original") || !contains(wrapped.Error(), "Coding Plan") {
		t.Errorf("wrapped error missing parts: %v", wrapped)
	}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }
