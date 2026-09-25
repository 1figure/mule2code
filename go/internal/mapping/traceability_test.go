// Package mapping holds MAPPING.md as a check: every flow, batch job/step, and every connector,
// transform and validation element of the two Mule implementation files must be named in a
// comment (or string) somewhere under go/. Loggers, set-variables and error-handler branches are
// excluded; they have no code counterpart of their own. Same scope as the C# MuleTraceabilityTests.
package mapping_test

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muletocode/internal/resources"
)

const (
	coreNS  = "http://www.mulesoft.org/schema/mule/core"
	docNS   = "http://www.mulesoft.org/schema/mule/documentation"
	batchNS = "http://www.mulesoft.org/schema/mule/batch"
)

var connectorNamespaces = map[string]string{
	batchNS: "batch:",
	"http://www.mulesoft.org/schema/mule/ee/core":    "ee:",
	"http://www.mulesoft.org/schema/mule/validation": "validation:",
	"http://www.mulesoft.org/schema/mule/db":         "db:",
	"http://www.mulesoft.org/schema/mule/http":       "http:",
	"http://www.mulesoft.org/schema/mule/sftp":       "sftp:",
	"http://www.mulesoft.org/schema/mule/email":      "email:",
}

var coreElementsWithDocName = map[string]bool{"parse-template": true, "foreach": true, "try": true, "on-error-continue": true}

var implementationFiles = []string{
	"mule/batch-contacts-csv-to-db/src/main/mule/batch-contacts-csv-to-db-impl.xml",
	"mule/contacts-api/src/main/mule/contacts-api-impl.xml",
}

// element is one traceable Mule element: how it is spelled in the report and the value that must appear in the source.
type element struct {
	file, spelling, value string
}

func attr(se xml.StartElement, space, local string) (string, bool) {
	for _, a := range se.Attr {
		if a.Name.Local == local && a.Name.Space == space {
			return a.Value, true
		}
	}
	return "", false
}

// muleElements lists the traceable elements of both implementation files.
func muleElements(t *testing.T, root string) []element {
	t.Helper()
	var out []element
	for _, rel := range implementationFiles {
		f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Base(rel)
		dec := xml.NewDecoder(f)
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			ns, local := se.Name.Space, se.Name.Local
			switch {
			case ns == coreNS && (local == "flow" || local == "sub-flow"):
				name, _ := attr(se, "", "name")
				out = append(out, element{file, fmt.Sprintf("flow name=%q", name), name})
			case ns == batchNS && (local == "step" || local == "job"):
				attribute := "name"
				if local == "job" {
					attribute = "jobName"
				}
				name, _ := attr(se, "", attribute)
				out = append(out, element{file, fmt.Sprintf("batch:%s %s=%q", local, attribute, name), name})
			default:
				docName, has := attr(se, docNS, "name")
				if !has {
					continue
				}
				prefix, connector := connectorNamespaces[ns]
				if connector || (ns == coreNS && coreElementsWithDocName[local]) {
					out = append(out, element{file, fmt.Sprintf("%s%s doc:name=%q", prefix, local, docName), docName})
				}
			}
		}
		f.Close()
	}
	return out
}

// sourceText concatenates every non-test Go file under go/.
func sourceText(t *testing.T, root string) string {
	t.Helper()
	var sb strings.Builder
	err := filepath.WalkDir(filepath.Join(root, "go"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sb.Write(data)
		sb.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestEveryMuleElementIsNamedInTheSource(t *testing.T) {
	root, err := resources.Root()
	if err != nil {
		t.Fatal(err)
	}
	elements := muleElements(t, root)
	source := sourceText(t, root)

	files := map[string]bool{}
	for _, e := range elements {
		files[e.file] = true
		t.Run(e.file+" <"+e.spelling+">", func(t *testing.T) {
			if !strings.Contains(source, e.value) {
				t.Errorf("%s: <%s> is not named anywhere under go/", e.file, e.spelling)
			}
		})
	}
	if len(files) != 2 {
		t.Errorf("the check covers %d files, want both implementation files", len(files))
	}
	if len(elements) < 30 {
		t.Errorf("expected the two XML files to yield at least 30 traceable elements, got %d", len(elements))
	}
}
