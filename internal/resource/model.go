package resource

type PDFSource struct {
	BookID       string   `json:"bookId"`
	Filename     string   `json:"filename"`
	ExpectedSize int64    `json:"expectedSize"`
	MD5          string   `json:"md5,omitempty"`
	Mirrors      []string `json:"mirrors"`
}
