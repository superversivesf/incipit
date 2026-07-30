package epub

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func createTestEPUB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	containerXML := `<?xml version="1.0"?>
<container version="1.0">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	opfXML := `<?xml version="1.0"?>
<package xmlns:dc="http://purl.org/dc/elements/1.1/"
         xmlns:opf="http://www.idpf.org/2007/opf/"
         version="3.0">
  <metadata>
    <dc:title>Leviathan Wakes</dc:title>
    <dc:creator opf:role="aut">James S. A. Corey</dc:creator>
    <dc:identifier opf:scheme="ISBN">9780316129084</dc:identifier>
    <dc:language>en</dc:language>
    <dc:publisher>Orbit Books</dc:publisher>
    <dc:date>2011-06-15</dc:date>
  </metadata>
</package>`

	files := map[string]string{
		"META-INF/container.xml": containerXML,
		"OEBPS/content.opf":      opfXML,
	}
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry %s: %v", name, err)
		}
		f.Write([]byte(content))
	}
	w.Close()
	return buf.Bytes()
}

func TestParseFromBytes(t *testing.T) {
	epubBytes := createTestEPUB(t)

	tmpFile := filepath.Join(t.TempDir(), "test.epub")
	if err := os.WriteFile(tmpFile, epubBytes, 0644); err != nil {
		t.Fatalf("writing temp epub: %v", err)
	}

	meta, err := Parse(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", meta.Title)
	}
	if meta.Creator != "James S. A. Corey" {
		t.Errorf("expected creator 'James S. A. Corey', got %q", meta.Creator)
	}
	if meta.Identifier != "9780316129084" {
		t.Errorf("expected identifier '9780316129084', got %q", meta.Identifier)
	}
	if meta.Language != "en" {
		t.Errorf("expected language 'en', got %q", meta.Language)
	}
	if meta.Publisher != "Orbit Books" {
		t.Errorf("expected publisher 'Orbit Books', got %q", meta.Publisher)
	}
	if meta.Date != "2011-06-15" {
		t.Errorf("expected date '2011-06-15', got %q", meta.Date)
	}
}

func TestParseOPFFromReader(t *testing.T) {
	opfXML := `<?xml version="1.0"?>
<package xmlns:dc="http://purl.org/dc/elements/1.1/"
         xmlns:opf="http://www.idpf.org/2007/opf/"
         version="3.0">
  <metadata>
    <dc:title>Test Book</dc:title>
    <dc:creator opf:role="aut">Test Author</dc:creator>
    <dc:identifier opf:scheme="ISBN">1234567890</dc:identifier>
  </metadata>
</package>`

	meta, err := ParseOPF(bytes.NewReader([]byte(opfXML)))
	if err != nil {
		t.Fatalf("ParseOPF failed: %v", err)
	}
	if meta.Title != "Test Book" {
		t.Errorf("expected title 'Test Book', got %q", meta.Title)
	}
	if meta.Creator != "Test Author" {
		t.Errorf("expected creator 'Test Author', got %q", meta.Creator)
	}
}
