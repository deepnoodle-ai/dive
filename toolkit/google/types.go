package google

type thumbnail struct {
	Src    string `json:"src"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

type image struct {
	Src string `json:"src"`
}

type metatag struct {
	Image       string `json:"og:image"`
	Type        string `json:"og:type"`
	SiteName    string `json:"og:site_name"`
	Title       string `json:"og:title"`
	Description string `json:"og:description"`
	URL         string `json:"og:url"`
	TwitterCard string `json:"twitter:card"`
	TwitterSite string `json:"twitter:site"`
	Locale      string `json:"og:locale"`
	Position    string `json:"position"`
}

type pagemap struct {
	Metatags     []metatag   `json:"metatags"`
	CseImage     []image     `json:"cse_image"`
	CseThumbnail []thumbnail `json:"cse_thumbnail"`
}

type searchItem struct {
	Title   string  `json:"title"`
	Link    string  `json:"link"`
	Snippet string  `json:"snippet"`
	Pagemap pagemap `json:"pagemap"`
}

func (item *searchItem) Thumbnail() string {
	if len(item.Pagemap.CseThumbnail) > 0 {
		return item.Pagemap.CseThumbnail[0].Src
	}
	return ""
}

func (item *searchItem) Image() string {
	if len(item.Pagemap.CseImage) > 0 {
		return item.Pagemap.CseImage[0].Src
	}
	return ""
}

type page struct {
	Title          string `json:"title"`
	TotalResults   string `json:"totalResults"`
	SearchTerms    string `json:"searchTerms"`
	Count          int    `json:"count"`
	StartIndex     int    `json:"startIndex"`
	InputEncoding  string `json:"inputEncoding"`
	OutputEncoding string `json:"outputEncoding"`
	Safe           string `json:"safe"`
	Cx             string `json:"cx"`
}

type queries struct {
	Request  []page `json:"request"`
	NextPage []page `json:"nextPage"`
}

type results struct {
	Items   []searchItem `json:"items"`
	Queries queries      `json:"queries"`
}
