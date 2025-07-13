package common

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/thoas/go-funk"
)

type Item struct {
	XMLName   xml.Name `xml:"item"`
	Title     string   `xml:"title"`
	Size      int64    `xml:"size"`
	Link      string   `xml:"link"`
	Enclosure struct {
		URL    string `xml:"url,attr"`
		Length int64  `xml:"length,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"enclosure"`
	Attrs []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"http://torznab.com/schemas/2015/feed attr"`
}

type Rss struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		XMLName     xml.Name `xml:"channel"`
		Link        string   `xml:"link"`
		Title       string   `xml:"title"`
		Description string   `xml:"description"`
		Language    string   `xml:"language"`
		Category    string   `xml:"category"`
		Items       []Item   `xml:"item"`
	} `xml:"channel"`
}

type Torrent struct {
	Scraper      string   `json:"scraper"`
	Hash         string   `json:"hash"`
	Size         uint64   `json:"size"`
	ReleaseTitle string   `json:"release_title"`
	Magnet       string   `json:"magnet"`
	Name         string   `json:"name"`
	Quality      string   `json:"quality"`
	Info         []string `json:"info"`
	Seeds        uint64   `json:"seeds"`
}

type Payload struct {
	Type          string `json:"type"`
	Title         string `json:"title"`
	Year          string `json:"year"`
	Imdb          string `json:"imdb"`
	Trakt         string `json:"trakt"`
	ShowImdb      string `json:"show_imdb"`
	ShowTvdb      string `json:"show_tvdb"`
	ShowTitle     string `json:"show_title"`
	ShowYear      string `json:"show_year"`
	SeasonNumber  string `json:"season_number"`
	EpisodeImdb   string `json:"episode_imdb"`
	EpisodeTvdb   string `json:"episode_tvdb"`
	EpisodeTitle  string `json:"episode_title"`
	EpisodeNumber string `json:"episode_number"`
	EpisodeTrakt  string `json:"episode_trakt"`
}

type DateReplacer struct {
	str   string
	add   [3]int
	day   int
	month int
	year  int
}

func ParseDates(str string, date time.Time) string {
	re := regexp.MustCompile(`::(year|month|day):(\+|-)?(\d+)?:`)
	now := date

	replacer := []DateReplacer{}

	matches := re.FindAllStringSubmatch(str, -1)

	for _, match := range matches {
		yearVal := 0
		monthVal := 0
		dayVal := 0
		if len(match) == 4 {
			val := 0
			if v, err := strconv.Atoi(match[3]); err == nil {
				val = v
			}
			if match[2] == "-" {
				val *= -1
			}
			switch match[1] {
			case "year":
				yearVal = val
			case "month":
				monthVal = val
			case "day":
				dayVal = val
			}
			replacer = append(replacer, DateReplacer{str: match[0], add: [3]int{yearVal, monthVal, dayVal}})
		}
	}
	newnow := now

	for i, j := 0, len(replacer)-1; i < j; i, j = i+1, j-1 {
		replacer[i], replacer[j] = replacer[j], replacer[i]
	}

	for i, r := range replacer {

		year := now.Year()
		month := int(now.Month())
		day := now.Day()

		year += r.add[0]
		month += r.add[1]
		if month < 1 {
			month = 12 + month
			year -= 1
		}

		day += r.add[2]

		replacer[i].year = year
		replacer[i].month = month
		replacer[i].day = day

		if strings.Contains(r.str, "year") {
			if i > 0 && strings.Contains(str, r.str+"-") {
				prev := replacer[i-1]
				if prev.year != year && prev.month != month {
					replacer[i].year = prev.year
					year = prev.year
				}
			}
			str = strings.ReplaceAll(str, r.str, fmt.Sprintf("%d", year))
		}
		if strings.Contains(r.str, "month") {
			if i > 0 && strings.Contains(str, r.str+"-") {
				prev := replacer[i-1]
				if prev.month != month && prev.day != day {
					replacer[i].month = prev.month
					month = prev.month
				}
			}

			str = strings.ReplaceAll(str, r.str, fmt.Sprintf("%d", month))
		}
		if strings.Contains(r.str, "day") {
			str = strings.ReplaceAll(str, r.str, fmt.Sprintf("%d", day))
		}
	}

	re = regexp.MustCompile(`(\d+)-(\d+)-(\d+)`)

	matches = re.FindAllStringSubmatch(str, -1)
	for _, match := range matches {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		newnow = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		rep := fmt.Sprintf("%d-%d-%d", newnow.Year(), int(newnow.Month()), newnow.Day())
		str = strings.ReplaceAll(str, match[0], rep)
	}

	re2 := regexp.MustCompile(`::daysuntilnow:(\+|-)?(\d+)?:`)

	matches2 := re2.FindAllStringSubmatch(str, -1)
	daysUntilNow := int(time.Since(newnow).Hours() / 24)
	val := 0
	for _, match := range matches2 {
		if len(match) > 2 {
			if v, err := strconv.Atoi(match[2]); err == nil {
				val = v
			}
		}

		if match[1] == "-" {
			val *= -1
		}
		daysUntilNow += val
		str = strings.ReplaceAll(str, match[0], fmt.Sprintf("%d", daysUntilNow))
	}

	return str
}

func SeparateByQuality(torrents []Torrent, payload Payload) []Torrent {
	res := map[string][]Torrent{}

	regexpPatterns := []*regexp.Regexp{
		regexp.MustCompile(
			fmt.Sprintf("s0?%s[.x]?e0?%s", payload.SeasonNumber, payload.EpisodeNumber),
		),
		regexp.MustCompile(
			fmt.Sprintf("Season 0?%s,? ?Episode 0?%s", payload.SeasonNumber, payload.EpisodeNumber),
		),
	}

	for _, q := range []string{"4K", "1080p", "720p", "SD", "CAM"} {
		res[q] = []Torrent{}
	}

	for _, t := range torrents {
		if _, ok := res[t.Quality]; !ok {
			res[t.Quality] = []Torrent{}
		}

		// sort SxEx episodes first
		if payload.Type == "episode" {
			title := strings.ToLower(t.ReleaseTitle)
			for _, pattern := range regexpPatterns {
				if pattern.MatchString(title) {
					res[t.Quality] = append([]Torrent{t}, res[t.Quality]...)
					break
				} else {
					res[t.Quality] = append(res[t.Quality], t)
				}
			}
		} else {
			res[t.Quality] = append(res[t.Quality], t)
		}
	}

	// if len(res["4K"]) > 20 {
	// 	res["4K"] = res["4K"][:20]
	// }

	// if len(res["1080p"]) > 20 {
	// 	res["1080p"] = res["1080p"][:20]
	// }

	// if len(res["720p"]) > 10 {
	// 	res["720p"] = res["720p"][:10]
	// }

	// if len(res["SD"]) > 10 {
	// 	res["SD"] = res["SD"][:10]
	// }

	// if len(res["4K"])+len(res["1080p"]) > 30 {

	// 	res["720p"] = []Torrent{}
	// 	res["SD"] = []Torrent{}
	// }

	// if len(res["4K"]) > 1 {
	// 	res["1080p"] = res["1080p"][:20]
	// }

	// if len(res["1080p"]) > 30 {
	// 	res["1080p"] = res["1080p"][:30]
	// }

	ret := append(res["4K"], res["1080p"]...)
	ret = append(ret, res["720p"]...)
	ret = append(ret, res["SD"]...)
	return ret
}

func Dedupe(torrents []Torrent) []Torrent {
	res := []Torrent{}
	hashes := []string{}
	for _, t := range torrents {
		if t.Magnet != "" && !funk.ContainsString(hashes, t.Hash) {
			res = append(res, t)
			hashes = append(hashes, t.Hash)
		}
	}
	return res
}

func GetInfos(title string) ([]string, string) {
	title = strings.ToLower(title)

	res := []string{}
	quality := "SD"
	infoTypes := map[string][]string{
		"AVC":   {"x264", "x 264", "h264", "h 264", "avc"},
		"HEVC":  {"x265", "x 265", "h265", "h 265", "hevc"},
		"XVID":  {"xvid"},
		"DIVX":  {"divx"},
		"MP4":   {"mp4"},
		"WMV":   {"wmv"},
		"MPEG":  {"mpeg"},
		"4K":    {"4k", "2160p", "216o"},
		"1080p": {"1080p", "1o80", "108o", "1o8p"},
		"720p":  {"720", "72o"},
		"REMUX": {"remux", "bdremux"},
		"DV":    {" dv ", "dovi", "dolby vision", "dolbyvision"},
		"HDR": {
			" hdr ",
			"hdr10",
			"hdr 10",
			"uhd bluray 2160p",
			"uhd blu ray 2160p",
			"2160p uhd bluray",
			"2160p uhd blu ray",
			"2160p bluray hevc truehd",
			"2160p bluray hevc dts",
			"2160p bluray hevc lpcm",
			"2160p us bluray hevc truehd",
			"2160p us bluray hevc dts",
		},
		"SDR":      {" sdr"},
		"AAC":      {"aac"},
		"DTS-HDMA": {"hd ma", "hdma"},
		"DTS-HDHR": {"hd hr", "hdhr", "dts hr", "dtshr"},
		"DTS-X":    {"dtsx", " dts x"},
		"ATMOS":    {"atmos"},
		"TRUEHD":   {"truehd", "true hd"},
		"DD+":      {"ddp", "eac3", " e ac3", " e ac 3", "dd+", "digital plus", "digitalplus"},
		"DD": {
			" dd ",
			"dd2",
			"dd5",
			"dd7",
			" ac3",
			" ac 3",
			"dolby digital",
			"dolbydigital",
			"dolby5",
		},
		"MP3":    {"mp3"},
		"WMA":    {" wma"},
		"2.0":    {"2 0 ", "2 0ch", "2ch"},
		"5.1":    {"5 1 ", "5 1ch", "6ch"},
		"7.1":    {"7 1 ", "7 1ch", "8ch"},
		"BLURAY": {"bluray", "blu ray", "bdrip", "bd rip", "brrip", "br rip"},
		"WEB":    {" web ", "webrip", "webdl", "web rip", "web dl", "webmux"},
		"HD-RIP": {" hdrip", " hd rip"},
		"DVDRIP": {"dvdrip", "dvd rip"},
		"HDTV":   {"hdtv"},
		"PDTV":   {"pdtv"},
		"CAMQUALITY": {
			" cam ", "camrip", "cam rip",
			"hdcam", "hd cam",
			" ts ", " ts1", " ts7",
			"hd ts", "hdts",
			"telesync",
			" tc ", " tc1", " tc7",
			"hd tc", "hdtc",
			"telecine",
			"xbet",
			"hcts", "hc ts",
			"hctc", "hc tc",
			"hqcam", "hq cam",
		},
		"SCR": {"scr ", "screener"},
		"HC": {
			"korsub", " kor ",
			" hc ", "hcsub", "hcts", "hctc", "hchdrip",
			"hardsub", "hard sub",
			"sub hard",
			"hardcode", "hard code",
			"vostfr", "vo stfr",
		},
		"3D": {" 3d"},
	}
	for baseInfo, infoType := range infoTypes {
		for _, info := range infoType {
			if strings.Contains(title, strings.ToLower(baseInfo)) {
				res = append(res, baseInfo)
				break
			}
			if strings.Contains(title, strings.ToLower(info)) {
				res = append(res, baseInfo)
				break
			}

			if strings.Contains(title, strings.ReplaceAll(strings.ToLower(info), " ", ".")) {
				res = append(res, baseInfo)
				break
			}
		}
	}

	if funk.Contains(res, "SDR") && funk.Contains(res, "HDR") {
		res = funk.FilterString(res, func(s string) bool {
			return s != "SDR"
		})
	}

	if funk.Contains(res, "DD") && funk.Contains(res, "DD+") {
		res = funk.FilterString(res, func(s string) bool {
			return s != "DD"
		})
	}

	if funk.ContainsString([]string{"2160p", "remux"}, title) &&
		!funk.Contains(res, []string{"HDR", "SDR"}) {
		res = append(res, "HDR")
	}

	if funk.Contains(res, "720p") {
		quality = "720p"
	}
	if funk.Contains(res, "1080p") {
		quality = "1080p"
	}

	if funk.Contains(res, "4K") {
		quality = "4K"
	}
	if funk.Contains(res, "CAMQUALITY") {
		quality = "CAM"
	}

	return res, quality
}

func SimplifyMagnet(magnet string) string {
	s := ""
	r := regexp.MustCompile(`magnet:\?xt=urn:btih:\s*(.*?)\s*&dn`)
	matches := r.FindAllStringSubmatch(magnet, -1)
	for _, v := range matches {
		s = v[1]
	}
	return "magnet:?xt=urn:btih:" + s
}

func Strip(s string) string {
	var result strings.Builder
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 32 && b <= 126 {
			result.WriteByte(b)
		}
	}
	return result.String()
}

func MqttClient() mqtt.Client {
	// mqtt.DEBUG = stdlog.New(os.Stdout, "", 0)
	// mqtt.ERROR = stdlog.New(os.Stdout, "", 0)
	opts := mqtt.NewClientOptions().
		AddBroker("ws://127.0.0.1:6060/ws/mqtt")
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		log.Error("MQTT", "conneced", c.IsConnected())
	} else {
		log.Info("MQTT", "connected", c.IsConnected())
	}

	return c
}
