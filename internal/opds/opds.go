package opds

import (
	"encoding/xml"
	"time"
)

type Feed struct {
	XMLName xml.Name `xml:"feed"`
	XMLNS   string   `xml:"xmlns,attr"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated"`
	Author  *Author  `xml:"author,omitempty"`
	Links   []Link   `xml:"link"`
	Entries []Entry  `xml:"entry"`
}

type Entry struct {
	ID         string     `xml:"id"`
	Title      string     `xml:"title"`
	Author     *Author    `xml:"author,omitempty"`
	Categories []Category `xml:"category,omitempty"`
	Content    *Content   `xml:"content,omitempty"`
	Links      []Link     `xml:"link"`
	Published  string     `xml:"published,omitempty"`
}

type Author struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

type Link struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
}

type Category struct {
	Term  string `xml:"term,attr"`
	Label string `xml:"label,attr"`
}

type Content struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

const (
	XMLNS           = "http://www.w3.org/2005/Atom"
	TypeNavigation  = "application/atom+xml; profile=opds-catalog; kind=navigation"
	TypeAcquisition = "application/atom+xml; profile=opds-catalog; kind=acquisition"
	TypeEPUB        = "application/epub+zip"
	TypeJPEG        = "image/jpeg"
	RelSelf         = "self"
	RelNext         = "next"
	RelStart        = "start"
	RelSubsection   = "subsection"
	RelSearch       = "search"
	RelImage        = "http://opds-spec.org/image"
	RelAcquisition  = "http://opds-spec.org/acquisition"
)

func NewFeed(id, title string) *Feed {
	return &Feed{
		XMLNS:   XMLNS,
		ID:      id,
		Title:   title,
		Updated: time.Now().UTC().Format(time.RFC3339),
	}
}

func (f *Feed) AddLink(rel, href, typ string) {
	f.Links = append(f.Links, Link{Rel: rel, Href: href, Type: typ})
}

func (f *Feed) AddEntry(e Entry) {
	f.Entries = append(f.Entries, e)
}

func (f *Feed) Marshal() ([]byte, error) {
	return xml.MarshalIndent(f, "", "  ")
}
