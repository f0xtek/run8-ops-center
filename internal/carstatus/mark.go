package carstatus

import (
	"path"
	"strings"
)

// Mark derives a reporting mark from a model file name. It takes the last
// '_'-separated token of the name without its ".xml" extension, drops trailing
// digits, and keeps the trailing run of two to four upper-case letters:
//
//	R8_Reefer_PCF_57_rbld_ARMN.xml -> ARMN
//	R8_Bulkhead_Thrall_TTJX01.xml  -> TTJX
//	Run8_Tank107GATX02.xml         -> GATX (mark glued to the type token)
//	Run8_Tank105_BNSF.xml          -> BNSF
//
// It returns "" when the token holds no such run, for example
// "Run8_52ftwell_53_40_SchneiderJBHunt.xml".
func Mark(model string) string {
	name := model
	if strings.EqualFold(path.Ext(name), ".xml") {
		name = name[:len(name)-len(".xml")]
	}
	if i := strings.LastIndexByte(name, '_'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimRight(name, "0123456789")
	end := len(name)
	start := end
	for start > 0 && name[start-1] >= 'A' && name[start-1] <= 'Z' {
		start--
	}
	if n := end - start; n < 2 || n > 4 {
		return ""
	}
	return name[start:end]
}

// Number is the displayed car number: the mark and the unit number, or just
// the unit number when no mark can be derived.
func Number(model, unit string) string {
	if m := Mark(model); m != "" {
		return m + " " + unit
	}
	return unit
}
