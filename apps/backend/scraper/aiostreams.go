package scraper

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/go-resty/resty/v2"
	"github.com/odin-movieshow/backend/common"
	"github.com/odin-movieshow/backend/types"
)

type Result struct {
	Data struct {
		Results []types.AIOItem `json:"results"`
	} `json:"data"`
}

func GetFromAIOStreams(data common.Payload) []types.Torrent {
	url := os.Getenv("AIOSTREAMS_URL")
	t := "movie"
	id := data.Imdb
	if data.Type == "episode" {
		t = "series"
		id = fmt.Sprintf("%s:%s:%s", data.ShowImdb, data.SeasonNumber, data.EpisodeNumber)
	}
	torrents := []types.Torrent{}
	result := Result{}
	endpoint := fmt.Sprintf("%s/api/v1/search?type=%s&id=%s", url, t, id)
	creds := os.Getenv("AIOSTREAMS_CREDENTIALS")
	sp := strings.Split(creds, ":")
	username := sp[0]
	password := sp[1]

	log.Info("AIOStreams", "endpoint", endpoint)

	request := resty.New().
		SetRetryCount(3).
		SetTimeout(time.Second*60).
		SetRetryWaitTime(time.Second).
		SetBasicAuth(username, password).
		R()

	res, err := request.Get(endpoint)

	if err == nil {
		body := res.Body()
		err = json.Unmarshal(body, &result)
	}

	if err != nil {
		log.Error("AIOStreams", "err", err)
		return []types.Torrent{}
	}
	log.Debug("AIOStreams", "found", len(result.Data.Results))

	for _, item := range result.Data.Results {
		info, q := common.GetInfos(item.Filename)
		torrents = append(torrents, types.Torrent{
			Url:          item.URL,
			Name:         item.Filename,
			Size:         item.Size,
			Info:         info,
			Quality:      q,
			ReleaseTitle: item.Filename,
			Links: []types.Unrestricted{
				{Filename: item.Filename, Download: item.URL, Filesize: int(item.Size)},
			},
		})
	}

	return torrents

}
