package scraper

import (
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/odin-movieshow/server/helpers"
	"github.com/odin-movieshow/server/realdebrid"
	"github.com/odin-movieshow/server/settings"
	"github.com/odin-movieshow/server/types"
	"github.com/thoas/go-funk"

	"github.com/charmbracelet/log"
	"github.com/go-resty/resty/v2"
	"github.com/pocketbase/pocketbase"
)

func GetLinks(data map[string]any, app *pocketbase.PocketBase, mqt mqtt.Client) {
	// mux := sync.Mutex{}
	j := settings.GetJackett(app)

	if j == nil {
		log.Error("jackett", "error", "no settings")
		return
	}

	topic := "odin-movieshow/" + data["type"].(string)
	indexertopic := "odin-movieshow/indexer/" + data["type"].(string)
	if data["type"] == "episode" {
		topic += "/" + data["episode_trakt"].(string)
		indexertopic += "/" + data["episode_trakt"].(string)
	} else {
		topic += "/" + data["trakt"].(string)
		indexertopic += "/" + data["trakt"].(string)
	}

	log.Debug("MQTT", "topic", indexertopic)
	torrentQueue := make(chan types.Torrent)

	allTorrentsUnrestricted := helpers.ReadRDCacheByResource(app, topic)
	for _, u := range allTorrentsUnrestricted {
		cstr, _ := json.Marshal(u)
		mqt.Publish(topic, 0, false, cstr)
	}

	if token := mqt.Subscribe(indexertopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		newTorrents := []types.Torrent{}
		json.Unmarshal(msg.Payload(), &newTorrents)
		log.Warn("NewTorrents", "length", len(newTorrents))
		go func() {
			for _, t := range newTorrents {
				if t.Magnet != "" {
					torrentQueue <- t
				}
			}
		}()
		// fmt.Printf("Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
	}); token.Wait() &&
		token.Error() != nil {
		log.Error("mqtt-subscribe-indexer", "error", token.Error())
	}

	log.Debug(data)

	res := resty.New().SetTimeout(15*time.Minute).
		R().
		SetBody(data).
		SetHeader("Content-Type", "application/json")

	go func() {
		done := []string{}
		for {
			select {
			case k := <-torrentQueue:
				if !funk.Contains(done, k.Magnet) {
					unrestrict(k, app, mqt, topic)
					done = append(done, k.Magnet)
				}

			}
		}
	}()

	_, err := res.Post(fmt.Sprintf("%s/scrape", settings.GetScraperUrl(app)))
	if err != nil {
		log.Error("scrape", err)
		return
	}

	<-torrentQueue
	log.Warn("DONE")
	// for _, k := range allTorrents {

	// 	q1s := funk.Filter(allTorrentsUnrestricted, func(t types.Torrent) bool {
	// 		return t.Quality == "4K" && len(t.RealDebrid) > 0
	// 	}).([]types.Torrent)

	// 	q2s := funk.Filter(allTorrentsUnrestricted, func(t types.Torrent) bool {
	// 		return t.Quality == "1080p" && len(t.RealDebrid) > 0
	// 	}).([]types.Torrent)

	// 	q3s := funk.Filter(allTorrentsUnrestricted, func(t types.Torrent) bool {
	// 		return t.Quality == "720p" && len(t.RealDebrid) > 0
	// 	}).([]types.Torrent)

	// 	q4s := funk.Filter(allTorrentsUnrestricted, func(t types.Torrent) bool {
	// 		return t.Quality == "SD" && len(t.RealDebrid) > 0
	// 	}).([]types.Torrent)

	// 	if k.Quality == "1080p" {
	// 		if len(q2s) > 20 {
	// 			continue
	// 		}
	// 	}

	// 	if k.Quality == "720p" {
	// 		if len(q1s)+len(q2s) > 30 {
	// 			continue
	// 		}
	// 		if len(q3s) > 10 {
	// 			continue
	// 		}
	// 	}
	// 	if k.Quality == "SD" {
	// 		if len(q1s)+len(q2s) > 30 {
	// 			continue
	// 		}
	// 		if len(q4s) > 10 {
	// 			continue
	// 		}
	// 	}

	// 	cache := helpers.ReadRDCache(app, topic, k.Magnet)
	// 	if cache != nil {
	// 		// allTorrentsUnrestricted = append(allTorrentsUnrestricted, *cache)
	// 		// cstr, _ := json.Marshal(cache)
	// 		// mqtt.Publish(topic, 0, false, cstr)
	// 		continue
	// 	}

	// 	continue

	// 	u := realdebrid.Unrestrict(k.Magnet, app)
	// 	k.RealDebrid = append(k.RealDebrid, u)

	// 	if len(k.RealDebrid) > 0 {
	// 		allTorrentsUnrestricted = append(allTorrentsUnrestricted, k)
	// 		helpers.WriteRDCache(app, topic, k.Magnet, k)
	// 		kstr, _ := json.Marshal(k)
	// 		mqt.Publish(topic, 0, false, kstr)
	// 	}
	// 	// mux.Unlock()
	// }

}

func unrestrict(k types.Torrent, app *pocketbase.PocketBase, mqt mqtt.Client, topic string) {

	cache := helpers.ReadRDCache(app, topic, k.Magnet)
	if cache != nil {
		cstr, _ := json.Marshal(cache)
		mqt.Publish(topic, 0, false, cstr)
		return
	}

	u := realdebrid.Unrestrict(k.Magnet, app)
	k.RealDebrid = append(k.RealDebrid, u)

	if len(k.RealDebrid) > 0 {
		helpers.WriteRDCache(app, topic, k.Magnet, k)
		kstr, _ := json.Marshal(k)
		mqt.Publish(topic, 0, false, kstr)
	}
}
