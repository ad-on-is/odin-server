// WIP - probably won't use

package prowlarr

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/odin-movieshow/backend/common"

	"github.com/charmbracelet/log"
	"github.com/go-resty/resty/v2"
	"github.com/thoas/go-funk"
)

type Indexer struct {
	Id    uint32 `json:"id"`
	Title string `json:"name"`
	Caps  struct {
		SupportsRawSearch bool     `json:"supportsRawSearch"`
		SearchParams      []string `json:"searchParams"`
		MovieSearchParams []string `json:"movieSearchParams"`
		TvSearchParams    []string `json:"tvSearchParams"`
		Categories        []struct {
			ID   uint32 `json:"id"`
			Name string `json:"name"`
		} `json:"categories"`
	} `json:"capabilities"`
}

func (indexer *Indexer) SearchAvailable() bool {
	return indexer.Caps.SupportsRawSearch
}

func (indexer *Indexer) MovieSearchAvailable() bool {
	return indexer.Caps.MovieSearchParams != nil && len(indexer.Caps.MovieSearchParams) > 0
}

func (indexer *Indexer) TvSearchAvailable() bool {
	return indexer.Caps.TvSearchParams != nil && len(indexer.Caps.TvSearchParams) > 0
}

func (indexer *Indexer) HasMovieParam(param string) bool {
	return indexer.MovieSearchAvailable() && funk.Contains(indexer.Caps.MovieSearchParams, param)
}

func (indexer *Indexer) HasTvParam(param string) bool {
	return indexer.TvSearchAvailable() && funk.Contains(indexer.Caps.TvSearchParams, param)
}

func Search(payload common.Payload) {
	payload.Title = url.QueryEscape(common.Strip(payload.Title))
	payload.EpisodeTitle = url.QueryEscape(common.Strip(payload.EpisodeTitle))
	payload.ShowTitle = url.QueryEscape(common.Strip(payload.ShowTitle))

	indexers := getIndexerList(payload)

	allTorrents := []common.Torrent{}
	l := log.Warn
	wg := sync.WaitGroup{}
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
				"torrents",
				len(ts),
				"took",
				fmt.Sprintf("%.1fs", t2.Sub(t1).Seconds()),
			)
			allTorrents = append(allTorrents, ts...)
		}(indexer)
	}

	wg.Wait()
}

func getIndexerList(payload common.Payload) []Indexer {
	cat := "Movies"

	prowlarrUrl := os.Getenv("PROWLARR_URL")
	prowlarrKey := os.Getenv("PROWLARR_KEY")

	if payload.Type == "episode" {
		cat = "TV"
	}
	var indexers []Indexer

	request := resty.New().
		SetRetryCount(3).
		SetTimeout(time.Second * 30).
		SetRetryWaitTime(time.Second * 2).
		R()
	_, err := request.SetResult(&indexers).Get(
		fmt.Sprintf(
			"%s/api/v1/indexer?apikey=%s&t=indexers&configured=true",
			prowlarrUrl,
			prowlarrKey,
		),
	)
	if err != nil {
		log.Error("getting indexers", "error", err.Error())
		return []Indexer{}
	}
	neededIndexers := []Indexer{}
	for _, indexer := range indexers {
		for _, category := range indexer.Caps.Categories {
			if category.Name == cat {
				neededIndexers = append(neededIndexers, indexer)
				break
			}
		}
	}

	log.Info("indexers", "cat", cat, "total", len(indexers), "needed", len(neededIndexers))

	return neededIndexers
}

func getTorrents(indexer Indexer, payload common.Payload) []common.Torrent {
	// var rss common.Rss
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
			q = payload.ShowTitle + "+S" + payload.SeasonNumber + "+E" + payload.EpisodeNumber
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
			// q = payload.ShowTitle
			season = payload.SeasonNumber
		}
		if indexer.HasTvParam("ep") {
			// q = payload.ShowTitle
			ep = payload.EpisodeNumber
		}

		if indexer.HasTvParam("traktid") {
			traktid = payload.Trakt
		}

	}

	query := "&t=" + t
	if q != "" {
		query = "&query=" + q
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

	torrents := []common.Torrent{}

	request := resty.New().
		// SetRetryCount(1).
		SetTimeout(time.Second * 30).
		// SetRetryWaitTime(time.Second * 2).
		R()

	prowlarrUrl := os.Getenv("PROWLARR_URL")
	prowlarrKey := os.Getenv("PROWLARR_KEY")

	url := fmt.Sprintf(
		"%s/api/v1/search?apikey=%s&indexerId=%d%s",
		prowlarrUrl,
		prowlarrKey,
		indexer.Id,
		query,
	)

	log.Info(url)

	type Res struct {
		Title       string `json:"title"`
		Seeders     uint64 `json:"seeders"`
		DownloadUrl string `json:"downloadUrl"`
		Indexer     string `json:"indexer"`
		Size        uint32 `json:"size"`
	}

	res := []Res{}
	_, err := request.SetResult(&res).Get(url)
	if err != nil {
		log.Error("get request", "indexer", indexer.Id, "error", err.Error())
		return torrents
	}

	wg := sync.WaitGroup{}

	for _, item := range res {

		wg.Add(1)
		go func(item Res, indexer *Indexer, wg *sync.WaitGroup, torrents *[]common.Torrent) {
			defer wg.Done()
			t := common.Torrent{}
			t.Scraper = indexer.Title
			t.Size = uint64(item.Size)
			t.ReleaseTitle = item.Title
			infos, q := common.GetInfos(item.Title)
			t.Info = infos
			t.Quality = q

			r := resty.New().
				SetTimeout(time.Second * 10).
				SetRedirectPolicy(resty.FlexibleRedirectPolicy(0)).
				R()
			magnet := ""
			resp, err := r.Get(item.DownloadUrl)
			// log.Debug(resp.Request.URL)
			if err == nil {
			}

			l := resp.Header().Get("Location")
			if strings.Contains(l, "magnet") {
				magnet = l
			}

			t.Name = item.Title
			t.Magnet = magnet
			t.Hash = magnet
			t.Seeds = item.Seeders
			*torrents = append(*torrents, t)
		}(
			item,
			&indexer,
			&wg,
			&torrents,
		)

	}

	wg.Wait()

	return torrents
}
