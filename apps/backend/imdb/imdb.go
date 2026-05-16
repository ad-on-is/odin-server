package imdb

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/go-resty/resty/v2"
)

type Item struct {
	Rating float64 `json:"rating"`
	Votes  int     `json:"votes"`
}

type ImdbItem struct {
	AggregateRating struct {
		RatingValue float64 `json:"ratingValue"`
		RatingCount int     `json:"ratingCount"`
	} `json:"aggregateRating"`
	Review struct {
		ReviewBody   string `json:"reviewBody"`
		ReviewRating int    `json:"reviewRating"`
	} `json:"review"`
}

type Reviews []struct {
	Summary string `json:"summary"`
	Text    string `json:"text"`
	Rating  int    `json:"authorRating"`
}

func Get(id string) ImdbItem {
	url := os.Getenv("IMDBAPI_URL")
	request := resty.New().
		SetRetryCount(3).
		SetTimeout(time.Second * 30).
		SetRetryWaitTime(time.Second).
		R()

	endpoint := fmt.Sprintf("%s/movie/%s", url, id)
	res, err := request.Get(endpoint)
	i := Item{}
	r := ImdbItem{}
	if err == nil {
		err = json.Unmarshal(res.Body(), &i)
		if err != nil {
			log.Error("Failed to unmarshal response", "error", err)
		} else {
			r.AggregateRating.RatingValue = i.Rating
			r.AggregateRating.RatingCount = i.Votes
		}

	}

	endpoint = fmt.Sprintf("%s/reviews/%s", url, id)
	res, err = request.Get(endpoint)

	if err == nil {
		var reviews Reviews
		err = json.Unmarshal(res.Body(), &reviews)
		if err != nil || len(reviews) == 0 {
			log.Error("Failed to unmarshal response", "error", err)
		} else {
			r.Review.ReviewBody = fmt.Sprintf("%s. %s", reviews[0].Summary, reviews[0].Text)
			r.Review.ReviewRating = reviews[0].Rating
		}
	}

	return r
}
