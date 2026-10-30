package run8save

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// rawVehicle mirrors a RailVehicleStateClass element. The per-truck values are
// two-element arrays in the file.
type rawVehicle struct {
	Model      string    `xml:"rvXMLfilename"`
	UnitType   string    `xml:"unitType"`
	UnitNumber string    `xml:"unitNumber"`
	LoadTons   float64   `xml:"loadWeightUSTons"`
	Tag        string    `xml:"destinationTag"`
	Hazmat     int       `xml:"hazmatPlacardIndex"`
	Prefix     []int     `xml:"currentRoutePrefix>int"`
	Section    []int     `xml:"currentTrackSectionIndex>int"`
	StartNode  []int     `xml:"startNodeIndex>int"`
	Distance   []float64 `xml:"distanceTravelledInMeters>float"`
	Reverse    []bool    `xml:"reverseDirection>boolean"`
}

// rawTrain mirrors a TrainLoader element.
type rawTrain struct {
	ID        string       `xml:"trainID"`
	AI        bool         `xml:"TrainWasAI"`
	Direction int          `xml:"DispatchTrainDirection"`
	Vehicles  []rawVehicle `xml:"unitLoaderList>RailVehicleStateClass"`
}

func (r rawVehicle) vehicle() (Vehicle, error) {
	n := len(r.Prefix)
	if len(r.Section) != n || len(r.StartNode) != n || len(r.Distance) != n || len(r.Reverse) != n {
		return Vehicle{}, fmt.Errorf("vehicle %s#%s: truck arrays differ in length (%d, %d, %d, %d, %d)",
			r.Model, r.UnitNumber, len(r.Prefix), len(r.Section), len(r.StartNode), len(r.Distance), len(r.Reverse))
	}
	v := Vehicle{
		Model:      strings.TrimSpace(r.Model),
		UnitType:   strings.TrimSpace(r.UnitType),
		UnitNumber: strings.TrimSpace(r.UnitNumber),
		LoadTons:   r.LoadTons,
		Tag:        r.Tag,
		Hazmat:     r.Hazmat,
	}
	if n > 0 {
		v.Trucks = make([]Truck, n)
		for i := range v.Trucks {
			v.Trucks[i] = Truck{
				RoutePrefix: r.Prefix[i],
				Section:     r.Section[i],
				StartNode:   r.StartNode[i],
				DistanceM:   r.Distance[i],
				Reverse:     r.Reverse[i],
			}
		}
	}
	return v, nil
}

func (r rawTrain) train() (Train, error) {
	t := Train{
		ID:                strings.TrimSpace(r.ID),
		AI:                r.AI,
		DispatchDirection: r.Direction,
		Vehicles:          make([]Vehicle, 0, len(r.Vehicles)),
	}
	for _, rv := range r.Vehicles {
		v, err := rv.vehicle()
		if err != nil {
			return Train{}, fmt.Errorf("train %s: %w", t.ID, err)
		}
		t.Vehicles = append(t.Vehicles, v)
	}
	return t, nil
}

// Parse decodes a Run8 ScnLoader save. It streams the document, decoding one
// TrainLoader at a time and skipping signals, switches, flags, bulletins and
// industry status. Any XML or numeric error, a document that is not an
// ScnLoader and a document that ends early all return (nil, err): a partial
// snapshot is never produced. FileName and WriteTime are left zero; see
// ParseFile.
func Parse(r io.Reader) (*Snapshot, error) {
	d := xml.NewDecoder(r)

	root, err := nextStart(d)
	if err != nil {
		return nil, fmt.Errorf("parse save: %w", err)
	}
	if root.Name.Local != "ScnLoader" {
		return nil, fmt.Errorf("parse save: root element is %q, want ScnLoader", root.Name.Local)
	}

	snap := &Snapshot{}
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, fmt.Errorf("parse save: %w", err)
		}
		switch el := tok.(type) {
		case xml.EndElement:
			// The only end element reachable here closes ScnLoader; the
			// decoder has already checked it matches.
			return snap, nil
		case xml.StartElement:
			if err := readSection(d, el, snap); err != nil {
				return nil, fmt.Errorf("parse save: %s: %w", el.Name.Local, err)
			}
		}
	}
}

// nextStart returns the first start element of the document.
func nextStart(d *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return xml.StartElement{}, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se, nil
		}
	}
}

// readSection handles one direct child of ScnLoader.
func readSection(d *xml.Decoder, start xml.StartElement, snap *Snapshot) error {
	switch start.Name.Local {
	case "trainList":
		return readTrainList(d, snap)
	case "date":
		var s string
		if err := d.DecodeElement(&s, &start); err != nil {
			return err
		}
		t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid date: %w", err)
		}
		snap.GameDate = t.UTC()
	case "saveGameType":
		var s string
		if err := d.DecodeElement(&s, &start); err != nil {
			return err
		}
		snap.SaveType = SaveType(strings.TrimSpace(s))
	case "description":
		return d.DecodeElement(&snap.Description, &start)
	default:
		// signals, switches, flagList, trackBulletins, AllIndustryStatus,
		// camera elements and anything unknown.
		return d.Skip()
	}
	return nil
}

// readTrainList decodes each TrainLoader in turn; the decoder is positioned
// just after the trainList start element.
func readTrainList(d *xml.Decoder, snap *Snapshot) error {
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return err
		}
		switch el := tok.(type) {
		case xml.EndElement:
			return nil
		case xml.StartElement:
			if el.Name.Local != "TrainLoader" {
				if err := d.Skip(); err != nil {
					return err
				}
				continue
			}
			var rt rawTrain
			if err := d.DecodeElement(&rt, &el); err != nil {
				return err
			}
			t, err := rt.train()
			if err != nil {
				return err
			}
			snap.Trains = append(snap.Trains, t)
		}
	}
}

// ParseFile parses the save at path. FileName is the base name without the
// .xml extension. WriteTime is the sidecar's FileTimestamp when the .info file
// exists and carries one, otherwise the file's modification time.
func ParseFile(path string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open save: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat save: %w", err)
	}
	snap, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	base := filepath.Base(path)
	if strings.EqualFold(filepath.Ext(base), ".xml") {
		base = base[:len(base)-len(".xml")]
	}
	snap.FileName = base

	snap.WriteTime = fi.ModTime()
	if info, ok := readInfoIfPresent(path); ok && !info.FileTimestamp.IsZero() {
		snap.WriteTime = info.FileTimestamp
	}
	return snap, nil
}
