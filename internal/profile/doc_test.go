package profile

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const routeProfilesDoc = "../../docs/route-profiles.md"

// schemaPaths lists the documented path of every yaml field under t. Lists of
// objects get a "[]" suffix, a map of objects or strings gets a ".<key>"
// segment, and lists of plain values keep the bare field name.
func schemaPaths(t reflect.Type, prefix string) []string {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if name == "" {
			continue
		}
		out = append(out, walkType(f.Type, joinPath(prefix, name))...)
	}
	return out
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func walkType(t reflect.Type, path string) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		return schemaPaths(t, path)
	case reflect.Slice:
		if e := t.Elem(); e.Kind() == reflect.Struct {
			return schemaPaths(e, path+"[]")
		}
		return []string{path}
	case reflect.Map:
		e := t.Elem()
		if e.Kind() == reflect.Struct {
			return schemaPaths(e, path+".<"+mapKeyName(path)+">")
		}
		return []string{path + ".<" + mapKeyName(path) + ">"}
	}
	return []string{path}
}

func mapKeyName(path string) string {
	if path == "aliases" {
		return "tag"
	}
	return "type"
}

var docRowRE = regexp.MustCompile("(?m)^\\| `([^`]+)` \\|")

// docSection returns the text between a heading and the next "## " heading.
func docSection(t *testing.T, doc, heading string) string {
	t.Helper()
	i := strings.Index(doc, "\n## "+heading+"\n")
	if i < 0 {
		t.Fatalf("heading %q not found in %s", heading, routeProfilesDoc)
	}
	rest := doc[i+1:]
	if j := strings.Index(rest[3:], "\n## "); j >= 0 {
		rest = rest[:j+3]
	}
	return rest
}

func readDoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(routeProfilesDoc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func docPaths(section string) []string {
	var out []string
	for _, m := range docRowRE.FindAllStringSubmatch(section, -1) {
		out = append(out, m[1])
	}
	return out
}

// TestDocumentedFieldsMatchSchema keeps docs/route-profiles.md and the yaml
// schema structs in step in both directions.
func TestDocumentedFieldsMatchSchema(t *testing.T) {
	schema := schemaPaths(reflect.TypeOf(fileSchema{}), "")
	documented := docPaths(docSection(t, readDoc(t), "profile.yaml"))
	sort.Strings(schema)
	sort.Strings(documented)

	inSchema := map[string]bool{}
	for _, p := range schema {
		inSchema[p] = true
	}
	inDoc := map[string]bool{}
	for _, p := range documented {
		if inDoc[p] {
			t.Errorf("field %q is documented twice", p)
		}
		inDoc[p] = true
		if !inSchema[p] {
			t.Errorf("documented field %q does not exist in the schema", p)
		}
	}
	for _, p := range schema {
		if !inDoc[p] {
			t.Errorf("schema field %q is not documented", p)
		}
	}
	if len(schema) < 30 {
		t.Errorf("schema walk found only %d fields: %v", len(schema), schema)
	}
}

func TestDocumentedProblemCodesMatchValidator(t *testing.T) {
	got := docPaths(docSection(t, readDoc(t), "Validation report"))
	want := []string{CodeOrphanTag, CodeMalformedTag, CodeUnservedIndustry, CodeUnknownRef, CodeDuplicateTag, CodeUnmappedCarType, CodeUnmatchedModel}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("documented codes %v, want %v", got, want)
	}
}

func TestDocumentedProfileFilesExist(t *testing.T) {
	doc := readDoc(t)
	for _, name := range []string{ProfileFile, IndustryFile, LimitsFile} {
		if !strings.Contains(doc, "`"+name+"`") {
			t.Errorf("%s is not mentioned in the doc", name)
		}
	}
}

// TestDocHasNoUnslopTells checks the mechanical rules of the unslop skill:
// no em or en dashes, curly quotes, emojis or listed filler vocabulary, and
// sentence-case headings.
func TestDocHasNoUnslopTells(t *testing.T) {
	doc := readDoc(t)
	for _, r := range []string{"\u2014", "\u2013", "\u2018", "\u2019", "\u201c", "\u201d"} {
		if strings.Contains(doc, r) {
			t.Errorf("doc contains %q", r)
		}
	}
	for _, r := range doc {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			t.Errorf("doc contains emoji %q", r)
		}
	}
	banned := regexp.MustCompile(`(?i)\b(additionally|crucial|delve|pivotal|tapestry|testament|underscore|vibrant|leverage|utilize|facilitate|seamless|robust)\b`)
	if m := banned.FindString(doc); m != "" {
		t.Errorf("doc contains %q", m)
	}
	titleCase := regexp.MustCompile(`^#+ [A-Z][a-z]+( [A-Z][a-z]+)+$`)
	for _, line := range strings.Split(doc, "\n") {
		if titleCase.MatchString(line) {
			t.Errorf("title-case heading: %q", line)
		}
	}
}
