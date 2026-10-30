package run8save

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InfoPath returns the .info sidecar path for a save file: "x.xml" becomes
// "x.info". A path without an .xml extension gets ".info" appended.
func InfoPath(xmlPath string) string {
	if strings.EqualFold(filepath.Ext(xmlPath), ".xml") {
		return xmlPath[:len(xmlPath)-len(".xml")] + ".info"
	}
	return xmlPath + ".info"
}

// scnInfo mirrors the ScnInfo document. The timestamp is kept as text so a
// missing element is distinguishable from a malformed one.
type scnInfo struct {
	XMLName         xml.Name `xml:"ScnInfo"`
	SaveType        string   `xml:"SaveType"`
	Filename        string   `xml:"Filename"`
	DisplayFilename string   `xml:"DisplayFilename"`
	FileTimestamp   string   `xml:"FileTimestamp"`
}

// ReadInfo decodes the .info sidecar at path (the .info file itself, see
// InfoPath). A missing FileTimestamp yields a zero Info.FileTimestamp; a
// present but malformed one is an error.
func ReadInfo(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, fmt.Errorf("read info: %w", err)
	}
	defer f.Close()

	var raw scnInfo
	if err := xml.NewDecoder(f).Decode(&raw); err != nil {
		return Info{}, fmt.Errorf("decode info %s: %w", path, err)
	}
	info := Info{
		SaveType:        SaveType(strings.TrimSpace(raw.SaveType)),
		Filename:        strings.TrimSpace(raw.Filename),
		DisplayFilename: strings.TrimSpace(raw.DisplayFilename),
	}
	if ts := strings.TrimSpace(raw.FileTimestamp); ts != "" {
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return Info{}, fmt.Errorf("decode info %s: FileTimestamp: %w", path, err)
		}
		info.FileTimestamp = t.UTC()
	}
	return info, nil
}

// readInfoIfPresent returns the sidecar for xmlPath, or ok=false when there is
// no usable sidecar (absent or unreadable). A bad sidecar must not stop the
// save itself from being used.
func readInfoIfPresent(xmlPath string) (info Info, ok bool) {
	info, err := ReadInfo(InfoPath(xmlPath))
	return info, err == nil
}
