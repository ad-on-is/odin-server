package jackett

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/go-resty/resty/v2"
	"github.com/odin-movieshow/backend/common"
)

type Indexer struct {
	Id    string `xml:"id,attr"`
	Title string `xml:"title"`
	Caps  struct {
		Searching struct {
			Search struct {
				Available       string `xml:"available,attr"`
				SupportedParams string `xml:"supportedParams,attr"`
			} `xml:"search"`
			MovieSearch struct {
				Available       string `xml:"available,attr"`
				SupportedParams string `xml:"supportedParams,attr"`
			} `xml:"movie-search"`
			TvSearch struct {
				Available       string `xml:"available,attr"`
				SupportedParams string `xml:"supportedParams,attr"`
			} `xml:"tv-search"`
		} `xml:"searching"`
		Categories struct {
			Category []struct {
				ID   string `xml:"id,attr"`
				Name string `xml:"name,attr"`
			} `xml:"category"`
		} `xml:"categories"`
	} `xml:"caps"`
}

func (indexer *Indexer) SearchAvailable() bool {
	return indexer.Caps.Searching.Search.Available == "yes"
}

func (indexer *Indexer) MovieSearchAvailable() bool {
	return indexer.Caps.Searching.MovieSearch.Available == "yes"
}

func (indexer *Indexer) TvSearchAvailable() bool {
	return indexer.Caps.Searching.TvSearch.Available == "yes"
}

func (indexer *Indexer) HasMovieParam(param string) bool {
	return strings.Contains(indexer.Caps.Searching.MovieSearch.SupportedParams, param)
}

func (indexer *Indexer) HasTvParam(param string) bool {
	return strings.Contains(indexer.Caps.Searching.TvSearch.SupportedParams, param)
}

func Search(payload common.Payload) {
	mq := common.MqttClient()

	payload.Title = url.QueryEscape(common.Strip(payload.Title))
	payload.EpisodeTitle = url.QueryEscape(common.Strip(payload.EpisodeTitle))
	payload.ShowTitle = url.QueryEscape(common.Strip(payload.ShowTitle))

	indexers := getIndexerList(payload)
	l := log.Debug
	wg := sync.WaitGroup{}
	indexertopic := "odin-movieshow/indexer/" + payload.Type
	if payload.Type == "episode" {
		indexertopic += "/" + payload.EpisodeTrakt
	} else {
		indexertopic += "/" + payload.Trakt
	}
	total := 0
	log.Debug("MQTT", "topic", indexertopic)
	for _, indexer := range indexers {
		wg.Add(1)
		go func(indexer Indexer) {
			defer wg.Done()
			t1 := time.Now()
			ts := getTorrents(indexer, payload)
			t2 := time.Now()
			if len(ts) > 0 {
				l = log.Info
			}
			l(
				"indexer",
				"id",
				indexer.Title,
				"type",
				payload.Type,
				"torrents",
				len(ts),
				"took",
				fmt.Sprintf("%.1fs", t2.Sub(t1).Seconds()),
			)
			if ts != nil {
				total += len(ts)
				tsstr, _ := json.Marshal(ts)
				mq.Publish(
					indexertopic,
					0,
					false,
					tsstr,
				)
			}
		}(indexer)
	}

	wg.Wait()
	mq.Publish(indexertopic, 0, false, "INDEXING_DONE")
	log.Info("Indexing done", "total", total)
}

func getIndexerList(payload common.Payload) []Indexer {
	cat := "Movies"

	if payload.Type == "episode" {
		cat = "TV"
	}
	var indexers struct {
		Indexers []Indexer `xml:"indexer"`
	}

	jackettUrl := os.Getenv("JACKETT_URL")
	jackettKey := os.Getenv("JACKETT_KEY")

	request := resty.New().
		SetRetryCount(3).
		SetTimeout(time.Second * 60).
		SetRetryWaitTime(time.Second * 2).
		R()
	resp, err := request.Get(
		fmt.Sprintf(
			"%s/api/v2.0/indexers/type:public/results/torznab/api?apikey=%s&t=indexers&configured=true",
			jackettUrl,
			jackettKey,
		),
	)
	if err != nil {
		log.Error("getting indexers", "error", err.Error())
		return []Indexer{}
	}

	if err := xml.Unmarshal(resp.Body(), &indexers); err != nil {
		log.Error("indexers", err)
	}

	neededIndexers := []Indexer{}
	for _, indexer := range indexers.Indexers {
		for _, category := range indexer.Caps.Categories.Category {
			if category.Name == cat {
				neededIndexers = append(neededIndexers, indexer)
				break
			}
		}
	}
	log.Info("indexers", "cat", cat, "total", len(indexers.Indexers), "needed", len(neededIndexers))
	return neededIndexers
}

