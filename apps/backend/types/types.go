package types

type Torrent struct {
	Scraper      string         `json:"scraper"`
	Hash         string         `json:"hash"`
	ReleaseTitle string         `json:"release_title"`
	Magnet       string         `json:"magnet"`
	Url          string         `json:"url"`
	Name         string         `json:"name"`
	Quality      string         `json:"quality"`
	Info         []string       `json:"info"`
	Links        []Unrestricted `json:"links"`
	Size         uint64         `json:"size"`
	Seeds        uint64         `json:"seeds"`
}

type TmdbItem struct {
	Credits *struct {
		Cast *[]any `json:"cast"`
		Crew *[]any `json:"crew"`
	} `json:"credits,omitempty"`
	Images *struct {
		Logos *[]struct {
			Iso_639_1 *string `json:"iso_639_1"`
			FilePath  string  `json:"file_path"`
		} `json:"logos"`
	} `json:"images,omitempty"`
	Original *interface{} `json:"original,omitempty"`
	LogoPath string       `json:"logo_path"`
}

type TraktItem struct {
	Tmdb      any          `json:"tmdb"`
	Original  *interface{} `json:"original,omitempty"`
	Show      *TraktItem   `json:"show,omitempty"`
	Movie     *TraktItem   `json:"movie,omitempty"`
	Episode   *TraktItem   `json:"episode,omitempty"`
	Episodes  *[]TraktItem `json:"episodes,omitempty"`
	Title     string       `json:"title"`
	Type      string       `json:"type"`
	WatchedAt string       `json:"watched_at"`
	Genres    []string     `json:"genres"`
	IDs       struct {
		Slug  string `json:"slug"`
		Imdb  string `json:"imdb"`
		Trakt uint   `json:"trakt"`
		Tmdb  uint   `json:"tmdb"`
		Tvdb  uint   `json:"tvdb"`
	} `json:"ids"`
	Runtime uint `json:"runtime"`
	Season  uint `json:"season"`
	Number  uint `json:"number"`
	Watched bool `json:"watched"`
}

type Unrestricted struct {
	Filename string `json:"filename"`
	Download string `json:"download"`
	Filesize int    `json:"filesize"`
}
