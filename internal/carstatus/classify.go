package carstatus

import (
	"strings"

	"run8-ops-center/internal/profile"
)

// Classify interprets a destination tag against a profile. Whitespace is
// normalised first (profile.NormTag), so "AB10  BRR" equals "AB10 BRR".
//
// The rules run in this order:
//
//  1. blank, "None" or a yard-internal tag such as "NOR Hump": DestUnassigned;
//  2. "<code> .": DestForeign;
//  3. "<x> <industry>" where the industry is carried off-map by a road train:
//     DestTrain for that train (so "A28 A28" waits for the A28 train);
//  4. "<x> <industry>" for an industry in the profile that at least one local
//     job serves: DestIndustry with Local = x. The first token is the CSV
//     local symbol (the block or carrying local), not necessarily the job that
//     serves the industry, so "AB45 BRL" is industry-bound even though BRL is
//     worked by another local;
//  5. "<road train> <industry>" for a road train symbol, such as "A28 PHS"
//     (a train that works the industry on the way): DestTrain for that train;
//  6. a class code or alias carried by a road train: DestTrain;
//  7. anything else: DestUnknown.
func Classify(tag string, p *profile.Profile) Dest {
	d := Dest{Raw: tag, Tag: profile.NormTag(tag)}
	switch {
	case d.Tag == "" || strings.EqualFold(d.Tag, "None") || p.IsYardInternalTag(d.Tag):
		d.Kind = DestUnassigned
		return d
	}

	f := strings.Fields(d.Tag)
	if len(f) == 2 {
		if f[1] == "." {
			d.Kind = DestForeign
			return d
		}
		if rt := p.RoadTrainForOffMapIndustry(f[1]); rt != nil {
			d.Kind, d.Train, d.Industry = DestTrain, rt.Symbol, f[1]
			return d
		}
		if p.IndustryByTag(f[1]) != nil && len(p.LocalJobsServing(f[1])) > 0 {
			d.Kind, d.Local, d.Industry = DestIndustry, f[0], f[1]
			return d
		}
		if rt := p.RoadTrainBySymbol(f[0]); rt != nil && p.IndustryByTag(f[1]) != nil {
			d.Kind, d.Train, d.Industry = DestTrain, rt.Symbol, f[1]
			return d
		}
	}

	if rt := p.RoadTrainForTag(d.Tag); rt != nil {
		d.Kind, d.Train = DestTrain, rt.Symbol
		return d
	}
	d.Kind = DestUnknown
	return d
}
