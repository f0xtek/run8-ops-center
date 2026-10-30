package carstatus

import "testing"

func TestClassify(t *testing.T) {
	p := testProfile()
	tests := []struct {
		in                           string
		kind                         DestKind
		norm, local, industry, train string
	}{
		{"AB45 HTI", DestIndustry, "AB45 HTI", "AB45", "HTI", ""},
		{"AB10  BRR", DestIndustry, "AB10 BRR", "AB10", "BRR", ""},
		{"CHA", DestTrain, "CHA", "", "", "108"},
		{"NOL1", DestTrain, "NOL1", "", "", "346"},
		{"NOL", DestTrain, "NOL", "", "", "346"}, // via alias
		{"A28 A28", DestTrain, "A28 A28", "", "A28", "A28"},
		{"A28 PHS", DestTrain, "A28 PHS", "", "PHS", "A28"},
		{"CPKC .", DestForeign, "CPKC .", "", "", ""},
		{"NOR Hump", DestUnassigned, "NOR Hump", "", "", ""},
		{" NOR  Hump ", DestUnassigned, "NOR Hump", "", "", ""},
		{"None", DestUnassigned, "None", "", "", ""},
		{"XYZ123", DestUnknown, "XYZ123", "", "", ""},
		// The first token is the CSV local symbol; the serving job may differ.
		{"AB45 BRR", DestIndustry, "AB45 BRR", "AB45", "BRR", ""},
		{"AB45 BRL", DestIndustry, "AB45 BRL", "AB45", "BRL", ""}, // BRL is served by csx-bes
		{"ABS ABS", DestIndustry, "ABS ABS", "ABS", "ABS", ""},    // served by a55
		{"AGR AGR", DestIndustry, "AGR AGR", "AGR", "AGR", ""},    // served by a56
		{"AB45 NOPE", DestUnknown, "AB45 NOPE", "", "", ""},
	}
	for _, tc := range tests {
		d := Classify(tc.in, p)
		if d.Kind != tc.kind || d.Tag != tc.norm || d.Raw != tc.in || d.Local != tc.local || d.Industry != tc.industry || d.Train != tc.train {
			t.Errorf("Classify(%q) = %+v", tc.in, d)
		}
	}
}
