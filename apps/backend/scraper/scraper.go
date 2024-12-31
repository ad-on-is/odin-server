package scraper

import (
	"encoding/json"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/odin-movieshow/backend/common"
	"github.com/odin-movieshow/backend/downloader/alldebrid"
	"github.com/odin-movieshow/backend/downloader/realdebrid"
	"github.com/odin-movieshow/backend/helpers"
	"github.com/odin-movieshow/backend/indexer"
	"github.com/odin-movieshow/backend/settings"
	"github.com/odin-movieshow/backend/types"
	"github.com/thoas/go-funk"

	"github.com/charmbracelet/log"
	"github.com/pocketbase/pocketbase"
)

type Scraper struct {
	app        *pocketbase.PocketBase
	settings   *settings.Settings
	helpers    *helpers.Helpers
	realdebrid *realdebrid.RealDebrid
	alldebrid  *alldebrid.AllDebrid
}

func New(
	app *pocketbase.PocketBase,
	settings *settings.Settings,
	helpers *helpers.Helpers,
	realdebrid *realdebrid.RealDebrid,
	alldebrid *alldebrid.AllDebrid,
) *Scraper {
	return &Scraper{app: app, settings: settings, helpers: helpers, realdebrid: realdebrid, alldebrid: alldebrid}
}

func (s *Scraper) GetLinks(data common.Payload, mqt mqtt.Client) {
	topic := "odin-movieshow/" + data.Type
	indexertopic := "odin-movieshow/indexer/" + data.Type
	if data.Type == "episode" {
		topic += "/" + data.EpisodeTrakt
		indexertopic += "/" + data.EpisodeTrakt
	} else {
		topic += "/" + data.Trakt
		indexertopic += "/" + data.Trakt
	}

	log.Debug("MQTT", "indexer topic", indexertopic)
	log.Debug("MQTT", "result topic", topic)
	torrentQueue := make(chan types.Torrent)

	allTorrentsUnrestricted := s.helpers.ReadRDCacheByResource(topic)
	for _, u := range allTorrentsUnrestricted {
		cstr, _ := json.Marshal(u)
		mqt.Publish(topic, 0, false, cstr)
	}

	if token := mqt.Subscribe(indexertopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		newTorrents := []types.Torrent{}
		json.Unmarshal(msg.Payload(), &newTorrents)
		go func() {
			for _, t := range newTorrents {
				torrentQueue <- t
			}
		}()
	}); token.Wait() &&
		token.Error() != nil {
		log.Error("mqtt-subscribe-indexer", "error", token.Error())
	}

	i := 0
	d := 0
	go func() {
		done := []string{}
		for k := range torrentQueue {
			i++
			// Filter quality from settings
			if !funk.Contains(done, k.Magnet) && k.Quality != "720p" && k.Quality != "SD" &&
				k.Quality != "CAM" {

				isUnrestricted := funk.Find(allTorrentsUnrestricted, func(s types.Torrent) bool {
					return s.Magnet == k.Magnet
				}) != nil

				if !isUnrestricted {
					if s.unrestrict(k, mqt, topic) {
						d++
					}
				}
				done = append(done, k.Magnet)
			}
		}
	}()
	indexer.Index(data)

	<-torrentQueue
	mqt.Publish(topic, 0, false, "SCRAPING_DONE")
	log.Warn("Scraping done", "unrestricted", d)
}

func (s *Scraper) unrestrict(
	k types.Torrent,
	mqt mqtt.Client,
	topic string,
) bool {
	us := s.alldebrid.Unrestrict(k.Magnet)
	if len(us) == 0 {
		us := s.realdebrid.Unrestrict(k.Magnet)
	}
	if len(us) == 0 {
		return false
	}
	k.Links = us
	log.Debug("Unrestricted: " + k.ReleaseTitle)
	s.helpers.WriteRDCache(topic, k.Magnet, k)
	kstr, _ := json.Marshal(k)
	mqt.Publish(topic, 0, false, kstr)
	return true
}