func getTorrents(indexer Indexer, payload common.Payload) []common.Torrent {
	var rss common.Rss
	t := "search"
	q := ""
	season := ""
	ep := ""
	traktid := ""
	imdbid := ""
	tvdbid := ""
	tmdbid := ""

	if indexer.SearchAvailable() {
		q = payload.Title + "+" + payload.Year
		if payload.Type == "episode" {
			q = payload.ShowTitle
		}
	}

	if indexer.MovieSearchAvailable() && payload.Type == "movie" {
		t = "movie"
		if indexer.HasMovieParam("imdbid") {
			imdbid = payload.Imdb
		}
		if indexer.HasMovieParam("traktid") {
			traktid = payload.Trakt
		}
	}
	if indexer.TvSearchAvailable() && payload.Type == "episode" {
		t = "tvsearch"
		if indexer.HasTvParam("imdbid") {
			imdbid = payload.EpisodeImdb
		}
		if indexer.HasTvParam("tvdbid") {
			tvdbid = payload.EpisodeTvdb
		}
		if indexer.HasTvParam("season") {
			q = payload.ShowTitle
			season = payload.SeasonNumber
		}
		if indexer.HasTvParam("ep") {
			q = payload.ShowTitle
			ep = payload.EpisodeNumber
		}

		if indexer.HasTvParam("traktid") {
			traktid = payload.Trakt
		}

	}

	query := ""
	if q != "" {
		query = "&q=" + q
	}
	if imdbid != "" {
		query = query + "&imdbid=" + imdbid
	}

	if tvdbid != "" {
		query = query + "&tvdbid=" + tvdbid
	}

	if tmdbid != "" {
		query = query + "&tmdbid=" + tmdbid
	}

	if season != "" {
		query = query + "&season=" + season
	}

	if ep != "" {
		query = query + "&ep=" + ep
	}

	if traktid != "" {
		query = query + "&traktid=" + traktid
	}

	var torrents []common.Torrent

	request := resty.New().
		// SetRetryCount(1).
		SetTimeout(time.Second * 60).
		// SetRetryWaitTime(time.Second * 2).
		R()

	jackettUrl := os.Getenv("JACKETT_URL")
	jackettKey := os.Getenv("JACKETT_KEY")

	url := fmt.Sprintf(
		"%s/api/v2.0/indexers/%s/results/torznab/api?apikey=%s&t=%s%s",
		jackettUrl,
		indexer.Id,
		jackettKey,
		t,
		query,
	)
	resp, err := request.Get(url)
	if err != nil {
		log.Error("get request", "indexer", indexer.Id, "error", err.Error())
		return torrents
	}

	if err := xml.Unmarshal(resp.Body(), &rss); err != nil {
		if err.Error() != "EOF" {
			log.Error("xml unmarshall", "indexer", indexer.Id, "error", err.Error())
		}
		return torrents
	}

	for _, item := range rss.Channel.Items {
		t := common.Torrent{}
		t.Scraper = indexer.Id
		t.Size = uint64(item.Size)
		t.ReleaseTitle = item.Title
		infos, q := common.GetInfos(item.Title)
		t.Info = infos
		t.Quality = q
		t.Name = item.Title
		for _, attr := range item.Attrs {
			if attr.Name == "magneturl" {
				t.Magnet = common.SimplifyMagnet(attr.Value)
			}
			if attr.Name == "infohash" {
				t.Hash = attr.Value
			}
			if attr.Name == "seeders" {
				c, err := strconv.Atoi(attr.Value)
				if err == nil {
					t.Seeds = uint64(c)
				}
			}
		}
		if t.Magnet != "" {
			torrents = append(torrents, t)
		}
	}

	return torrents
}
