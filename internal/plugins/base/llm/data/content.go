package data

const (
	ContentTypeText     = "text"
	ContentTypeImageUrl = "image_url"
)

type Content interface {
	Type() string
}

type ContentType struct {
	ContentType string `json:"type" bson:"type"`
}

func NewContentType(contentType string) ContentType {
	if contentType != ContentTypeText &&
		contentType != ContentTypeImageUrl {
		contentType = ContentTypeText
	}
	return ContentType{
		ContentType: contentType,
	}
}

func (c *ContentType) Type() string {
	return c.ContentType
}

type TextContent struct {
	ContentType `bson:",inline"`
	Text        string `json:"text" bson:"text"`
}

func NewTextContent(text string) *TextContent {
	return &TextContent{
		ContentType: NewContentType(ContentTypeText),
		Text:        text,
	}
}

type ImageUrl struct {
	Url    string `json:"url" bson:"url"`
	Detail string `json:"detail,omitempty" bson:"detail,omitempty"`
}

type ImageContent struct {
	ContentType `bson:",inline"`
	ImageUrl    ImageUrl `json:"image_url" bson:"image_url"`
}
