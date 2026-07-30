package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/jason/incipit/internal/models"
)

type container struct {
	XMLName   xml.Name `xml:"container"`
	Rootfiles []struct {
		FullPath  string `xml:"full-path,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"rootfiles>rootfile"`
}

type opfPackage struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title    string `xml:"http://purl.org/dc/elements/1.1/ title"`
		Creators []struct {
			Text string `xml:",chardata"`
			Role string `xml:"http://www.idpf.org/2007/opf/ role,attr"`
		} `xml:"http://purl.org/dc/elements/1.1/ creator"`
		Identifiers []struct {
			Text   string `xml:",chardata"`
			Scheme string `xml:"http://www.idpf.org/2007/opf/ scheme,attr"`
		} `xml:"http://purl.org/dc/elements/1.1/ identifier"`
		Language  string `xml:"http://purl.org/dc/elements/1.1/ language"`
		Publisher string `xml:"http://purl.org/dc/elements/1.1/ publisher"`
		Date      string `xml:"http://purl.org/dc/elements/1.1/ date"`
	} `xml:"metadata"`
}

func Parse(path string) (*models.Metadata, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening epub zip: %w", err)
	}
	defer r.Close()

	var opfPath string
	for _, f := range r.File {
		if f.Name == "META-INF/container.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("opening container.xml: %w", err)
			}
			var c container
			if err := xml.NewDecoder(rc).Decode(&c); err != nil {
				rc.Close()
				return nil, fmt.Errorf("parsing container.xml: %w", err)
			}
			rc.Close()
			if len(c.Rootfiles) > 0 {
				opfPath = c.Rootfiles[0].FullPath
			}
			break
		}
	}

	if opfPath == "" {
		return nil, fmt.Errorf("no OPF path found in container.xml")
	}

	for _, f := range r.File {
		if f.Name == opfPath {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("opening OPF file: %w", err)
			}
			defer rc.Close()
			return ParseOPF(rc)
		}
	}

	return nil, fmt.Errorf("OPF file %q not found in epub", opfPath)
}

func ParseOPF(r io.Reader) (*models.Metadata, error) {
	var pkg opfPackage
	if err := xml.NewDecoder(r).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("parsing OPF: %w", err)
	}

	meta := &models.Metadata{
		Title:     pkg.Metadata.Title,
		Language:  pkg.Metadata.Language,
		Publisher: pkg.Metadata.Publisher,
		Date:      pkg.Metadata.Date,
	}

	for _, creator := range pkg.Metadata.Creators {
		if creator.Role == "aut" || (creator.Role == "" && meta.Creator == "") {
			if meta.Creator != "" {
				meta.Creator += ", "
			}
			meta.Creator += creator.Text
		}
	}

	for _, id := range pkg.Metadata.Identifiers {
		isbn := extractISBN(id.Text, id.Scheme)
		if isbn != "" {
			meta.Identifier = isbn
			break
		}
	}

	return meta, nil
}

func extractISBN(text, scheme string) string {
	if scheme == "ISBN" {
		return normalizeISBN(text)
	}
	text = strings.TrimPrefix(text, "urn:isbn:")
	return normalizeISBN(text)
}

func normalizeISBN(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
