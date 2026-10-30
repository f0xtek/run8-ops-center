package profile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Defaults applied when profile.yaml leaves a value out.
const (
	// DefaultMinLengthPct is the minimum length percentage of a road train
	// that has an origin yard.
	DefaultMinLengthPct = 50

	fallbackLengthFt = 60
	fallbackTareTons = 30
)

// Load reads the profile in dir: profile.yaml, IndustryConfiguration.csv and
// train_limits.tsv (optional). It returns an error only when a file cannot be
// read or parsed. Validation problems never fail the load. They are recorded
// in Profile.Report.
func Load(dir string) (*Profile, error) {
	data, err := os.ReadFile(filepath.Join(dir, ProfileFile))
	if err != nil {
		return nil, fmt.Errorf("profile: read %s: %w", ProfileFile, err)
	}
	var s fileSchema
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("profile: %s is empty", ProfileFile)
		}
		return nil, fmt.Errorf("profile: parse %s: %w", ProfileFile, err)
	}
	version := schemaVersionOf(s.SchemaVersion)
	if version > BundledSchemaVersion {
		return nil, fmt.Errorf("profile: %s has schemaVersion %d, but this build supports up to %d", ProfileFile, version, BundledSchemaVersion)
	}

	f, err := os.Open(filepath.Join(dir, IndustryFile))
	if err != nil {
		return nil, fmt.Errorf("profile: open %s: %w", IndustryFile, err)
	}
	industries, err := LoadIndustryCSV(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("profile: %s: %w", IndustryFile, err)
	}

	var rows []limitRow
	lf, err := os.Open(filepath.Join(dir, LimitsFile))
	switch {
	case err == nil:
		rows, err = parseLimitRows(lf)
		lf.Close()
		if err != nil {
			return nil, fmt.Errorf("profile: %s: %w", LimitsFile, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// A route without a limits table is allowed: nothing is limited
		// unless profile.yaml sets limits.
	default:
		return nil, fmt.Errorf("profile: open %s: %w", LimitsFile, err)
	}

	p := build(dir, &s, industries, rows)
	p.SchemaVersion = version
	var run8Problems []Problem
	p.Run8, run8Problems = buildRun8(p, s.Run8)
	p.Report = validate(p, rows)
	p.Report.Problems = append(p.Report.Problems, run8Problems...)
	return p, nil
}

// schemaVersionOf returns the effective schema version: a profile without
// one, or with a value below 1, is version 1.
func schemaVersionOf(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// build assembles a Profile from the parsed files.
func build(dir string, s *fileSchema, industries []Industry, rows []limitRow) *Profile {
	byBase := make(map[string]limitRow, len(rows))
	for _, r := range rows {
		byBase[r.Symbol] = r
	}

	p := &Profile{
		ID:                 strings.TrimSpace(s.ID),
		Name:               strings.TrimSpace(s.Name),
		Dir:                dir,
		DefaultSaveDir:     strings.TrimSpace(s.DefaultSaveDir),
		DefaultAutoSaveDir: strings.TrimSpace(s.DefaultAutoSaveDir),
		Industries:         industries,
		RoutePrefixes:      append([]int(nil), s.RoutePrefixes...),
		CarDefaults:        make(map[CarType]CarDefaults, len(s.CarDefaults)),
		Aliases:            make(map[string]string, len(s.Aliases)),
	}
	if p.ID == "" {
		p.ID = filepath.Base(dir)
	}
	if p.Name == "" {
		p.Name = p.ID
	}

	for _, y := range s.Yards {
		p.Yards = append(p.Yards, Yard{
			Code:             strings.TrimSpace(y.Code),
			Name:             strings.TrimSpace(y.Name),
			AnchorMileposts:  normTags(y.AnchorMileposts),
			AnchorIndustries: normTags(y.AnchorIndustries),
		})
	}

	for _, j := range s.LocalJobs {
		p.LocalJobs = append(p.LocalJobs, buildLocalJob(j, industries, byBase))
	}
	for _, t := range s.RoadTrains {
		p.RoadTrains = append(p.RoadTrains, buildRoadTrain(t, byBase))
	}

	for _, r := range s.CarTypes {
		p.CarTypes = append(p.CarTypes, CarTypeRule{Prefix: strings.TrimSpace(r.Prefix), Type: CarType(strings.TrimSpace(r.Type))})
	}
	for t, d := range s.CarDefaults {
		p.CarDefaults[CarType(strings.TrimSpace(t))] = CarDefaults{LengthFt: d.LengthFt, TareTons: d.TareTons}
	}
	p.UnknownDefaults = CarDefaults{LengthFt: s.UnknownDefaults.LengthFt, TareTons: s.UnknownDefaults.TareTons}
	if p.UnknownDefaults == (CarDefaults{}) {
		p.UnknownDefaults = CarDefaults{LengthFt: fallbackLengthFt, TareTons: fallbackTareTons}
	}

	for from, to := range s.Aliases {
		p.Aliases[NormTag(from)] = NormTag(to)
	}
	for _, t := range s.YardInternalTags {
		if t = NormTag(t); t != "" {
			p.YardInternalTags = append(p.YardInternalTags, t)
		}
	}

	for i := range p.RoadTrains {
		for j := range p.RoadTrains[i].Stops {
			st := &p.RoadTrains[i].Stops[j]
			if st.Kind != "" {
				continue
			}
			switch {
			case p.YardByCode(st.Target) != nil:
				st.Kind = "yard"
			case p.IndustryByTag(st.Target) != nil:
				st.Kind = "industry"
			}
		}
	}
	return p
}

func buildLocalJob(j localJobSchema, industries []Industry, byBase map[string]limitRow) LocalJob {
	job := LocalJob{
		ID:       strings.TrimSpace(j.ID),
		Symbol:   NormTag(j.Symbol),
		HomeYard: strings.TrimSpace(j.HomeYard),
	}
	if job.ID == "" {
		job.ID = strings.ToLower(job.Symbol)
	}
	if j.Industries == nil {
		for _, in := range industries {
			if in.LocalSymbol == job.Symbol {
				job.Industries = append(job.Industries, in.Tag)
			}
		}
	} else {
		job.Industries = normTags(j.Industries)
	}
	for _, yl := range j.YardLines {
		job.YardLines = append(job.YardLines, YardLine{Yard: strings.TrimSpace(yl.Yard), Carries: normTags(yl.Carries)})
	}
	job.Limit = j.Limit.apply(byBase[job.Symbol].Limit)
	return job
}

func buildRoadTrain(t roadTrainSchema, byBase map[string]limitRow) RoadTrain {
	rt := RoadTrain{
		Symbol:           NormTag(t.Symbol),
		Origin:           strings.TrimSpace(t.Origin),
		OffMapIndustries: normTags(t.OffMapIndustries),
		MaxCarAgeDays:    t.MaxCarAgeDays,
	}
	row := byBase[rt.Symbol]
	rt.Limit = t.Limit.apply(row.Limit)

	rt.Destination = strings.TrimSpace(t.Destination)
	if rt.Destination == "" {
		rt.Destination = row.Dest
	}
	if t.ClassCodes != nil {
		rt.ClassCodes = normTags(t.ClassCodes)
	} else {
		rt.ClassCodes = append([]string(nil), row.Codes...)
	}

	switch {
	case t.MinLengthPct != nil:
		rt.MinLengthPct = *t.MinLengthPct
	case rt.Origin != "":
		rt.MinLengthPct = DefaultMinLengthPct
	}

	for _, st := range t.Stops {
		stop := Stop{
			Kind:   strings.TrimSpace(st.Kind),
			Target: NormTag(st.Target),
			SetOut: normTags(st.SetOut),
			PickUp: normTags(st.PickUp),
		}
		for _, ct := range st.PickUpCarTypes {
			stop.PickUpCarTypes = append(stop.PickUpCarTypes, CarType(strings.TrimSpace(ct)))
		}
		rt.Stops = append(rt.Stops, stop)
	}
	return rt
}

// apply returns base with the fields set in o replaced.
func (o limitSchema) apply(base Limit) Limit {
	if o.MaxLengthFt != nil {
		base.MaxLengthFt = *o.MaxLengthFt
	}
	if o.MaxTons != nil {
		base.MaxTons = *o.MaxTons
	}
	if o.MinHPT != nil {
		base.MinHPT = *o.MinHPT
	}
	return base
}

// normTags normalises every tag and drops empty ones. A nil input gives nil.
func normTags(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t = NormTag(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
